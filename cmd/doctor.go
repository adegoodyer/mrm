package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/browser"
	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/discover"
	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/git"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/schedule"
	"github.com/adegoodyer/mrm/internal/secret"
	"github.com/adegoodyer/mrm/internal/term"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check git, the config, the roots, the repos and ssh",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor(cmd.Context())
	},
}

func init() { rootCmd.AddCommand(doctorCmd) }

type checker struct {
	style  term.Style
	failed bool
}

func (c *checker) ok(msg string)   { fmt.Println(c.style.Green("✓") + " " + msg) }
func (c *checker) warn(msg string) { fmt.Println(c.style.Yellow("!") + " " + msg) }
func (c *checker) fail(msg string) {
	fmt.Println(c.style.Red("✗") + " " + msg)
	c.failed = true
}

// minGit is the oldest git with `status --show-stash` in porcelain v2.
var minGit = [2]int{2, 35}

func runDoctor(ctx context.Context) error {
	c := &checker{style: term.NewStyle(os.Stdout, flagNoColor)}

	if v, err := git.Version(ctx); err != nil {
		c.fail("git not found on PATH")
	} else if !atLeast(v, minGit) {
		c.fail(fmt.Sprintf("git %s is too old: mrm needs %d.%d or newer", v, minGit[0], minGit[1]))
	} else {
		c.ok("git " + v)
	}

	path := configPath()
	fi, statErr := os.Stat(path)
	cfg, err := config.Load(path)
	switch {
	case err != nil:
		c.fail("config: " + err.Error())
		return &exitError{code: 1}
	case statErr != nil:
		c.warn("no config at " + paths.Tilde(path) + " - using the current directory (mrm config init)")
	default:
		c.ok("config " + paths.Tilde(path))
		for _, r := range cfg.Renamed {
			old, new, _ := strings.Cut(r, " -> ")
			c.warn(fmt.Sprintf("config: %s is now called %s - it still works, but rename it", old, new))
		}
		if fi.Mode().Perm()&0o022 != 0 {
			c.warn(fmt.Sprintf("config is group- or world-writable (%s): it can run commands (hooks, token_cmd)", fi.Mode().Perm()))
		}
	}

	wss, err := cfg.Select(flagWorkspaces)
	if err != nil {
		c.fail(err.Error())
		return &exitError{code: 1}
	}
	eng := engine.New(cfg.Timeout.Duration, cfg.Concurrency)
	for _, ws := range wss {
		repos, errs := discover.Workspace(cfg, ws)
		for _, e := range errs {
			c.fail(fmt.Sprintf("workspace %s: %v", ws.Name, e))
		}
		if len(errs) > 0 && len(repos) == 0 {
			continue
		}
		var noHead, broken []string
		for _, r := range eng.Collect(ctx, repos, engine.Options{Op: engine.OpStatus}) {
			switch {
			case r.Status.Error != "":
				broken = append(broken, r.Repo.Display()+" ("+r.Status.Error+")")
			case r.Status.HasRemote && !r.Status.HasOriginHead:
				noHead = append(noHead, r.Repo.Display())
			}
		}
		c.ok(fmt.Sprintf("workspace %s: %d repos", ws.Name, len(repos)))
		if len(noHead) > 0 {
			c.warn(fmt.Sprintf("workspace %s: %d repos have no %s/HEAD, so the default branch is guessed: %s\n    fix each with: git -C <repo> remote set-head %s --auto",
				ws.Name, len(noHead), ws.Sync.Remote, strings.Join(noHead, ", "), ws.Sync.Remote))
		}
		for _, b := range broken {
			c.fail(fmt.Sprintf("workspace %s: %s", ws.Name, b))
		}
	}

	switch err := exec.Command("ssh-add", "-l").Run(); {
	case err == nil:
		c.ok("ssh agent has keys loaded")
	case os.Getenv("SSH_AUTH_SOCK") == "":
		c.warn("no ssh agent (SSH_AUTH_SOCK unset): fine for keys without a passphrase, else ssh remotes will fail")
	default:
		c.warn("ssh agent has no keys loaded: ssh remotes using a passphrase-protected key will fail (ssh-add)")
	}

	for name, ep := range map[string]*config.Endpoint{"gotify": cfg.Notify.Gotify, "ntfy": cfg.Notify.Ntfy} {
		if ep == nil {
			continue
		}
		if ep.TokenCmd != "" {
			bin := strings.Fields(ep.TokenCmd)[0]
			if _, err := exec.LookPath(bin); err != nil {
				c.fail(fmt.Sprintf("notify.%s.token_cmd: %s not found", name, bin))
				continue
			}
		}
		if ep.TokenEnv != "" && os.Getenv(ep.TokenEnv) == "" {
			c.warn(fmt.Sprintf("notify.%s.token_env: $%s is not set here", name, ep.TokenEnv))
			continue
		}
		c.ok("notify " + name)
	}

	hosts := make([]string, 0, len(cfg.ForgeTokens))
	for h := range cfg.ForgeTokens {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		t := cfg.ForgeTokens[h]
		if _, err := secret.Resolve(ctx, t.TokenCmd, t.TokenEnv); err != nil {
			c.fail(fmt.Sprintf("host_tokens.%s: %v", h, err))
			continue
		}
		c.ok("API token for " + h)
	}

	if err := schedule.Available(); err != nil {
		c.warn("schedules: " + err.Error())
	} else if timers, _ := schedule.List(); len(timers) > 0 {
		c.ok(fmt.Sprintf("schedules: %d timer(s) installed (mrm schedule list)", len(timers)))
	} else {
		c.ok("schedules: systemd user instance available")
	}

	if argv, err := browser.Command(cfg.BrowserCmd, "https://example.com"); err != nil {
		c.warn("open: " + err.Error())
	} else {
		c.ok("open uses " + argv[0])
	}

	if c.failed {
		return &exitError{code: 1}
	}
	return nil
}

// atLeast compares a dotted version with major.minor.
func atLeast(v string, want [2]int) bool {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major > want[0] || (major == want[0] && minor >= want[1])
}
