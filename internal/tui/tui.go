// Package tui is mrm's interactive fleet view: one dense, grouped, live list
// of every repo, with a command palette, filters, marks and bulk actions,
// built on tview. It drives the same engine as the CLI.
package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.uber.org/zap"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/discover"
	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/filter"
	"github.com/adegoodyer/mrm/internal/forgeapi"
	"github.com/adegoodyer/mrm/internal/logger"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/store"
	"github.com/adegoodyer/mrm/internal/view"
)

// Options configure a TUI session.
type Options struct {
	Config    *config.Config
	Engine    *engine.Engine
	Workspace string // initial workspace; "" for all
	Query     string // initial filter
	Screen    tcell.Screen
	NoRefresh bool             // don't read status on start (tests)
	Repos     []model.Repo     // skip discovery (tests)
	Results   []model.Result   // seed results instead of the cache (tests)
	CachePath string           // default paths.StatusCache()
	HistPath  string           // default paths.History()
	Forge     *forgeapi.Client // PR and CI info; nil leaves it out
	StatePath string           // where the view is remembered; "" doesn't (tests)
}

type ui struct {
	opts Options
	cfg  *config.Config
	eng  *engine.Engine
	ctx  context.Context

	app    *tview.Application
	pages  *tview.Pages
	root   *tview.Flex
	top    *tview.Flex
	body   *tview.Flex
	header *tview.TextView
	status *tview.TextView
	footer *tview.TextView
	input  *tview.InputField
	detail *tview.TextView
	list   *list

	repos     []model.Repo
	results   map[string]model.Result  // by path
	forge     map[string]forgeapi.Info // by path
	running   map[string]bool
	marks     map[string]bool
	collapsed map[string]bool
	keys      []binding

	ws         string
	query      string
	filt       filter.Filter
	attention  bool
	showDetail bool
	filtering  bool

	loading   bool
	busy      string // label of the running operation
	busyFlag  atomic.Bool
	detailSeq atomic.Int64
	forgeBusy atomic.Bool
	msgFlag   atomic.Bool // a footer message is showing; read by tick
	progress  [2]int      // done, total
	lastSync  time.Time
	message   string
	msgStyle  string
	msgAt     time.Time
	quitArmed bool
	frame     int
	width     int // of the screen, for the footer
}

// Run starts the TUI and returns when the user quits.
func Run(ctx context.Context, opts Options) error {
	start := time.Now()
	u, err := newUI(ctx, opts)
	if err != nil {
		return err
	}
	var once sync.Once
	u.app.SetAfterDrawFunc(func(tcell.Screen) {
		once.Do(func() {
			logger.L.Debug("tui first frame", zap.Duration("took", time.Since(start)), zap.Int("repos", len(u.repos)))
		})
	})
	return u.run()
}

func newUI(ctx context.Context, opts Options) (*ui, error) {
	if opts.CachePath == "" {
		opts.CachePath = paths.StatusCache()
	}
	if opts.HistPath == "" {
		opts.HistPath = paths.History()
	}
	f, err := filter.Parse(opts.Query)
	if err != nil {
		return nil, err
	}
	u := &ui{
		opts: opts, cfg: opts.Config, eng: opts.Engine, ctx: ctx,
		results: map[string]model.Result{}, forge: map[string]forgeapi.Info{}, running: map[string]bool{},
		marks: map[string]bool{}, collapsed: map[string]bool{},
		ws: opts.Workspace, query: opts.Query, filt: f.WithForges(opts.Config.Forges),
		keys: bindings(),
	}
	u.build()
	u.loadState()
	u.load()
	return u, nil
}

// load discovers repos and paints them from the cache, so the first frame
// is immediate; run() then refreshes status in the background.
func (u *ui) load() {
	u.repos = u.opts.Repos
	if u.repos == nil {
		wss, _ := u.cfg.Select(nil)
		u.repos, _ = discover.All(u.cfg, wss)
	}
	if u.opts.Results != nil {
		for _, r := range u.opts.Results {
			u.results[r.Repo.Path] = r
		}
	} else {
		cache := store.LoadCache(u.opts.CachePath)
		for _, r := range u.repos {
			u.results[r.Path] = model.Result{Repo: r, Status: cache.Repos[r.Path]}
		}
	}
	for _, r := range u.repos {
		res := u.results[r.Path]
		res.Repo = r
		u.results[r.Path] = res
		if u.opts.Forge != nil {
			if i, ok := u.opts.Forge.CachedFor(forgeTarget(res)); ok {
				u.forge[r.Path] = i
			}
		}
	}
	if run, ok := store.LastRun(u.opts.HistPath, "sync"); ok {
		u.lastSync = run.Started.Add(time.Duration(run.DurationMS) * time.Millisecond)
	}
	u.loading = false
	u.refresh()
}

func (u *ui) build() {
	u.app = tview.NewApplication()
	if u.opts.Screen != nil {
		u.app.SetScreen(u.opts.Screen)
	}
	u.header = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	u.status = tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetTextAlign(tview.AlignRight)
	u.footer = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	u.detail = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	u.detail.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	u.list = newList(u)

	u.input = tview.NewInputField().SetLabel("/ ").SetFieldBackgroundColor(tcell.ColorDefault)
	u.input.SetLabelStyle(stAccent)
	u.input.SetChangedFunc(u.onFilterChanged)
	u.input.SetDoneFunc(u.onFilterDone)

	u.top = tview.NewFlex().
		AddItem(u.header, 0, 1, false).
		AddItem(u.status, 30, 0, false)
	u.body = tview.NewFlex().AddItem(u.list, 0, 1, true)
	u.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(u.top, 1, 0, false).
		AddItem(u.body, 0, 1, true).
		AddItem(u.footer, 1, 0, false)

	u.pages = tview.NewPages().AddPage("main", u.root, true, true)
	u.app.SetRoot(u.pages, true).SetFocus(u.list)
	u.app.SetInputCapture(u.onKey)
	u.app.EnableMouse(u.cfg.TUI.MouseOn())
	u.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		// Refit the footer when the terminal is resized. This runs on the
		// UI goroutine, inside a draw: change state only, never Draw.
		if w, _ := screen.Size(); w != u.width {
			u.width = w
			u.drawFooter()
		}
		return false
	})
	u.loading = true
}

func (u *ui) run() error {
	if !u.opts.NoRefresh {
		go func() {
			u.app.QueueUpdateDraw(func() {
				u.startOp(engine.OpStatus, u.repos, u.cfg.TUI.FetchOnStart, "reading")
			})
		}()
	}
	stop := make(chan struct{})
	go u.tick(stop)
	defer close(stop)
	err := u.app.Run()
	u.saveState()
	return err
}

// tick animates spinners while an operation runs and expires footer
// messages. It runs on its own goroutine and only ever queues work: nothing
// queued may itself call QueueUpdate or Draw, which wait for the UI loop
// that is running them - a deadlock that freezes the terminal.
func (u *ui) tick(stop chan struct{}) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if !u.busyFlag.Load() && !u.msgFlag.Load() {
				continue // idle: don't wake the UI
			}
			u.app.QueueUpdateDraw(func() {
				if u.busy != "" {
					u.frame++
				}
				if u.message != "" && time.Since(u.msgAt) > msgTTL {
					u.message = ""
					u.msgFlag.Store(false)
				}
				u.drawChrome()
			})
		}
	}
}

// msgTTL is how long a footer message stays (tests shorten it).
var msgTTL = 6 * time.Second

// visible returns the repos that pass the workspace, filter and attention
// view, in display order.
func (u *ui) visible() []model.Repo {
	var out []model.Repo
	for _, r := range u.repos {
		if u.ws != "" && r.Workspace != u.ws {
			continue
		}
		res := u.results[r.Path]
		if !u.filt.Match(r, res.Status) {
			continue
		}
		if u.attention && view.Reason(res) == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// groupOf is the group key and label for a repo in the current view.
func (u *ui) groupOf(r model.Repo) (string, string) {
	if u.attention {
		reason := view.Reason(u.results[r.Path])
		return "reason:" + reason, reason
	}
	key := r.Workspace + "/" + r.Group
	group := r.Group
	if group == "" {
		// A workspace without groups is one section named after it; in one
		// with groups, the rest are "(no group)", not the workspace's name.
		if !u.hasGroups(r.Workspace) {
			return key, r.Workspace
		}
		group = "(no group)"
	}
	if u.ws == "" && u.workspaceCount() > 1 {
		return key, r.Workspace + " · " + group
	}
	return key, group
}

// hasGroups reports whether any repo in workspace ws is in a group.
func (u *ui) hasGroups(ws string) bool {
	for _, r := range u.repos {
		if r.Workspace == ws && r.Group != "" {
			return true
		}
	}
	return false
}

func (u *ui) workspaceCount() int { return len(u.cfg.Workspaces) }

// refresh rebuilds the list items and redraws the chrome.
func (u *ui) refresh() {
	repos := u.visible()
	type group struct {
		key, label string
		repos      []model.Repo
	}
	var groups []*group
	byKey := map[string]*group{}
	for _, r := range repos {
		key, label := u.groupOf(r)
		g := byKey[key]
		if g == nil {
			g = &group{key: key, label: label}
			byKey[key] = g
			groups = append(groups, g)
		}
		g.repos = append(g.repos, r)
	}
	if u.attention {
		// Failures first, then skips, then states.
		rank := func(label string) int {
			switch {
			case strings.HasPrefix(label, "failed"):
				return 0
			case strings.HasPrefix(label, "skipped"):
				return 1
			}
			return 2
		}
		sort.SliceStable(groups, func(i, j int) bool {
			if rank(groups[i].label) != rank(groups[j].label) {
				return rank(groups[i].label) < rank(groups[j].label)
			}
			return groups[i].label < groups[j].label
		})
	}
	var items []item
	for _, g := range groups {
		c := u.collapsed[g.key]
		items = append(items, item{header: true, group: g.key, label: g.label, count: len(g.repos), collapsed: c})
		if c {
			continue
		}
		for _, r := range g.repos {
			items = append(items, item{group: g.key, path: r.Path})
		}
	}
	u.list.setItems(items)
	u.drawChrome()
	u.updateDetail()
}

// drawChrome redraws the header, status and footer lines.
func (u *ui) drawChrome() {
	ws := u.ws
	if ws == "" {
		ws = "all"
	}
	var inWS []model.Result
	for _, r := range u.repos {
		if u.ws == "" || r.Workspace == u.ws {
			inWS = append(inWS, u.results[r.Path])
		}
	}
	shown := 0
	for _, it := range u.list.items {
		if !it.header {
			shown++
		}
	}
	var b strings.Builder
	b.WriteString(" [::b]mrm[::-]  ")
	if u.attention {
		// First, so a narrow terminal never cuts it off.
		b.WriteString("[yellow::b]! needs attention[-::-]  ")
	}
	fmt.Fprintf(&b, "%s · %s", tview.Escape(ws), repoCount(len(inWS)))
	if shown != len(inWS) && !u.anyCollapsed() {
		fmt.Fprintf(&b, " [::d](%d shown)[::-]", shown)
	}
	counts := stateCounts(inWS)
	b.WriteString("  ")
	for _, s := range model.States {
		if counts[s] > 0 {
			fmt.Fprintf(&b, "  [%s]%s[-] %d", tagColor(s), s.Glyph(), counts[s])
		}
	}
	u.header.SetText(b.String())

	var st string
	switch {
	case u.busy != "":
		st = fmt.Sprintf("[aqua]%s %s %d/%d[-]", spinner[u.frame%len(spinner)], u.busy, u.progress[0], u.progress[1])
	case !u.lastSync.IsZero():
		st = "[::d]synced " + view.Age(u.lastSync) + " ago[::-]"
	default:
		st = "[::d]never synced[::-]"
	}
	st += "   [::d]ctrl+k[::-] "
	u.status.SetText(st)
	// The status takes what it needs; the header gets the rest.
	u.top.ResizeItem(u.status, tview.TaggedStringWidth(st)+1, 0)

	u.drawFooter()
}

func (u *ui) anyCollapsed() bool {
	for _, c := range u.collapsed {
		if c {
			return true
		}
	}
	return false
}

func tagColor(s model.State) string {
	switch s {
	case model.StateError:
		return "red"
	case model.StateConflict, model.StateDiverged:
		return "fuchsia"
	case model.StateDirty:
		return "yellow"
	case model.StateUnpushed:
		return "aqua"
	case model.StateBehind:
		return "blue"
	}
	return "green"
}

func repoCount(n int) string {
	if n == 1 {
		return "1 repo"
	}
	return fmt.Sprintf("%d repos", n)
}

// say shows a message in the footer for a few seconds.
func (u *ui) say(msg string) { u.show(msg, "[::b]") }

func (u *ui) warn(msg string) { u.show(msg, "[yellow::b]") }

func (u *ui) show(msg, style string) {
	u.message, u.msgStyle, u.msgAt = msg, style, time.Now()
	u.msgFlag.Store(true)
	u.drawChrome()
}

// onKey handles keys for the list from the key table (keys.go); overlays
// and the filter input get their keys first through their own handlers.
func (u *ui) onKey(ev *tcell.EventKey) *tcell.EventKey {
	if front, _ := u.pages.GetFrontPage(); front != "main" {
		return ev // an overlay is open
	}
	if u.filtering {
		return ev // the input field handles it
	}
	if ev.Key() == tcell.KeyCtrlC {
		u.quit()
		return nil
	}
	if ev.Key() != tcell.KeyRune || ev.Rune() != 'q' {
		u.quitArmed = false
	}
	b, ok := u.bindingFor(ev)
	if !ok {
		if ev.Key() == tcell.KeyRune {
			return nil // unbound letters do nothing
		}
		return ev
	}
	before, _ := u.list.current()
	b.run(u)
	u.cursorMoved(before)
	return nil
}

// cursorMoved updates what follows the cursor - the detail pane and the
// footer's hints - when it is on a different row than before.
func (u *ui) cursorMoved(before item) {
	if after, _ := u.list.current(); after != before {
		u.updateDetail()
		u.drawFooter()
	}
}

func (u *ui) quit() {
	if u.busy != "" && !u.quitArmed {
		u.quitArmed = true
		u.warn(u.busy + " is running - press q again to quit (git finishes what it started)")
		return
	}
	u.app.Stop()
}

// targets are the repos an action applies to: the marked ones, else the
// group under the cursor, else the repo under it.
func (u *ui) targets() []model.Repo {
	if len(u.marks) > 0 {
		var out []model.Repo
		for _, r := range u.repos {
			if u.marks[r.Path] {
				out = append(out, r)
			}
		}
		return out
	}
	cur, ok := u.list.current()
	if !ok {
		return nil
	}
	if cur.header {
		var out []model.Repo
		for _, r := range u.visible() {
			if k, _ := u.groupOf(r); k == cur.group {
				out = append(out, r)
			}
		}
		return out
	}
	return []model.Repo{u.results[cur.path].Repo}
}

func (u *ui) currentRepo() (model.Repo, bool) {
	cur, ok := u.list.current()
	if !ok || cur.header {
		return model.Repo{}, false
	}
	return u.results[cur.path].Repo, true
}

func (u *ui) toggleMark() {
	cur, ok := u.list.current()
	if !ok {
		return
	}
	if cur.header {
		for _, r := range u.targets() {
			u.marks[r.Path] = true
		}
	} else if u.marks[cur.path] {
		delete(u.marks, cur.path)
	} else {
		u.marks[cur.path] = true
	}
	u.list.move(1)
	u.drawChrome()
}

func (u *ui) markVisible() {
	for _, r := range u.visible() {
		u.marks[r.Path] = true
	}
	u.drawChrome()
}

func (u *ui) toggleGroup() {
	cur, ok := u.list.current()
	if !ok {
		return
	}
	u.collapsed[cur.group] = !u.collapsed[cur.group]
	u.refresh()
	// Land on the group header so tab toggles it straight back.
	for i, it := range u.list.items {
		if it.header && it.group == cur.group {
			u.list.cursor = i
			break
		}
	}
}

// Filter input.

func (u *ui) startFilter() {
	u.filtering = true
	u.input.SetText(u.query)
	u.root.RemoveItem(u.footer)
	u.root.AddItem(u.input, 1, 0, true)
	u.app.SetFocus(u.input)
}

func (u *ui) endFilter() {
	u.filtering = false
	u.root.RemoveItem(u.input)
	u.root.AddItem(u.footer, 1, 0, false)
	u.app.SetFocus(u.list)
	u.drawChrome()
}

func (u *ui) onFilterChanged(text string) {
	f, err := filter.Parse(text)
	if err != nil {
		u.input.SetLabelStyle(stError)
		return
	}
	u.input.SetLabelStyle(stAccent)
	u.query, u.filt = strings.TrimSpace(text), f.WithForges(u.cfg.Forges)
	u.refresh()
}

func (u *ui) onFilterDone(key tcell.Key) {
	if key == tcell.KeyEscape {
		u.setFilter("")
	}
	u.endFilter()
}

func (u *ui) setFilter(q string) {
	f, err := filter.Parse(q)
	if err != nil {
		u.warn(err.Error())
		return
	}
	u.query, u.filt = q, f.WithForges(u.cfg.Forges)
	u.refresh()
}

// modal centres p in a box of the given size over the main page.
func modal(p tview.Primitive, w, h int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, h, 0, true).
			AddItem(nil, 0, 1, false), w, 0, true).
		AddItem(nil, 0, 1, false)
}

func (u *ui) closeOverlay(name string) {
	u.pages.RemovePage(name)
	u.app.SetFocus(u.list)
}

func init() {
	// Keep tview from swallowing the terminal's own background.
	if os.Getenv("NO_COLOR") != "" {
		tview.Styles.SecondaryTextColor = tcell.ColorDefault
	}
}
