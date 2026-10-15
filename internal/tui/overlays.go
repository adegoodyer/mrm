package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/forge"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/view"
)

// action is one entry in the command palette.
type action struct {
	label string
	keys  string
	run   func()
}

func (u *ui) actions() []action {
	acts := []action{
		{"Sync marked, group or repo", "s", func() { u.act(engine.OpSync, u.targets()) }},
		{"Sync everything shown", "S", func() { u.act(engine.OpSync, u.visible()) }},
		{"Fetch marked, group or repo", "f", func() { u.act(engine.OpFetch, u.targets()) }},
		{"Fetch everything shown", "F", func() { u.act(engine.OpFetch, u.visible()) }},
		{"Refresh status", "r", func() { u.startOp(engine.OpStatus, u.repos, false, "reading") }},
		{"Open on forge", "o", func() { u.open(openWeb) }},
		{"Open pull requests", "O", func() { u.open(openPRs) }},
		{"Open CI", "", func() { u.open(openCI) }},
		{"Open current branch on forge", "", func() { u.open(openBranch) }},
		{"Open in " + u.cfg.TUI.OpenCmd, "enter", func() { u.handoff(handoffGit) }},
		{"Open in editor", "e", func() { u.handoff(handoffEditor) }},
		{"Open a shell in the repo", "t", func() { u.handoff(handoffShell) }},
		{"Toggle detail pane", "l", func() { u.toggleDetail() }},
		{"Needs attention view", "!", func() { u.attention = true; u.refresh() }},
		{"Show every repo", "!", func() { u.attention = false; u.refresh() }},
		{"Filter", "/", func() { u.startFilter() }},
		{"Clear filter", "esc", func() { u.setFilter("") }},
		{"Mark everything shown", "a", func() { u.markVisible() }},
		{"Clear marks", "A", func() { u.marks = map[string]bool{}; u.drawChrome() }},
		{"Collapse or expand group", "tab", func() { u.toggleGroup() }},
		{"Collapse every group", "", func() { u.setAllCollapsed(true) }},
		{"Expand every group", "", func() { u.setAllCollapsed(false) }},
		{"Workspace: all", "w", func() { u.setWorkspace("") }},
	}
	for _, name := range u.cfg.WorkspaceNames() {
		name := name
		acts = append(acts, action{"Workspace: " + name, "w", func() { u.setWorkspace(name) }})
	}
	for _, q := range []string{"is:dirty", "is:behind", "is:unpushed", "is:stash", "is:not-default", "is:error"} {
		q := q
		acts = append(acts, action{"Filter " + q, "", func() { u.setFilter(q) }})
	}
	return append(acts,
		action{"Help", "?", func() { u.openHelp() }},
		action{"Quit", "q", func() { u.quit() }},
	)
}

// openPalette shows the command palette: type to fuzzy-search every
// action and workspace, enter runs the best match.
func (u *ui) openPalette() {
	acts := u.actions()
	input := tview.NewInputField().SetLabel("> ").SetFieldBackgroundColor(tcell.ColorDefault)
	input.SetLabelStyle(stAccent)
	results := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	results.SetSelectedStyle(stSelected)
	results.SetMainTextStyle(stDefault)

	var shown []action
	fillList := func(q string) {
		shown = rank(acts, q)
		results.Clear()
		for _, a := range shown {
			label := a.label
			if a.keys != "" {
				label += "  [::d]" + tview.Escape(a.keys) + "[::-]"
			}
			results.AddItem(label, "", 0, nil)
		}
	}
	fillList("")

	run := func() {
		i := results.GetCurrentItem()
		if i < 0 || i >= len(shown) {
			return
		}
		u.closeOverlay("palette")
		shown[i].run()
	}
	input.SetChangedFunc(fillList)
	input.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyDown, tcell.KeyCtrlN:
			results.SetCurrentItem((results.GetCurrentItem() + 1) % max(results.GetItemCount(), 1))
			return nil
		case tcell.KeyUp, tcell.KeyCtrlP:
			n := max(results.GetItemCount(), 1)
			results.SetCurrentItem((results.GetCurrentItem() - 1 + n) % n)
			return nil
		case tcell.KeyEnter:
			run()
			return nil
		case tcell.KeyEscape, tcell.KeyCtrlK:
			u.closeOverlay("palette")
			return nil
		}
		return ev
	})

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true).
		AddItem(results, 0, 1, false)
	box.SetBorder(true).SetTitle(" commands ").SetBorderPadding(0, 0, 1, 1)
	u.pages.AddPage("palette", modal(box, 60, 18), true, true)
	u.app.SetFocus(input)
}

// rank orders actions by fuzzy match against q: every character in order,
// scored up for consecutive runs and word starts. No match drops out.
func rank(acts []action, q string) []action {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return acts
	}
	type scored struct {
		a     action
		score int
		i     int
	}
	var out []scored
	for i, a := range acts {
		if s, ok := fuzzyScore(q, strings.ToLower(a.label)); ok {
			out = append(out, scored{a, s, i})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].i < out[j].i
	})
	res := make([]action, len(out))
	for i, s := range out {
		res[i] = s.a
	}
	return res
}

func fuzzyScore(q, s string) (int, bool) {
	qr, sr := []rune(q), []rune(s)
	score, qi, prev := 0, 0, -2
	for si := 0; si < len(sr) && qi < len(qr); si++ {
		if sr[si] != qr[qi] {
			continue
		}
		score += 10
		if si == prev+1 {
			score += 8
		}
		if si == 0 || !unicode.IsLetter(sr[si-1]) {
			score += 6
		}
		prev = si
		qi++
	}
	if qi < len(qr) {
		return 0, false
	}
	return score - len(sr)/8, true
}

func (u *ui) setWorkspace(name string) {
	u.ws = name
	u.refresh()
}

func (u *ui) setAllCollapsed(c bool) {
	for _, r := range u.visible() {
		k, _ := u.groupOf(r)
		u.collapsed[k] = c
	}
	u.refresh()
}

// openWorkspaces is the workspace switcher.
func (u *ui) openWorkspaces() {
	l := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	l.SetSelectedStyle(stSelected)
	names := append([]string{""}, u.cfg.WorkspaceNames()...)
	for i, n := range names {
		label := n
		if n == "" {
			label = "all"
		}
		count := 0
		for _, r := range u.repos {
			if n == "" || r.Workspace == n {
				count++
			}
		}
		l.AddItem(fmt.Sprintf("%s  [::d]%s[::-]", tview.Escape(label), repoCount(count)), "", 0, nil)
		if n == u.ws {
			l.SetCurrentItem(i)
		}
	}
	l.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		u.closeOverlay("workspaces")
		u.setWorkspace(names[i])
	})
	l.SetDoneFunc(func() { u.closeOverlay("workspaces") })
	l.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyRune && (ev.Rune() == 'q' || ev.Rune() == 'w') {
			u.closeOverlay("workspaces")
			return nil
		}
		if ev.Key() == tcell.KeyRune && ev.Rune() == 'j' {
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		}
		if ev.Key() == tcell.KeyRune && ev.Rune() == 'k' {
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		}
		return ev
	})
	l.SetBorder(true).SetTitle(" workspace ").SetBorderPadding(0, 0, 1, 1)
	u.pages.AddPage("workspaces", modal(l, 40, len(names)+2), true, true)
	u.app.SetFocus(l)
}

const helpText = `[::b]Move[::-]
  j k ↑ ↓   up / down          g G       top / bottom
  PgUp PgDn page               tab       collapse / expand group

[::b]Select[::-]
  space     mark repo (or whole group on a header)
  a         mark everything shown     A   clear marks

[::b]Act[::-]   on marked repos, else the group or repo under the cursor
  s / S     sync / sync everything shown
  f / F     fetch / fetch everything shown
  r         refresh status
  o / O     open on forge / open pull requests
  enter     hand off to lazygit (tui.open_cmd); back to mrm on exit
  e / t     editor / shell in the repo

[::b]View[::-]
  /         filter: is:dirty ws:work tag:x branch:feat/* age:>30d -is:clean
  esc       clear filter, leave attention view, clear marks
  !         needs attention: everything not clean, grouped by reason
  l         detail pane      w   workspace      ctrl+k   command palette
  ?         this help        q   quit`

func (u *ui) openHelp() {
	t := tview.NewTextView().SetDynamicColors(true).SetText(helpText)
	t.SetBorder(true).SetTitle(" keys ").SetBorderPadding(0, 0, 1, 1)
	t.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		u.closeOverlay("help")
		return nil
	})
	u.pages.AddPage("help", modal(t, 76, 26), true, true)
	u.app.SetFocus(t)
}

// Detail pane.

func (u *ui) toggleDetail() {
	u.showDetail = !u.showDetail
	if u.showDetail {
		u.body.AddItem(u.detail, 0, 1, false)
	} else {
		u.body.RemoveItem(u.detail)
	}
	u.updateDetail()
}

// updateDetail fills the pane from the status at once, then adds
// branches, stashes and commits read in the background.
func (u *ui) updateDetail() {
	if !u.showDetail {
		return
	}
	r, ok := u.currentRepo()
	if !ok {
		u.detail.SetTitle("")
		u.detail.SetText("[::d]move to a repo[::-]")
		return
	}
	res := u.results[r.Path]
	st := res.Status
	v := view.Build(res)
	var b strings.Builder
	fmt.Fprintf(&b, "[::d]%s[::-]\n", tview.Escape(paths.Tilde(r.Path)))
	if web, err := forge.Parse(st.RemoteURL, u.cfg.Forges).Web(); err == nil {
		fmt.Fprintf(&b, "[::d]%s[::-]\n", tview.Escape(web))
	}
	fmt.Fprintf(&b, "\n[%s]%s %s[-]", tagColor(v.State), v.State.Glyph(), v.State)
	if v.Error != "" {
		fmt.Fprintf(&b, "  [red]%s[-]", tview.Escape(v.Error))
	}
	fmt.Fprintf(&b, "\nbranch   %s", tview.Escape(v.Branch))
	if st.Upstream != "" {
		fmt.Fprintf(&b, " → %s", tview.Escape(st.Upstream))
	}
	if v.AB != "" {
		fmt.Fprintf(&b, "  %s", v.AB)
	}
	fmt.Fprintf(&b, "\ndefault  %s", tview.Escape(st.DefaultBranch))
	if v.Changes != "" {
		fmt.Fprintf(&b, "\nchanges  %s", v.Changes)
	}
	if res.Outcome != "" {
		fmt.Fprintf(&b, "\nlast run %s", tview.Escape(res.Summary()))
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(&b, "\ntags     %s", tview.Escape(strings.Join(r.Tags, ", ")))
	}
	base := b.String()
	u.detail.SetTitle(" " + r.Display() + " ")
	u.detail.SetText(base + "\n\n[::d]reading…[::-]")

	path := r.Path
	seq := u.detailSeq.Add(1)
	go func() {
		// Debounce: holding j should not read every repo it passes.
		time.Sleep(120 * time.Millisecond)
		if u.detailSeq.Load() != seq {
			return
		}
		ctx, cancel := context.WithTimeout(u.ctx, 10*time.Second)
		defer cancel()
		d := u.eng.Git.ReadDetail(ctx, path, 12)
		u.app.QueueUpdateDraw(func() {
			if cur, ok := u.currentRepo(); !ok || cur.Path != path || !u.showDetail {
				return
			}
			var b strings.Builder
			b.WriteString(base)
			b.WriteString("\n\n[::b]branches[::-]")
			for _, br := range d.Branches {
				mark := "  "
				if br.Current {
					mark = "* "
				}
				fmt.Fprintf(&b, "\n%s%s", mark, tview.Escape(br.Name))
				if br.Track != "" {
					fmt.Fprintf(&b, " [::d][%s][::-]", tview.Escape(br.Track))
				} else if br.Upstream == "" {
					b.WriteString(" [::d](local only)[::-]")
				}
			}
			if len(d.Stashes) > 0 {
				b.WriteString("\n\n[::b]stashes[::-]")
				for _, s := range d.Stashes {
					fmt.Fprintf(&b, "\n%s", tview.Escape(s))
				}
			}
			b.WriteString("\n\n[::b]commits[::-]")
			for _, c := range d.Commits {
				fmt.Fprintf(&b, "\n[::d]%s %4s[::-] %s", c.SHA, view.Age(c.When), tview.Escape(c.Subject))
			}
			u.detail.SetText(b.String()).ScrollToBeginning()
		})
	}()
}
