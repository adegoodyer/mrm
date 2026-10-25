package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/adegoodyer/mrm/internal/engine"
)

// One table of keys. The list's key handler, the help overlay and the
// command palette all read it, and a test checks the user guide documents
// every key, so a binding can't exist without being listed everywhere.

// key is one way to press a binding.
type key struct {
	k   tcell.Key
	r   rune          // for tcell.KeyRune
	mod tcell.ModMask // checked for arrows and when set; terminals add their own to control keys
}

func ch(r rune) key { return key{k: tcell.KeyRune, r: r} }

func (k key) match(ev *tcell.EventKey) bool {
	if k.k == tcell.KeyRune {
		return ev.Key() == tcell.KeyRune && ev.Rune() == k.r
	}
	if ev.Key() != k.k {
		return false
	}
	switch k.k {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight:
		return ev.Modifiers()&tcell.ModShift == k.mod&tcell.ModShift
	}
	return k.mod == 0 || ev.Modifiers()&k.mod != 0
}

// binding is one thing a key does.
type binding struct {
	section string // help heading
	show    string // how help and docs write the keys: tokens separated by spaces
	label   string // what it does, for help and the palette
	keys    []key
	run     func(u *ui)
	palette bool // offer it in the command palette
}

// Help sections, in order.
const (
	secMove   = "Move"
	secSelect = "Select"
	secAct    = "Act"
	secOpen   = "Open"
	secView   = "View"
)

var sections = []string{secMove, secSelect, secAct, secOpen, secView}

func bindings() []binding {
	return []binding{
		{secMove, "j ↓", "down", []key{ch('j'), {k: tcell.KeyDown}}, func(u *ui) { u.list.move(1) }, false},
		{secMove, "k ↑", "up", []key{ch('k'), {k: tcell.KeyUp}}, func(u *ui) { u.list.move(-1) }, false},
		{secMove, "J shift+↓", "down 10", []key{ch('J'), {k: tcell.KeyDown, mod: tcell.ModShift}}, func(u *ui) { u.list.move(10) }, false},
		{secMove, "K shift+↑", "up 10", []key{ch('K'), {k: tcell.KeyUp, mod: tcell.ModShift}}, func(u *ui) { u.list.move(-10) }, false},
		{secMove, "PgDn PgUp", "page down / up", []key{{k: tcell.KeyPgDn}}, func(u *ui) { u.list.page(1) }, false},
		{secMove, "", "", []key{{k: tcell.KeyPgUp}}, func(u *ui) { u.list.page(-1) }, false},
		{secMove, "g Home", "top", []key{ch('g'), {k: tcell.KeyHome}}, func(u *ui) { u.list.top() }, false},
		{secMove, "G End", "bottom", []key{ch('G'), {k: tcell.KeyEnd}}, func(u *ui) { u.list.bottom() }, false},
		{secMove, "tab", "collapse or expand the group", []key{{k: tcell.KeyTab}}, func(u *ui) { u.toggleGroup() }, true},
		{secMove, "shift+tab", "collapse every group, or expand them all when all are collapsed", []key{{k: tcell.KeyBacktab}}, func(u *ui) { u.toggleAllGroups() }, true},
		{secMove, "← →", "previous / next workspace (all is one of them)", []key{{k: tcell.KeyLeft}}, func(u *ui) { u.cycleWorkspace(-1) }, false},
		{secMove, "", "", []key{{k: tcell.KeyRight}}, func(u *ui) { u.cycleWorkspace(1) }, false},

		{secSelect, "space", "mark the repo, or the whole group on its header", []key{ch(' ')}, func(u *ui) { u.toggleMark() }, false},
		{secSelect, "a", "mark everything shown", []key{ch('a')}, func(u *ui) { u.markVisible() }, true},
		{secSelect, "A", "clear marks", []key{ch('A')}, func(u *ui) { u.clearMarks() }, true},

		{secAct, "s", "sync the marked repos, else the group or repo", []key{ch('s')}, func(u *ui) { u.act(engine.OpSync, u.targets()) }, true},
		{secAct, "S", "sync everything shown", []key{ch('S')}, func(u *ui) { u.act(engine.OpSync, u.visible()) }, true},
		{secAct, "f", "fetch the marked repos, else the group or repo", []key{ch('f')}, func(u *ui) { u.act(engine.OpFetch, u.targets()) }, true},
		{secAct, "F", "fetch everything shown", []key{ch('F')}, func(u *ui) { u.act(engine.OpFetch, u.visible()) }, true},
		{secAct, "d", "move the marked repos, else the group or repo, to the trash (asks first)", []key{ch('d')}, func(u *ui) { u.trashTargets() }, true},
		{secAct, "r", "look for repos again and read their status", []key{ch('r')}, func(u *ui) { u.reread() }, true},

		{secOpen, "o", "open the repo in the browser (GitHub, GitLab, Bitbucket, Gitea)", []key{ch('o')}, func(u *ui) { u.open(openWeb) }, true},
		{secOpen, "p", "open its pull requests", []key{ch('p')}, func(u *ui) { u.open(openPRs) }, true},
		{secOpen, "P", "open its pipelines / actions (CI)", []key{ch('P')}, func(u *ui) { u.open(openCI) }, true},
		{secOpen, "b", "open the current branch in the browser", []key{ch('b')}, func(u *ui) { u.open(openBranch) }, true},
		{secOpen, "enter", "hand off to lazygit (tui.open_cmd); back to mrm when it exits", []key{{k: tcell.KeyEnter}}, func(u *ui) { u.enter() }, true},
		{secOpen, "e", "$VISUAL / $EDITOR in the repo", []key{ch('e')}, func(u *ui) { u.handoff(handoffEditor) }, true},
		{secOpen, "t", "$SHELL in the repo", []key{ch('t')}, func(u *ui) { u.handoff(handoffShell) }, true},

		{secView, "/", "filter as you type: is:dirty ws:work tag:x branch:feat/* age:>30d -is:clean", []key{ch('/')}, func(u *ui) { u.startFilter() }, true},
		{secView, "!", "needs attention: everything not clean, grouped by reason", []key{ch('!')}, func(u *ui) { u.toggleAttention() }, true},
		{secView, "l", "detail pane", []key{ch('l')}, func(u *ui) { u.toggleDetail() }, true},
		{secView, "L", "commit graph of every branch (esc closes)", []key{ch('L')}, func(u *ui) { u.openGraph() }, true},
		{secView, "ctrl+d ctrl+u", "scroll the detail pane half a page (the list, when it's closed)", []key{{k: tcell.KeyCtrlD}}, func(u *ui) { u.scrollDetail(1) }, false},
		{secView, "", "", []key{{k: tcell.KeyCtrlU}}, func(u *ui) { u.scrollDetail(-1) }, false},
		{secView, "w", "pick a workspace (← → cycle them)", []key{ch('w')}, func(u *ui) { u.openWorkspaces() }, false},
		{secView, "esc", "clear the filter, else leave the attention view, else clear marks", []key{{k: tcell.KeyEscape}}, func(u *ui) { u.escape() }, false},
		{secView, "ctrl+k", "command palette", []key{{k: tcell.KeyCtrlK}}, func(u *ui) { u.openPalette() }, false},
		{secView, "?", "this help", []key{ch('?')}, func(u *ui) { u.openHelp() }, true},
		{secView, "q", "quit", []key{ch('q')}, func(u *ui) { u.quit() }, true},
	}
}

// binding for a key event, if any.
func (u *ui) bindingFor(ev *tcell.EventKey) (binding, bool) {
	for _, b := range u.keys {
		for _, k := range b.keys {
			if k.match(ev) {
				return b, true
			}
		}
	}
	return binding{}, false
}

// helpText is the help overlay, from the table.
func helpText(bs []binding) string {
	var b strings.Builder
	b.WriteString("[::d]s, f, d and the browser keys (o p P b) act on the marked repos, else the group or repo\nunder the cursor; the other keys act on the repo under the cursor.[::-]\n")
	for _, sec := range sections {
		fmt.Fprintf(&b, "\n[::b]%s[::-]\n", sec)
		for _, k := range bs {
			if k.section == sec && k.show != "" {
				fmt.Fprintf(&b, "  %-14s %s\n", k.show, k.label)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// Small actions the table runs.

func (u *ui) clearMarks() {
	u.marks = map[string]bool{}
	u.drawChrome()
}

func (u *ui) toggleAttention() {
	u.attention = !u.attention
	u.refresh()
}

// enter toggles a group on its header, else hands off to lazygit.
func (u *ui) enter() {
	if cur, ok := u.list.current(); ok && cur.header {
		u.toggleGroup()
		return
	}
	u.handoff(handoffGit)
}

// escape undoes the most recent narrowing: filter, attention view, marks.
func (u *ui) escape() {
	switch {
	case u.query != "":
		u.setFilter("")
	case u.attention:
		u.attention = false
		u.refresh()
	case len(u.marks) > 0:
		u.clearMarks()
	}
}

// cycleWorkspace moves to the previous or next workspace, with "all" as
// the first, wrapping at both ends.
func (u *ui) cycleWorkspace(dir int) {
	names := append([]string{""}, u.cfg.WorkspaceNames()...)
	i := 0
	for j, n := range names {
		if n == u.ws {
			i = j
		}
	}
	u.setWorkspace(names[(i+dir+len(names))%len(names)])
}

// toggleAllGroups collapses every group shown, or expands them all when
// every one is already collapsed.
func (u *ui) toggleAllGroups() {
	open := false
	for _, it := range u.list.items {
		if it.header && !it.collapsed {
			open = true
		}
	}
	u.setAllCollapsed(open)
}

// scrollDetail moves the detail pane half a page; with the pane closed it
// moves the list instead.
func (u *ui) scrollDetail(dir int) {
	if !u.showDetail {
		u.list.move(dir * max(u.list.height/2, 1))
		return
	}
	_, _, _, h := u.detail.GetInnerRect()
	row, _ := u.detail.GetScrollOffset()
	u.detail.ScrollTo(max(row+dir*max(h/2, 1), 0), 0)
}
