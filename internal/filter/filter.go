// Package filter is the query language shared by the CLI (--filter), the
// TUI (/) and the web UI (search): terms are ANDed, a bare word matches
// part of the name or path, and a leading - negates a term.
//
//	is:dirty  is:stash  ws:work  group:platform  tag:backend
//	branch:feature/*  forge:bitbucket  age:>30d  age:<1d  -tag:archived
package filter

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adegoodyer/mrm/internal/forge"
	"github.com/adegoodyer/mrm/internal/model"
)

// Filter is a parsed query. The zero value matches everything.
type Filter struct {
	terms  []term
	forges map[string]string // self-hosted host -> forge kind, from config
}

// WithForges makes forge: terms recognise self-hosted hosts mapped under
// `hosts:` in the config.
func (f Filter) WithForges(hosts map[string]string) Filter {
	f.forges = hosts
	return f
}

type term struct {
	neg   bool
	key   string // "" for a fuzzy word
	value string
	// age terms
	older bool
	dur   time.Duration
}

var keyOrder = []string{"is", "ws", "group", "tag", "branch", "host", "age"}

var keys = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range keyOrder {
		m[k] = true
	}
	return m
}()

// Keys are the term keys, in display order (for shell completion).
func Keys() []string { return append([]string(nil), keyOrder...) }

// IsValues are the states and flags is: accepts, sorted.
func IsValues() []string {
	out := make([]string, 0, len(isValues))
	for v := range isValues {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

var isValues = func() map[string]bool {
	m := map[string]bool{"ahead": true}
	for _, s := range model.States {
		m[string(s)] = true
	}
	for _, f := range []string{model.FlagStash, model.FlagUntracked, model.FlagDetached, model.FlagNoRemote,
		model.FlagUpstreamGone, model.FlagNotDefault, model.FlagNoUpstream, model.FlagStale} {
		m[f] = true
	}
	return m
}()

// Parse parses a query. Words are split on spaces; "double quotes" keep
// spaces in a value.
func Parse(q string) (Filter, error) {
	var f Filter
	for _, w := range split(q) {
		t := term{}
		if strings.HasPrefix(w, "-") && len(w) > 1 {
			t.neg = true
			w = w[1:]
		}
		k, v, ok := strings.Cut(w, ":")
		if k == "forge" {
			k = "host" // the name before 0.3.0
		}
		if !ok || !keys[k] {
			t.value = strings.ToLower(w)
			f.terms = append(f.terms, t)
			continue
		}
		if v == "" {
			return Filter{}, fmt.Errorf("filter %q: missing value after %s:", w, k)
		}
		t.key, t.value = k, v
		switch k {
		case "is":
			if !isValues[v] {
				return Filter{}, fmt.Errorf("filter %q: unknown state or flag %q", w, v)
			}
		case "age":
			if err := parseAge(&t); err != nil {
				return Filter{}, fmt.Errorf("filter %q: %w", w, err)
			}
		case "branch":
			if _, err := filepath.Match(v, ""); err != nil {
				return Filter{}, fmt.Errorf("filter %q: bad glob", w)
			}
		}
		f.terms = append(f.terms, t)
	}
	return f, nil
}

// MustParse is Parse for constant queries.
func MustParse(q string) Filter {
	f, err := Parse(q)
	if err != nil {
		panic(err)
	}
	return f
}

// Empty reports whether the filter matches everything.
func (f Filter) Empty() bool { return len(f.terms) == 0 }

// NeedsStatus reports whether matching needs the repo's git status.
func (f Filter) NeedsStatus() bool {
	for _, t := range f.terms {
		switch t.key {
		case "is", "branch", "host", "age":
			return true
		}
	}
	return false
}

// Match reports whether a repo (and its status, which may be zero when
// NeedsStatus is false) matches every term.
func (f Filter) Match(r model.Repo, st model.Status) bool {
	for _, t := range f.terms {
		if t.match(r, st, f.forges) == t.neg {
			return false
		}
	}
	return true
}

func (t term) match(r model.Repo, st model.Status, forges map[string]string) bool {
	switch t.key {
	case "":
		// Substring, not fuzzy: an unranked filter has to be predictable.
		return strings.Contains(strings.ToLower(r.Path), t.value) || strings.Contains(strings.ToLower(r.Display()), t.value)
	case "is":
		switch {
		case t.value == "ahead":
			return st.Ahead > 0
		case t.value == string(st.State()):
			return true
		default:
			return st.HasFlag(t.value)
		}
	case "ws":
		return strings.EqualFold(r.Workspace, t.value)
	case "group":
		return strings.EqualFold(r.Group, t.value)
	case "tag":
		for _, tag := range r.Tags {
			if strings.EqualFold(tag, t.value) {
				return true
			}
		}
		return false
	case "branch":
		ok, _ := filepath.Match(t.value, st.Branch)
		return ok
	case "host":
		// The kind (host:github) or the hostname (host:git.example.com).
		f := forge.Parse(st.RemoteURL, forges)
		return string(f.Kind) == strings.ToLower(t.value) || f.Host == strings.ToLower(t.value)
	case "age":
		if st.LastCommit.IsZero() {
			return false
		}
		age := time.Since(st.LastCommit)
		if t.older {
			return age > t.dur
		}
		return age < t.dur
	}
	return false
}

func parseAge(t *term) error {
	v := t.value
	switch {
	case strings.HasPrefix(v, ">"):
		t.older = true
	case strings.HasPrefix(v, "<"):
	default:
		return fmt.Errorf("age needs > or <, e.g. age:>30d")
	}
	d, err := parseDuration(v[1:])
	if err != nil {
		return err
	}
	t.dur = d
	return nil
}

// parseDuration accepts Go durations plus d (days) and w (weeks).
func parseDuration(s string) (time.Duration, error) {
	if n := len(s); n > 1 && (s[n-1] == 'd' || s[n-1] == 'w') {
		v, err := strconv.Atoi(s[:n-1])
		if err != nil {
			return 0, fmt.Errorf("bad age %q", s)
		}
		unit := 24 * time.Hour
		if s[n-1] == 'w' {
			unit *= 7
		}
		return time.Duration(v) * unit, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("bad age %q (e.g. 30d, 2w, 12h)", s)
	}
	return d, nil
}

func split(q string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, c := range q {
		switch {
		case c == '"':
			inQuote = !inQuote
		case c == ' ' && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
