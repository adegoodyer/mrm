package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// hint is one key in the footer.
type hint struct{ keys, what string }

// footerHints are the keys worth showing now, most useful first: they
// depend on whether repos are marked and what the cursor is on.
func (u *ui) footerHints() (lead string, hints []hint) {
	cur, _ := u.list.current()
	attention := hint{"!", "attention"}
	if u.attention {
		attention = hint{"!", "all repos"}
	}
	switch {
	case len(u.marks) > 0:
		lead = fmt.Sprintf("[fuchsia::b]%d marked[-::-]", len(u.marks))
		hints = []hint{{"s", "sync"}, {"f", "fetch"}, {"space", "mark/unmark"}, {"a", "mark all shown"}, {"A", "clear marks"}, {"ctrl+k", "more"}}
	case cur.header:
		toggle := "collapse"
		if cur.collapsed {
			toggle = "expand"
		}
		hints = []hint{{"tab", toggle}, {"s", "sync group"}, {"f", "fetch group"}, {"space", "mark group"},
			{"shift+tab", "all groups"}, {"← →", "workspace"}, {"/", "filter"}, attention, {"ctrl+k", "more"}}
	default:
		hints = []hint{{"/", "filter"}, {"space", "mark"}, {"s", "sync"}, {"f", "fetch"}, {"enter", u.cfg.TUI.OpenCmd},
			{"l", "detail"}, {"o", "open"}, {"p", "PRs"}, attention, {"← →", "workspace"}, {"r", "refresh"},
			{"shift+tab", "groups"}, {"e", "editor"}, {"t", "shell"}, {"ctrl+k", "more"}}
	}
	if u.query != "" {
		f := "[aqua]/" + tview.Escape(u.query) + "[-]  [::d]esc[::-] clear"
		if lead == "" {
			lead = f
		} else {
			lead += "  " + f
		}
	}
	return lead, hints
}

// drawFooter fits as many hints as the width allows, always keeping the
// way to help. A message (a warning, a result) replaces the hints.
func (u *ui) drawFooter() {
	if u.message != "" {
		u.footer.SetText(" " + u.msgStyle + tview.Escape(u.message) + "[-::-]")
		return
	}
	w := u.width
	if w <= 0 {
		w = 100
	}
	lead, hints := u.footerHints()
	help := hint{"?", "help"}
	text := func(h hint) string {
		return "[::b]" + tview.Escape(h.keys) + "[::-] [::d]" + tview.Escape(h.what) + "[::-]"
	}
	parts := []string{}
	if lead != "" {
		parts = append(parts, lead)
	}
	used := tview.TaggedStringWidth(strings.Join(parts, "  ")) + 1 + 2 + tview.TaggedStringWidth(text(help))
	for _, h := range hints {
		t := text(h)
		n := tview.TaggedStringWidth(t) + 2
		if used+n > w-1 {
			break
		}
		parts = append(parts, t)
		used += n
	}
	parts = append(parts, text(help))
	u.footer.SetText(" " + strings.Join(parts, "  "))
}
