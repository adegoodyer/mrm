// Package discover finds git repos under a workspace's roots.
package discover

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/model"
)

// Workspace walks every root of ws down to ws.Depth and returns the repos
// found, sorted by group then name. A folder holding `.git` (a directory or
// a file, for worktrees) is a repo, and the walk does not descend into it,
// so submodules and nested worktrees are not listed separately. Hidden
// folders and folders matching an ignore glob are skipped.
func Workspace(cfg *config.Config, ws *config.Workspace) ([]model.Repo, []error) {
	var repos []model.Repo
	var errs []error
	seen := map[string]bool{}
	for _, root := range ws.Roots {
		found, err := walk(root, ws)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, r := range found {
			if seen[r.Path] {
				continue
			}
			seen[r.Path] = true
			res := cfg.Resolve(r.Name, r.Path)
			r.Workspace = ws.Name
			r.Tags = res.Tags
			r.Branch = res.Branch
			r.NoSync = res.NoSync
			r.Remote = ws.Sync.Remote
			r.FFCurrent = ws.Sync.FFCurrent
			r.FFDirty = ws.Sync.FFDirty
			r.SwitchMerged = ws.Sync.SwitchMerged
			repos = append(repos, r)
		}
	}
	sort.Slice(repos, func(i, j int) bool {
		if repos[i].Group != repos[j].Group {
			return repos[i].Group < repos[j].Group
		}
		return repos[i].Name < repos[j].Name
	})
	return repos, errs
}

// All discovers every workspace in order.
func All(cfg *config.Config, wss []*config.Workspace) ([]model.Repo, []error) {
	var repos []model.Repo
	var errs []error
	for _, ws := range wss {
		r, e := Workspace(cfg, ws)
		repos = append(repos, r...)
		errs = append(errs, e...)
	}
	return repos, errs
}

func walk(root string, ws *config.Workspace) ([]model.Repo, error) {
	fi, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, &fs.PathError{Op: "discover", Path: root, Err: fs.ErrInvalid}
	}
	var repos []model.Repo
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Unreadable folder: skip it, keep walking.
			if d != nil && d.IsDir() && p != root {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if p != root {
			if strings.HasPrefix(d.Name(), ".") || ignored(ws.Ignore, d.Name(), rel) {
				return fs.SkipDir
			}
		}
		if isRepo(p) {
			group := filepath.Dir(rel)
			if group == "." {
				group = ""
			}
			name := d.Name()
			if p == root {
				name = filepath.Base(root)
			}
			repos = append(repos, model.Repo{Name: name, Path: p, Group: filepath.ToSlash(group)})
			return fs.SkipDir
		}
		if depth(rel) >= ws.Depth {
			return fs.SkipDir
		}
		return nil
	})
	return repos, err
}

func isRepo(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

func depth(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	return strings.Count(filepath.ToSlash(rel), "/") + 1
}

func ignored(globs []string, name, rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, g := range globs {
		if ok, _ := filepath.Match(g, name); ok {
			return true
		}
		if ok, _ := filepath.Match(g, rel); ok {
			return true
		}
	}
	return false
}
