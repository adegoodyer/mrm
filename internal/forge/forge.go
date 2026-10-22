// Package forge turns a git remote URL into web URLs for the repo, its pull
// requests, its CI and a branch.
package forge

import (
	"fmt"
	"net/url"
	"strings"
)

// Kind identifies a forge.
type Kind string

const (
	GitHub    Kind = "github"
	GitLab    Kind = "gitlab"
	Bitbucket Kind = "bitbucket"
	Gitea     Kind = "gitea"
	Unknown   Kind = "unknown"
	None      Kind = "" // no remote, or a local path
)

// Name is how the kind is written for people: GitHub, GitLab...
func (k Kind) Name() string {
	switch k {
	case GitHub:
		return "GitHub"
	case GitLab:
		return "GitLab"
	case Bitbucket:
		return "Bitbucket"
	case Gitea:
		return "Gitea"
	}
	return "the git host"
}

// Forge is a parsed remote.
type Forge struct {
	Kind Kind
	Host string // without port
	Path string // owner/repo (GitLab may have subgroups), no .git
}

// Parse reads a remote URL: scp-like (git@host:owner/repo.git), ssh://,
// https:// or git://. hosts maps self-hosted hostnames to a kind.
func Parse(remote string, hosts map[string]string) Forge {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return Forge{}
	}
	var host, path string
	switch {
	case strings.Contains(remote, "://"):
		u, err := url.Parse(remote)
		if err != nil || u.Scheme == "file" || u.Host == "" {
			return Forge{}
		}
		host, path = u.Hostname(), u.Path
	case strings.Contains(remote, ":") && !strings.HasPrefix(remote, "/"):
		// scp-like: [user@]host:path
		h, p, _ := strings.Cut(remote, ":")
		if i := strings.LastIndex(h, "@"); i >= 0 {
			h = h[i+1:]
		}
		host, path = h, p
	default:
		return Forge{} // local path
	}
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" {
		return Forge{}
	}
	host = strings.ToLower(host)
	return Forge{Kind: detect(host, hosts), Host: host, Path: path}
}

func detect(host string, hosts map[string]string) Kind {
	if k, ok := hosts[host]; ok {
		return Kind(k)
	}
	h := strings.TrimPrefix(host, "www.")
	switch {
	case h == "github.com" || strings.HasSuffix(h, ".ghe.com"):
		return GitHub
	case h == "gitlab.com" || strings.Contains(h, "gitlab"):
		return GitLab
	case h == "bitbucket.org" || strings.Contains(h, "bitbucket"):
		return Bitbucket
	case h == "codeberg.org" || strings.Contains(h, "gitea") || strings.Contains(h, "forgejo"):
		return Gitea
	}
	return Unknown
}

// Web is the repo's home page.
func (f Forge) Web() (string, error) {
	if f.Kind == None {
		return "", fmt.Errorf("no web URL: the remote is missing or local")
	}
	return "https://" + f.Host + "/" + f.Path, nil
}

// PRs is the repo's pull/merge request list.
func (f Forge) PRs() (string, error) {
	return f.page(map[Kind]string{GitHub: "/pulls", GitLab: "/-/merge_requests", Bitbucket: "/pull-requests", Gitea: "/pulls"}, "pull requests")
}

// CI is the repo's pipelines / actions page.
func (f Forge) CI() (string, error) {
	return f.page(map[Kind]string{GitHub: "/actions", GitLab: "/-/pipelines", Bitbucket: "/pipelines", Gitea: "/actions"}, "CI")
}

// Branch is the page for a branch's tree.
func (f Forge) Branch(branch string) (string, error) {
	if branch == "" {
		return "", fmt.Errorf("no branch (detached HEAD)")
	}
	segs := strings.Split(branch, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	b := strings.Join(segs, "/")
	return f.page(map[Kind]string{GitHub: "/tree/" + b, GitLab: "/-/tree/" + b, Bitbucket: "/src/" + b, Gitea: "/src/branch/" + b}, "branch")
}

func (f Forge) page(suffix map[Kind]string, what string) (string, error) {
	base, err := f.Web()
	if err != nil {
		return "", err
	}
	s, ok := suffix[f.Kind]
	if !ok {
		return "", fmt.Errorf("don't know the %s page layout for %s (say which kind it is under `hosts:` in the config)", what, f.Host)
	}
	return base + s, nil
}
