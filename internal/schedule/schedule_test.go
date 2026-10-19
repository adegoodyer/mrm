package schedule

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
)

// fake replaces systemctl/systemd-analyze and records the calls.
func fake(t *testing.T) *[]string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	xdg.Reload()
	var calls []string
	old := Run
	Run = func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		switch {
		case strings.HasPrefix(call, "systemd-analyze calendar nonsense"):
			return []byte("Failed to parse calendar specification 'nonsense'"), errors.New("exit 1")
		case strings.HasPrefix(call, "systemd-analyze calendar"):
			return []byte("  Original form: x\n    Next elapse: Mon 2026-10-19 08:30:00 BST\n"), nil
		case strings.Contains(call, " show "):
			return []byte("ActiveState=active\nNextElapseUSecRealtime=Mon 2026-10-19 08:30:00 BST\nLastTriggerUSec=n/a\n"), nil
		}
		return nil, nil
	}
	t.Cleanup(func() { Run = old; xdg.Reload() })
	return &calls
}

// 4.4: install writes a oneshot service and a persistent timer, enables it;
// list reads it back; remove disables and deletes it.
func TestInstallListRemove(t *testing.T) {
	calls := fake(t)
	spec := Spec{Workspaces: []string{"work"}, Calendar: "Mon..Fri 08:30", Binary: "/opt/my tools/mrm", Path: "/usr/bin:/bin"}
	if next, err := CheckCalendar(spec.Calendar); err != nil || !strings.Contains(next, "08:30") {
		t.Fatalf("calendar: %q %v", next, err)
	}
	if err := Install(spec); err != nil {
		t.Fatal(err)
	}
	service, _ := os.ReadFile(filepath.Join(UnitsDir(), "mrm-sync-work.service"))
	timer, _ := os.ReadFile(filepath.Join(UnitsDir(), "mrm-sync-work.timer"))
	for _, want := range []string{"Type=oneshot", `ExecStart="/opt/my tools/mrm" sync -w work`, `Environment="PATH=/usr/bin:/bin"`} {
		if !strings.Contains(string(service), want) {
			t.Errorf("service missing %q:\n%s", want, service)
		}
	}
	for _, want := range []string{"OnCalendar=Mon..Fri 08:30", "Persistent=true", "WantedBy=timers.target"} {
		if !strings.Contains(string(timer), want) {
			t.Errorf("timer missing %q:\n%s", want, timer)
		}
	}
	if !strings.Contains(strings.Join(*calls, "\n"), "systemctl --user enable --now mrm-sync-work.timer") {
		t.Fatalf("not enabled: %v", *calls)
	}

	timers, _ := List()
	if len(timers) != 1 || timers[0].Name != "mrm-sync-work" || timers[0].Calendar != "Mon..Fri 08:30" || timers[0].State != "active" {
		t.Fatalf("list %+v", timers)
	}

	if err := Remove("work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(UnitsDir(), "mrm-sync-work.timer")); !os.IsNotExist(err) {
		t.Fatal("timer file left behind")
	}
	if !strings.Contains(strings.Join(*calls, "\n"), "systemctl --user disable --now mrm-sync-work.timer") {
		t.Fatalf("not disabled: %v", *calls)
	}
	if err := Remove("work"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second remove: %v", err)
	}
}

func TestBadCalendar(t *testing.T) {
	fake(t)
	if _, err := CheckCalendar("nonsense"); err == nil || !strings.Contains(err.Error(), "Mon..Fri 08:30") {
		t.Fatalf("err %v", err)
	}
}

func TestNameForAll(t *testing.T) {
	if n := (Spec{}).Name(); n != "mrm-sync-all" {
		t.Fatal(n)
	}
	if n := (Spec{Workspaces: []string{"b", "a"}}).Name(); n != "mrm-sync-a-b" {
		t.Fatal(n)
	}
}
