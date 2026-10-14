// Package term is mrm's plain terminal output: ANSI colour that honours
// NO_COLOR and --no-color, column padding, ages, and a live progress line.
package term

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	xterm "golang.org/x/term"

	"github.com/adegoodyer/mrm/internal/model"
)

// IsTTY reports whether f is a terminal.
func IsTTY(f *os.File) bool { return xterm.IsTerminal(int(f.Fd())) }

// Width of the terminal on f, or 0 when unknown.
func Width(f *os.File) int {
	w, _, err := xterm.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return w
}

// Style colours text when enabled.
type Style struct{ Enabled bool }

// NewStyle enables colour on a terminal unless NO_COLOR is set or noColor.
func NewStyle(f *os.File, noColor bool) Style {
	return Style{Enabled: !noColor && os.Getenv("NO_COLOR") == "" && IsTTY(f)}
}

func (s Style) wrap(code, text string) string {
	if !s.Enabled || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s Style) Bold(t string) string    { return s.wrap("1", t) }
func (s Style) Dim(t string) string     { return s.wrap("2", t) }
func (s Style) Red(t string) string     { return s.wrap("31", t) }
func (s Style) Green(t string) string   { return s.wrap("32", t) }
func (s Style) Yellow(t string) string  { return s.wrap("33", t) }
func (s Style) Blue(t string) string    { return s.wrap("34", t) }
func (s Style) Magenta(t string) string { return s.wrap("35", t) }
func (s Style) Cyan(t string) string    { return s.wrap("36", t) }

// State colours a state's glyph.
func (s Style) State(st model.State) string {
	g := st.Glyph()
	switch st {
	case model.StateError:
		return s.Red(g)
	case model.StateConflict, model.StateDiverged:
		return s.Magenta(g)
	case model.StateDirty:
		return s.Yellow(g)
	case model.StateUnpushed:
		return s.Cyan(g)
	case model.StateBehind:
		return s.Blue(g)
	default:
		return s.Green(g)
	}
}

// Outcome renders an outcome's glyph.
func (s Style) Outcome(o model.Outcome) string {
	switch o {
	case model.OutcomeSynced, model.OutcomeFetched:
		return s.Green("✓")
	case model.OutcomeWould:
		return s.Cyan("→")
	case model.OutcomeCurrent:
		return s.Dim("·")
	case model.OutcomeExcluded:
		return s.Dim("-")
	case model.OutcomeSkipped:
		return s.Yellow("!")
	case model.OutcomeFailed:
		return s.Red("✗")
	}
	return " "
}

// Pad pads text (which may hold ANSI codes) to width visible runes, and
// truncates plain text that is too long with an ellipsis.
func Pad(text string, width int) string {
	n := VisibleLen(text)
	if n > width && !strings.Contains(text, "\x1b[") {
		return Truncate(text, width)
	}
	if n >= width {
		return text
	}
	return text + strings.Repeat(" ", width-n)
}

// Truncate shortens plain text to width runes.
func Truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= width {
		return text
	}
	r := []rune(text)
	return string(r[:width-1]) + "…"
}

// VisibleLen counts runes outside ANSI escapes.
func VisibleLen(text string) int {
	n, esc := 0, false
	for _, c := range text {
		switch {
		case esc:
			if c == 'm' {
				esc = false
			}
		case c == '\x1b':
			esc = true
		default:
			n++
		}
	}
	return n
}

// Age renders a duration since t as 45s, 12m, 3h, 5d, 7w, 2y.
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

// Progress is a single status line redrawn in place on a terminal (stderr).
// Lines printed through it go to out, above the status line. Without a
// terminal it only passes lines through.
type Progress struct {
	mu      sync.Mutex
	out     io.Writer
	w       *os.File
	live    bool
	shown   bool
	status  string
	frame   int
	stop    chan struct{}
	stopped chan struct{}
}

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewProgress prints lines to out and, when live and f is a terminal,
// redraws a status line on f.
func NewProgress(out io.Writer, f *os.File, live bool) *Progress {
	p := &Progress{out: out, w: f, live: live && IsTTY(f), stop: make(chan struct{}), stopped: make(chan struct{})}
	if p.live {
		go p.tick()
	} else {
		close(p.stopped)
	}
	return p
}

func (p *Progress) tick() {
	defer close(p.stopped)
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.mu.Lock()
			p.frame++
			p.draw()
			p.mu.Unlock()
		}
	}
}

func (p *Progress) draw() {
	if !p.live || p.status == "" {
		return
	}
	line := spinner[p.frame%len(spinner)] + " " + p.status
	if w := Width(p.w); w > 4 {
		line = Truncate(line, w-1)
	}
	fmt.Fprint(p.w, "\r\x1b[K"+line)
	p.shown = true
}

func (p *Progress) clear() {
	if p.shown {
		fmt.Fprint(p.w, "\r\x1b[K")
		p.shown = false
	}
}

// Status sets the text of the live line.
func (p *Progress) Status(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = s
	p.draw()
}

// Println prints a line above the live line.
func (p *Progress) Println(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clear()
	fmt.Fprintln(p.out, s)
	p.draw()
}

// Done removes the live line.
func (p *Progress) Done() {
	if p.live {
		select {
		case <-p.stop:
		default:
			close(p.stop)
		}
	}
	<-p.stopped
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = ""
	p.clear()
}
