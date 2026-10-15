// Package runner runs an engine operation the way every front end must:
// a sync takes the workspace lock, results stream back as they finish, and
// the status cache and run history are written at the end. The TUI and the
// web UI both use it; the CLI does the same steps inline.
package runner

import (
	"context"
	"sort"
	"time"

	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/store"
)

// Runner holds where results are written.
type Runner struct {
	Eng       *engine.Engine
	CachePath string
	HistPath  string
	LockDir   string
}

// Start runs op on repos in the background. onDone is called with each
// repo's result as it finishes, then finish with all results once the
// cache and history are written and the lock is released. Both are called
// from the runner's goroutine. Start returns an error, without starting,
// when a sync can't take its workspace lock.
func (r *Runner) Start(ctx context.Context, op engine.Op, repos []model.Repo, fetch bool,
	onDone func(model.Result), finish func([]model.Result)) error {
	var lock *engine.Lock
	if op == engine.OpSync {
		var err error
		if lock, err = engine.LockWorkspaces(r.LockDir, Workspaces(repos)); err != nil {
			return err
		}
	}
	started := time.Now()
	go func() {
		defer lock.Release()
		results := make([]model.Result, len(repos))
		for ev := range r.Eng.Run(ctx, repos, engine.Options{Op: op, Fetch: fetch}) {
			if ev.Kind == engine.EventDone {
				results[ev.Index] = ev.Result
				if onDone != nil {
					onDone(ev.Result)
				}
			}
		}
		_ = store.UpdateCache(r.CachePath, results)
		if op != engine.OpStatus {
			_ = store.AppendRun(r.HistPath, store.NewRun(string(op), false, started, Workspaces(repos), results))
		}
		lock.Release()
		if finish != nil {
			finish(results)
		}
	}()
	return nil
}

// Workspaces lists the distinct workspaces of repos, sorted.
func Workspaces(repos []model.Repo) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range repos {
		if !seen[r.Workspace] {
			seen[r.Workspace] = true
			out = append(out, r.Workspace)
		}
	}
	sort.Strings(out)
	return out
}
