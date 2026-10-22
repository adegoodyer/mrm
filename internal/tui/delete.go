package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rivo/tview"

	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/trash"
)

// trashTargets moves the target repos' folders to the trash after one
// confirmation. Their status is read again first, so the dialog can flag
// work that exists only on this machine.
func (u *ui) trashTargets() {
	if u.busy != "" {
		u.warn(u.busy + " is running - wait for it to finish")
		return
	}
	var ok []model.Repo
	var refused []string
	for _, r := range u.targets() {
		if why := u.cantTrash(r); why != "" {
			refused = append(refused, r.Display()+" ("+why+")")
			continue
		}
		ok = append(ok, r)
	}
	if len(ok) == 0 {
		if len(refused) > 0 {
			u.warn("left alone: " + strings.Join(refused, ", "))
		} else {
			u.warn("move to a repo first")
		}
		return
	}
	u.say("checking " + repoCount(len(ok)) + "…")
	go func() {
		results := u.eng.Collect(u.ctx, ok, engine.Options{Op: engine.OpStatus})
		u.app.QueueUpdateDraw(func() {
			var lines []string
			risky := 0
			for _, res := range results {
				line := tview.Escape(res.Repo.Display())
				if risk := atRisk(res.Status); len(risk) > 0 {
					line += "  [yellow]! " + strings.Join(risk, ", ") + "[-]"
					risky++
				}
				lines = append(lines, line)
			}
			lines = append(lines, "")
			if risky > 0 {
				lines = append(lines, fmt.Sprintf("[yellow]! %d with work that exists only on this machine - it goes to the trash too.[-]", risky))
			}
			lines = append(lines, "[::d]The trash keeps them until it is emptied: "+tview.Escape(paths.Tilde(trash.Dir()))+"[::-]")
			if len(refused) > 0 {
				lines = append(lines, "[::d]Left alone: "+tview.Escape(strings.Join(refused, ", "))+"[::-]")
			}
			u.confirm(fmt.Sprintf("Move %s to the trash?", repoCount(len(ok))), lines, "move to the trash", func() { u.doTrash(ok) })
		})
	}()
}

// doTrash moves the folders in the background, then drops the repos from
// the view.
func (u *ui) doTrash(repos []model.Repo) {
	u.say("moving " + repoCount(len(repos)) + " to the trash…")
	go func() {
		var moved []string
		var failed []string
		for _, r := range repos {
			if _, err := trash.Move(r.Path); err != nil {
				failed = append(failed, r.Display()+": "+err.Error())
				continue
			}
			moved = append(moved, r.Path)
		}
		u.app.QueueUpdateDraw(func() {
			gone := map[string]bool{}
			for _, p := range moved {
				gone[p] = true
				delete(u.results, p)
				delete(u.forge, p)
				delete(u.marks, p)
			}
			kept := u.repos[:0]
			for _, r := range u.repos {
				if !gone[r.Path] {
					kept = append(kept, r)
				}
			}
			u.repos = kept
			u.refresh()
			msg := fmt.Sprintf("moved %s to the trash (%s)", repoCount(len(moved)), paths.Tilde(trash.Dir()))
			if len(failed) > 0 {
				u.warn(msg + "; not moved: " + strings.Join(failed, "; "))
			} else {
				u.say(msg)
			}
		})
	}()
}

// cantTrash says why a repo's folder must not be moved, or "".
func (u *ui) cantTrash(r model.Repo) string {
	p := filepath.Clean(r.Path)
	if home, _ := os.UserHomeDir(); p == "/" || p == filepath.Clean(home) {
		return "not a repo folder"
	}
	for _, ws := range u.cfg.Workspaces {
		for _, root := range ws.Roots {
			if filepath.Clean(paths.ExpandHome(root)) == p {
				return "a workspace root"
			}
		}
	}
	if _, err := os.Stat(filepath.Join(p, ".git")); err != nil {
		return "not a git repo"
	}
	return ""
}

// atRisk is the work in a repo that exists only on this machine.
func atRisk(st model.Status) []string {
	var out []string
	if n := max(st.Unpushed, st.Ahead); n > 0 {
		out = append(out, plural(n, "unpushed commit"))
	}
	if st.Dirty() {
		out = append(out, "uncommitted changes")
	}
	if st.Untracked > 0 {
		out = append(out, plural(st.Untracked, "untracked file"))
	}
	if st.Stashes > 0 {
		out = append(out, plural(st.Stashes, "stash"))
	}
	return out
}

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	if strings.HasSuffix(one, "sh") {
		return fmt.Sprintf("%d %ses", n, one)
	}
	return fmt.Sprintf("%d %ss", n, one)
}
