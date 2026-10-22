package git

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Branch is a local branch and how it tracks its upstream.
type Branch struct {
	Name     string
	Current  bool
	Upstream string
	Track    string // "ahead 2, behind 1", "gone", "" when level
}

// Commit is one line of history.
type Commit struct {
	SHA     string
	When    time.Time
	Author  string
	Subject string
}

// Detail is what the TUI's detail pane shows beyond Status.
type Detail struct {
	Branches []Branch
	Stashes  []string
	Commits  []Commit
}

// ReadDetail reads branches, stashes and the last n commits. Parts that
// fail are left empty.
func (r *Runner) ReadDetail(ctx context.Context, dir string, n int) Detail {
	var d Detail
	if out, err := r.Run(ctx, dir, "for-each-ref", "--format=%(HEAD)%00%(refname:short)%00%(upstream:short)%00%(upstream:track,nobracket)", "refs/heads"); err == nil {
		for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			f := strings.SplitN(line, "\x00", 4)
			if len(f) == 4 {
				d.Branches = append(d.Branches, Branch{Current: f[0] == "*", Name: f[1], Upstream: f[2], Track: f[3]})
			}
		}
	}
	if out, err := r.Run(ctx, dir, "stash", "list", "--format=%gd%x00%cr%x00%s"); err == nil {
		for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			if f := strings.SplitN(line, "\x00", 3); len(f) == 3 {
				d.Stashes = append(d.Stashes, f[0]+"  "+f[1]+"  "+f[2])
			}
		}
	}
	if out, err := r.Run(ctx, dir, "log", "-n", strconv.Itoa(max(n, 1)), "--format=%h%x00%ct%x00%an%x00%s"); err == nil {
		for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			if f := strings.SplitN(line, "\x00", 4); len(f) == 4 {
				d.Commits = append(d.Commits, Commit{SHA: f[0], When: unix(f[1]), Author: f[2], Subject: f[3]})
			}
		}
	}
	return d
}

// GraphLimit is how many commits Graph reads.
const GraphLimit = 300

// Graph is `git log --graph` over every branch, tag and remote, newest
// first, with git's own colours (ANSI) and at most GraphLimit commits, and
// how many commits there are in all.
func (r *Runner) Graph(ctx context.Context, dir string) (string, int, error) {
	out, err := r.Run(ctx, dir, "log", "--graph", "--all", "--color=always", "--date-order",
		"-n", strconv.Itoa(GraphLimit),
		"--format=%C(auto)%h%d%C(reset) %s %C(dim)%cr · %an%C(reset)")
	if err != nil {
		return "", 0, err
	}
	total := 0
	if n, err := r.Run(ctx, dir, "rev-list", "--all", "--count"); err == nil {
		total, _ = strconv.Atoi(strings.TrimSpace(n))
	}
	return out, total, nil
}
