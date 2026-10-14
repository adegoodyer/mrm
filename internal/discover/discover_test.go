package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adegoodyer/mrm/internal/config"
)

func mkRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func mkDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// 1.1: groups found, non-repo folders and anything inside a repo skipped.
func TestWorkspace(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, filepath.Join(root, "app"))
	mkRepo(t, filepath.Join(root, "app", "vendor", "sub"))      // inside a repo: not listed
	mkRepo(t, filepath.Join(root, "app", ".worktrees", "feat")) // inside a repo: not listed
	mkRepo(t, filepath.Join(root, "platform", "billing-api"))
	mkRepo(t, filepath.Join(root, "services", "api"))
	mkDir(t, filepath.Join(root, "logs"))                     // not a repo: skipped
	mkRepo(t, filepath.Join(root, ".hidden", "repo"))         // hidden: skipped
	mkRepo(t, filepath.Join(root, "legacy-portal"))           // ignored by glob
	mkRepo(t, filepath.Join(root, "a", "b", "c", "too-deep")) // depth 4: skipped
	mkRepo(t, filepath.Join(root, "a", "b", "deep-enough"))   // depth 3: found
	// A worktree or submodule checkout has a .git file, not a folder.
	mkDir(t, filepath.Join(root, "wt"))
	if err := os.WriteFile(filepath.Join(root, "wt", ".git"), []byte("gitdir: /x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Repos: map[string]config.Override{"api": {Tags: []string{"backend"}}}}
	ws := &config.Workspace{Name: "work", Roots: []string{root}, Depth: 3, Ignore: []string{"legacy-*"}, Sync: config.SyncPolicy{Remote: "origin"}}
	repos, errs := Workspace(cfg, ws)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	var got []string
	for _, r := range repos {
		got = append(got, r.Display())
		if r.Workspace != "work" || r.Remote != "origin" {
			t.Fatalf("%s: workspace %q remote %q", r.Name, r.Workspace, r.Remote)
		}
	}
	want := "app,wt,a/b/deep-enough,platform/billing-api,services/api"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %s\nwant %s", strings.Join(got, ","), want)
	}
	for _, r := range repos {
		if r.Name == "api" && strings.Join(r.Tags, ",") != "backend" {
			t.Fatalf("override not applied: %v", r.Tags)
		}
	}
}

func TestMissingRoot(t *testing.T) {
	ws := &config.Workspace{Name: "x", Roots: []string{filepath.Join(t.TempDir(), "nope")}, Depth: 3}
	if _, errs := Workspace(&config.Config{}, ws); len(errs) != 1 {
		t.Fatalf("errs %v", errs)
	}
}

func TestRootIsARepo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "solo")
	mkRepo(t, root)
	repos, _ := Workspace(&config.Config{}, &config.Workspace{Name: "x", Roots: []string{root}, Depth: 3})
	if len(repos) != 1 || repos[0].Name != "solo" || repos[0].Group != "" {
		t.Fatalf("%+v", repos)
	}
}
