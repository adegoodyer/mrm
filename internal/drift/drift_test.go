package drift

import (
	"testing"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/forgeapi"
	"github.com/adegoodyer/mrm/internal/model"
)

// 4.5: exact keys with no repo are stale; globs never are.
func TestStaleKeys(t *testing.T) {
	cfg := &config.Config{Repos: map[string]config.Override{
		"api": {}, "gone": {}, "legacy-*": {}, "/no/such/path": {}, "~/also/[ab]": {},
	}}
	got := StaleKeys(cfg, []model.Repo{{Name: "api", Path: "/w/api"}})
	if len(got) != 2 || got[0].Key != "/no/such/path" || got[1].Key != "gone" {
		t.Fatalf("%+v", got)
	}
}

// 4.5: clones are matched by remote anywhere on disk, case-insensitively;
// archived and ignored repos are kept apart.
func TestCompareOrg(t *testing.T) {
	local := map[string]bool{}
	for _, r := range []string{"git@github.com:Acme/API.git", "https://github.com/acme/web", "/local/path"} {
		if k := RemoteKey(r); k != "" {
			local[k] = true
		}
	}
	listed := []forgeapi.OrgRepo{
		{Path: "acme/api"}, {Path: "acme/web"}, {Path: "acme/new"},
		{Path: "acme/old", Archived: true}, {Path: "acme/legacy-x"},
	}
	o := CompareOrg("github.com/acme", listed, local, []string{"legacy-*"})
	if o.Listed != 5 || len(o.NotCloned) != 1 || o.NotCloned[0].Path != "acme/new" ||
		len(o.Archived) != 1 || o.Ignored != 1 {
		t.Fatalf("%+v", o)
	}
}
