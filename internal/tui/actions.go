package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/adegoodyer/mrm/internal/browser"
	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/forge"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/runner"
)

// act runs a sync or fetch on repos, clearing the marks once it starts.
func (u *ui) act(op engine.Op, repos []model.Repo) {
	if len(repos) == 0 {
		u.warn("nothing to " + string(op))
		return
	}
	verb := map[engine.Op]string{engine.OpSync: "syncing", engine.OpFetch: "fetching"}[op]
	if u.startOp(op, repos, false, verb) {
		u.marks = map[string]bool{}
		u.drawChrome()
	}
}

// startOp runs an engine operation in the background and streams its
// events into the list. One operation at a time; a sync takes the same
// workspace lock as `mrm sync`. Called on the UI goroutine.
func (u *ui) startOp(op engine.Op, repos []model.Repo, fetch bool, verb string) bool {
	if u.busy != "" {
		u.warn(u.busy + " is still running")
		return false
	}
	run := &runner.Runner{Eng: u.eng, CachePath: u.opts.CachePath, HistPath: u.opts.HistPath, LockDir: paths.LockDir()}
	err := run.Start(u.ctx, op, repos, fetch,
		func(res model.Result) {
			u.app.QueueUpdateDraw(func() {
				delete(u.running, res.Repo.Path)
				u.results[res.Repo.Path] = res
				u.progress[0]++
				// Re-group only when the row could move between groups.
				if u.attention || !u.filt.Empty() {
					u.refresh()
				} else {
					u.drawChrome()
					u.updateDetail()
				}
			})
		},
		func(results []model.Result) {
			u.app.QueueUpdateDraw(func() { u.finishOp(op, results) })
		})
	if err != nil {
		u.warn(err.Error())
		return false
	}
	u.busy, u.progress = verb, [2]int{0, len(repos)}
	u.busyFlag.Store(true)
	for _, r := range repos {
		u.running[r.Path] = true
	}
	u.drawChrome()
	return true
}

func (u *ui) finishOp(op engine.Op, results []model.Result) {
	u.busy = ""
	u.busyFlag.Store(false)
	u.quitArmed = false
	if op == engine.OpSync {
		u.lastSync = time.Now()
	}
	n := map[model.Outcome]int{}
	attention := 0
	for _, r := range results {
		n[r.Outcome]++
		if r.Outcome.NeedsAttention() {
			attention++
		}
	}
	switch op {
	case engine.OpStatus:
		u.say(fmt.Sprintf("read %s", repoCount(len(results))))
	default:
		var parts []string
		for _, o := range []model.Outcome{model.OutcomeSynced, model.OutcomeFetched, model.OutcomeCurrent, model.OutcomeExcluded, model.OutcomeSkipped, model.OutcomeFailed} {
			if n[o] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n[o], o))
			}
		}
		msg := fmt.Sprintf("%s %s: %s", map[engine.Op]string{engine.OpSync: "synced", engine.OpFetch: "fetched"}[op], repoCount(len(results)), strings.Join(parts, ", "))
		if attention > 0 {
			u.warn(msg + " - showing what needs attention (! for all)")
			// A sync that left something to look at ends on the attention view.
			u.attention = true
		} else {
			u.say(msg)
		}
	}
	u.refresh()
}

type openKind int

const (
	openWeb openKind = iota
	openPRs
	openCI
	openBranch
)

// open opens the current repo's page on its forge.
func (u *ui) open(kind openKind) {
	r, ok := u.currentRepo()
	if !ok {
		u.warn("move to a repo first")
		return
	}
	st := u.results[r.Path].Status
	f := forge.Parse(st.RemoteURL, u.cfg.Forges)
	var url string
	var err error
	switch kind {
	case openPRs:
		url, err = f.PRs()
	case openCI:
		url, err = f.CI()
	case openBranch:
		url, err = f.Branch(st.Branch)
	default:
		url, err = f.Web()
	}
	if err == nil {
		err = browser.Open(u.cfg.BrowserCmd, url)
	}
	if err != nil {
		u.warn(r.Display() + ": " + err.Error())
		return
	}
	u.say("opened " + url)
}

type handoffKind int

const (
	handoffGit handoffKind = iota
	handoffEditor
	handoffShell
)

// handoff suspends the TUI, runs lazygit (or the editor, or a shell) in the
// current repo, and refreshes that repo when it exits.
func (u *ui) handoff(kind handoffKind) {
	r, ok := u.currentRepo()
	if !ok {
		u.warn("move to a repo first")
		return
	}
	var argv []string
	switch kind {
	case handoffGit:
		argv = strings.Fields(u.cfg.TUI.OpenCmd)
	case handoffEditor:
		ed := os.Getenv("VISUAL")
		if ed == "" {
			ed = os.Getenv("EDITOR")
		}
		if ed == "" {
			ed = "vi"
		}
		argv = append(strings.Fields(ed), ".")
	case handoffShell:
		sh := os.Getenv("SHELL")
		if sh == "" {
			sh = "/bin/sh"
		}
		argv = []string{sh}
	}
	if len(argv) == 0 {
		u.warn("tui.open_cmd is empty")
		return
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		u.warn(argv[0] + " not found on PATH (set tui.open_cmd)")
		return
	}
	var runErr error
	u.app.Suspend(func() {
		if kind == handoffShell {
			fmt.Printf("mrm: shell in %s - exit to return\n", paths.Tilde(r.Path))
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = r.Path
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		runErr = cmd.Run()
	})
	if runErr != nil {
		if _, isExit := runErr.(*exec.ExitError); !isExit {
			u.warn(argv[0] + ": " + runErr.Error())
		}
	}
	if u.busy == "" {
		u.startOp(engine.OpStatus, []model.Repo{r}, false, "reading")
	}
}
