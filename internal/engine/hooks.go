package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/adegoodyer/mrm/internal/model"
)

// Hook is a command run after a sync moved a repo's checked-out branch.
type Hook struct {
	Name   string
	IfFile string // only in repos that have this file (relative path)
	Run    string // run with sh -c in the repo
}

// HookTimeout bounds one hook (pre-commit installs can take a while).
var HookTimeout = 5 * time.Minute

// runHooks runs every matching hook. A hook that fails becomes a warning on
// the result; the sync itself still counts as done.
func (e *Engine) runHooks(ctx context.Context, r model.Repo, res *model.Result) {
	for _, h := range e.Hooks {
		if h.IfFile != "" {
			if _, err := os.Stat(filepath.Join(r.Path, h.IfFile)); err != nil {
				continue
			}
		}
		hctx, cancel := context.WithTimeout(ctx, HookTimeout)
		cmd := exec.CommandContext(hctx, "sh", "-c", h.Run)
		cmd.Dir = r.Path
		cmd.Env = append(os.Environ(),
			"MRM_REPO="+r.Path, "MRM_REPO_NAME="+r.Display(), "MRM_WORKSPACE="+r.Workspace,
			"MRM_BRANCH="+res.Status.Branch)
		cmd.WaitDelay = 2 * time.Second
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			res.Warnings = append(res.Warnings, "hook "+h.Name+" failed: "+lastLine(string(out), err))
			continue
		}
		res.Actions = append(res.Actions, "hook "+h.Name)
	}
}

func lastLine(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return err.Error()
}
