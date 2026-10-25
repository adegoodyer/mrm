package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// confirm asks a yes/no question in a dialog drawn like the rest of the
// view (never a terminal prompt): the question, the lines it is about, and
// y to go ahead or n/esc to cancel. Enter does nothing, so a key pressed
// out of habit can't confirm something like a delete.
func (u *ui) confirm(question string, lines []string, yes string, onYes func()) {
	var b strings.Builder
	fmt.Fprintf(&b, "[::b]%s[::-]\n\n", tview.Escape(question))
	for _, l := range lines {
		fmt.Fprintf(&b, "  %s\n", l)
	}
	fmt.Fprintf(&b, "\n[::b]y[::-] %s    [::b]n[::-] / [::b]esc[::-] cancel", tview.Escape(yes))
	t := tview.NewTextView().SetDynamicColors(true).SetText(b.String())
	t.SetBorder(true).SetTitle(" confirm ").SetBorderPadding(0, 0, 1, 1)
	t.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyRune && (ev.Rune() == 'y' || ev.Rune() == 'Y'):
			u.closeOverlay("confirm")
			onYes()
			return nil
		case ev.Key() == tcell.KeyEscape, ev.Key() == tcell.KeyRune && (ev.Rune() == 'n' || ev.Rune() == 'N' || ev.Rune() == 'q'):
			u.closeOverlay("confirm")
			u.say("cancelled")
			return nil
		}
		return ev // arrows, PgUp/PgDn scroll a long list
	})
	w := 50
	for _, l := range append([]string{question}, lines...) {
		w = max(w, tview.TaggedStringWidth(l)+8)
	}
	_, _, sw, sh := u.root.GetRect()
	h := min(len(lines)+7, max(sh-2, 10))
	u.pages.AddPage("confirm", modal(t, min(w, max(sw-4, 40)), h), true, true)
	u.app.SetFocus(t)
}
