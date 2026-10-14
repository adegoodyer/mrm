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
