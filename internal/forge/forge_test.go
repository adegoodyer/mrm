package forge

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		remote     string
		kind       Kind
		host, path string
	}{
		{"git@bitbucket.org:acme/api.git", Bitbucket, "bitbucket.org", "acme/api"},
		{"git@gitlab.com:acme/releases.git", GitLab, "gitlab.com", "acme/releases"},
		{"git@github.com:adegoodyer/mrm.git", GitHub, "github.com", "adegoodyer/mrm"},
		{"https://github.com/adegoodyer/mrm", GitHub, "github.com", "adegoodyer/mrm"},
		{"https://user@gitlab.com/group/sub/repo.git/", GitLab, "gitlab.com", "group/sub/repo"},
		{"ssh://git@git.example.com:2222/team/repo.git", Unknown, "git.example.com", "team/repo"},
		{"https://codeberg.org/someone/thing.git", Gitea, "codeberg.org", "someone/thing"},
		{"/srv/git/repo.git", None, "", ""},
		{"file:///srv/git/repo.git", None, "", ""},
		{"", None, "", ""},
	}
	for _, c := range cases {
		f := Parse(c.remote, nil)
		if f.Kind != c.kind || f.Host != c.host || f.Path != c.path {
			t.Errorf("%q -> %+v, want %s %s %s", c.remote, f, c.kind, c.host, c.path)
		}
	}
}

func TestSelfHostedMapping(t *testing.T) {
	f := Parse("git@git.example.com:team/repo.git", map[string]string{"git.example.com": "gitlab"})
	u, err := f.PRs()
	if err != nil || u != "https://git.example.com/team/repo/-/merge_requests" {
		t.Fatalf("%q %v", u, err)
	}
}

// 1.13: the right page per forge.
func TestURLs(t *testing.T) {
	cases := []struct {
		remote, pr, ci, branch string
	}{
		{"git@github.com:o/r.git", "https://github.com/o/r/pulls", "https://github.com/o/r/actions", "https://github.com/o/r/tree/feature/a%20b"},
		{"git@gitlab.com:o/r.git", "https://gitlab.com/o/r/-/merge_requests", "https://gitlab.com/o/r/-/pipelines", "https://gitlab.com/o/r/-/tree/feature/a%20b"},
		{"git@bitbucket.org:o/r.git", "https://bitbucket.org/o/r/pull-requests", "https://bitbucket.org/o/r/pipelines", "https://bitbucket.org/o/r/src/feature/a%20b"},
		{"https://codeberg.org/o/r", "https://codeberg.org/o/r/pulls", "https://codeberg.org/o/r/actions", "https://codeberg.org/o/r/src/branch/feature/a%20b"},
	}
	for _, c := range cases {
		f := Parse(c.remote, nil)
		pr, _ := f.PRs()
		ci, _ := f.CI()
		br, _ := f.Branch("feature/a b")
		if pr != c.pr || ci != c.ci || br != c.branch {
			t.Errorf("%s:\n got %s %s %s\nwant %s %s %s", c.remote, pr, ci, br, c.pr, c.ci, c.branch)
		}
	}
	if _, err := Parse("git@git.example.com:o/r.git", nil).PRs(); err == nil {
		t.Error("unknown forge PRs: no error")
	}
	if _, err := Parse("/local", nil).Web(); err == nil {
		t.Error("local path web: no error")
	}
}
