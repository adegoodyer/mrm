package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/adegoodyer/mrm/internal/model"
)

// sync is fetch + fast-forward, per the spec's step table. It never stashes,
// resets, rebases, merges or forces anything; when a step isn't safe it
// stops and says why.
func (e *Engine) sync(ctx context.Context, r model.Repo, dryRun bool) model.Result {
	if r.NoSync {
		st := e.status(ctx, r)
		return model.Result{Repo: r, Status: st, Outcome: model.OutcomeExcluded, Reason: "sync: false in config"}
	}

	// 1 - fetch
	res := e.fetch(ctx, r)
	if res.Outcome != model.OutcomeFetched {
		return res // failed, or excluded (no remote)
	}
	st := res.Status
	headBefore := st.Head
	res.Outcome = model.OutcomeCurrent
	rem := remote(r)

	skip := func(reason string) model.Result {
		res.Outcome, res.Reason = model.OutcomeSkipped, reason
		res.Status = e.status(ctx, r)
		return res
	}
	fail := func(step string, err error) model.Result {
		res.Outcome, res.Reason = model.OutcomeFailed, step+": "+err.Error()
		res.Status = e.status(ctx, r)
		return res
	}
	act := func(a string) {
		res.Actions = append(res.Actions, a)
		if dryRun {
			res.Outcome = model.OutcomeWould
		} else {
			res.Outcome = model.OutcomeSynced
		}
	}

	// 2 - an operation in progress: hands off
	if st.Operation != "" {
		return skip(st.Operation + " in progress")
	}
	if st.Detached {
		return skip("detached HEAD")
	}

	def := st.DefaultBranch
	switched := false

	// 6 - switch to the default when the current branch was pushed and is
	// fully merged. A branch that was never pushed is left alone: a new
	// branch with no commits yet is also "merged".
	pushed := st.Upstream != ""
	if r.SwitchMerged && def != "" && st.Branch != def && pushed && !st.Dirty() {
		merged, err := e.Git.IsAncestor(ctx, r.Path, "HEAD", "refs/remotes/"+rem+"/"+def)
		if err == nil && merged {
			from := st.Branch
			if !dryRun {
				if err := e.Git.Switch(ctx, r.Path, def); err != nil {
					return fail("switch to "+def, err)
				}
				st = e.status(ctx, r)
			}
			act(fmt.Sprintf("switched %s -> %s (merged)", from, def))
			switched = true
		}
	}

	if dryRun && switched {
		// Nothing moved, so report the default branch from its own counts.
		if st.DefaultAhead > 0 && st.DefaultBehind > 0 {
			return skip(fmt.Sprintf("default branch %s diverged from %s/%s", def, rem, def))
		}
		if st.DefaultBehind > 0 {
			act(fmt.Sprintf("%s +%d", def, st.DefaultBehind))
		}
	} else if st.Branch == def {
		// 3 - fast-forward the default branch
		if r := e.ffCurrent(ctx, r, st, dryRun, &res, act); r != nil {
			return *r
		}
	} else {
		// 4 - update the default branch in place
		if def != "" && st.DefaultBehind > 0 {
			if st.DefaultAhead > 0 {
				return skip(fmt.Sprintf("default branch %s diverged from %s/%s", def, rem, def))
			}
			if !dryRun {
				if err := e.Git.UpdateBranch(ctx, r.Path, rem, def); err != nil {
					return fail("update "+def, err)
				}
			}
			act(fmt.Sprintf("%s +%d", def, st.DefaultBehind))
		}
		// 5 - optionally fast-forward the current branch too
		if r.FFCurrent {
			if r := e.ffCurrent(ctx, r, st, dryRun, &res, act); r != nil {
				return *r
			}
		}
	}

	res.Status = e.status(ctx, r)
	// 7 - post-sync hooks, only when the checked-out branch moved.
	if !dryRun && res.Status.Head != headBefore && res.Status.Error == "" {
		e.runHooks(ctx, r, &res)
	}
	return res
}

// ffCurrent fast-forwards the checked-out branch to its upstream when that
// is safe. It returns a result to stop with, or nil to carry on.
func (e *Engine) ffCurrent(ctx context.Context, r model.Repo, st model.Status, dryRun bool, res *model.Result, act func(string)) *model.Result {
	stop := func(outcome model.Outcome, reason string) *model.Result {
		res.Outcome, res.Reason = outcome, reason
		res.Status = e.status(ctx, r)
		return res
	}
	switch {
	case st.UpstreamGone:
		return stop(model.OutcomeSkipped, "upstream "+st.Upstream+" is gone")
	case st.Upstream == "":
		return stop(model.OutcomeSkipped, "no upstream for "+st.Branch)
	case st.Behind == 0:
		return nil // current (maybe ahead: that's unpushed work, not a sync problem)
	case st.Ahead > 0:
		return stop(model.OutcomeSkipped, fmt.Sprintf("diverged (%d ahead, %d behind %s)", st.Ahead, st.Behind, st.Upstream))
	case st.Dirty() && (!r.FFDirty || st.Conflicted > 0):
		return stop(model.OutcomeSkipped, fmt.Sprintf("dirty (%d behind %s)", st.Behind, st.Upstream))
	case st.Dirty():
		// ff_dirty: safe when the update touches none of the changed files.
		both, err := e.Git.Overlap(ctx, r.Path, "@{u}")
		if err != nil {
			return stop(model.OutcomeFailed, "compare local changes: "+err.Error())
		}
		if len(both) > 0 {
			return stop(model.OutcomeSkipped, fmt.Sprintf("dirty: the update changes %s too", fileList(both)))
		}
	}
	if !dryRun {
		if err := e.Git.FastForward(ctx, r.Path); err != nil {
			if st.Dirty() {
				// git's own check, behind ours: it refused, nothing changed.
				return stop(model.OutcomeSkipped, "dirty: "+err.Error())
			}
			return stop(model.OutcomeFailed, "fast-forward: "+err.Error())
		}
	}
	act(fmt.Sprintf("%s +%d", st.Branch, st.Behind))
	return nil
}

// fileList is "a.go, b.go and 3 more".
func fileList(files []string) string {
	if len(files) <= 3 {
		return strings.Join(files, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(files[:3], ", "), len(files)-3)
}
