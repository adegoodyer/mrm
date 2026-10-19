package forgeapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/forge"
)

// Opt-in, read-only check against the real GitHub API:
// MRM_FORGE_LIVE=owner/repo go test ./internal/forgeapi -run Live (uses gh auth token).
func TestLiveGitHub(t *testing.T) {
	repo := os.Getenv("MRM_FORGE_LIVE")
	if repo == "" {
		t.Skip("set MRM_FORGE_LIVE=owner/repo to run")
	}
	cfg := &config.Config{ForgeTokens: map[string]config.ForgeToken{"github.com": {TokenCmd: "gh auth token"}}}
	cfg.ForgeCache.Duration = 0
	c := New(cfg, filepath.Join(t.TempDir(), "forge.json"))
	f := forge.Parse("git@github.com:"+repo+".git", nil)
	i := c.Get(context.Background(), f, "master")
	if i.Err != "" {
		t.Fatal(i.Err)
	}
	t.Logf("%s: %d PRs, CI %s %s", repo, i.PRs, i.CI, i.CIURL)
	owner := filepath.Dir(repo)
	repos, err := c.OrgRepos(context.Background(), "github.com/"+owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("github.com/%s: %d repos, e.g. %+v", owner, len(repos), repos[0])
}
