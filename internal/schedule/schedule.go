// Package schedule installs systemd user timers that run `mrm sync`.
package schedule

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/adrg/xdg"
)

const prefix = "mrm-sync-"

// Run runs a command and returns its combined output; tests replace it so
// they never touch the real systemd.
var Run = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// UnitsDir is where systemd looks for user units.
func UnitsDir() string { return filepath.Join(xdg.ConfigHome, "systemd", "user") }

// Spec is one scheduled sync.
type Spec struct {
	Workspaces []string // empty: every workspace
	Calendar   string   // systemd OnCalendar, e.g. "Mon..Fri 08:30"
	Binary     string   // absolute path to mrm
	Config     string   // config file, when not the default
	Path       string   // PATH for git and ssh
}

// Name is the unit name stem: mrm-sync-<workspaces>, or mrm-sync-all.
func (s Spec) Name() string {
	if len(s.Workspaces) == 0 {
		return prefix + "all"
	}
	ws := append([]string(nil), s.Workspaces...)
	sort.Strings(ws)
	return prefix + strings.Join(ws, "-")
}

var serviceTmpl = template.Must(template.New("service").Parse(`# Written by mrm schedule; remove with: mrm schedule remove {{.Name}}
[Unit]
Description=mrm sync ({{.Label}})
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart={{.Exec}}
Environment="PATH={{.Path}}"
{{- if .Config}}
Environment="MRM_CONFIG={{.Config}}"
{{- end}}
Nice=10
`))

var timerTmpl = template.Must(template.New("timer").Parse(`# Written by mrm schedule; remove with: mrm schedule remove {{.Name}}
[Unit]
Description=mrm sync ({{.Label}}) on a schedule

[Timer]
OnCalendar={{.Calendar}}
# Run a missed sync as soon as the machine is back.
Persistent=true
RandomizedDelaySec=60

[Install]
WantedBy=timers.target
`))

// Units renders the service and timer files.
func (s Spec) Units() (service, timer string, err error) {
	label := "every workspace"
	if len(s.Workspaces) > 0 {
		label = strings.Join(s.Workspaces, ", ")
	}
	execLine := quote(s.Binary) + " sync"
	for _, w := range s.Workspaces {
		execLine += " -w " + quote(w)
	}
	data := map[string]string{"Name": s.Name(), "Label": label, "Exec": execLine, "Path": s.Path, "Config": s.Config, "Calendar": s.Calendar}
	var a, b bytes.Buffer
	if err := serviceTmpl.Execute(&a, data); err != nil {
		return "", "", err
	}
	if err := timerTmpl.Execute(&b, data); err != nil {
		return "", "", err
	}
	return a.String(), b.String(), nil
}

func quote(s string) string {
	if strings.ContainsAny(s, " \t\"'\\") {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	return s
}

// CheckCalendar asks systemd whether the OnCalendar expression is valid
// and returns when it next fires.
func CheckCalendar(expr string) (string, error) {
	out, err := Run("systemd-analyze", "calendar", expr)
	if err != nil {
		return "", fmt.Errorf("%q is not a systemd calendar expression (e.g. \"Mon..Fri 08:30\", \"daily\", \"*-*-* 07,13:00\"): %s", expr, strings.TrimSpace(string(out)))
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "Next elapse:") {
			return strings.TrimSpace(strings.SplitN(l, ":", 2)[1]), nil
		}
	}
	return "", nil
}

// Available reports whether a systemd user instance is running.
func Available() error {
	if out, err := Run("systemctl", "--user", "show-environment"); err != nil {
		return fmt.Errorf("no systemd user instance (%s) - on WSL, set systemd=true under [boot] in /etc/wsl.conf", strings.TrimSpace(string(out)))
	}
	return nil
}

// Install writes the units and enables the timer.
func Install(s Spec) error {
	service, timer, err := s.Units()
	if err != nil {
		return err
	}
	dir := UnitsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, s.Name()+".service"), []byte(service), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, s.Name()+".timer"), []byte(timer), 0o644); err != nil {
		return err
	}
	if out, err := Run("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s", strings.TrimSpace(string(out)))
	}
	if out, err := Run("systemctl", "--user", "enable", "--now", s.Name()+".timer"); err != nil {
		return fmt.Errorf("systemctl enable: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Timer is an installed schedule.
type Timer struct {
	Name, Calendar, Next, Last, State string
}

// List finds the installed mrm timers and asks systemd when they run.
func List() ([]Timer, error) {
	paths, _ := filepath.Glob(filepath.Join(UnitsDir(), prefix+"*.timer"))
	var out []Timer
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".timer")
		t := Timer{Name: name}
		if data, err := os.ReadFile(p); err == nil {
			for _, l := range strings.Split(string(data), "\n") {
				if v, ok := strings.CutPrefix(l, "OnCalendar="); ok {
					t.Calendar = v
				}
			}
		}
		if props, err := Run("systemctl", "--user", "show", name+".timer", "-p", "ActiveState", "-p", "NextElapseUSecRealtime", "-p", "LastTriggerUSec"); err == nil {
			for _, l := range strings.Split(string(props), "\n") {
				k, v, _ := strings.Cut(strings.TrimSpace(l), "=")
				switch k {
				case "ActiveState":
					t.State = v
				case "NextElapseUSecRealtime":
					t.Next = v
				case "LastTriggerUSec":
					t.Last = v
				}
			}
		}
		out = append(out, t)
	}
	return out, nil
}

// ErrNotFound: no mrm timer by that name.
var ErrNotFound = errors.New("no such schedule")

// Remove disables the timer and deletes both units.
func Remove(name string) error {
	if !strings.HasPrefix(name, prefix) {
		name = prefix + name
	}
	dir := UnitsDir()
	if _, err := os.Stat(filepath.Join(dir, name+".timer")); err != nil {
		return fmt.Errorf("%w: %s (see mrm schedule list)", ErrNotFound, name)
	}
	_, _ = Run("systemctl", "--user", "disable", "--now", name+".timer")
	for _, ext := range []string{".timer", ".service"} {
		if err := os.Remove(filepath.Join(dir, name+ext)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	_, _ = Run("systemctl", "--user", "daemon-reload")
	return nil
}
