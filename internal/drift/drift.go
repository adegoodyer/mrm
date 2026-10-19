// Package drift compares what mrm finds on disk with what the config and
// the forges say should be there. It reports; it never clones or deletes.
package drift

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/forge"
	"github.com/adegoodyer/mrm/internal/forgeapi"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
)

// Stale is a `repos:` key that names one repo, which isn't on disk.
type Stale struct {
	Key, Why string
}

// StaleKeys lists the exact-name and exact-path keys under `repos:` that
// match no discovered repo. Glob keys are left alone: matching nothing
// today is what a glob may well mean.
func StaleKeys(cfg *config.Config, repos []model.Repo) []Stale {
	names := map[string]bool{}
	for _, r := range repos {
		names[r.Name] = true
	}
	var out []Stale
	for key := range cfg.Repos {
		if strings.ContainsAny(key, `*?[\`) {
			continue
		}
		if strings.Contains(key, "/") || strings.HasPrefix(key, "~") {
			if _, err := os.Stat(paths.ExpandHome(key)); err != nil {
				out = append(out, Stale{key, "no such path"})
			}
			continue
		}
		if !names[key] {
			out = append(out, Stale{key, "no repo with that name"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// RemoteKey is how a remote is compared with an org listing:
// host/owner/name, lower case. "" for a local or unparseable remote.
func RemoteKey(remote string) string {
	f := forge.Parse(remote, nil)
	if f.Host == "" {
		return ""
	}
	return f.Host + "/" + strings.ToLower(f.Path)
}

// Org is one org's listing compared with the clones on disk.
type Org struct {
	Org       string             `json:"org"`
	Listed    int                `json:"listed"`
	NotCloned []forgeapi.OrgRepo `json:"not_cloned"`
	Archived  []forgeapi.OrgRepo `json:"archived_not_cloned,omitempty"`
	Ignored   int                `json:"ignored,omitempty"`
	Err       string             `json:"error,omitempty"`
}

// CompareOrg sorts an org's repos that have no clone (by remote, anywhere
// on disk) into live and archived. Repos whose name matches the
// workspace's ignore globs are counted, not listed.
func CompareOrg(org string, listed []forgeapi.OrgRepo, local map[string]bool, ignore []string) Org {
	host, _, _ := strings.Cut(org, "/")
	o := Org{Org: org, Listed: len(listed)}
	for _, r := range listed {
		if local[host+"/"+strings.ToLower(r.Path)] {
			continue
		}
		if ignored(ignore, filepath.Base(r.Path)) {
			o.Ignored++
			continue
		}
		if r.Archived {
			o.Archived = append(o.Archived, r)
		} else {
			o.NotCloned = append(o.NotCloned, r)
		}
	}
	by := func(s []forgeapi.OrgRepo) {
		sort.Slice(s, func(i, j int) bool { return strings.ToLower(s[i].Path) < strings.ToLower(s[j].Path) })
	}
	by(o.NotCloned)
	by(o.Archived)
	return o
}

func ignored(globs []string, name string) bool {
	for _, g := range globs {
		if ok, _ := filepath.Match(g, name); ok {
			return true
		}
	}
	return false
}
