package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adegoodyer/mrm/internal/model"
)

// ReadStatus reads a repo's status without touching the network. remote is
// the remote to compare against (usually "origin"); branch overrides the
// default branch when not empty. A repo that can't be read comes back with
// Status.Error set rather than an error.
func (r *Runner) ReadStatus(ctx context.Context, dir, remote, branch string) model.Status {
	st := model.Status{ReadAt: time.Now()}
	if err := r.readStatus(ctx, dir, remote, branch, &st); err != nil {
		st.Error = err.Error()
	}
	return st
}

func (r *Runner) readStatus(ctx context.Context, dir, remote, branchOverride string, st *model.Status) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("missing: %s", dir)
	}
	out, err := r.Run(ctx, dir, "rev-parse", "--absolute-git-dir", "--git-common-dir")
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	gitDir := lines[0]
	commonDir := gitDir
	if len(lines) > 1 {
		commonDir = lines[1]
		if !filepath.IsAbs(commonDir) {
			commonDir = filepath.Join(dir, commonDir)
		}
	}

	out, err = r.Run(ctx, dir, "status", "--porcelain=v2", "--branch", "--show-stash", "-z")
	if err != nil {
		return err
	}
	ParsePorcelainV2(out, st)
	st.Operation = operation(gitDir)

	if fi, err := os.Stat(filepath.Join(commonDir, "FETCH_HEAD")); err == nil {
		st.LastFetch = fi.ModTime()
	}

	if url, err := r.Run(ctx, dir, "remote", "get-url", remote); err == nil {
		st.HasRemote = true
		st.RemoteURL = strings.TrimSpace(url)
	}

	refs, err := r.refs(ctx, dir, remote)
	if err != nil {
		return err
	}
	st.DefaultBranch, st.HasOriginHead = defaultBranch(refs, remote, branchOverride, st.Branch)

	if st.HasRemote {
		r.countUnpushed(ctx, dir, refs, st)
	}

	def := st.DefaultBranch
	if def != "" && st.Branch != def && refs.local[def] && refs.remote[def] {
		a, b, err := r.AheadBehind(ctx, dir, "refs/heads/"+def, "refs/remotes/"+remote+"/"+def)
		if err == nil {
			st.DefaultAhead, st.DefaultBehind = a, b
		}
	}

	switch {
	case refs.head != nil:
		st.LastCommit, st.LastAuthor, st.LastSubject = refs.head.when, refs.head.author, refs.head.subject
	case st.Head != "" && st.Head != "(initial)":
		// Detached: for-each-ref has no current branch to report.
		out, err := r.Run(ctx, dir, "log", "-1", "--format=%ct%x00%an%x00%s")
		if err == nil {
			parts := strings.SplitN(strings.TrimRight(out, "\n"), "\x00", 3)
			if len(parts) == 3 {
				st.LastCommit = unix(parts[0])
				st.LastAuthor, st.LastSubject = parts[1], parts[2]
			}
		}
	}
	return nil
}

func unix(s string) time.Time {
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// ParsePorcelainV2 fills st from `git status --porcelain=v2 --branch
// --show-stash -z` output.
func ParsePorcelainV2(out string, st *model.Status) {
	fields := strings.Split(out, "\x00")
	sawAB := false
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" {
			continue
		}
		switch {
		case strings.HasPrefix(f, "# branch.oid "):
			st.Head = strings.TrimPrefix(f, "# branch.oid ")
		case strings.HasPrefix(f, "# branch.head "):
			h := strings.TrimPrefix(f, "# branch.head ")
			if h == "(detached)" {
				st.Detached = true
			} else {
				st.Branch = h
			}
		case strings.HasPrefix(f, "# branch.upstream "):
			st.Upstream = strings.TrimPrefix(f, "# branch.upstream ")
		case strings.HasPrefix(f, "# branch.ab "):
			sawAB = true
			var a, b int
			fmt.Sscanf(strings.TrimPrefix(f, "# branch.ab "), "+%d -%d", &a, &b)
			st.Ahead, st.Behind = a, b
		case strings.HasPrefix(f, "# stash "):
			st.Stashes, _ = strconv.Atoi(strings.TrimPrefix(f, "# stash "))
		case strings.HasPrefix(f, "1 "), strings.HasPrefix(f, "2 "):
			if len(f) >= 4 {
				x, y := f[2], f[3]
				if x != '.' {
					st.Staged++
				}
				if y != '.' {
					st.Modified++
				}
			}
			if f[0] == '2' {
				i++ // renamed/copied entries carry the original path as the next field
			}
		case strings.HasPrefix(f, "u "):
			st.Conflicted++
		case strings.HasPrefix(f, "? "):
			st.Untracked++
		}
	}
	// An upstream with no ahead/behind line means the upstream ref is gone.
	if st.Upstream != "" && !sawAB {
		st.UpstreamGone = true
	}
}

// operation names a merge, rebase, cherry-pick or revert in progress.
func operation(gitDir string) string {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(gitDir, name))
		return err == nil
	}
	switch {
	case exists("rebase-merge"), exists("rebase-apply"):
		return "rebase"
	case exists("MERGE_HEAD"):
		return "merge"
	case exists("CHERRY_PICK_HEAD"):
		return "cherry-pick"
	case exists("REVERT_HEAD"):
		return "revert"
	}
	return ""
}

type refSet struct {
	local      map[string]bool // short names under refs/heads
	remote     map[string]bool // short names under refs/remotes/<remote>, minus HEAD
	remoteHead string          // target of refs/remotes/<remote>/HEAD, short
	head       *commitInfo     // tip of the checked-out branch
	branches   []localBranch
}

type localBranch struct {
	name, upstream, track string
}

type commitInfo struct {
	when            time.Time
	author, subject string
}

func (r *Runner) refs(ctx context.Context, dir, remote string) (refSet, error) {
	rs := refSet{local: map[string]bool{}, remote: map[string]bool{}}
	prefix := "refs/remotes/" + remote + "/"
	format := "--format=%(refname)%00%(symref)%00%(HEAD)%00%(upstream)%00%(upstream:track,nobracket)%00%(committerdate:unix)%00%(authorname)%00%(subject)"
	out, err := r.Run(ctx, dir, "for-each-ref", format, "refs/heads", strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return rs, err
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\x00", 8)
		if len(f) < 8 {
			continue
		}
		name, symref := f[0], f[1]
		switch {
		case strings.HasPrefix(name, "refs/heads/"):
			short := strings.TrimPrefix(name, "refs/heads/")
			rs.local[short] = true
			rs.branches = append(rs.branches, localBranch{name: short, upstream: f[3], track: f[4]})
			if f[2] == "*" {
				rs.head = &commitInfo{when: unix(f[5]), author: f[6], subject: f[7]}
			}
		case name == prefix+"HEAD":
			rs.remoteHead = strings.TrimPrefix(symref, prefix)
		case strings.HasPrefix(name, prefix):
			rs.remote[strings.TrimPrefix(name, prefix)] = true
		}
	}
	return rs, nil
}

// defaultBranch resolves the default branch: the override, else
// <remote>/HEAD, else main or master on the remote, else locally, else the
// current branch. The bool is true when <remote>/HEAD was used or an
// override was given.
func defaultBranch(rs refSet, remote, override, current string) (string, bool) {
	if override != "" {
		return override, true
	}
	if rs.remoteHead != "" {
		return rs.remoteHead, true
	}
	for _, b := range []string{"main", "master"} {
		if rs.remote[b] {
			return b, false
		}
	}
	for _, b := range []string{"main", "master"} {
		if rs.local[b] {
			return b, false
		}
	}
	return current, false
}

// countUnpushed counts work that exists only here: commits on branches that
// were never pushed, plus commits ahead of an upstream that still exists.
// A branch whose upstream is gone (deleted on the forge, then pruned) is
// stale, not unpushed: its commits were pushed once.
func (r *Runner) countUnpushed(ctx context.Context, dir string, rs refSet, st *model.Status) {
	var never []string
	for _, b := range rs.branches {
		switch {
		case strings.Contains(b.track, "gone"):
			st.StaleBranches++
		case b.upstream == "":
			never = append(never, "refs/heads/"+b.name)
		default:
			st.Unpushed += aheadOf(b.track)
		}
	}
	if len(never) == 0 {
		return
	}
	args := append([]string{"rev-list", "--count"}, never...)
	args = append(args, "--not", "--remotes")
	if out, err := r.Run(ctx, dir, args...); err == nil {
		n, _ := strconv.Atoi(strings.TrimSpace(out))
		st.Unpushed += n
	}
}

// aheadOf reads N from "ahead N" or "ahead N, behind M".
func aheadOf(track string) int {
	var n int
	if i := strings.Index(track, "ahead "); i >= 0 {
		fmt.Sscanf(track[i:], "ahead %d", &n)
	}
	return n
}
