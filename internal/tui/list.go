package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/view"
)

// item is one line of the list: a group header or a repo.
type item struct {
	header    bool
	group     string // group key; for repos, the group they sit in
	label     string // header text
	count     int    // repos in the group
	collapsed bool
	path      string // repo
}

// list is the fleet view: a dense, grouped list drawn cell by cell, so group
// headers can span the full width and every column truncates cleanly.
type list struct {
	*tview.Box
	u      *ui
	items  []item
	cursor int
	offset int
	height int
}

func newList(u *ui) *list {
	return &list{Box: tview.NewBox(), u: u}
}

// setItems replaces the items, keeping the cursor on the same repo or
// group when it is still there.
func (l *list) setItems(items []item) {
	var keepPath, keepGroup string
	if cur, ok := l.current(); ok {
		keepPath, keepGroup = cur.path, cur.group
		if cur.header {
			keepPath = ""
		}
	}
	l.items = items
	l.cursor = 0
	for i, it := range items {
		if (keepPath != "" && it.path == keepPath) || (keepPath == "" && it.header && it.group == keepGroup) {
			l.cursor = i
			return
		}
	}
	// The repo is gone (filtered out): stay near where we were.
	for i, it := range items {
		if !it.header {
			l.cursor = i
			return
		}
	}
}

func (l *list) current() (item, bool) {
	if l.cursor < 0 || l.cursor >= len(l.items) {
		return item{}, false
	}
	return l.items[l.cursor], true
}

func (l *list) move(delta int) {
	if len(l.items) == 0 {
		return
	}
	l.cursor = min(max(l.cursor+delta, 0), len(l.items)-1)
}

func (l *list) page(dir int) { l.move(dir * max(l.height-1, 1)) }
func (l *list) top()         { l.cursor = 0 }
func (l *list) bottom()      { l.cursor = max(len(l.items)-1, 0) }

// columns is the width of each column at a given total width.
type columns struct {
	name, branch, ab, changes, age int
}

const (
	markW  = 2 // mark + space
	glyphW = 3 // glyph + 2 spaces
	gap    = 2
)

func (l *list) layout(w int, rows []view.Row) columns {
	c := columns{age: 4}
	for _, r := range rows {
		c.name = max(c.name, width(r.Name))
		c.branch = max(c.branch, width(r.Branch))
		c.ab = max(c.ab, width(r.AB))
		c.changes = max(c.changes, width(r.Changes))
	}
	c.name = min(max(c.name, 4), max(16, w*32/100))
	c.branch = min(max(c.branch, 6), max(8, w*16/100))
	c.ab = min(max(c.ab, 1), 9)
	c.changes = min(c.changes, 12)
	return c
}

func (l *list) Draw(screen tcell.Screen) {
	l.Box.DrawForSubclass(screen, l)
	x, y, w, h := l.GetInnerRect()
	l.height = h
	if len(l.items) == 0 {
		msg := "no repos"
		if l.u.loading {
			msg = "finding repos…"
		} else if !l.u.filt.Empty() || l.u.attention {
			msg = "nothing matches - esc clears the filter, ! leaves the attention view"
		}
		put(screen, x+2, y+1, x+w, msg, stDim)
		return
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+h {
		l.offset = l.cursor - h + 1
	}
	l.offset = max(0, min(l.offset, len(l.items)-1))

	rows := make([]view.Row, 0, len(l.items))
	for _, it := range l.items {
		if !it.header {
			rows = append(rows, view.Build(l.u.results[it.path]))
		}
	}
	cols := l.layout(w, rows)

	for i := 0; i < h && l.offset+i < len(l.items); i++ {
		idx := l.offset + i
		it := l.items[idx]
		sel := idx == l.cursor && l.HasFocus()
		if it.header {
			l.drawHeader(screen, x, y+i, w, it, sel)
		} else {
			l.drawRepo(screen, x, y+i, w, it, cols, sel)
		}
	}
}

func (l *list) drawHeader(screen tcell.Screen, x, y, w int, it item, sel bool) {
	style := stHeader
	if sel {
		style = stSelected
	}
	arrow := "▾"
	if it.collapsed {
		arrow = "▸"
	}
	text := fmt.Sprintf(" %s %s · %d ", arrow, it.label, it.count)
	end := put(screen, x, y, x+w, "─", style)
	end = put(screen, end, y, x+w, text, style)
	fill(screen, end, y, x+w, '─', style.Bold(false).Dim(!sel))
}

func (l *list) drawRepo(screen tcell.Screen, x, y, w int, it item, c columns, sel bool) {
	u := l.u
	res := u.results[it.path]
	r := view.Build(res)
	maxX := x + w
	base := stDefault
	if sel {
		base = stSelected
		fill(screen, x, y, maxX, ' ', base)
	}
	st := func(s tcell.Style) tcell.Style {
		if sel {
			return s.Reverse(true)
		}
		return s
	}

	cx := x
	if u.marks[it.path] {
		put(screen, cx, y, maxX, "▌", st(stMark))
	}
	cx += markW

	glyph, gstyle := r.State.Glyph(), stateStyle(r.State)
	if u.running[it.path] {
		glyph, gstyle = spinner[u.frame%len(spinner)], stAccent
	} else if res.Status.ReadAt.IsZero() {
		glyph, gstyle = "·", stDim
	}
	put(screen, cx, y, maxX, glyph, st(gstyle))
	cx += glyphW

	put(screen, cx, y, maxX, pad(r.Name, c.name), st(stBold))
	cx += c.name + gap

	if r.Error != "" {
		put(screen, cx, y, maxX, r.Error, st(stError))
		return
	}
	put(screen, cx, y, maxX, pad(r.Branch, c.branch), base)
	cx += c.branch + gap
	ab := r.AB
	abStyle := base
	if ab == "" {
		ab, abStyle = "·", st(stDim)
	}
	put(screen, cx, y, maxX, pad(ab, c.ab), abStyle)
	cx += c.ab + gap
	if c.changes > 0 {
		put(screen, cx, y, maxX, pad(r.Changes, c.changes), st(stWarn))
		cx += c.changes + gap
	}
	put(screen, cx, y, maxX, fmt.Sprintf("%*s", c.age, r.Age), st(stDim))
	cx += c.age + gap

	// Last column: the outcome of the last run when it needs attention,
	// else notes, else the last commit subject.
	switch {
	case u.running[it.path]:
		put(screen, cx, y, maxX, u.busy+"…", st(stAccent))
	case res.Outcome.NeedsAttention():
		put(screen, cx, y, maxX, res.Summary(), st(outcomeStyle(res.Outcome)))
	case len(r.Notes) > 0:
		put(screen, cx, y, maxX, strings.Join(r.Notes, " · "), st(stDim))
	default:
		put(screen, cx, y, maxX, res.Status.LastSubject, st(stDim))
	}
}

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// stateCounts is "✓ 71  ↓ 3  ● 2" for the header.
func stateCounts(results []model.Result) map[model.State]int {
	n := map[model.State]int{}
	for _, r := range results {
		if !r.Status.ReadAt.IsZero() {
			n[r.Status.State()]++
		}
	}
	return n
}
