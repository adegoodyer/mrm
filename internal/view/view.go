// Package view turns a repo's status into the plain text both front ends
// show - the CLI table and the TUI list - so they never disagree.
package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/adegoodyer/mrm/internal/model"
)

// Row is one repo as text, without colour.
type Row struct {
	State   model.State
	Name    string
	Branch  string
	AB      string // "2↑ 4↓", "3↓", "" when level
	Changes string // "1S 3M 2?"
	Age     string // last commit
	Notes   []string
	Error   string
}

// Build makes the row for a result.
func Build(r model.Result) Row {
	st := r.Status
	row := Row{State: st.State(), Name: r.Repo.Display()}
	if st.Error != "" {
		row.Error = st.Error
		return row
	}
	row.Branch = st.Branch
	if st.Detached {
		row.Branch = "(" + ShortSHA(st.Head) + ")"
	}
	switch {
	case st.Ahead > 0 && st.Behind > 0:
		row.AB = fmt.Sprintf("%d↑ %d↓", st.Ahead, st.Behind)
	case st.Ahead > 0:
		row.AB = fmt.Sprintf("%d↑", st.Ahead)
	case st.Behind > 0:
		row.AB = fmt.Sprintf("%d↓", st.Behind)
	}
	var ch []string
	for _, c := range []struct {
		n   int
		tag string
	}{{st.Staged, "S"}, {st.Modified, "M"}, {st.Conflicted, "U"}, {st.Untracked, "?"}} {
		if c.n > 0 {
			ch = append(ch, fmt.Sprintf("%d%s", c.n, c.tag))
		}
	}
	row.Changes = strings.Join(ch, " ")
	row.Age = Age(st.LastCommit)

	for _, f := range st.Flags() {
		switch f {
		case model.FlagNotDefault: // shown with the default branch below
		case model.FlagStale: // last: low priority, added below
		default:
			row.Notes = append(row.Notes, f)
		}
	}
	if st.Branch != st.DefaultBranch && st.DefaultBranch != "" && !st.Detached {
		n := "on " + st.Branch + ", default " + st.DefaultBranch
		if st.DefaultBehind > 0 {
			n += fmt.Sprintf(" %d↓", st.DefaultBehind)
		}
		row.Notes = append(row.Notes, n)
	}
	if st.Unpushed > 0 && st.HasRemote && st.Ahead == 0 {
		row.Notes = append(row.Notes, fmt.Sprintf("%d unpushed", st.Unpushed))
	}
	if r.Outcome.NeedsAttention() {
		row.Notes = append(row.Notes, r.Summary())
	} else if st.FetchError != "" {
		row.Notes = append(row.Notes, "last fetch failed: "+st.FetchError)
	}
	row.Notes = append(row.Notes, r.Warnings...)
	if st.StaleBranches > 0 {
		row.Notes = append(row.Notes, plural(st.StaleBranches, "stale branch", "stale branches"))
	}
	return row
}

// Reason is the needs-attention group a result belongs in: the outcome and
// its reason up to any detail in brackets ("skipped: dirty"), else the
// state ("unpushed"). Clean, current repos have no reason.
func Reason(r model.Result) string {
	if r.Outcome.NeedsAttention() {
		reason := r.Reason
		if i := strings.Index(reason, " ("); i > 0 {
			reason = reason[:i]
		}
		return string(r.Outcome) + ": " + reason
	}
	if r.Status.Error == "" && r.Status.FetchError != "" {
		return "last fetch failed: " + r.Status.FetchError
	}
	if s := r.Status.State(); s != model.StateClean {
		return string(s)
	}
	return ""
}

// ShortSHA is the first 7 characters of a commit id.
func ShortSHA(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// Age renders the time since t as 45s, 12m, 3h, 5d, 7w, 2y.
func Age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
