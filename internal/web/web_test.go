package web

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/engine"
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

const testToken = "test-token-0123456789"

type harness struct {
	t    *testing.T
	base string
	srv  *Server
	c    *http.Client // has the session cookie
	f    *gittest.Fixture
	repo string
}

// start serves a web UI over two fixture repos: "api" (one commit behind)
// and "platform/billing-api" (dirty).
func start(t *testing.T, cfgExtra string) *harness {
	t.Helper()
	f := gittest.New(t)
	api := f.Clone("api")
	billing := f.Clone("platform/billing-api")
	gittest.WriteFile(t, filepath.Join(billing, "README.md"), "edited\n")
	f.Push("master", "a.txt", "a\n")
	gittest.Git(t, api, "fetch", "-q")

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	gittest.WriteFile(t, cfgPath, "workspaces:\n  work:\n    roots: ["+filepath.Join(f.Root, "repos")+"]\n"+cfgExtra)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dir := t.TempDir()
	srv, err := New(ctx, Options{
		Config: cfg, Engine: engine.New(30*time.Second, 4), Token: testToken, NoRefresh: true,
		CachePath: filepath.Join(dir, "status.json"), HistPath: filepath.Join(dir, "runs.jsonl"), LockDir: filepath.Join(dir, "locks"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	h := &harness{t: t, base: "http://" + ln.Addr().String(), srv: srv, f: f, repo: api}
	// Read status once so pages have something to show.
	if err := srv.start("status", srv.repos, false); err != nil {
		t.Fatal(err)
	}
	h.waitIdle()
	jar, _ := cookiejar.New(nil)
	h.c = &http.Client{Jar: jar, Timeout: 10 * time.Second}
	resp, err := h.c.Get(h.base + "/?token=" + testToken)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("login: %v %v", err, resp)
	}
	resp.Body.Close()
	return h
}

func (h *harness) waitIdle() {
	h.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		h.srv.mu.Lock()
		busy := h.srv.busy
		h.srv.mu.Unlock()
		if busy == "" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatal("operation never finished")
}

func (h *harness) get(path string) (int, string) {
	h.t.Helper()
	resp, err := h.c.Get(h.base + path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (h *harness) post(path string, form url.Values, withHeader bool) (int, string) {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.base+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if withHeader {
		req.Header.Set(tokenHeader, testToken)
	}
	resp, err := h.c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// X.2: token, Host check and the POST header.
func TestSecurity(t *testing.T) {
	h := start(t, "")
	plain := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	resp, _ := plain.Get(h.base + "/")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: %d, want 401", resp.StatusCode)
	}
	resp, _ = plain.Get(h.base + "/?token=wrong")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d, want 401", resp.StatusCode)
	}
	resp, _ = plain.Get(h.base + "/?token=" + testToken)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("token: %d %q, want 303 to /", resp.StatusCode, resp.Header.Get("Location"))
	}
	ck := resp.Cookies()
	if len(ck) != 1 || !ck[0].HttpOnly || ck[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v", ck)
	}

	// DNS rebinding: the right token on the wrong Host is refused.
	req, _ := http.NewRequest(http.MethodGet, h.base+"/?token="+testToken, nil)
	req.Host = "evil.example"
	resp, _ = plain.Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad Host: %d, want 403", resp.StatusCode)
	}

	// CSRF: the cookie alone can't POST.
	if code, _ := h.post("/api/refresh", nil, false); code != http.StatusForbidden {
		t.Fatalf("POST without header: %d, want 403", code)
	}

	resp, _ = h.c.Get(h.base + "/")
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP %q", csp)
	}
	if !strings.Contains(h.base, "127.0.0.1") {
		t.Fatal("test server not on loopback")
	}
}

// 3.2: the overview shows workspaces, attention, risk and every repo.
func TestOverview(t *testing.T) {
	h := start(t, "")
	code, body := h.get("/")
	if code != 200 {
		t.Fatalf("overview %d", code)
	}
	for _, want := range []string{"work <small>2 repos", "Needs attention", "behind · 1", "dirty · 1", "Work at risk", "platform/", "billing-api", `id="rows"`, "2 of 2 repos"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	code, rows := h.get("/rows?q=is:dirty")
	if code != 200 || !strings.Contains(rows, "1 of 2 repos") || !strings.Contains(rows, "billing-api") {
		t.Fatalf("filtered rows %d:\n%s", code, rows)
	}
	if code, _ := h.get("/rows?q=is:nonsense"); code != http.StatusBadRequest {
		t.Fatalf("bad filter %d, want 400", code)
	}
}

// 3.4: the repo page shows branches, commits and forge links.
func TestRepoPage(t *testing.T) {
	h := start(t, "")
	id := repoID(h.repo)
	code, body := h.get("/repo/" + id)
	if code != 200 {
		t.Fatalf("repo page %d", code)
	}
	for _, want := range []string{"<h1>", "api", "Branches", "* master", "Commits", "initial", "behind"} {
		if !strings.Contains(body, want) {
			t.Errorf("repo page missing %q", want)
		}
	}
	if code, _ := h.get("/repo/nope"); code != http.StatusNotFound {
		t.Fatalf("unknown repo %d, want 404", code)
	}
}

// 3.3 / 3.5: a sync started from the page streams live rows, writes the
// history and shows on the Activity page.
func TestSyncStreamsAndRecords(t *testing.T) {
	h := start(t, "")
	events := make(chan string, 64)
	req, _ := http.NewRequest(http.MethodGet, h.base+"/events", nil)
	resp, err := h.c.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("events: %v", err)
	}
	defer resp.Body.Close()
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "event: ") {
				events <- strings.TrimPrefix(line, "event: ")
			}
		}
		close(events)
	}()

	if code, msg := h.post("/api/sync", url.Values{"repo": {repoID(h.repo)}}, true); code != http.StatusAccepted {
		t.Fatalf("sync: %d %s", code, msg)
	}
	seen := map[string]bool{}
	timeout := time.After(20 * time.Second)
	for !seen["toast"] {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("event stream closed")
			}
			seen[ev] = true
		case <-timeout:
			t.Fatalf("no toast; saw %v", seen)
		}
	}
	for _, want := range []string{"busy", "panels", "row", "toast"} {
		if !seen[want] {
			t.Errorf("no %q event", want)
		}
	}
	if gittest.Head(t, h.repo, "HEAD") != gittest.Head(t, h.repo, "origin/master") {
		t.Fatal("repo not fast-forwarded")
	}
	h.waitIdle()
	if _, body := h.get("/activity"); !strings.Contains(body, "<b>sync</b>") || !strings.Contains(body, "1 synced") {
		t.Fatalf("activity:\n%s", body)
	}

	// One operation at a time.
	h.srv.mu.Lock()
	h.srv.busy = "syncing"
	h.srv.mu.Unlock()
	if code, _ := h.post("/api/fetch", url.Values{"scope": {"all"}}, true); code != http.StatusConflict {
		t.Fatalf("second op: %d, want 409", code)
	}
}

// 3.6 / X.1: Settings shows the config with secrets hidden.
func TestSettingsHidesSecrets(t *testing.T) {
	h := start(t, "notify:\n  gotify:\n    url: https://gotify.example.com\n    token_cmd: echo s3cret-value\nweb:\n  text_size: large\n")
	_, body := h.get("/settings")
	if strings.Contains(body, "s3cret-value") {
		t.Fatal("secret shown on Settings")
	}
	for _, want := range []string{"token_cmd: •••••• (hidden)", "Text size", "(large)", `data-text-size="18"`} {
		if !strings.Contains(body, want) {
			t.Errorf("settings missing %q", want)
		}
	}
}

// The page's own assets are served without the token (they hold no data).
func TestStatic(t *testing.T) {
	h := start(t, "")
	for _, p := range []string{"/static/mrm.css", "/static/mrm.js", "/static/boot.js", "/static/fonts/instrument-sans.woff2"} {
		resp, err := http.Get(h.base + p)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", p, err, resp)
		}
		resp.Body.Close()
	}
}

// Another mrm holding a workspace's sync lock shows in the top bar.
func TestExternalLockShown(t *testing.T) {
	h := start(t, "")
	lock, err := engine.LockWorkspaces(h.srv.opts.LockDir, []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		h.srv.mu.Lock()
		ext := h.srv.external
		h.srv.mu.Unlock()
		if strings.Contains(ext, "another mrm") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("external lock not noticed")
}
