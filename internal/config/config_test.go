package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The example from the spec's Configuration section must load.
const specExample = `
concurrency: 8
timeout: 30s
browser_cmd: ""

workspaces:
  work:
    roots: [~/work]
    depth: 3
    ignore: [legacy-*, "*-archive"]
    sync:
      ff_current: false
      switch_merged: false
  personal:
    roots: [~/src/github.com/adegoodyer]

repos:
  payments-service:
    branch: develop
    tags: [backend]
  "~/work/terraform-*":
    tags: [terraform]
  "~/work/legacy-portal":
    sync: false

hooks:
  post_sync:
    - name: pre-commit
      if_file: .pre-commit-config.yaml
      run: pre-commit install --install-hooks

notify:
  on: attention
  gotify:
    url: https://gotify.example.com
    token_cmd: pass show gotify/mrm
  ntfy:
    url: https://ntfy.example.com/topic
    token_env: MRM_NTFY_TOKEN

tui:
  open_cmd: lazygit
  fetch_on_start: false

web:
  port: 7420
  text_size: large
`

func TestParseSpecExample(t *testing.T) {
	cfg, err := Parse([]byte(specExample))
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if got := cfg.Workspaces["work"].Roots[0]; got != filepath.Join(home, "work") {
		t.Fatalf("root %q not expanded", got)
	}
	if cfg.Workspaces["work"].Sync.Remote != "origin" || cfg.Workspaces["personal"].Depth != 3 {
		t.Fatal("defaults not applied")
	}
	if cfg.Timeout.Duration != 30*time.Second {
		t.Fatalf("timeout %v", cfg.Timeout)
	}
	if got := cfg.WorkspaceNames(); strings.Join(got, ",") != "personal,work" {
		t.Fatalf("names %v", got)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"unknown key":    "workspaces:\n  work:\n    roots: [/tmp]\n    rootz: [/x]\n",
		"no roots":       "workspaces:\n  work: {}\n",
		"no workspaces":  "concurrency: 4\n",
		"bad duration":   "timeout: soon\nworkspaces:\n  work:\n    roots: [/tmp]\n",
		"bad notify.on":  "notify:\n  on: sometimes\nworkspaces:\n  work:\n    roots: [/tmp]\n",
		"bad text size":  "web:\n  text_size: huge\nworkspaces:\n  work:\n    roots: [/tmp]\n",
		"bad forge kind": "forges:\n  git.example.com: sourcehut\nworkspaces:\n  work:\n    roots: [/tmp]\n",
	}
	for name, yaml := range cases {
		if _, err := Parse([]byte(yaml)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestLiteralTokenRefusedWithHint(t *testing.T) {
	_, err := Parse([]byte("notify:\n  gotify:\n    url: https://x\n    token: abc\nworkspaces:\n  work:\n    roots: [/tmp]\n"))
	if err == nil || !strings.Contains(err.Error(), "token_cmd") {
		t.Fatalf("err %v, want a token_cmd hint", err)
	}
}

func TestResolvePrecedence(t *testing.T) {
	f := false
	cfg := &Config{Repos: map[string]Override{
		"/work/terraform-*":  {Tags: []string{"terraform"}, Branch: "main"},
		"terraform-*":        {Tags: []string{"tf"}, Branch: "trunk"},
		"terraform-module-x": {Branch: "develop", Sync: &f},
		"something-else":     {Tags: []string{"nope"}},
		"/elsewhere/*":       {Tags: []string{"nope"}},
	}}
	r := cfg.Resolve("terraform-module-x", "/work/terraform-module-x")
	if r.Branch != "develop" || !r.NoSync {
		t.Fatalf("exact name should win: %+v", r)
	}
	if strings.Join(r.Tags, ",") != "terraform,tf" {
		t.Fatalf("tags %v", r.Tags)
	}
	if r := cfg.Resolve("terraform-module-y", "/work/terraform-module-y"); r.Branch != "trunk" || r.NoSync {
		t.Fatalf("name glob should beat path glob: %+v", r)
	}
}

// group: like branch, the most specific key wins.
func TestResolveGroup(t *testing.T) {
	cfg := &Config{Repos: map[string]Override{
		"~/work/*": {Group: "work-folder"},
		"inf-*":    {Group: "infra"},
		"inf-odd":  {Group: "odd"},
		"inf-tags": {Tags: []string{"x"}}, // no group: the glob's stays
	}}
	for name, want := range map[string]string{"inf-base": "infra", "inf-odd": "odd", "inf-tags": "infra", "other": ""} {
		if got := cfg.Resolve(name, "/elsewhere/"+name).Group; got != want {
			t.Errorf("%s: group %q, want %q", name, got, want)
		}
	}
}

func TestSelect(t *testing.T) {
	cfg, err := Parse([]byte("workspaces:\n  b:\n    roots: [/tmp]\n  a:\n    roots: [/tmp]\n"))
	if err != nil {
		t.Fatal(err)
	}
	wss, _ := cfg.Select(nil)
	if len(wss) != 2 || wss[0].Name != "a" {
		t.Fatalf("select all: %v", wss)
	}
	if _, err := cfg.Select([]string{"zz"}); err == nil || !strings.Contains(err.Error(), "have: a, b") {
		t.Fatalf("unknown workspace: %v", err)
	}
}

func TestInitContentIsValid(t *testing.T) {
	data, err := InitContent()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "token_cmd") {
		t.Fatal("template lost its comments")
	}
}

// config init offers a workspace per common clone folder that exists.
func TestInitFindsCommonRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, d := range []string{"code", "work", "not-a-root"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	data, err := InitContent()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.WorkspaceNames(), ","); got != "code,work" {
		t.Fatalf("workspaces %q, want code,work", got)
	}
	if cfg.Workspaces["work"].Roots[0] != filepath.Join(home, "work") {
		t.Fatalf("root %q", cfg.Workspaces["work"].Roots[0])
	}
}

func TestWriteInitRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mrm", "config.yaml")
	if err := WriteInit(path, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteInit(path, false); err == nil {
		t.Fatal("overwrote without --force")
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestEmptyFileIsDefaults(t *testing.T) {
	_, err := Parse([]byte(""))
	if err == nil || !strings.Contains(err.Error(), "no workspaces") {
		t.Fatalf("err %v", err)
	}
}

// The pre-0.3.0 names forges, forge_tokens and forge_cache still load, and
// are noted for doctor; old and new together is an error.
func TestOldHostNames(t *testing.T) {
	ws := "workspaces:\n  work:\n    roots: [/tmp]\n"
	cfg, err := Parse([]byte(ws + "forges:\n  git.example.com: gitlab\nforge_tokens:\n  github.com:\n    token_env: GH\nforge_cache: 5m\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Forges["git.example.com"] != "gitlab" || cfg.ForgeTokens["github.com"].TokenEnv != "GH" || cfg.ForgeCache.Minutes() != 5 {
		t.Fatalf("old names not taken: %+v", cfg)
	}
	if len(cfg.Renamed) != 3 {
		t.Fatalf("renamed %v", cfg.Renamed)
	}
	if _, err := Parse([]byte(ws + "forge_cache: 5m\nhost_cache: 5m\n")); err == nil || !strings.Contains(err.Error(), "keep only host_cache") {
		t.Fatalf("both names: %v", err)
	}
	cfg, err = Parse([]byte(ws + "hosts:\n  git.example.com: gitlab\nhost_cache: 2m\n"))
	if err != nil || cfg.Forges["git.example.com"] != "gitlab" || len(cfg.Renamed) != 0 {
		t.Fatalf("new names: %+v %v", cfg, err)
	}
}
