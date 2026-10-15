// Package store keeps the status cache ($XDG_CACHE_HOME/mrm/status.json)
// and the run history ($XDG_STATE_HOME/mrm/runs.jsonl).
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/adegoodyer/mrm/internal/model"
)

// Cache is the last known status per repo path.
type Cache struct {
	Updated time.Time               `json:"updated"`
	Repos   map[string]model.Status `json:"repos"`
}

// LoadCache reads the cache; a missing or unreadable file gives an empty one.
func LoadCache(path string) Cache {
	c := Cache{Repos: map[string]model.Status{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	if json.Unmarshal(data, &c) != nil || c.Repos == nil {
		return Cache{Repos: map[string]model.Status{}}
	}
	return c
}

// UpdateCache merges results into the cache file, atomically.
func UpdateCache(path string, results []model.Result) error {
	c := LoadCache(path)
	for _, r := range results {
		if r.Repo.Path != "" && !r.Status.ReadAt.IsZero() {
			c.Repos[r.Repo.Path] = r.Status
		}
	}
	c.Updated = time.Now()
	return writeJSON(path, c)
}

// Run is one line of the history.
type Run struct {
	Started    time.Time   `json:"started"`
	DurationMS int64       `json:"duration_ms"`
	Op         string      `json:"op"`
	DryRun     bool        `json:"dry_run,omitempty"`
	Workspaces []string    `json:"workspaces"`
	Results    []RunResult `json:"results"`
}

// RunResult is one repo in a run.
type RunResult struct {
	Path       string   `json:"path"`
	Name       string   `json:"name"`
	Workspace  string   `json:"workspace"`
	Outcome    string   `json:"outcome"`
	Reason     string   `json:"reason,omitempty"`
	Actions    []string `json:"actions,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}

// NewRun builds a history record from results.
func NewRun(op string, dryRun bool, started time.Time, workspaces []string, results []model.Result) Run {
	run := Run{Started: started, DurationMS: time.Since(started).Milliseconds(), Op: op, DryRun: dryRun, Workspaces: workspaces}
	for _, r := range results {
		run.Results = append(run.Results, RunResult{
			Path: r.Repo.Path, Name: r.Repo.Display(), Workspace: r.Repo.Workspace,
			Outcome: string(r.Outcome), Reason: r.Reason, Actions: r.Actions,
			DurationMS: r.Duration.Milliseconds(),
		})
	}
	return run
}

// AppendRun appends a run to the history file.
func AppendRun(path string, run Run) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// Runs returns up to n runs from the history file, newest first (dry runs
// included, so the Activity page shows them).
func Runs(path string, n int) []Run {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var all []Run
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var r Run
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			all = append(all, r)
		}
	}
	out := make([]Run, 0, n)
	for i := len(all) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, all[i])
	}
	return out
}

// LastRun returns the most recent run of op (dry runs excluded) from the
// history file.
func LastRun(path, op string) (Run, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Run{}, false
	}
	defer f.Close()
	var last Run
	found := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024) // a run line holds every repo
	for sc.Scan() {
		var r Run
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Op == op && !r.DryRun {
			last, found = r, true
		}
	}
	return last, found
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".status-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Exists reports whether path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}
