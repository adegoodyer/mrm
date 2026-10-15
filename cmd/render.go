package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/term"
	"github.com/adegoodyer/mrm/internal/view"
)

// nameCap and branchCap size those columns from the terminal width, so
// long names show in full on a wide terminal; piped output is never cut.
func nameCap() int   { return capFor(40, 20) }
func branchCap() int { return capFor(20, 12) }

func capFor(percent, least int) int {
	if !term.IsTTY(os.Stdout) {
		return 1 << 16
	}
	w := term.Width(os.Stdout)
	if w <= 0 {
		w = 120
	}
	return max(least, w*percent/100)
}

// statusRow is one repo in `mrm status`.
type statusRow struct {
	glyph, name, branch, ab, changes, age, notes string
}

func (a *app) statusRow(r model.Result) statusRow {
	s, v := a.style, view.Build(r)
	row := statusRow{glyph: s.State(v.State), name: v.Name}
	if v.Error != "" {
		row.notes = s.Red(v.Error)
		return row
	}
	row.branch, row.changes, row.age, row.ab = v.Branch, v.Changes, v.Age, v.AB
	if row.ab == "" {
		row.ab = s.Dim("·")
	}
	row.notes = s.Dim(strings.Join(v.Notes, " · "))
	return row
}

// printStatus prints the status table, with a heading per workspace when
// there is more than one, and a count line.
func (a *app) printStatus(w io.Writer, results []model.Result) {
	rows := make([]statusRow, len(results))
	nameW, branchW, abW, chW := 4, 6, 1, 0
	for i, r := range results {
		rows[i] = a.statusRow(r)
		nameW = max(nameW, min(term.VisibleLen(rows[i].name), nameCap()))
		branchW = max(branchW, min(term.VisibleLen(rows[i].branch), branchCap()))
		abW = max(abW, term.VisibleLen(rows[i].ab))
		chW = max(chW, term.VisibleLen(rows[i].changes))
	}
	multi := len(a.workspaces) > 1
	lastWS := ""
	for i, r := range results {
		if multi && r.Repo.Workspace != lastWS {
			if lastWS != "" {
				fmt.Fprintln(w)
			}
			lastWS = r.Repo.Workspace
			fmt.Fprintln(w, a.style.Bold(lastWS))
		}
		row := rows[i]
		if r.Status.Error != "" {
			fmt.Fprintf(w, "%s %s  %s\n", row.glyph, term.Pad(row.name, nameW), row.notes)
			continue
		}
		line := fmt.Sprintf("%s %s  %s  %s  %s  %4s  %s", row.glyph,
			term.Pad(row.name, nameW), term.Pad(row.branch, branchW), term.Pad(row.ab, abW),
			term.Pad(row.changes, chW), row.age, row.notes)
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, a.counts(results))
}

// counts is "78 repos  ✓ 71  ↓ 3  ● 2 ...".
func (a *app) counts(results []model.Result) string {
	n := map[model.State]int{}
	for _, r := range results {
		n[r.Status.State()]++
	}
	parts := []string{repoCount(len(results))}
	for _, s := range model.States {
		if n[s] > 0 {
			parts = append(parts, a.style.State(s)+" "+fmt.Sprint(n[s]))
		}
	}
	return strings.Join(parts, "   ")
}

func repoCount(n int) string {
	if n == 1 {
		return "1 repo"
	}
	return fmt.Sprintf("%d repos", n)
}

// outcomeLine is one repo's line during a sync or fetch.
func (a *app) outcomeLine(r model.Result, nameW int) string {
	s := a.style
	text := r.Summary()
	switch r.Outcome {
	case model.OutcomeCurrent, model.OutcomeExcluded:
		text = s.Dim(text)
	case model.OutcomeSkipped:
		text = s.Yellow(text)
	case model.OutcomeFailed:
		text = s.Red(text)
	}
	return fmt.Sprintf("%s %s  %s", s.Outcome(r.Outcome), term.Pad(r.Repo.Display(), nameW), text)
}

// printSummary prints the totals and the repos that need attention.
func (a *app) printSummary(w io.Writer, op string, results []model.Result, took time.Duration) {
	n := map[model.Outcome]int{}
	var attention []model.Result
	for _, r := range results {
		n[r.Outcome]++
		if r.Outcome.NeedsAttention() {
			attention = append(attention, r)
		}
	}
	order := []struct {
		o     model.Outcome
		label string
	}{
		{model.OutcomeSynced, "synced"}, {model.OutcomeFetched, "fetched"}, {model.OutcomeWould, "would change"},
		{model.OutcomeCurrent, "current"}, {model.OutcomeExcluded, "excluded"},
		{model.OutcomeSkipped, "skipped"}, {model.OutcomeFailed, "failed"},
	}
	var parts []string
	for _, x := range order {
		if n[x.o] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n[x.o], x.label))
		}
	}
	fmt.Fprintf(w, "\n%s %s in %s: %s\n", op, repoCount(len(results)), took.Round(100*time.Millisecond), strings.Join(parts, ", "))
	if len(attention) == 0 {
		return
	}
	if offline(attention, len(results)) {
		fmt.Fprintln(w, a.style.Red("every fetch failed - offline?"))
	}
	sort.SliceStable(attention, func(i, j int) bool {
		if attention[i].Outcome != attention[j].Outcome {
			return attention[i].Outcome == model.OutcomeFailed
		}
		return attention[i].Reason < attention[j].Reason
	})
	fmt.Fprintln(w, "\n"+a.style.Bold("Needs attention"))
	nameW := 0
	for _, r := range attention {
		nameW = max(nameW, min(len(r.Repo.Display()), nameCap()))
	}
	for _, r := range attention {
		fmt.Fprintln(w, "  "+a.outcomeLine(r, nameW))
	}
}

// offline guesses that the network is down when every repo with a remote
// failed to fetch.
func offline(attention []model.Result, total int) bool {
	failed := 0
	for _, r := range attention {
		if r.Outcome == model.OutcomeFailed && strings.HasPrefix(r.Reason, "fetch:") {
			failed++
		}
	}
	return failed > 2 && failed == total
}

// statusJSON is one repo in --json output.
type statusJSON struct {
	model.Repo
	State   model.State   `json:"state"`
	Flags   []string      `json:"flags"`
	Status  model.Status  `json:"status"`
	Outcome model.Outcome `json:"outcome,omitempty"`
	Reason  string        `json:"reason,omitempty"`
	Actions []string      `json:"actions,omitempty"`
}

func printJSON(results []model.Result) error {
	out := make([]statusJSON, len(results))
	for i, r := range results {
		flags := r.Status.Flags()
		if flags == nil {
			flags = []string{}
		}
		out[i] = statusJSON{Repo: r.Repo, State: r.Status.State(), Flags: flags, Status: r.Status,
			Outcome: r.Outcome, Reason: r.Reason, Actions: r.Actions}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
