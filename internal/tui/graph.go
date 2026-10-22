package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/adegoodyer/mrm/internal/git"
)

// openGraph shows the repo's commit graph - every branch, tag and remote -
// in a modal. It is read in the background; j/k, arrows, PgUp/PgDn, g/G
// and ctrl+d/ctrl+u scroll, h/l and ←/→ pan, esc, q or L close.
func (u *ui) openGraph() {
	r, ok := u.currentRepo()
	if !ok {
		u.warn("move to a repo first")
		return
	}
	t := tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetText("[::d]reading…[::-]")
	t.SetBorder(true).SetTitle(" "+tview.Escape(r.Display())+"  [::d]graph · esc closes[::-] ").SetBorderPadding(0, 0, 1, 1)
	t.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		_, _, _, h := t.GetInnerRect()
		row, col := t.GetScrollOffset()
		switch {
		case ev.Key() == tcell.KeyEscape, ev.Key() == tcell.KeyRune && (ev.Rune() == 'q' || ev.Rune() == 'L'):
			u.closeOverlay("graph")
			return nil
		case ev.Key() == tcell.KeyCtrlD:
			t.ScrollTo(row+max(h/2, 1), col)
			return nil
		case ev.Key() == tcell.KeyCtrlU:
			t.ScrollTo(max(row-max(h/2, 1), 0), col)
			return nil
		}
		return ev
	})
	_, _, w, h := u.root.GetRect()
	u.pages.AddPage("graph", modal(t, max(w-4, 40), max(h-2, 10)), true, true)
	u.app.SetFocus(t)

	path := r.Path
	go func() {
		ctx, cancel := context.WithTimeout(u.ctx, 15*time.Second)
		defer cancel()
		out, total, err := u.eng.Git.Graph(ctx, path)
		u.app.QueueUpdateDraw(func() {
			switch {
			case err != nil:
				t.SetText("[red]" + tview.Escape(err.Error()) + "[-]")
			case strings.TrimSpace(out) == "":
				t.SetText("[::d]no commits yet[::-]")
			default:
				text := tview.TranslateANSI(out)
				if total > git.GraphLimit {
					text += fmt.Sprintf("\n[::d]… the newest %d of %d commits[::-]", git.GraphLimit, total)
				}
				t.SetText(text).ScrollToBeginning()
			}
		})
	}()
}
