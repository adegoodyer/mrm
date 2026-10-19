package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adegoodyer/mrm/internal/gittest"
	"github.com/adegoodyer/mrm/internal/paths"
)

func TestMain(m *testing.M) {
	cleanup := gittest.Isolate()
	paths.Reload()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// run executes the command line with fresh flags and returns stdout and the
// exit code.
func run(t *testing.T, args ...string) (string, int) {
	t.Helper()
	flagWorkspaces, flagTags, flagFilter = nil, nil, ""
	flagJSON, flagNoColor, flagConfig, flagVerbose = false, false, "", false
	flagDryRun, flagStatusFetch = false, false
	flagExecYes, flagExecSerial, flagPruneYes, flagPruneDry, flagPruneForce = false, false, false, false, false

	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	w.Close()
	os.Stdout = stdout
	var buf bytes.Buffer
	io.Copy(&buf, r)

	code := 0
	if err != nil {
		code = 1
		var ee *exitError
		if errors.As(err, &ee) {
			code = ee.code
		}
	}
	return buf.String(), code
}

func writeConfig(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	gittest.WriteFile(t, path, "workspaces:\n  test:\n    roots: ["+root+"]\n")
	return path
}

// 1.10: exit codes 0 / 2 / 1.
func TestSyncExitCodes(t *testing.T) {
	f := gittest.New(t)
	clean := f.Clone("clean")
	dirty := f.Clone("dirty")
	cfg := writeConfig(t, filepath.Join(f.Root, "repos"))

	if out, code := run(t, "sync", "-c", cfg, "--no-color"); code != 0 {
		t.Fatalf("all current: exit %d\n%s", code, out)
	}

	f.Push("master", "a.txt", "a\n")
	gittest.WriteFile(t, filepath.Join(dirty, "README.md"), "edit\n")
	out, code := run(t, "sync", "-c", cfg, "--no-color")
	if code != 2 {
		t.Fatalf("one dirty: exit %d, want 2\n%s", code, out)
	}
	if !strings.Contains(out, "Needs attention") || !strings.Contains(out, "dirty") {
		t.Fatalf("summary missing attention list:\n%s", out)
	}
	if gittest.Head(t, clean, "HEAD") != gittest.Head(t, clean, "origin/master") {
		t.Fatal("clean repo not synced")
	}

	bad := filepath.Join(t.TempDir(), "bad.yaml")
	gittest.WriteFile(t, bad, "workspaces: [nope\n")
	if _, code := run(t, "sync", "-c", bad); code != 1 {
		t.Fatalf("bad config: exit %d, want 1", code)
	}
}

// 1.11 / 1.12: filters narrow the set; --json carries every status field.
func TestStatusJSONAndFilter(t *testing.T) {
	f := gittest.New(t)
	f.Clone("alpha")
	beta := f.Clone("group/beta")
	gittest.WriteFile(t, filepath.Join(beta, "new.txt"), "x\n")
	cfg := writeConfig(t, filepath.Join(f.Root, "repos"))

	out, code := run(t, "status", "-c", cfg, "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	st := rows[1]["status"].(map[string]any)
	for _, k := range []string{"branch", "default_branch", "ahead", "behind", "staged", "modified", "untracked", "stashes", "unpushed", "has_remote"} {
		if _, ok := st[k]; !ok {
			t.Errorf("status missing %q", k)
		}
	}
	if rows[1]["group"] != "group" || rows[1]["state"] != "clean" {
		t.Fatalf("row %v", rows[1])
	}

	out, _ = run(t, "status", "-c", cfg, "--json", "-f", "is:untracked")
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 || rows[0]["name"] != "beta" {
		t.Fatalf("filter: %v %s", err, out)
	}
}

func TestOpenPrint(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	gittest.Git(t, dir, "remote", "set-url", "origin", "git@bitbucket.org:acme/app.git")
	cfg := writeConfig(t, filepath.Join(f.Root, "repos"))
	out, code := run(t, "open", "app", "--pr", "--print", "-c", cfg)
	if code != 0 || strings.TrimSpace(out) != "https://bitbucket.org/acme/app/pull-requests" {
		t.Fatalf("exit %d: %q", code, out)
	}
}

// Shell completion offers workspace, tag, filter and repo values.
func TestCompletion(t *testing.T) {
	f := gittest.New(t)
	f.Clone("platform/billing-api")
	f.Clone("web")
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	gittest.WriteFile(t, cfg, "workspaces:\n  work:\n    roots: ["+filepath.Join(f.Root, "repos")+"]\n  personal:\n    roots: ["+t.TempDir()+"]\nrepos:\n  web:\n    tags: [frontend]\n")

	complete := func(args ...string) []string {
		t.Helper()
		out, _ := run(t, append([]string{"__complete", "-c", cfg}, args...)...)
		var vals []string
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if !strings.HasPrefix(line, ":") {
				vals = append(vals, strings.SplitN(line, "\t", 2)[0])
			}
		}
		return vals
	}
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"status", "-w", ""}, "personal,work"},
		{[]string{"status", "-t", ""}, "frontend"},
		{[]string{"status", "-f", "is:di"}, "is:dirty,is:diverged"},
		{[]string{"status", "-f", "ws:work -tag:"}, "ws:work -tag:frontend"},
		{[]string{"status", "-f", "group:"}, "group:platform"},
		{[]string{"status", "-f", "fo"}, "forge:"},
		{[]string{"open", "bil"}, "billing-api"},
		{[]string{"open", "platform/"}, "platform/billing-api"},
	}
	for _, c := range cases {
		if got := strings.Join(complete(c.args...), ","); got != c.want {
			t.Errorf("%v: got %q, want %q", c.args, got, c.want)
		}
	}
}

// 4.1 / 4.3: without a terminal, exec and prune refuse unless --yes.
func TestConfirmWithoutTerminal(t *testing.T) {
	f := gittest.New(t)
	repo := f.Clone("one")
	gittest.Git(t, repo, "branch", "done")
	cfg := writeConfig(t, filepath.Join(f.Root, "repos"))
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	stdin := os.Stdin
	os.Stdin = devnull
	defer func() { os.Stdin = stdin }()

	if _, code := run(t, "exec", "-c", cfg, "--", "touch", "ran"); code != 1 {
		t.Fatalf("exec without --yes: exit %d, want 1", code)
	}
	if _, err := os.Stat(filepath.Join(repo, "ran")); err == nil {
		t.Fatal("exec ran without confirmation")
	}
	if _, code := run(t, "exec", "-c", cfg, "--yes", "--", "touch", "ran"); code != 0 {
		t.Fatalf("exec --yes: exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(repo, "ran")); err != nil {
		t.Fatal("exec --yes did not run")
	}

	gittest.Git(t, repo, "push", "-q", "-u", "origin", "done")
	if out, code := run(t, "prune", "-c", cfg, "--no-color"); code != 1 || !strings.Contains(out, "done") {
		t.Fatalf("prune without --yes: exit %d, want 1\n%s", code, out)
	}
	if gittest.Head(t, repo, "done") == "" {
		t.Fatal("prune deleted without confirmation")
	}
}
