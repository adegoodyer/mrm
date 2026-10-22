package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Fetch fetches a remote and prunes deleted branches.
func (r *Runner) Fetch(ctx context.Context, dir, remote string) error {
	_, err := r.Run(ctx, dir, "fetch", "--prune", "--quiet", remote)
	return err
}

// RefreshRemoteHead points <remote>/HEAD at the remote's current default
// branch. It is set at clone time and goes stale when the default changes
// on the forge; git before 2.48 never refreshes it on fetch. It costs a
// network round trip, so callers check RemoteHeadAge first. The ref file is
// touched afterwards so its age counts from this check even when nothing
// changed.
func (r *Runner) RefreshRemoteHead(ctx context.Context, dir, commonDir, remote string) error {
	if _, err := r.Run(ctx, dir, "remote", "set-head", remote, "--auto"); err != nil {
		return err
	}
	if commonDir != "" {
		now := time.Now()
		_ = os.Chtimes(remoteHeadFile(commonDir, remote), now, now)
	}
	return nil
}

// RemoteHeadAge is how long ago <remote>/HEAD was last set or checked; ok
// is false when it isn't a plain ref file (missing, or another ref store).
func RemoteHeadAge(commonDir, remote string) (time.Duration, bool) {
	if commonDir == "" {
		return 0, false
	}
	fi, err := os.Stat(remoteHeadFile(commonDir, remote))
	if err != nil {
		return 0, false
	}
	return time.Since(fi.ModTime()), true
}

func remoteHeadFile(commonDir, remote string) string {
	return filepath.Join(commonDir, "refs", "remotes", remote, "HEAD")
}

// FastForward moves the current branch to its upstream; git refuses anything
// but a fast-forward.
func (r *Runner) FastForward(ctx context.Context, dir string) error {
	_, err := r.Run(ctx, dir, "merge", "--ff-only", "--quiet", "@{u}")
	return err
}

// UpdateBranch moves a branch that is not checked out to
// <remote>/<branch>, from the already-fetched remote ref. git refuses a
// non-fast-forward and refuses to touch the checked-out branch.
func (r *Runner) UpdateBranch(ctx context.Context, dir, remote, branch string) error {
	spec := fmt.Sprintf("refs/remotes/%s/%s:refs/heads/%s", remote, branch, branch)
	_, err := r.Run(ctx, dir, "fetch", "--quiet", "--no-write-fetch-head", ".", spec)
	return err
}

// Switch checks out an existing (or remote-tracking) branch.
func (r *Runner) Switch(ctx context.Context, dir, branch string) error {
	_, err := r.Run(ctx, dir, "switch", "--quiet", branch)
	return err
}

// Overlap lists the files changed locally - staged or not, against HEAD -
// that the commits between HEAD and ref also change. A fast-forward to ref
// over local changes is safe exactly when it is empty.
func (r *Runner) Overlap(ctx context.Context, dir, ref string) ([]string, error) {
	local, err := r.Run(ctx, dir, "diff", "--name-only", "-z", "HEAD")
	if err != nil {
		return nil, err
	}
	incoming, err := r.Run(ctx, dir, "diff", "--name-only", "-z", "HEAD", ref)
	if err != nil {
		return nil, err
	}
	mine := map[string]bool{}
	for _, f := range strings.Split(local, "\x00") {
		if f != "" {
			mine[f] = true
		}
	}
	var both []string
	for _, f := range strings.Split(incoming, "\x00") {
		if mine[f] {
			both = append(both, f)
		}
	}
	return both, nil
}

// IsAncestor reports whether commit a is an ancestor of b.
func (r *Runner) IsAncestor(ctx context.Context, dir, a, b string) (bool, error) {
	_, err := r.Run(ctx, dir, "merge-base", "--is-ancestor", a, b)
	if err == nil {
		return true, nil
	}
	var ge *Error
	if errors.As(err, &ge) && ge.ExitCode == 1 {
		return false, nil
	}
	return false, err
}

// AheadBehind counts commits in a not in b (ahead) and in b not in a (behind).
func (r *Runner) AheadBehind(ctx context.Context, dir, a, b string) (int, int, error) {
	out, err := r.Run(ctx, dir, "rev-list", "--left-right", "--count", a+"..."+b)
	if err != nil {
		return 0, 0, err
	}
	var ahead, behind int
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d\t%d", &ahead, &behind); err != nil {
		return 0, 0, fmt.Errorf("parse rev-list output %q: %w", out, err)
	}
	return ahead, behind, nil
}

// TopLevel returns the root of the repo containing dir.
func (r *Runner) TopLevel(ctx context.Context, dir string) (string, error) {
	out, err := r.Run(ctx, dir, "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

// MergedBranches lists local branches whose tip is reachable from target.
func (r *Runner) MergedBranches(ctx context.Context, dir, target string) (map[string]bool, error) {
	out, err := r.Run(ctx, dir, "for-each-ref", "--format=%(refname:short)", "--merged", target, "refs/heads")
	if err != nil {
		return nil, err
	}
	m := map[string]bool{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			m[l] = true
		}
	}
	return m, nil
}

// DeleteBranch deletes a local branch. Callers check it is safe first:
// this uses -D because git's own -d only checks against HEAD and upstream.
func (r *Runner) DeleteBranch(ctx context.Context, dir, branch string) error {
	_, err := r.Run(ctx, dir, "branch", "-D", "--", branch)
	return err
}
