package git

import (
	"strings"
	"testing"

	"github.com/adegoodyer/mrm/internal/model"
)

func z(lines ...string) string { return strings.Join(lines, "\x00") + "\x00" }

func TestParsePorcelainV2(t *testing.T) {
	out := z(
		"# branch.oid 1234567890abcdef",
		"# branch.head feature/x",
		"# branch.upstream origin/feature/x",
		"# branch.ab +2 -3",
		"# stash 4",
		"1 M. N... 100644 100644 100644 aaa bbb staged.txt",
		"1 .M N... 100644 100644 100644 aaa bbb modified.txt",
		"1 MM N... 100644 100644 100644 aaa bbb both.txt",
		"2 R. N... 100644 100644 100644 aaa bbb R100 new name.txt", "old name.txt",
		"u UU N... 100644 100644 100644 100644 aaa bbb ccc conflict.txt",
		"? untracked one.txt",
		"? untracked two.txt",
	)
	var st model.Status
	ParsePorcelainV2(out, &st)
	if st.Branch != "feature/x" || st.Upstream != "origin/feature/x" || st.Ahead != 2 || st.Behind != 3 || st.Stashes != 4 {
		t.Fatalf("branch info: %+v", st)
	}
	if st.Staged != 3 || st.Modified != 2 || st.Conflicted != 1 || st.Untracked != 2 {
		t.Fatalf("counts: staged %d modified %d conflicted %d untracked %d", st.Staged, st.Modified, st.Conflicted, st.Untracked)
	}
	if st.UpstreamGone {
		t.Fatal("upstream should not be gone")
	}
}

func TestParseDetachedAndGone(t *testing.T) {
	var st model.Status
	ParsePorcelainV2(z("# branch.oid abc", "# branch.head (detached)"), &st)
	if !st.Detached || st.Branch != "" {
		t.Fatalf("%+v", st)
	}
	st = model.Status{}
	ParsePorcelainV2(z("# branch.oid abc", "# branch.head x", "# branch.upstream origin/x"), &st)
	if !st.UpstreamGone {
		t.Fatal("upstream without ab line should be gone")
	}
}

func TestStatePrecedence(t *testing.T) {
	cases := []struct {
		st   model.Status
		want model.State
	}{
		{model.Status{Error: "x", Operation: "merge"}, model.StateError},
		{model.Status{Operation: "rebase", Modified: 1}, model.StateConflict},
		{model.Status{Ahead: 1, Behind: 1, Modified: 1}, model.StateDiverged},
		{model.Status{Modified: 1, Behind: 2}, model.StateDirty},
		{model.Status{Untracked: 3}, model.StateClean},
		{model.Status{HasRemote: true, Unpushed: 2, Behind: 1}, model.StateUnpushed},
		{model.Status{Unpushed: 2}, model.StateClean}, // no remote: nothing is "unpushed"
		{model.Status{DefaultBehind: 4}, model.StateBehind},
	}
	for i, c := range cases {
		if got := c.st.State(); got != c.want {
			t.Errorf("%d: %s, want %s", i, got, c.want)
		}
	}
}

func TestErrorMessage(t *testing.T) {
	e := &Error{Stderr: "warning: something\nfatal: Could not read from remote repository.\n\nPlease make sure"}
	if e.Message() != "fatal: Could not read from remote repository." {
		t.Fatalf("%q", e.Message())
	}
	bb := &Error{Stderr: "You may not have access to this repository or it no longer exists in this workspace.\nfatal: Could not read from remote repository.\n"}
	if bb.Message() != "remote repo not found, or no access" {
		t.Fatalf("%q", bb.Message())
	}
}
