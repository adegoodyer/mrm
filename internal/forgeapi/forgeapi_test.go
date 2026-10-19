package forgeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/forge"
)

// fakeForge answers like the four APIs do, and records the auth it saw.
func fakeForge(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		p := r.URL.EscapedPath()
		js := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		// GitHub
		case p == "/repos/acme/api/pulls":
			if r.Header.Get("Authorization") != "Bearer gh-tok" {
				w.WriteHeader(401)
				return
			}
			js([]map[string]any{{"id": 1}, {"id": 2}})
		case p == "/repos/acme/api/actions/runs":
			if r.URL.Query().Get("branch") != "main" {
				t.Errorf("github branch %q", r.URL.Query().Get("branch"))
			}
			js(map[string]any{"workflow_runs": []map[string]any{{"status": "completed", "conclusion": "failure", "html_url": "https://github.com/acme/api/actions/runs/9"}}})
		case p == "/repos/acme/busy/pulls":
			w.Header().Set("Link", `<https://x/next>; rel="next"`)
			prs := make([]map[string]any, 100)
			js(prs)
		case p == "/repos/acme/busy/actions/runs":
			js(map[string]any{"workflow_runs": []map[string]any{{"status": "in_progress"}}})
		// GitLab
		case p == "/projects/group%2Fsub%2Fsvc/merge_requests":
			if r.Header.Get("PRIVATE-TOKEN") != "gl-tok" {
				w.WriteHeader(401)
				return
			}
			w.Header().Set("X-Total", "7")
			js([]any{})
		case p == "/projects/group%2Fsub%2Fsvc/pipelines":
			js([]map[string]any{{"status": "success", "web_url": "https://gitlab.com/group/sub/svc/-/pipelines/3"}})
		case p == "/groups/group/projects":
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("X-Next-Page", "2")
				js([]map[string]any{{"path_with_namespace": "group/a", "ssh_url_to_repo": "git@gitlab.com:group/a.git"}})
			} else {
				js([]map[string]any{{"path_with_namespace": "group/sub/b", "archived": true}})
			}
		// Bitbucket
		case p == "/repositories/acme/app/pullrequests":
			if u, pw, ok := r.BasicAuth(); !ok || u != "me@example.com" || pw != "bb-tok" {
				w.WriteHeader(401)
				return
			}
			js(map[string]any{"size": 3})
		case p == "/repositories/acme/app/pipelines/":
			js(map[string]any{"values": []map[string]any{{"build_number": 42, "state": map[string]any{"name": "COMPLETED", "result": map[string]any{"name": "SUCCESSFUL"}}}}})
		case p == "/repositories/acme":
			if r.URL.Query().Get("page") == "" {
				js(map[string]any{"next": "http://" + r.Host + "/repositories/acme?page=2", "values": []map[string]any{{"full_name": "acme/app", "links": map[string]any{"clone": []map[string]any{{"name": "ssh", "href": "git@bitbucket.org:acme/app.git"}}}}}})
			} else {
				js(map[string]any{"values": []map[string]any{{"full_name": "acme/new"}}})
			}
		// Gitea
		case p == "/repos/me/tool/pulls":
			if r.Header.Get("Authorization") != "token gt-tok" {
				w.WriteHeader(401)
				return
			}
			w.Header().Set("X-Total-Count", "1")
			js([]any{})
		case p == "/repos/me/tool/commits/main/status":
			js(map[string]any{"state": "pending", "total_count": 2})
		default:
			w.WriteHeader(404)
		}
	}))
}

func client(t *testing.T, base string) *Client {
	t.Helper()
	t.Setenv("MRM_TEST_GL", "gl-tok")
	cfg := &config.Config{
		ForgeTokens: map[string]config.ForgeToken{
			"github.com":    {TokenCmd: "echo gh-tok"},
			"gitlab.com":    {TokenEnv: "MRM_TEST_GL"},
			"bitbucket.org": {User: "me@example.com", TokenCmd: "echo bb-tok"},
			"codeberg.org":  {TokenCmd: "echo gt-tok"},
		},
	}
	cfg.ForgeCache.Duration = time.Hour
	c := New(cfg, filepath.Join(t.TempDir(), "forge.json"))
	c.BaseURL = map[string]string{"github.com": base, "gitlab.com": base, "bitbucket.org": base, "codeberg.org": base}
	return c
}

// 4.5: PR counts and CI state per forge, each with its own auth.
func TestInfoPerForge(t *testing.T) {
	var calls atomic.Int32
	srv := fakeForge(t, &calls)
	defer srv.Close()
	c := client(t, srv.URL)
	ctx := context.Background()
	cases := []struct {
		remote string
		want   Info
	}{
		{"git@github.com:acme/api.git", Info{PRs: 2, CI: CIFailed, CIURL: "https://github.com/acme/api/actions/runs/9"}},
		{"git@github.com:acme/busy.git", Info{PRs: 100, PRsMore: true, CI: CIRunning}},
		{"git@gitlab.com:group/sub/svc.git", Info{PRs: 7, CI: CIPassing, CIURL: "https://gitlab.com/group/sub/svc/-/pipelines/3"}},
		{"git@bitbucket.org:acme/app.git", Info{PRs: 3, CI: CIPassing, CIURL: "https://bitbucket.org/acme/app/pipelines/results/42"}},
		{"https://codeberg.org/me/tool.git", Info{PRs: 1, CI: CIRunning}},
	}
	for _, tc := range cases {
		got := c.Get(ctx, forge.Parse(tc.remote, nil), "main")
		got.Fetched = time.Time{}
		if got != tc.want {
			t.Errorf("%s:\n got %+v\nwant %+v", tc.remote, got, tc.want)
		}
	}
	if n := Notes(Info{PRs: 2, CI: CIFailed}); strings.Join(n, ",") != "2 PRs,CI failed" {
		t.Errorf("notes %v", n)
	}
	if n := Notes(Info{CI: CIPassing}); len(n) != 0 {
		t.Errorf("a passing CI isn't news: %v", n)
	}
}

// Answers are cached for the TTL and saved to disk.
func TestCache(t *testing.T) {
	var calls atomic.Int32
	srv := fakeForge(t, &calls)
	defer srv.Close()
	c := client(t, srv.URL)
	f := forge.Parse("git@github.com:acme/api.git", nil)
	c.Get(context.Background(), f, "main")
	n := calls.Load()
	c.Get(context.Background(), f, "main")
	if calls.Load() != n {
		t.Fatal("second Get within the TTL asked the forge again")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2 := New(&config.Config{}, c.CachePath)
	if i, ok := c2.Cached(f); !ok || i.PRs != 2 {
		t.Fatalf("cache not reloaded: %+v %v", i, ok)
	}
}

// Without a token a host is never contacted; a rejected token is an error.
func TestTokens(t *testing.T) {
	var calls atomic.Int32
	srv := fakeForge(t, &calls)
	defer srv.Close()
	c := client(t, srv.URL)
	if c.Enabled(forge.Parse("git@git.example.com:x/y.git", nil)) {
		t.Fatal("unknown host enabled")
	}
	c.Tokens["github.com"] = config.ForgeToken{TokenCmd: "echo wrong"}
	c.tokens = map[string]string{}
	i := c.Get(context.Background(), forge.Parse("git@github.com:acme/api.git", nil), "main")
	if !strings.Contains(i.Err, "401") {
		t.Fatalf("err %q", i.Err)
	}
}

// drift: org listings follow every page.
func TestOrgRepos(t *testing.T) {
	var calls atomic.Int32
	srv := fakeForge(t, &calls)
	defer srv.Close()
	c := client(t, srv.URL)
	gl, err := c.OrgRepos(context.Background(), "gitlab.com/group")
	if err != nil || len(gl) != 2 || gl[1].Path != "group/sub/b" || !gl[1].Archived {
		t.Fatalf("gitlab %+v %v", gl, err)
	}
	bb, err := c.OrgRepos(context.Background(), "bitbucket.org/acme")
	if err != nil || len(bb) != 2 || bb[0].CloneURL != "git@bitbucket.org:acme/app.git" {
		t.Fatalf("bitbucket %+v %v", bb, err)
	}
	if _, err := c.OrgRepos(context.Background(), "git.example.com/x"); err == nil {
		t.Fatal("no token: no error")
	}
}
