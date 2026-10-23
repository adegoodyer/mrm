// Package forgeapi asks GitHub, GitLab, Bitbucket Cloud and Gitea for what
// git can't tell: open pull requests, the default branch's CI status, and
// the repos in an org (for mrm drift). Only hosts with a token in
// host_tokens are contacted; answers are cached on disk.
package forgeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/forge"
	"github.com/adegoodyer/mrm/internal/secret"
)

// CI states.
const (
	CIPassing   = "passing"
	CIFailed    = "failed"
	CIRunning   = "running"
	CICancelled = "cancelled"
	CINone      = "none" // no runs on the default branch
)

// Info is one repo's forge answer.
type Info struct {
	PRs     int       `json:"prs"`
	PRsMore bool      `json:"prs_more,omitempty"` // PRs is a lower bound (100+)
	CI      string    `json:"ci,omitempty"`
	CIURL   string    `json:"ci_url,omitempty"`
	Err     string    `json:"error,omitempty"`
	Fetched time.Time `json:"fetched"`
}

// OrgRepo is a repo listed in an org on the forge.
type OrgRepo struct {
	Path     string `json:"path"`                // owner/name
	CloneURL string `json:"clone_url,omitempty"` // ssh when the forge gives one
	Archived bool   `json:"archived,omitempty"`
}

// Client talks to the forges that have tokens.
type Client struct {
	HTTP      *http.Client
	Tokens    map[string]config.ForgeToken
	Kinds     map[string]string // self-hosted host -> kind
	CachePath string
	TTL       time.Duration
	// BaseURL overrides the API base for a host (tests).
	BaseURL map[string]string

	mu     sync.Mutex
	cache  map[string]Info
	tokens map[string]string
}

// New builds a client from the config.
func New(cfg *config.Config, cachePath string) *Client {
	c := &Client{
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		Tokens:    cfg.ForgeTokens,
		Kinds:     cfg.Forges,
		CachePath: cachePath,
		TTL:       cfg.ForgeCache.Duration,
		cache:     map[string]Info{},
		tokens:    map[string]string{},
	}
	if data, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(data, &c.cache)
	}
	return c
}

// Any reports whether any host has a token.
func (c *Client) Any() bool { return len(c.Tokens) > 0 }

// Enabled reports whether f's host has a token and a known API.
func (c *Client) Enabled(f forge.Forge) bool {
	_, ok := c.Tokens[f.Host]
	return ok && f.Kind != forge.None && f.Kind != forge.Unknown
}

func key(f forge.Forge) string { return f.Host + "/" + strings.ToLower(f.Path) }

// Cached returns the last answer for f, however old.
func (c *Client) Cached(f forge.Forge) (Info, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	i, ok := c.cache[key(f)]
	return i, ok
}

// Get returns a fresh enough answer for f, asking the forge when the cache
// is older than TTL (errors are retried after a minute).
func (c *Client) Get(ctx context.Context, f forge.Forge, defaultBranch string) Info {
	if i, ok := c.Cached(f); ok {
		ttl := c.TTL
		if i.Err != "" {
			ttl = time.Minute
		}
		if time.Since(i.Fetched) < ttl {
			return i
		}
	}
	i := c.fetch(ctx, f, defaultBranch)
	i.Fetched = time.Now()
	c.mu.Lock()
	c.cache[key(f)] = i
	c.mu.Unlock()
	return i
}

// Save writes the cache.
func (c *Client) Save() error {
	c.mu.Lock()
	data, err := json.Marshal(c.cache)
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.CachePath), 0o700); err != nil {
		return err
	}
	tmp := c.CachePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.CachePath)
}

func (c *Client) fetch(ctx context.Context, f forge.Forge, def string) Info {
	if !c.Enabled(f) {
		return Info{Err: "no token for " + f.Host}
	}
	var i Info
	var err error
	switch f.Kind {
	case forge.GitHub:
		i, err = c.github(ctx, f, def)
	case forge.GitLab:
		i, err = c.gitlab(ctx, f, def)
	case forge.Bitbucket:
		i, err = c.bitbucket(ctx, f, def)
	case forge.Gitea:
		i, err = c.gitea(ctx, f, def)
	}
	if err != nil {
		i.Err = err.Error()
	}
	return i
}

// base is the API root for a host.
func (c *Client) base(f forge.Forge) string {
	if b, ok := c.BaseURL[f.Host]; ok {
		return strings.TrimRight(b, "/")
	}
	switch f.Kind {
	case forge.GitHub:
		if f.Host == "github.com" {
			return "https://api.github.com"
		}
		return "https://" + f.Host + "/api/v3"
	case forge.GitLab:
		return "https://" + f.Host + "/api/v4"
	case forge.Bitbucket:
		return "https://api.bitbucket.org/2.0"
	case forge.Gitea:
		return "https://" + f.Host + "/api/v1"
	}
	return ""
}

func (c *Client) token(ctx context.Context, host string) (string, error) {
	c.mu.Lock()
	t, ok := c.tokens[host]
	c.mu.Unlock()
	if ok {
		return t, nil
	}
	ft := c.Tokens[host]
	t, err := secret.Resolve(ctx, ft.TokenCmd, ft.TokenEnv)
	if err != nil {
		return "", fmt.Errorf("token for %s: %w", host, err)
	}
	c.mu.Lock()
	c.tokens[host] = t
	c.mu.Unlock()
	return t, nil
}

// get fetches a URL as JSON into v (nil to skip the body) and returns the
// response headers.
func (c *Client) get(ctx context.Context, f forge.Forge, rawURL string, v any) (http.Header, error) {
	tok, err := c.token(ctx, f.Host)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	switch f.Kind {
	case forge.GitHub:
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/vnd.github+json")
	case forge.GitLab:
		req.Header.Set("PRIVATE-TOKEN", tok)
	case forge.Bitbucket:
		if u := c.Tokens[f.Host].User; u != "" {
			req.SetBasicAuth(u, tok)
		} else {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	case forge.Gitea:
		req.Header.Set("Authorization", "token "+tok)
	}
	req.Header.Set("User-Agent", "mrm")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, errors.New("token rejected (HTTP 401)")
	case resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0", resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("rate limited - try again later")
	case resp.StatusCode == http.StatusForbidden:
		return nil, errors.New("forbidden (HTTP 403) - does the token have read access?")
	case resp.StatusCode == http.StatusNotFound:
		return resp.Header, errNotFound
	case resp.StatusCode/100 != 2:
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return nil, fmt.Errorf("unexpected answer: %w", err)
		}
	}
	return resp.Header, nil
}

var errNotFound = errors.New("not found - a private repo the token can't see?")

func pathEsc(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// GitHub: open PRs (up to 100, then "100+") and the latest Actions run on
// the default branch.
func (c *Client) github(ctx context.Context, f forge.Forge, def string) (Info, error) {
	var i Info
	var prs []json.RawMessage
	h, err := c.get(ctx, f, c.base(f)+"/repos/"+pathEsc(f.Path)+"/pulls?state=open&per_page=100", &prs)
	if err != nil {
		return i, err
	}
	i.PRs, i.PRsMore = len(prs), strings.Contains(h.Get("Link"), `rel="next"`)
	var runs struct {
		Runs []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			URL        string `json:"html_url"`
		} `json:"workflow_runs"`
	}
	if _, err := c.get(ctx, f, c.base(f)+"/repos/"+pathEsc(f.Path)+"/actions/runs?per_page=1&branch="+url.QueryEscape(def), &runs); err != nil && err != errNotFound {
		return i, err
	}
	i.CI = CINone
	if len(runs.Runs) > 0 {
		r := runs.Runs[0]
		i.CIURL = r.URL
		switch {
		case r.Status != "completed":
			i.CI = CIRunning
		case r.Conclusion == "success" || r.Conclusion == "skipped" || r.Conclusion == "neutral":
			i.CI = CIPassing
		case r.Conclusion == "cancelled":
			i.CI = CICancelled
		default:
			i.CI = CIFailed
		}
	}
	return i, nil
}

// GitLab: open merge requests (X-Total) and the latest pipeline on the
// default branch.
func (c *Client) gitlab(ctx context.Context, f forge.Forge, def string) (Info, error) {
	var i Info
	id := url.PathEscape(f.Path)
	h, err := c.get(ctx, f, c.base(f)+"/projects/"+id+"/merge_requests?state=opened&per_page=1", nil)
	if err != nil {
		return i, err
	}
	i.PRs, _ = strconv.Atoi(h.Get("X-Total"))
	var pipes []struct {
		Status string `json:"status"`
		URL    string `json:"web_url"`
	}
	if _, err := c.get(ctx, f, c.base(f)+"/projects/"+id+"/pipelines?per_page=1&ref="+url.QueryEscape(def), &pipes); err != nil && err != errNotFound {
		return i, err
	}
	i.CI = CINone
	if len(pipes) > 0 {
		i.CIURL = pipes[0].URL
		switch pipes[0].Status {
		case "success", "skipped", "manual":
			i.CI = CIPassing
		case "failed":
			i.CI = CIFailed
		case "canceled":
			i.CI = CICancelled
		default: // created, waiting_for_resource, preparing, pending, running, scheduled
			i.CI = CIRunning
		}
	}
	return i, nil
}

// Bitbucket Cloud: open pull requests ("size") and the latest pipeline on
// the default branch (none when Pipelines isn't enabled).
func (c *Client) bitbucket(ctx context.Context, f forge.Forge, def string) (Info, error) {
	var i Info
	var prs struct {
		Size int `json:"size"`
	}
	if _, err := c.get(ctx, f, c.base(f)+"/repositories/"+pathEsc(f.Path)+"/pullrequests?state=OPEN&pagelen=1", &prs); err != nil {
		return i, err
	}
	i.PRs = prs.Size
	var pipes struct {
		Values []struct {
			BuildNumber int `json:"build_number"`
			State       struct {
				Name   string `json:"name"`
				Result struct {
					Name string `json:"name"`
				} `json:"result"`
			} `json:"state"`
		} `json:"values"`
	}
	if _, err := c.get(ctx, f, c.base(f)+"/repositories/"+pathEsc(f.Path)+"/pipelines/?sort=-created_on&pagelen=1&target.branch="+url.QueryEscape(def), &pipes); err != nil && err != errNotFound {
		return i, err
	}
	i.CI = CINone
	if len(pipes.Values) > 0 {
		p := pipes.Values[0]
		i.CIURL = fmt.Sprintf("https://bitbucket.org/%s/pipelines/results/%d", f.Path, p.BuildNumber)
		switch {
		case p.State.Name != "COMPLETED":
			i.CI = CIRunning
		case p.State.Result.Name == "SUCCESSFUL":
			i.CI = CIPassing
		case p.State.Result.Name == "STOPPED":
			i.CI = CICancelled
		default:
			i.CI = CIFailed
		}
	}
	return i, nil
}

// Gitea / Forgejo / Codeberg: open PRs (X-Total-Count) and the combined
// commit status of the default branch.
func (c *Client) gitea(ctx context.Context, f forge.Forge, def string) (Info, error) {
	var i Info
	h, err := c.get(ctx, f, c.base(f)+"/repos/"+pathEsc(f.Path)+"/pulls?state=open&limit=1", nil)
	if err != nil {
		return i, err
	}
	i.PRs, _ = strconv.Atoi(h.Get("X-Total-Count"))
	var st struct {
		State      string `json:"state"`
		TotalCount int    `json:"total_count"`
	}
	if _, err := c.get(ctx, f, c.base(f)+"/repos/"+pathEsc(f.Path)+"/commits/"+url.PathEscape(def)+"/status", &st); err != nil && err != errNotFound {
		return i, err
	}
	switch {
	case st.TotalCount == 0:
		i.CI = CINone
	case st.State == "success":
		i.CI = CIPassing
	case st.State == "pending":
		i.CI = CIRunning
	default:
		i.CI = CIFailed
	}
	return i, nil
}

// OrgRepos lists the repos of an org (or user) on a forge, following every
// page. org is "host/owner".
func (c *Client) OrgRepos(ctx context.Context, org string) ([]OrgRepo, error) {
	host, owner, _ := strings.Cut(org, "/")
	f := forge.Parse("https://"+org+"/x", c.Kinds)
	f.Host, f.Path = host, owner
	if _, ok := c.Tokens[host]; !ok {
		return nil, fmt.Errorf("no token for %s in host_tokens", host)
	}
	switch f.Kind {
	case forge.GitHub:
		return c.githubOrg(ctx, f, owner)
	case forge.GitLab:
		return c.gitlabOrg(ctx, f, owner)
	case forge.Bitbucket:
		return c.bitbucketOrg(ctx, f, owner)
	case forge.Gitea:
		return c.giteaOrg(ctx, f, owner)
	}
	return nil, fmt.Errorf("%s: unknown VCS host (say which kind it is under hosts:)", host)
}

func (c *Client) githubOrg(ctx context.Context, f forge.Forge, owner string) ([]OrgRepo, error) {
	// /users/<name>/repos lists public repos only; for the token's own
	// account /user/repos includes the private ones.
	var me struct {
		Login string `json:"login"`
	}
	_, _ = c.get(ctx, f, c.base(f)+"/user", &me)
	var out []OrgRepo
	for _, kind := range []string{"orgs", "users"} {
		out = nil
		next := c.base(f) + "/" + kind + "/" + url.PathEscape(owner) + "/repos?per_page=100&type=all"
		if kind == "users" && strings.EqualFold(me.Login, owner) {
			next = c.base(f) + "/user/repos?per_page=100&affiliation=owner"
		}
		var err error
		for next != "" && err == nil {
			var page []struct {
				FullName string `json:"full_name"`
				SSH      string `json:"ssh_url"`
				Archived bool   `json:"archived"`
			}
			var h http.Header
			h, err = c.get(ctx, f, next, &page)
			for _, r := range page {
				out = append(out, OrgRepo{Path: r.FullName, CloneURL: r.SSH, Archived: r.Archived})
			}
			next = ""
			if err == nil {
				next = linkNext(h.Get("Link"))
			}
		}
		if err == errNotFound {
			continue // not an org: try as a user
		}
		return out, err
	}
	return nil, fmt.Errorf("%s: no such org or user", owner)
}

func (c *Client) gitlabOrg(ctx context.Context, f forge.Forge, owner string) ([]OrgRepo, error) {
	for _, kind := range []string{"groups", "users"} {
		var out []OrgRepo
		page, err := "1", error(nil)
		for page != "" && err == nil {
			var items []struct {
				Path     string `json:"path_with_namespace"`
				SSH      string `json:"ssh_url_to_repo"`
				Archived bool   `json:"archived"`
			}
			var h http.Header
			extra := ""
			if kind == "groups" {
				extra = "&include_subgroups=true"
			}
			h, err = c.get(ctx, f, c.base(f)+"/"+kind+"/"+url.PathEscape(owner)+"/projects?per_page=100&page="+page+extra, &items)
			for _, r := range items {
				out = append(out, OrgRepo{Path: r.Path, CloneURL: r.SSH, Archived: r.Archived})
			}
			page = ""
			if err == nil {
				page = h.Get("X-Next-Page")
			}
		}
		if err == errNotFound {
			continue
		}
		return out, err
	}
	return nil, fmt.Errorf("%s: no such group or user", owner)
}

func (c *Client) bitbucketOrg(ctx context.Context, f forge.Forge, owner string) ([]OrgRepo, error) {
	var out []OrgRepo
	next := c.base(f) + "/repositories/" + url.PathEscape(owner) + "?pagelen=100"
	for next != "" {
		var page struct {
			Next   string `json:"next"`
			Values []struct {
				FullName string `json:"full_name"`
				Links    struct {
					Clone []struct {
						Name string `json:"name"`
						Href string `json:"href"`
					} `json:"clone"`
				} `json:"links"`
			} `json:"values"`
		}
		if _, err := c.get(ctx, f, next, &page); err != nil {
			return out, err
		}
		for _, r := range page.Values {
			repo := OrgRepo{Path: r.FullName}
			for _, l := range r.Links.Clone {
				if l.Name == "ssh" {
					repo.CloneURL = l.Href
				}
			}
			out = append(out, repo)
		}
		next = page.Next
	}
	return out, nil
}

func (c *Client) giteaOrg(ctx context.Context, f forge.Forge, owner string) ([]OrgRepo, error) {
	for _, kind := range []string{"orgs", "users"} {
		var out []OrgRepo
		var err error
		for page := 1; ; page++ {
			var items []struct {
				FullName string `json:"full_name"`
				SSH      string `json:"ssh_url"`
				Archived bool   `json:"archived"`
			}
			_, err = c.get(ctx, f, fmt.Sprintf("%s/%s/%s/repos?limit=50&page=%d", c.base(f), kind, url.PathEscape(owner), page), &items)
			if err != nil || len(items) == 0 {
				break
			}
			for _, r := range items {
				out = append(out, OrgRepo{Path: r.FullName, CloneURL: r.SSH, Archived: r.Archived})
			}
		}
		if err == errNotFound {
			continue
		}
		return out, err
	}
	return nil, fmt.Errorf("%s: no such org or user", owner)
}

// linkNext reads rel="next" from a Link header.
func linkNext(link string) string {
	for _, part := range strings.Split(link, ",") {
		if strings.Contains(part, `rel="next"`) {
			if s, e := strings.Index(part, "<"), strings.Index(part, ">"); s >= 0 && e > s {
				return part[s+1 : e]
			}
		}
	}
	return ""
}

// Notes is the short text for a list row: open PRs and a CI problem, or
// nothing (a passing CI isn't news).
func Notes(i Info) []string {
	var n []string
	if i.Err != "" {
		return []string{"PRs/CI: " + i.Err}
	}
	switch {
	case i.PRsMore:
		n = append(n, fmt.Sprintf("%d+ PRs", i.PRs))
	case i.PRs == 1:
		n = append(n, "1 PR")
	case i.PRs > 1:
		n = append(n, fmt.Sprintf("%d PRs", i.PRs))
	}
	switch i.CI {
	case CIFailed:
		n = append(n, "CI failed")
	case CIRunning:
		n = append(n, "CI running")
	}
	return n
}

// Target is one repo to ask about.
type Target struct {
	Path, RemoteURL, DefaultBranch string
}

// CachedFor returns the last answer for a target, however old.
func (c *Client) CachedFor(t Target) (Info, bool) {
	f := forge.Parse(t.RemoteURL, c.Kinds)
	if !c.Enabled(f) {
		return Info{}, false
	}
	return c.Cached(f)
}

// Refresh asks about every target whose host has a token, a few at a time,
// calls onInfo with each answer (from its goroutine), then saves the cache.
func (c *Client) Refresh(ctx context.Context, targets []Target, concurrency int, onInfo func(path string, i Info)) {
	sem := make(chan struct{}, max(concurrency, 1))
	var wg sync.WaitGroup
	for _, t := range targets {
		f := forge.Parse(t.RemoteURL, c.Kinds)
		if !c.Enabled(f) || t.DefaultBranch == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			i := c.Get(ctx, f, t.DefaultBranch)
			if onInfo != nil {
				onInfo(t.Path, i)
			}
		}()
	}
	wg.Wait()
	_ = c.Save()
}
