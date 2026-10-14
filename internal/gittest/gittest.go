// Package gittest builds real git fixtures in temp dirs: a bare "remote",
// a seed clone that pushes to it, and clones in any state a test needs.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Isolate points git at an empty global config and a fixed identity so the
// developer's own config (signing, hooks, default branch) can't leak into
// tests. Call it from TestMain before m.Run.
func Isolate() (cleanup func()) {
	dir, err := os.MkdirTemp("", "mrm-gittest-")
	if err != nil {
		panic(err)
	}
	global := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(global, []byte("[init]\n\tdefaultBranch = master\n[protocol \"file\"]\n\tallow = always\n"), 0o600); err != nil {
		panic(err)
	}
	env := map[string]string{
		"GIT_CONFIG_GLOBAL":   global,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME":     "Dev User",
		"GIT_AUTHOR_EMAIL":    "dev@example.com",
		"GIT_COMMITTER_NAME":  "Dev User",
		"GIT_COMMITTER_EMAIL": "dev@example.com",
		"HOME":                dir,
		"XDG_CONFIG_HOME":     filepath.Join(dir, "config"),
		"XDG_STATE_HOME":      filepath.Join(dir, "state"),
		"XDG_CACHE_HOME":      filepath.Join(dir, "cache"),
		"XDG_RUNTIME_DIR":     "",
	}
	for k, v := range env {
		os.Setenv(k, v)
	}
	return func() { os.RemoveAll(dir) }
}

// Git runs git in dir and fails the test on error.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	out, err := GitErr(dir, args...)
	if err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(out)
}

// GitErr runs git in dir and returns combined output and the error.
func GitErr(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// WriteFile writes a file, creating parent dirs.
func WriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ReadFile reads a file.
func ReadFile(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Commit writes file in dir and commits it.
func Commit(t testing.TB, dir, file, content, msg string) {
	t.Helper()
	WriteFile(t, filepath.Join(dir, file), content)
	Git(t, dir, "add", "--", file)
	Git(t, dir, "commit", "-q", "-m", msg)
}

// Fixture is a bare remote plus a seed clone that pushes to it.
type Fixture struct {
	t      testing.TB
	Root   string // temp dir holding everything
	Remote string // bare repo
	Seed   string // clone used to push "upstream" changes
}

// New makes a remote whose master has one commit.
func New(t testing.TB) *Fixture {
	t.Helper()
	root := t.TempDir()
	f := &Fixture{t: t, Root: root, Remote: filepath.Join(root, "remote.git"), Seed: filepath.Join(root, "seed")}
	Git(t, root, "init", "-q", "--bare", "-b", "master", f.Remote)
	Git(t, root, "clone", "-q", f.Remote, f.Seed)
	Commit(t, f.Seed, "README.md", "hello\n", "initial")
	Git(t, f.Seed, "push", "-q", "origin", "master")
	return f
}

// Clone clones the remote to Root/<name> (name may contain / for groups).
func (f *Fixture) Clone(name string) string {
	f.t.Helper()
	dir := filepath.Join(f.Root, "repos", name)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		f.t.Fatal(err)
	}
	Git(f.t, f.Root, "clone", "-q", f.Remote, dir)
	return dir
}

// Push commits a change on branch in the seed and pushes it, so clones fall
// behind.
func (f *Fixture) Push(branch, file, content string) {
	f.t.Helper()
	Git(f.t, f.Seed, "checkout", "-q", "-B", branch, "origin/"+branch)
	Commit(f.t, f.Seed, file, content, "upstream: "+file)
	Git(f.t, f.Seed, "push", "-q", "origin", branch)
}

// Branch creates a branch on the remote from master.
func (f *Fixture) Branch(name string) {
	f.t.Helper()
	Git(f.t, f.Seed, "fetch", "-q")
	Git(f.t, f.Seed, "push", "-q", "origin", "origin/master:refs/heads/"+name)
}

// SetDefault changes the remote's default branch (its HEAD).
func (f *Fixture) SetDefault(branch string) {
	f.t.Helper()
	Git(f.t, f.Remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
}

// Head returns the commit a ref points at in dir.
func Head(t testing.TB, dir, ref string) string {
	t.Helper()
	return Git(t, dir, "rev-parse", ref)
}
