package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/adegoodyer/mrm/internal/model"
)

// PruneCandidate is a local branch that `mrm prune` offers to delete.
type PruneCandidate struct {
	Repo   model.Repo
	Branch string
	Target string // what it is merged into, e.g. origin/main
	Merged bool   // false: upstream gone but not merged - kept unless forced
}

// Reason says why the branch is offered.
func (c PruneCandidate) Reason() string {
	if c.Merged {
		return "merged into " + c.Target
	}
	return "upstream gone, not merged into " + c.Target
}

// PruneCandidates lists, for every repo, the local branches merged into the
// remote default branch or whose upstream is gone. The current and default
// branches are never offered.
func (e *Engine) PruneCandidates(ctx context.Context, repos []model.Repo) []PruneCandidate {
	per := make([][]PruneCandidate, len(repos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(e.Concurrency, 1))
	for i, r := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			per[i] = e.pruneCandidates(ctx, r)
		}()
	}
	wg.Wait()
	var out []PruneCandidate
	for _, p := range per {
		out = append(out, p...)
	}
	return out
}

func (e *Engine) pruneCandidates(ctx context.Context, r model.Repo) []PruneCandidate {
	st := e.status(ctx, r)
	if st.Error != "" || st.DefaultBranch == "" {
		return nil
	}
	target := e.mergeTarget(ctx, r, st.DefaultBranch)
	merged, err := e.Git.MergedBranches(ctx, r.Path, target)
	if err != nil {
		return nil
	}
	branches := e.Git.ReadDetail(ctx, r.Path, 1).Branches
	var out []PruneCandidate
	for _, b := range branches {
		if b.Current || b.Name == st.DefaultBranch || b.Name == st.Branch {
			continue
		}
		switch {
		case merged[b.Name]:
			out = append(out, PruneCandidate{Repo: r, Branch: b.Name, Target: short(target), Merged: true})
		case strings.Contains(b.Track, "gone"):
			out = append(out, PruneCandidate{Repo: r, Branch: b.Name, Target: short(target)})
		}
	}
	return out
}

// mergeTarget is <remote>/<default> when it exists (the freshest view of
// what's merged), else the local default branch.
func (e *Engine) mergeTarget(ctx context.Context, r model.Repo, def string) string {
	remoteRef := "refs/remotes/" + remote(r) + "/" + def
	if _, err := e.Git.Run(ctx, r.Path, "rev-parse", "--verify", "--quiet", remoteRef); err == nil {
		return remoteRef
	}
	return "refs/heads/" + def
}

func short(ref string) string {
	return strings.TrimPrefix(strings.TrimPrefix(ref, "refs/remotes/"), "refs/heads/")
}

// Prune deletes a candidate. A merged branch is checked again first; an
// unmerged one is deleted only when force is set.
func (e *Engine) Prune(ctx context.Context, c PruneCandidate, force bool) error {
	if !c.Merged && !force {
		return fmt.Errorf("not merged into %s - kept (--force deletes it)", c.Target)
	}
	if c.Merged {
		target := e.mergeTarget(ctx, c.Repo, strings.TrimPrefix(c.Target, remote(c.Repo)+"/"))
		ok, err := e.Git.IsAncestor(ctx, c.Repo.Path, "refs/heads/"+c.Branch, target)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("no longer merged into %s - kept", c.Target)
		}
	}
	return e.Git.DeleteBranch(ctx, c.Repo.Path, c.Branch)
}
