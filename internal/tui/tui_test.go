package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/gittest"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
)

func TestMain(m *testing.M) {
	cleanup := gittest.Isolate()
	paths.Reload()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

type harness struct {
	t    *testing.T
	u    *ui
	s    tcell.SimulationScreen
	done chan error
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte("workspaces:\n  work:\n    roots: [/work]\n  personal:\n    roots: [/src]\ntui:\n  open_cmd: lazygit\n"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func start(t *testing.T, opts Options, w, h int) *harness {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	opts.Screen = s
	if opts.Config == nil {
		opts.Config = testConfig(t)
	}
	if opts.Engine == nil {
		opts.Engine = engine.New(30*time.Second, 4)
	}
	dir := t.TempDir()
	opts.CachePath, opts.HistPath = filepath.Join(dir, "status.json"), filepath.Join(dir, "runs.jsonl")
	u, err := newUI(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSize(w, h)
	h0 := &harness{t: t, u: u, s: s, done: make(chan error, 1)}
	go func() { h0.done <- u.run() }()
	t.Cleanup(func() {
		u.app.Stop()
		select {
		case <-h0.done:
		case <-time.After(3 * time.Second):
		}
	})
	h0.waitFor("mrm")
	return h0
}

// text reads the screen on the UI goroutine, so it never races a draw.
func (h *harness) text() string {
	ch := make(chan string, 1)
	h.u.app.QueueUpdate(func() { ch <- h.contents() })
	select {
	case s := <-ch:
		return s
	case <-time.After(2 * time.Second):
		h.t.Fatal("UI goroutine not responding")
		return ""
	}
}

func (h *harness) contents() string {
	cells, w, ht := h.s.GetContents()
	var b strings.Builder
	for y := 0; y < ht; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) == 0 || c.Runes[0] == 0 {
				b.WriteRune(' ')
			} else {
				b.WriteRune(c.Runes[0])
			}
		}
		b.WriteRune('\n')
	}
	return b.String()
}

// waitFor polls the screen until it holds every string (or, with a leading
// "!", does not hold it).
func (h *harness) waitFor(want ...string) string {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		txt := h.text()
		ok := true
		for _, w := range want {
			if neg := strings.HasPrefix(w, "!"); neg {
				ok = ok && !strings.Contains(txt, w[1:])
			} else {
				ok = ok && strings.Contains(txt, w)
			}
		}
		if ok {
			return txt
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("screen never showed %q:\n%s", want, txt)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (h *harness) keys(s string) {
	for _, r := range s {
		h.s.InjectKey(tcell.KeyRune, r, tcell.ModNone)
	}
}

func (h *harness) key(k tcell.Key) { h.s.InjectKey(k, 0, tcell.ModNone) }

func fixtures() ([]model.Repo, []model.Result) {
	now := time.Now()
	repos := []model.Repo{
		{Name: "api", Path: "/work/api", Workspace: "work"},
		{Name: "web", Path: "/work/web", Workspace: "work"},
		{Name: "billing-api", Path: "/work/platform/billing-api", Workspace: "work", Group: "platform"},
		{Name: "a-really-long-repository-name-that-will-not-fit-in-the-column", Path: "/work/long", Workspace: "work"},
		{Name: "dotfiles", Path: "/src/dotfiles", Workspace: "personal"},
	}
	st := func(s model.Status) model.Status {
		s.ReadAt, s.HasRemote, s.LastCommit = now, true, now.Add(-2*time.Hour)
		if s.DefaultBranch == "" {
			s.DefaultBranch = "main"
		}
		if s.Branch == "" && !s.Detached {
			s.Branch = "main"
		}
		if s.Upstream == "" {
			s.Upstream = "origin/" + s.Branch
		}
		return s
	}
	results := []model.Result{
		{Repo: repos[0], Status: st(model.Status{Behind: 3, LastSubject: "fix: rounding"})},
		{Repo: repos[1], Status: st(model.Status{Modified: 2, Behind: 1}), Outcome: model.OutcomeSkipped, Reason: "dirty (1 behind origin/main)"},
		{Repo: repos[2], Status: st(model.Status{Error: "missing: /work/platform/billing-api"})},
		{Repo: repos[3], Status: st(model.Status{})},
		{Repo: repos[4], Status: st(model.Status{Stashes: 1})},
	}
	return repos, results
}

// 2.2 / 2.9: grouped rows, header counts, 80 x 24 with truncation.
func TestRender(t *testing.T) {
	repos, results := fixtures()
	h := start(t, Options{Repos: repos, Results: results, NoRefresh: true}, 80, 24)
	txt := h.waitFor("all · 5 repos", "▾ work · 3", "▾ work · platform · 1", "▾ personal · 1", "fix: rounding")
	if !strings.Contains(txt, "…") {
		t.Fatalf("long name not truncated with an ellipsis:\n%s", txt)
	}
	for _, want := range []string{"✗", "●", "↓", "✓", "skipped: dirty"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "a-really-long-repository-name-that-will-not-fit-in-the-column") {
		t.Fatal("long name shown in full")
	}
}

// 2.3: / filters as you type; esc clears.
func TestFilter(t *testing.T) {
	repos, results := fixtures()
	h := start(t, Options{Repos: repos, Results: results, NoRefresh: true}, 100, 24)
	h.keys("/api")
	h.waitFor("billing-api", "!dotfiles", "!web ")
	h.key(tcell.KeyEnter)
	h.waitFor("/api", "(2 shown)")
	h.key(tcell.KeyEscape)
	h.waitFor("dotfiles", "!(2 shown)")

	h.keys("/is:dirty")
	h.waitFor("web", "!dotfiles")
	h.key(tcell.KeyEscape)
	h.waitFor("dotfiles")
}

// 2.6: ! shows everything not clean, grouped by reason.
func TestAttentionView(t *testing.T) {
	repos, results := fixtures()
	h := start(t, Options{Repos: repos, Results: results, NoRefresh: true}, 100, 24)
	h.keys("!")
	txt := h.waitFor("needs attention", "skipped: dirty · 1", "error · 1", "behind · 1", "!a-really-long", "!dotfiles")
	if strings.Index(txt, "skipped: dirty") > strings.Index(txt, "behind ·") {
		t.Fatalf("skips should come before states:\n%s", txt)
	}
	h.keys("!")
	h.waitFor("a-really-long", "!needs attention")
}

// 2.5: space marks, a marks everything shown, A clears.
func TestMarks(t *testing.T) {
	repos, results := fixtures()
	h := start(t, Options{Repos: repos, Results: results, NoRefresh: true}, 100, 24)
	h.keys(" ")
	h.waitFor("1 marked")
	h.keys("a")
	h.waitFor("5 marked")
	h.keys("A")
	h.waitFor("!marked")
}

// 2.2: tab collapses and expands a group.
func TestCollapse(t *testing.T) {
	repos, results := fixtures()
	h := start(t, Options{Repos: repos, Results: results, NoRefresh: true}, 100, 24)
	h.keys("g")
	h.key(tcell.KeyTab)
	h.waitFor("▸ work · 3", "!fix: rounding")
	h.key(tcell.KeyTab)
	h.waitFor("▾ work · 3", "fix: rounding")
}

// 2.4: ctrl+k finds actions by fuzzy search; w switches workspace.
func TestPaletteAndWorkspaces(t *testing.T) {
	repos, results := fixtures()
	h := start(t, Options{Repos: repos, Results: results, NoRefresh: true}, 100, 30)
	h.key(tcell.KeyCtrlK)
	h.waitFor("commands")
	h.keys("hlp")
	h.key(tcell.KeyEnter)
	h.waitFor("keys", "command palette")
	h.keys("x") // any key closes help
	h.waitFor("!command palette")

	h.key(tcell.KeyCtrlK)
	h.keys("wspers")
	h.key(tcell.KeyEnter)
	h.waitFor("personal · 1 repo", "!api")

	h.keys("w")
	h.waitFor("workspace")
	h.key(tcell.KeyUp)
	h.key(tcell.KeyEnter)
	h.waitFor("all · 5 repos")
}

func TestFuzzyRank(t *testing.T) {
	acts := []action{{label: "Sync everything shown"}, {label: "Open pull requests"}, {label: "Show every repo"}, {label: "Help"}}
	got := rank(acts, "opr")
	if len(got) == 0 || got[0].label != "Open pull requests" {
		t.Fatalf("got %v", got)
	}
	if got := rank(acts, "zzz"); len(got) != 0 {
		t.Fatalf("no match should drop out: %v", got)
	}
}

// 2.5 / 2.6 end to end: s syncs the repo under the cursor with a live row,
// on a real fixture.
func TestSyncFromTUI(t *testing.T) {
	f := gittest.New(t)
	dir := f.Clone("app")
	f.Push("master", "a.txt", "a\n")
	cfg, err := config.Parse([]byte("workspaces:\n  work:\n    roots: [" + filepath.Join(f.Root, "repos") + "]\n"))
	if err != nil {
		t.Fatal(err)
	}
	repos := []model.Repo{{Name: "app", Path: dir, Workspace: "work", Remote: "origin"}}
	h := start(t, Options{Config: cfg, Repos: repos}, 100, 24)
	h.waitFor("read 1 repo")
	h.keys("j") // from the group header to the repo
	h.keys("s")
	h.waitFor("synced 1 repo: 1 synced")
	if gittest.Head(t, dir, "HEAD") != gittest.Head(t, dir, "origin/master") {
		t.Fatal("repo not fast-forwarded")
	}
}
