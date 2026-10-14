// Package model holds the types shared by the engine and every front end:
// a discovered repo, its status, and the result of an operation on it.
package model

import (
	"path/filepath"
	"time"
)

// Repo is a git repository found under a workspace root.
type Repo struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Workspace string   `json:"workspace"`
	Group     string   `json:"group,omitempty"`
	Tags      []string `json:"tags,omitempty"`

	// From config overrides.
	Branch       string `json:"-"` // default branch override
	NoSync       bool   `json:"-"` // status only
	Remote       string `json:"-"` // remote to fetch, default "origin"
	FFCurrent    bool   `json:"-"`
	SwitchMerged bool   `json:"-"`
}

// Display is the name shown in lists: group/name for nested repos.
func (r Repo) Display() string {
	if r.Group == "" {
		return r.Name
	}
	return filepath.ToSlash(filepath.Join(r.Group, r.Name))
}

// State is the primary state of a repo; the first that applies wins.
type State string

const (
	StateError    State = "error"
	StateConflict State = "conflict"
	StateDiverged State = "diverged"
	StateDirty    State = "dirty"
	StateUnpushed State = "unpushed"
	StateBehind   State = "behind"
	StateClean    State = "clean"
)

// States in precedence order.
var States = []State{StateError, StateConflict, StateDiverged, StateDirty, StateUnpushed, StateBehind, StateClean}

// Glyph is the one-character marker for a state.
func (s State) Glyph() string {
	switch s {
	case StateError:
		return "✗"
	case StateConflict:
		return "≠"
	case StateDiverged:
		return "⇅"
	case StateDirty:
		return "●"
	case StateUnpushed:
		return "↑"
	case StateBehind:
		return "↓"
	default:
		return "✓"
	}
}

// Flag names shown alongside the primary state.
const (
	FlagStash        = "stash"
	FlagUntracked    = "untracked"
	FlagDetached     = "detached"
	FlagNoRemote     = "no-remote"
	FlagUpstreamGone = "upstream-gone"
	FlagNotDefault   = "not-default"
	FlagNoUpstream   = "no-upstream"
)

// Status is everything mrm reads from a repo without touching the network.
type Status struct {
	Branch        string `json:"branch"` // empty when detached
	Detached      bool   `json:"detached"`
	Head          string `json:"head,omitempty"`
	DefaultBranch string `json:"default_branch"`
	Upstream      string `json:"upstream,omitempty"`
	UpstreamGone  bool   `json:"upstream_gone,omitempty"`
	Ahead         int    `json:"ahead"`
	Behind        int    `json:"behind"`

	// Local default branch against <remote>/<default>; only set when the
	// current branch is not the default.
	DefaultAhead  int `json:"default_ahead,omitempty"`
	DefaultBehind int `json:"default_behind,omitempty"`

	Staged     int    `json:"staged"`
	Modified   int    `json:"modified"`
	Untracked  int    `json:"untracked"`
	Conflicted int    `json:"conflicted"`
	Stashes    int    `json:"stashes"`
	Operation  string `json:"operation,omitempty"` // merge, rebase, cherry-pick, revert

	Unpushed int `json:"unpushed"` // commits on local branches that are on no remote

	RemoteURL string `json:"remote_url,omitempty"`
	HasRemote bool   `json:"has_remote"`
	// HasOriginHead is false when <remote>/HEAD is not set and the default
	// branch was guessed.
	HasOriginHead bool `json:"has_origin_head"`

	LastCommit  time.Time `json:"last_commit,omitzero"`
	LastAuthor  string    `json:"last_author,omitempty"`
	LastSubject string    `json:"last_subject,omitempty"`
	LastFetch   time.Time `json:"last_fetch,omitzero"`
	Error       string    `json:"error,omitempty"`
	ReadAt      time.Time `json:"read_at"`
}

// Dirty is true when tracked files differ from HEAD. Untracked files alone
// don't count.
func (s Status) Dirty() bool { return s.Staged+s.Modified+s.Conflicted > 0 }

// State works out the primary state.
func (s Status) State() State {
	switch {
	case s.Error != "":
		return StateError
	case s.Operation != "":
		return StateConflict
	case s.Ahead > 0 && s.Behind > 0:
		return StateDiverged
	case s.Dirty():
		return StateDirty
	case s.Ahead > 0 || (s.HasRemote && s.Unpushed > 0):
		return StateUnpushed
	case s.Behind > 0 || s.DefaultBehind > 0:
		return StateBehind
	default:
		return StateClean
	}
}

// Flags lists the secondary markers in a fixed order.
func (s Status) Flags() []string {
	var f []string
	if s.Stashes > 0 {
		f = append(f, FlagStash)
	}
	if s.Untracked > 0 {
		f = append(f, FlagUntracked)
	}
	if s.Detached {
		f = append(f, FlagDetached)
	}
	if !s.HasRemote {
		f = append(f, FlagNoRemote)
	}
	if s.UpstreamGone {
		f = append(f, FlagUpstreamGone)
	} else if s.HasRemote && !s.Detached && s.Upstream == "" && s.Branch != "" {
		f = append(f, FlagNoUpstream)
	}
	if !s.Detached && s.DefaultBranch != "" && s.Branch != s.DefaultBranch {
		f = append(f, FlagNotDefault)
	}
	return f
}

// HasFlag reports whether a flag is set.
func (s Status) HasFlag(flag string) bool {
	for _, f := range s.Flags() {
		if f == flag {
			return true
		}
	}
	return false
}

// Outcome of an operation on one repo.
type Outcome string

const (
	OutcomeNone     Outcome = ""         // status only
	OutcomeCurrent  Outcome = "current"  // nothing to do
	OutcomeSynced   Outcome = "synced"   // something moved
	OutcomeFetched  Outcome = "fetched"  // fetch only
	OutcomeWould    Outcome = "would"    // dry run: something would move
	OutcomeExcluded Outcome = "excluded" // left alone on purpose (sync: false, no remote)
	OutcomeSkipped  Outcome = "skipped"  // left alone because it was not safe
	OutcomeFailed   Outcome = "failed"
)

// NeedsAttention is true for outcomes the operator should look at.
func (o Outcome) NeedsAttention() bool { return o == OutcomeSkipped || o == OutcomeFailed }

// Result of running an operation on one repo.
type Result struct {
	Repo     Repo          `json:"repo"`
	Status   Status        `json:"status"`
	Outcome  Outcome       `json:"outcome,omitempty"`
	Reason   string        `json:"reason,omitempty"`
	Actions  []string      `json:"actions,omitempty"`
	Duration time.Duration `json:"duration_ns"`
}

// Summary line for an outcome, e.g. "skipped: dirty".
func (r Result) Summary() string {
	switch {
	case r.Outcome == OutcomeNone:
		return ""
	case r.Reason != "":
		return string(r.Outcome) + ": " + r.Reason
	case len(r.Actions) > 0 && (r.Outcome == OutcomeSynced || r.Outcome == OutcomeWould):
		s := ""
		for i, a := range r.Actions {
			if i > 0 {
				s += "; "
			}
			s += a
		}
		if r.Outcome == OutcomeWould {
			return "would " + s
		}
		return s
	default:
		return string(r.Outcome)
	}
}
