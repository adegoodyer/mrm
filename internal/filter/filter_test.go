package filter

import (
	"testing"
	"time"

	"github.com/adegoodyer/mrm/internal/model"
)

func TestMatch(t *testing.T) {
	app := model.Repo{Name: "app", Path: "/work/app", Workspace: "work", Tags: []string{"core"}}
	api := model.Repo{Name: "billing-api", Path: "/work/platform/billing-api", Workspace: "work", Group: "platform", Tags: []string{"archived"}}
	dirty := model.Status{Branch: "feature/x", Modified: 2, HasRemote: true, DefaultBranch: "master", Upstream: "origin/feature/x",
		RemoteURL: "git@bitbucket.org:acme/app.git", LastCommit: time.Now().Add(-40 * 24 * time.Hour)}
	clean := model.Status{Branch: "master", HasRemote: true, DefaultBranch: "master", Upstream: "origin/master", Stashes: 1,
		RemoteURL: "https://github.com/acme/api.git", LastCommit: time.Now().Add(-time.Hour)}

	cases := []struct {
		q        string
		app, api bool
	}{
		{"", true, true},
		{"app", true, false},
		{"api", false, true},
		{"mapi", false, false}, // substring, not fuzzy
		{"platform/", false, true},
		{"is:dirty", true, false},
		{"is:clean", false, true},
		{"is:stash", false, true},
		{"is:not-default", true, false},
		{"-is:dirty", false, true},
		{"ws:work group:platform", false, true},
		{"tag:core", true, false},
		{"-tag:archived", true, false},
		{"branch:feature/*", true, false},
		{"forge:bitbucket", true, false},
		{"age:>30d", true, false},
		{"age:<1d", false, true},
	}
	for _, c := range cases {
		f, err := Parse(c.q)
		if err != nil {
			t.Fatalf("%q: %v", c.q, err)
		}
		if got := f.Match(app, dirty); got != c.app {
			t.Errorf("%q on app = %v, want %v", c.q, got, c.app)
		}
		if got := f.Match(api, clean); got != c.api {
			t.Errorf("%q on api = %v, want %v", c.q, got, c.api)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, q := range []string{"is:nonsense", "age:30d", "age:>soon", "tag:", "branch:[", "is:"} {
		if _, err := Parse(q); err == nil {
			t.Errorf("%q: no error", q)
		}
	}
}

func TestNeedsStatus(t *testing.T) {
	if MustParse("tag:x ws:work app").NeedsStatus() {
		t.Fatal("tag/ws/word don't need status")
	}
	if !MustParse("is:dirty").NeedsStatus() {
		t.Fatal("is: needs status")
	}
}

func TestQuotes(t *testing.T) {
	f := MustParse(`"my repo" tag:x`)
	if len(f.terms) != 2 || f.terms[0].value != "my repo" {
		t.Fatalf("%+v", f.terms)
	}
}

func TestForgeUsesSelfHostedMapping(t *testing.T) {
	r := model.Repo{Name: "x", Path: "/x"}
	st := model.Status{RemoteURL: "git@git.example.com:team/x.git", ReadAt: time.Now()}
	if MustParse("forge:gitlab").Match(r, st) {
		t.Fatal("unmapped self-hosted host should not be gitlab")
	}
	if !MustParse("forge:gitlab").WithForges(map[string]string{"git.example.com": "gitlab"}).Match(r, st) {
		t.Fatal("mapped self-hosted host should be gitlab")
	}
}
