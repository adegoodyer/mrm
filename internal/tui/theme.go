package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/adegoodyer/mrm/internal/model"
)

// Colours are the terminal's own ANSI palette and its default background,
// so light and dark themes both read well. tcell drops colour under
// NO_COLOR; the attributes (bold, dim, reverse) still carry the meaning.
var (
	stDefault  = tcell.StyleDefault
	stDim      = tcell.StyleDefault.Dim(true)
	stBold     = tcell.StyleDefault.Bold(true)
	stAccent   = tcell.StyleDefault.Foreground(tcell.ColorAqua).Bold(true)
	stMark     = tcell.StyleDefault.Foreground(tcell.ColorFuchsia).Bold(true)
	stSelected = tcell.StyleDefault.Reverse(true)
	stHeader   = tcell.StyleDefault.Foreground(tcell.ColorSilver).Bold(true)
	stWarn     = tcell.StyleDefault.Foreground(tcell.ColorYellow)
	stError    = tcell.StyleDefault.Foreground(tcell.ColorRed)
)

func init() {
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = tcell.ColorDefault
	tview.Styles.MoreContrastBackgroundColor = tcell.ColorDefault
	tview.Styles.PrimaryTextColor = tcell.ColorDefault
	tview.Styles.BorderColor = tcell.ColorSilver
	tview.Styles.TitleColor = tcell.ColorDefault
	tview.Styles.SecondaryTextColor = tcell.ColorAqua
}

// stateStyle colours a state glyph like the CLI does.
func stateStyle(s model.State) tcell.Style {
	switch s {
	case model.StateError:
		return tcell.StyleDefault.Foreground(tcell.ColorRed).Bold(true)
	case model.StateConflict, model.StateDiverged:
		return tcell.StyleDefault.Foreground(tcell.ColorFuchsia).Bold(true)
	case model.StateDirty:
		return tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true)
	case model.StateUnpushed:
		return tcell.StyleDefault.Foreground(tcell.ColorAqua).Bold(true)
	case model.StateBehind:
		return tcell.StyleDefault.Foreground(tcell.ColorBlue).Bold(true)
	default:
		return tcell.StyleDefault.Foreground(tcell.ColorGreen)
	}
}

// outcomeStyle colours the result of the last operation on a repo.
func outcomeStyle(o model.Outcome) tcell.Style {
	switch o {
	case model.OutcomeSynced, model.OutcomeFetched:
		return tcell.StyleDefault.Foreground(tcell.ColorGreen)
	case model.OutcomeSkipped:
		return stWarn
	case model.OutcomeFailed:
		return stError
	}
	return stDim
}

// width is the number of terminal cells s takes.
func width(s string) int { return uniseg.StringWidth(s) }

// put draws s at (x, y) and returns the x after it. Text that would pass
// maxX is cut with an ellipsis.
func put(screen tcell.Screen, x, y, maxX int, s string, style tcell.Style) int {
	if x >= maxX {
		return x
	}
	if x+width(s) > maxX {
		s = truncate(s, maxX-x)
	}
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		rs := g.Runes()
		w := g.Width()
		if w == 0 {
			continue
		}
		screen.SetContent(x, y, rs[0], rs[1:], style)
		x += w
	}
	return x
}

// fill paints cells from x to maxX with r.
func fill(screen tcell.Screen, x, y, maxX int, r rune, style tcell.Style) {
	for ; x < maxX; x++ {
		screen.SetContent(x, y, r, nil, style)
	}
}

// truncate cuts s to w cells, ending in an ellipsis when it was cut.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if width(s) <= w {
		return s
	}
	out, n := "", 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		if n+g.Width() > w-1 {
			break
		}
		out += g.Str()
		n += g.Width()
	}
	return out + "…"
}

// pad right-pads s to w cells (truncating when longer).
func pad(s string, w int) string {
	s = truncate(s, w)
	for n := width(s); n < w; n++ {
		s += " "
	}
	return s
}
