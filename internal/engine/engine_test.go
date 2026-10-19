package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adegoodyer/mrm/internal/gittest"
	"github.com/adegoodyer/mrm/internal/model"
)

func TestMain(m *testing.M) {
	cleanup := gittest.Isolate()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func newEngine() *Engine { return New(30*time.Second, 4) }

func repo(dir string) model.Repo {
	return model.Repo{Name: filepath.Base(dir), Path: dir, Remote: "origin"}
}

func syncOne(t *testing.T, r model.Repo, dryRun bool) model.Result {
	t.Helper()
	res := newEngine().Collect(context.Background(), []model.Repo{r}, Options{Op: OpSync, DryRun: dryRun})
	return res[0]
}

func want(t *testing.T, res model.Result, outcome model.Outcome, reasonPart string) {
	t.Helper()
	if res.Outcome != outcome || !strings.Contains(res.Reason, reasonPart) {
		t.Fatalf("got %s %q (actions %v), want %s containing %q", res.Outcome, res.Reason, res.Actions, outcome, reasonPart)
	}
}

// 1.6: clean and behind on the default branch -> fast-forwarded, no merge commit.
func TestSyncFastForwardsCleanDefault(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	f.Push("master", "a.txt", "a\n")

	res := syncOne(t, repo(dir), false)
	want(t, res, model.OutcomeSynced, "")
	if got, remote := gittest.Head(t, dir, "HEAD"), gittest.Head(t, dir, "origin/master"); got != remote {
		t.Fatalf("HEAD %s, want origin/master %s", got, remote)
	}
	if parents := gittest.Git(t, dir, "log", "-1", "--format=%P"); strings.Contains(parents, " ") {
		t.Fatalf("merge commit created: parents %q", parents)
	}
	if res.Status.State() != model.StateClean {
		t.Fatalf("state after sync = %s, want clean", res.Status.State())
	}
}

func TestSyncCurrentDoesNothing(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	want(t, syncOne(t, repo(dir), false), model.OutcomeCurrent, "")
}

// 1.7: on a feature branch -> branch and tree untouched, local master updated in place.
func TestSyncUpdatesDefaultInPlaceFromFeatureBranch(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.Git(t, dir, "switch", "-q", "-c", "feature")
	gittest.Commit(t, dir, "feature.txt", "wip\n", "wip")
	gittest.WriteFile(t, filepath.Join(dir, "scratch.txt"), "uncommitted\n")
	feature := gittest.Head(t, dir, "HEAD")
	f.Push("master", "a.txt", "a\n")

	res := syncOne(t, repo(dir), false)
	want(t, res, model.OutcomeSynced, "")
	if got := gittest.Git(t, dir, "branch", "--show-current"); got != "feature" {
		t.Fatalf("current branch %q, want feature", got)
	}
	if got := gittest.Head(t, dir, "HEAD"); got != feature {
		t.Fatal("feature branch moved")
	}
	if gittest.Head(t, dir, "master") != gittest.Head(t, dir, "origin/master") {
		t.Fatal("local master not updated to origin/master")
	}
	if gittest.ReadFile(t, filepath.Join(dir, "scratch.txt")) != "uncommitted\n" {
		t.Fatal("working tree changed")
	}
}

// N.1: dirty default branch, remote ahead -> skipped, tree and index identical.
func TestSyncSkipsDirty(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.WriteFile(t, filepath.Join(dir, "README.md"), "local edit\n")
	gittest.WriteFile(t, filepath.Join(dir, "staged.txt"), "staged\n")
	gittest.Git(t, dir, "add", "staged.txt")
	before := gittest.Git(t, dir, "status", "--porcelain=v2")
	head := gittest.Head(t, dir, "HEAD")
	f.Push("master", "a.txt", "a\n")

	want(t, syncOne(t, repo(dir), false), model.OutcomeSkipped, "dirty")
	if gittest.Head(t, dir, "HEAD") != head {
		t.Fatal("HEAD moved")
	}
	if after := gittest.Git(t, dir, "status", "--porcelain=v2"); after != before {
		t.Fatalf("status changed:\n%s\n->\n%s", before, after)
	}
	if gittest.ReadFile(t, filepath.Join(dir, "README.md")) != "local edit\n" {
		t.Fatal("edit lost")
	}
}

// N.2: local default has commits not on the remote, remote moved too -> skipped, no merge, no reset.
func TestSyncSkipsDiverged(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.Commit(t, dir, "local.txt", "mine\n", "local")
	head := gittest.Head(t, dir, "HEAD")
	f.Push("master", "a.txt", "a\n")

	res := syncOne(t, repo(dir), false)
	want(t, res, model.OutcomeSkipped, "diverged")
	if gittest.Head(t, dir, "HEAD") != head {
		t.Fatal("HEAD moved")
	}
	if res.Status.State() != model.StateDiverged {
		t.Fatalf("state %s, want diverged", res.Status.State())
	}
}

// Ahead only is unpushed work, not a sync problem.
func TestSyncAheadIsCurrent(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.Commit(t, dir, "local.txt", "mine\n", "local")
	res := syncOne(t, repo(dir), false)
	want(t, res, model.OutcomeCurrent, "")
	if res.Status.State() != model.StateUnpushed {
		t.Fatalf("state %s, want unpushed", res.Status.State())
	}
}

// N.3: merge in progress -> fetch only, conflict.
func TestSyncSkipsMergeInProgress(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.Commit(t, dir, "README.md", "mine\n", "local")
	f.Push("master", "README.md", "theirs\n")
	gittest.Git(t, dir, "fetch", "-q")
	if _, err := gittest.GitErr(dir, "merge", "origin/master"); err == nil {
		t.Fatal("expected a merge conflict")
	}
	res := syncOne(t, repo(dir), false)
	want(t, res, model.OutcomeSkipped, "merge in progress")
	if res.Status.State() != model.StateConflict {
		t.Fatalf("state %s, want conflict", res.Status.State())
	}
}

// N.4: detached HEAD -> fetch only.
func TestSyncSkipsDetached(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.Git(t, dir, "switch", "-q", "--detach")
	f.Push("master", "a.txt", "a\n")
	res := syncOne(t, repo(dir), false)
	want(t, res, model.OutcomeSkipped, "detached")
	if !res.Status.HasFlag(model.FlagDetached) {
		t.Fatal("no detached flag")
	}
}

// 1.9: dry run changes no local branch and no working tree.
func TestSyncDryRun(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	f.Push("master", "a.txt", "a\n")
	before := gittest.Git(t, dir, "for-each-ref", "refs/heads")

	res := syncOne(t, repo(dir), true)
	want(t, res, model.OutcomeWould, "")
	if !strings.HasPrefix(res.Summary(), "would master +1") {
		t.Fatalf("summary %q", res.Summary())
	}
	if after := gittest.Git(t, dir, "for-each-ref", "refs/heads"); after != before {
		t.Fatalf("local branches changed:\n%s\n->\n%s", before, after)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err == nil {
		t.Fatal("working tree changed")
	}
}

// N.14: untracked files don't block a fast-forward unless it would overwrite one.
func TestSyncUntracked(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.WriteFile(t, filepath.Join(dir, "notes.txt"), "mine\n")
	f.Push("master", "a.txt", "a\n")
	want(t, syncOne(t, repo(dir), false), model.OutcomeSynced, "")
	if gittest.ReadFile(t, filepath.Join(dir, "notes.txt")) != "mine\n" {
		t.Fatal("untracked file changed")
	}

	gittest.WriteFile(t, filepath.Join(dir, "clash.txt"), "mine\n")
	f.Push("master", "clash.txt", "theirs\n")
	res := syncOne(t, repo(dir), false)
	want(t, res, model.OutcomeFailed, "untracked")
	if gittest.ReadFile(t, filepath.Join(dir, "clash.txt")) != "mine\n" {
		t.Fatal("untracked file overwritten")
	}
}

// N.9: no remote -> excluded quietly, status still works.
func TestSyncNoRemote(t *testing.T) {
	dir := t.TempDir()
	gittest.Git(t, dir, "init", "-q")
	gittest.Commit(t, dir, "a.txt", "a\n", "a")
	res := syncOne(t, repo(dir), false)
	want(t, res, model.OutcomeExcluded, "no remote")
	if res.Outcome.NeedsAttention() {
		t.Fatal("no remote should not need attention")
	}
	if !res.Status.HasFlag(model.FlagNoRemote) {
		t.Fatal("no no-remote flag")
	}
}

// sync: false in config -> status only.
func TestSyncNoSync(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	f.Push("master", "a.txt", "a\n")
	r := repo(dir)
	r.NoSync = true
	before := gittest.Git(t, dir, "for-each-ref")
	want(t, syncOne(t, r, false), model.OutcomeExcluded, "sync: false")
	if after := gittest.Git(t, dir, "for-each-ref"); after != before {
		t.Fatal("refs changed: the repo was fetched or synced")
	}
}

// N.13: paths with spaces and non-ASCII characters.
func TestSyncOddPath(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("group with space/répo ünïcode")
	f.Push("master", "a.txt", "a\n")
	want(t, syncOne(t, repo(dir), false), model.OutcomeSynced, "")
}

// A stale origin/HEAD (set at clone time) is refreshed on fetch.
func TestFetchRefreshesStaleDefault(t *testing.T) {
	f := gittest.New(t)
	f.Branch("develop")
	dir := f.Clone("app")
	if st := newEngine().Collect(context.Background(), []model.Repo{repo(dir)}, Options{Op: OpStatus})[0].Status; st.DefaultBranch != "master" {
		t.Fatalf("default %q, want master", st.DefaultBranch)
	}
	f.SetDefault("develop")

	// Checked within the last day (the clone just set it): not re-checked,
	// so a fetch costs one network round trip, not two.
	res := newEngine().Collect(context.Background(), []model.Repo{repo(dir)}, Options{Op: OpFetch})[0]
	if res.Status.DefaultBranch != "master" {
		t.Fatalf("default after a fetch within a day %q, want master (not re-checked yet)", res.Status.DefaultBranch)
	}

	// Older than a day: re-checked, and the file's age resets.
	head := filepath.Join(dir, ".git", "refs", "remotes", "origin", "HEAD")
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(head, old, old); err != nil {
		t.Fatal(err)
	}
	res = newEngine().Collect(context.Background(), []model.Repo{repo(dir)}, Options{Op: OpFetch})[0]
	if res.Status.DefaultBranch != "develop" {
		t.Fatalf("default after fetch %q, want develop", res.Status.DefaultBranch)
	}
	if fi, err := os.Stat(head); err != nil || time.Since(fi.ModTime()) > time.Minute {
		t.Fatalf("origin/HEAD age not reset: %v", err)
	}
}

// A branch override in config wins over origin/HEAD.
func TestBranchOverride(t *testing.T) {
	f := gittest.New(t)
	f.Branch("develop")
	dir := f.Clone("app")
	r := repo(dir)
	r.Branch = "develop"
	st := newEngine().Collect(context.Background(), []model.Repo{r}, Options{Op: OpStatus})[0].Status
	if st.DefaultBranch != "develop" || !st.HasFlag(model.FlagNotDefault) {
		t.Fatalf("default %q flags %v", st.DefaultBranch, st.Flags())
	}
}

func TestSwitchMerged(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	// A pushed branch that has been merged on the remote.
	gittest.Git(t, dir, "switch", "-q", "-c", "done")
	gittest.Commit(t, dir, "done.txt", "done\n", "done")
	gittest.Git(t, dir, "push", "-q", "-u", "origin", "done")
	gittest.Git(t, f.Seed, "fetch", "-q")
	gittest.Git(t, f.Seed, "checkout", "-q", "-B", "master", "origin/master")
	gittest.Git(t, f.Seed, "merge", "-q", "--ff-only", "origin/done")
	gittest.Git(t, f.Seed, "push", "-q", "origin", "master")

	r := repo(dir)
	r.SwitchMerged = true
	res := syncOne(t, r, false)
	want(t, res, model.OutcomeSynced, "")
	if got := gittest.Git(t, dir, "branch", "--show-current"); got != "master" {
		t.Fatalf("on %q, want master", got)
	}
	if gittest.Head(t, dir, "HEAD") != gittest.Head(t, dir, "origin/master") {
		t.Fatal("master not current after switch")
	}

	// A new branch that was never pushed is left alone.
	gittest.Git(t, dir, "switch", "-q", "-c", "brand-new")
	res = syncOne(t, r, false)
	if got := gittest.Git(t, dir, "branch", "--show-current"); got != "brand-new" {
		t.Fatalf("switched off a new branch: on %q (%s)", got, res.Summary())
	}
}

// N.8: a deleted upstream branch is flagged and the ff is skipped.
func TestUpstreamGone(t *testing.T) {
	f := gittest.New(t)
	f.Branch("feature")
	dir := f.Clone("app")
	gittest.Git(t, dir, "switch", "-q", "feature")
	gittest.Git(t, f.Remote, "branch", "-D", "feature")
	r := repo(dir)
	r.FFCurrent = true
	res := syncOne(t, r, false)
	want(t, res, model.OutcomeSkipped, "gone")
	if !res.Status.HasFlag(model.FlagUpstreamGone) {
		t.Fatalf("flags %v", res.Status.Flags())
	}
}

// A branch whose upstream was deleted on the forge (and pruned) is stale,
// not unpushed work: its commits were pushed once.
func TestStaleBranchIsNotUnpushed(t *testing.T) {
	f := gittest.New(t)
	f.Branch("develop")
	f.Push("develop", "d.txt", "on develop only\n")
	dir := f.Clone("app")
	gittest.Git(t, dir, "branch", "-q", "--track", "develop", "origin/develop")
	gittest.Git(t, f.Remote, "branch", "-D", "develop")

	res := newEngine().Collect(context.Background(), []model.Repo{repo(dir)}, Options{Op: OpFetch})[0]
	st := res.Status
	if st.Unpushed != 0 || st.StaleBranches != 1 || st.State() != model.StateClean || !st.HasFlag(model.FlagStale) {
		t.Fatalf("unpushed %d stale %d state %s flags %v", st.Unpushed, st.StaleBranches, st.State(), st.Flags())
	}
}

// Commits on a branch that was never pushed are unpushed work.
func TestNeverPushedBranchIsUnpushed(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.Git(t, dir, "switch", "-q", "-c", "local-only")
	gittest.Commit(t, dir, "a.txt", "a\n", "a")
	gittest.Commit(t, dir, "b.txt", "b\n", "b")
	gittest.Git(t, dir, "switch", "-q", "master")

	st := newEngine().Collect(context.Background(), []model.Repo{repo(dir)}, Options{Op: OpStatus})[0].Status
	if st.Unpushed != 2 || st.State() != model.StateUnpushed {
		t.Fatalf("unpushed %d state %s", st.Unpushed, st.State())
	}
}

// N.6 / N.11: an unreachable remote fails within the timeout without prompting.
func TestFetchFailsFast(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.Git(t, dir, "remote", "set-url", "origin", "ssh://git@127.0.0.1:1/nowhere.git")
	start := time.Now()
	res := syncOne(t, repo(dir), false)
	want(t, res, model.OutcomeFailed, "fetch:")
	if took := time.Since(start); took > 20*time.Second {
		t.Fatalf("took %s", took)
	}
}

func TestTimeout(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	e := New(time.Nanosecond, 1)
	err := e.Git.Fetch(context.Background(), dir, "origin")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err %v, want a timeout", err)
	}
}

// N.10: a second sync of the same workspace is refused while one runs.
func TestLock(t *testing.T) {
	dir := t.TempDir()
	l1, err := LockWorkspaces(dir, []string{"work", "personal"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockWorkspaces(dir, []string{"personal"}); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock: %v, want ErrLocked", err)
	}
	if pid, held := LockHolder(dir, "personal"); !held || pid == "" {
		t.Fatalf("LockHolder while held: %q %v", pid, held)
	}
	l1.Release()
	if _, held := LockHolder(dir, "personal"); held {
		t.Fatal("LockHolder after release: still held")
	}
	l2, err := LockWorkspaces(dir, []string{"personal"})
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	l2.Release()
}

func TestRunEmitsEventsInOrderOfCompletion(t *testing.T) {
	f := gittest.New(t)
	var repos []model.Repo
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		repos = append(repos, repo(f.Clone(n)))
	}
	started, done := 0, 0
	seen := map[int]bool{}
	for ev := range newEngine().Run(context.Background(), repos, Options{Op: OpStatus}) {
		switch ev.Kind {
		case EventStarted:
			started++
		case EventDone:
			done++
			seen[ev.Index] = true
			if ev.Result.Repo.Path != repos[ev.Index].Path {
				t.Fatal("result does not match index")
			}
		}
	}
	if started != 5 || done != 5 || len(seen) != 5 {
		t.Fatalf("started %d done %d seen %d", started, done, len(seen))
	}
}

// 4.2: a post_sync hook runs only where its file exists and only when the
// checked-out branch moved; a failing hook is a warning, not a failed sync.
func TestPostSyncHooks(t *testing.T) {
	f := gittest.New(t)
	withFile := f.Clone("with")
	gittest.Commit(t, f.Seed, ".pre-commit-config.yaml", "repos: []\n", "add pre-commit")
	gittest.Git(t, f.Seed, "push", "-q", "origin", "HEAD:master")

	marker := filepath.Join(t.TempDir(), "ran")
	e := newEngine()
	e.Hooks = []Hook{
		{Name: "mark", IfFile: ".pre-commit-config.yaml", Run: "echo \"$MRM_REPO_NAME\" >> " + marker},
		{Name: "broken", IfFile: ".pre-commit-config.yaml", Run: "echo nope >&2; exit 4"},
		{Name: "elsewhere", IfFile: "Makefile", Run: "echo should-not-run >> " + marker},
	}
	res := e.Collect(context.Background(), []model.Repo{repo(withFile)}, Options{Op: OpSync})[0]
	if res.Outcome != model.OutcomeSynced {
		t.Fatalf("outcome %s %q", res.Outcome, res.Reason)
	}
	if got := strings.TrimSpace(gittest.ReadFile(t, marker)); got != "with" {
		t.Fatalf("hook ran for %q", got)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "hook broken failed: nope") {
		t.Fatalf("warnings %v", res.Warnings)
	}

	// Already current: HEAD doesn't move, no hook.
	res = e.Collect(context.Background(), []model.Repo{repo(withFile)}, Options{Op: OpSync})[0]
	if res.Outcome != model.OutcomeCurrent || len(res.Warnings) != 0 {
		t.Fatalf("second sync %s %v", res.Outcome, res.Warnings)
	}
	if strings.Count(gittest.ReadFile(t, marker), "with") != 1 {
		t.Fatal("hook ran without HEAD moving")
	}

	// Dry run: nothing runs.
	f.Push("master", "b.txt", "b\n")
	res = e.Collect(context.Background(), []model.Repo{repo(withFile)}, Options{Op: OpSync, DryRun: true})[0]
	if res.Outcome != model.OutcomeWould || strings.Count(gittest.ReadFile(t, marker), "with") != 1 {
		t.Fatalf("dry run ran a hook: %s", res.Outcome)
	}
	if strings.Contains(gittest.ReadFile(t, marker), "should-not-run") {
		t.Fatal("hook ran without its if_file")
	}
}
