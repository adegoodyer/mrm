package web

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/filter"
	"github.com/adegoodyer/mrm/internal/forge"
	"github.com/adegoodyer/mrm/internal/forgeapi"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/runner"
	"github.com/adegoodyer/mrm/internal/store"
	"github.com/adegoodyer/mrm/internal/view"
)

// repoID is a short, stable id for a repo path, safe in URLs and HTML ids.
func repoID(path string) string {
	sum := sha1.Sum([]byte(path))
	return hex.EncodeToString(sum[:5])
}

// View models: everything a template shows, computed here.

type pageData struct {
	Page, Title, Token string
	TextSize           int
	Busy               busyVM
	Query              string
	Chips              []string
	WS                 []wsVM
	Attention          []groupVM
	AttentionCount     int
	Risk               []groupVM
	Rows               rowsVM
	Repo               repoVM
	Runs               []runVM
	HistPath           string
	Sizes              []sizeVM
	DefaultSize        string
	ConfigPath         string
	ConfigText         string
}

type busyVM struct {
	Busy     bool   `json:"busy"`
	Text     string `json:"text"`
	Finished bool   `json:"finished"`
}

type wsVM struct {
	Name, Roots, Synced string
	Count               int
	Bar                 []segVM
	Legend              []legendVM
}

type segVM struct {
	State string
	X, W  float64
	N     int
}

type legendVM struct {
	State, Glyph string
	N            int
}

type groupVM struct {
	Title string
	Items []itemVM
	Count int    // all items, before the cap
	More  int    // items not shown
	Query string // filter that lists them all
}

// groupCap keeps one long group (37 stale branches) from swamping a panel;
// the rest are a link to the filtered list.
const groupCap = 8

func capped(title, query string, items []itemVM) groupVM {
	g := groupVM{Title: title, Items: items, Count: len(items), Query: query}
	if len(items) > groupCap {
		g.Items, g.More = items[:groupCap], len(items)-groupCap
	}
	return g
}

type itemVM struct{ ID, State, Glyph, Name, Why string }

type rowsVM struct {
	Rows         []rowVM
	Shown, Total int
}

type rowVM struct {
	ID, State, Glyph, Name, Group, WS, Branch, AB, Changes, Age, Notes, NoteClass, Busy string
	Tags                                                                                []string
	Running                                                                             bool
}

type repoVM struct {
	ID, Name, Display, WS, Path, State, Glyph, Error     string
	Branch, Upstream, AB, Default, LastRun, LastRunClass string
	Web, PRs, CI                                         string
	PRCount, CIState, ForgeErr, HostName                 string
	Branches                                             []branchVM
	Stashes                                              []string
	Commits                                              []commitVM
}

type branchVM struct {
	Name, Track string
	Current     bool
}

type commitVM struct{ SHA, Subject, Age string }

type runVM struct {
	Op, When, WS, Took string
	Pills              []pillVM
	Items              []runItemVM
	Open               bool
}

type pillVM struct {
	Label, Class string
	N            int
}

type runItemVM struct{ Name, Text, Class string }

type sizeVM struct {
	Label string
	PX    int
}

var chips = []string{"is:dirty", "is:behind", "is:unpushed", "is:stash", "is:stale-branches", "is:not-default", "is:error"}

func (s *Server) base(page, title string) pageData {
	return pageData{
		Page: page, Title: title, Token: s.token,
		TextSize: textSizePX(s.cfg.Web.TextSize),
		Busy:     s.busyLocked(false),
	}
}

func textSizePX(name string) int {
	if px, ok := config.TextSizes[name]; ok {
		return px
	}
	return 16
}

func (s *Server) render(name string, data any) string {
	var b bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&b, name, data); err != nil {
		return "<!-- " + err.Error() + " -->"
	}
	return b.String()
}

func (s *Server) page(w http.ResponseWriter, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(w, name, data)
}

func (s *Server) busyLocked(finished bool) busyVM {
	switch {
	case s.busy != "":
		return busyVM{Busy: true, Text: fmt.Sprintf("%s %d/%d", s.busy, s.progress[0], s.progress[1])}
	case s.external != "":
		return busyVM{Busy: true, Text: s.external}
	}
	text := "never synced"
	if !s.lastSync.IsZero() {
		text = "synced " + view.Age(s.lastSync) + " ago"
	}
	return busyVM{Text: text, Finished: finished}
}

// Rows.

func (s *Server) rowLocked(res model.Result) rowVM {
	v := view.Build(res)
	r := res.Repo
	row := rowVM{
		ID: repoID(r.Path), State: string(v.State), Glyph: v.State.Glyph(), Name: r.Name, Group: r.Group,
		WS: r.Workspace, Branch: v.Branch, AB: v.AB, Changes: v.Changes, Age: v.Age, Tags: r.Tags,
		Running: s.running[r.Path], Busy: s.busy,
	}
	if row.AB == "" {
		row.AB = "·"
	}
	switch {
	case v.Error != "":
		row.Notes, row.NoteClass, row.Age = v.Error, "s-error", "-"
	case res.Status.ReadAt.IsZero():
		row.State, row.Glyph, row.Notes = "unknown", "·", "reading…"
	case len(v.Notes) > 0 || len(forgeapi.Notes(s.forge[r.Path])) > 0:
		row.Notes = strings.Join(append(v.Notes, forgeapi.Notes(s.forge[r.Path])...), " · ")
	default:
		row.Notes = res.Status.LastSubject
	}
	switch res.Outcome {
	case model.OutcomeSkipped, model.OutcomeFailed:
		row.NoteClass = "o-" + string(res.Outcome)
	case model.OutcomeSynced:
		row.Notes, row.NoteClass = res.Summary(), "o-synced"
	}
	return row
}

func (s *Server) rowsLocked(f filter.Filter) rowsVM {
	vm := rowsVM{Total: len(s.repos)}
	for _, r := range s.repos {
		res := s.results[r.Path]
		if f.Match(r, res.Status) {
			vm.Rows = append(vm.Rows, s.rowLocked(res))
		}
	}
	vm.Shown = len(vm.Rows)
	return vm
}

// Panels: workspace summaries, needs attention, work at risk.

func (s *Server) wsLocked() []wsVM {
	var out []wsVM
	for _, name := range s.cfg.WorkspaceNames() {
		ws := s.cfg.Workspaces[name]
		vm := wsVM{Name: name}
		var roots []string
		for _, r := range ws.Roots {
			roots = append(roots, paths.Tilde(r))
		}
		vm.Roots = strings.Join(roots, ", ")
		counts := map[model.State]int{}
		running := false
		for _, r := range s.repos {
			if r.Workspace != name {
				continue
			}
			vm.Count++
			st := s.results[r.Path].Status
			if !st.ReadAt.IsZero() {
				counts[st.State()]++
			}
			running = running || s.running[r.Path]
		}
		if vm.Count == 0 {
			continue
		}
		x := 0.0
		for _, st := range model.States {
			if n := counts[st]; n > 0 {
				w := float64(n) / float64(vm.Count) * 100
				vm.Bar = append(vm.Bar, segVM{State: string(st), X: x, W: w, N: n})
				vm.Legend = append(vm.Legend, legendVM{State: string(st), Glyph: st.Glyph(), N: n})
				x += w
			}
		}
		vm.Synced = s.busyLocked(false).Text
		if running {
			vm.Synced = s.busy + "…"
		}
		out = append(out, vm)
	}
	return out
}

func (s *Server) itemLocked(res model.Result, why string) itemVM {
	st := res.Status.State()
	return itemVM{ID: repoID(res.Repo.Path), State: string(st), Glyph: st.Glyph(), Name: res.Repo.Display(), Why: why}
}

func (s *Server) attentionLocked() ([]groupVM, int) {
	groups := map[string][]itemVM{}
	n := 0
	for _, r := range s.repos {
		res := s.results[r.Path]
		if res.Status.ReadAt.IsZero() {
			continue
		}
		reason := view.Reason(res)
		if reason == "" {
			continue
		}
		groups[reason] = append(groups[reason], s.itemLocked(res, strings.Join(view.Build(res).Notes, " · ")))
		n++
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	rank := func(k string) int {
		switch {
		case strings.HasPrefix(k, "failed"):
			return 0
		case strings.HasPrefix(k, "skipped"):
			return 1
		}
		for i, st := range model.States {
			if string(st) == k {
				return 2 + i
			}
		}
		return 99
	}
	sort.Slice(keys, func(i, j int) bool {
		if rank(keys[i]) != rank(keys[j]) {
			return rank(keys[i]) < rank(keys[j])
		}
		return keys[i] < keys[j]
	})
	out := make([]groupVM, len(keys))
	for i, k := range keys {
		q := ""
		if rank(k) >= 2 && rank(k) < 99 {
			q = "is:" + k
		}
		out[i] = capped(k, q, groups[k])
	}
	return out, n
}

// riskLocked lists work that exists only on this machine.
func (s *Server) riskLocked() []groupVM {
	var unpushed, stashes, dirty, stale []itemVM
	for _, r := range s.repos {
		res := s.results[r.Path]
		st := res.Status
		if st.ReadAt.IsZero() || st.Error != "" {
			continue
		}
		if n := max(st.Unpushed, st.Ahead); n > 0 {
			unpushed = append(unpushed, s.itemLocked(res, plural(n, "commit", "commits")+" only here"))
		}
		if st.Stashes > 0 {
			stashes = append(stashes, s.itemLocked(res, plural(st.Stashes, "stash", "stashes")))
		}
		if st.Dirty() && !st.LastCommit.IsZero() && time.Since(st.LastCommit) > 24*time.Hour {
			dirty = append(dirty, s.itemLocked(res, view.Build(res).Changes+" · last commit "+view.Age(st.LastCommit)+" ago"))
		}
		if st.StaleBranches > 0 {
			stale = append(stale, s.itemLocked(res, plural(st.StaleBranches, "branch", "branches")+" whose upstream is gone"))
		}
	}
	var out []groupVM
	for _, g := range []groupVM{
		capped("unpushed commits", "is:unpushed", unpushed), capped("stashes", "is:stash", stashes),
		capped("dirty, last commit over a day ago", "is:dirty", dirty), capped("stale branches", "is:stale-branches", stale),
	} {
		if g.Count > 0 {
			out = append(out, g)
		}
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (s *Server) panelsLocked() map[string]string {
	att, n := s.attentionLocked()
	d := pageData{WS: s.wsLocked(), Attention: att, AttentionCount: n, Risk: s.riskLocked(), Busy: s.busyLocked(false)}
	return map[string]string{
		"ws":        s.render("ws", d),
		"attention": s.render("attention", d),
		"risk":      s.render("risk", d),
	}
}

// Handlers.

func (s *Server) parseQuery(q string) (filter.Filter, error) {
	f, err := filter.Parse(q)
	if err != nil {
		return filter.Filter{}, err
	}
	return f.WithForges(s.cfg.Forges), nil
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	f, err := s.parseQuery(q)
	if err != nil {
		f = filter.Filter{}
	}
	s.mu.Lock()
	s.rediscoverLocked()
	d := s.base("overview", "Overview")
	d.Query, d.Chips = q, chips
	d.WS = s.wsLocked()
	d.Attention, d.AttentionCount = s.attentionLocked()
	d.Risk = s.riskLocked()
	d.Rows = s.rowsLocked(f)
	s.mu.Unlock()
	s.page(w, "overview", d)
}

func (s *Server) rows(w http.ResponseWriter, r *http.Request) {
	f, err := s.parseQuery(r.URL.Query().Get("q"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	vm := s.rowsLocked(f)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(w, "rows", vm)
}

func (s *Server) repoPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	path, ok := s.byID[id]
	res := s.results[path]
	d := s.base("repo", res.Repo.Name)
	s.mu.Unlock()
	if !ok {
		http.Error(w, "no such repo - it may have moved; go back to the overview", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	detail := s.run.Eng.Git.ReadDetail(ctx, path, 20)

	st := res.Status
	v := view.Build(res)
	vm := repoVM{
		ID: id, Name: res.Repo.Name, Display: res.Repo.Display(), WS: res.Repo.Workspace, Path: paths.Tilde(path),
		State: string(v.State), Glyph: v.State.Glyph(), Error: v.Error,
		Branch: v.Branch, Upstream: st.Upstream, AB: v.AB, Default: st.DefaultBranch,
		LastRun: "-",
	}
	if res.Outcome != "" {
		vm.LastRun, vm.LastRunClass = res.Summary(), "o-"+string(res.Outcome)
	}
	f := forge.Parse(st.RemoteURL, s.cfg.Forges)
	vm.Web, _ = f.Web()
	vm.HostName = f.Kind.Name()
	vm.PRs, _ = f.PRs()
	vm.CI, _ = f.CI()
	s.mu.Lock()
	fi, hasForge := s.forge[path]
	s.mu.Unlock()
	if hasForge {
		vm.ForgeErr = fi.Err
		if fi.Err == "" {
			vm.PRCount = fmt.Sprint(fi.PRs)
			if fi.PRsMore {
				vm.PRCount += "+"
			}
			if fi.CI != forgeapi.CINone {
				vm.CIState = fi.CI
			}
			if fi.CIURL != "" {
				vm.CI = fi.CIURL
			}
		}
	}
	for _, b := range detail.Branches {
		track := ""
		switch {
		case b.Track != "":
			track = "[" + b.Track + "]"
		case b.Upstream == "":
			track = "(local only)"
		}
		vm.Branches = append(vm.Branches, branchVM{Name: b.Name, Current: b.Current, Track: track})
	}
	vm.Stashes = detail.Stashes
	for _, c := range detail.Commits {
		vm.Commits = append(vm.Commits, commitVM{SHA: c.SHA, Subject: c.Subject, Age: view.Age(c.When)})
	}
	d.Repo = vm
	s.page(w, "repo", d)
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	d := s.base("activity", "Activity")
	s.mu.Unlock()
	d.HistPath = paths.Tilde(s.opts.HistPath)
	for i, run := range store.Runs(s.opts.HistPath, 30) {
		vm := runVM{
			Op:   run.Op,
			When: run.Started.Local().Format("Mon 2 Jan 15:04") + " (" + view.Age(run.Started) + " ago)",
			WS:   strings.Join(run.Workspaces, ", "),
			Took: fmt.Sprintf("%.1f s", float64(run.DurationMS)/1000),
			Open: i == 0,
		}
		if run.DryRun {
			vm.Op += " --dry-run"
		}
		counts := map[string]int{}
		for _, res := range run.Results {
			counts[res.Outcome]++
			switch model.Outcome(res.Outcome) {
			case model.OutcomeSkipped, model.OutcomeFailed, model.OutcomeSynced, model.OutcomeWould:
				text := res.Outcome
				if res.Reason != "" {
					text += ": " + res.Reason
				} else if len(res.Actions) > 0 {
					text += ": " + strings.Join(res.Actions, "; ")
				}
				vm.Items = append(vm.Items, runItemVM{Name: res.Name, Text: text, Class: "o-" + res.Outcome})
			}
		}
		for _, o := range []string{"synced", "fetched", "would", "current", "excluded", "skipped", "failed"} {
			if counts[o] > 0 {
				vm.Pills = append(vm.Pills, pillVM{Label: o, N: counts[o], Class: "o-" + o})
			}
		}
		sort.SliceStable(vm.Items, func(i, j int) bool { return outcomeRank(vm.Items[i].Class) < outcomeRank(vm.Items[j].Class) })
		d.Runs = append(d.Runs, vm)
	}
	s.page(w, "activity", d)
}

func outcomeRank(class string) int {
	switch class {
	case "o-failed":
		return 0
	case "o-skipped":
		return 1
	}
	return 2
}

var secretLine = regexp.MustCompile(`(?m)^(\s*[#]?\s*(?:token_cmd|token_env|token|password)\s*:\s*)\S.*$`)

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	d := s.base("settings", "Settings")
	s.mu.Unlock()
	d.Sizes = []sizeVM{{"Small", 14}, {"Medium", 16}, {"Large", 18}, {"Extra large", 20}}
	d.DefaultSize = s.cfg.Web.TextSize
	if s.cfg.Path != "" {
		d.ConfigPath = paths.Tilde(s.cfg.Path)
		if data, err := os.ReadFile(s.cfg.Path); err == nil {
			d.ConfigText = secretLine.ReplaceAllString(string(data), "${1}•••••• (hidden)")
		}
	}
	s.page(w, "settings", d)
}

// action starts a sync, fetch or status refresh: on every repo, a
// workspace, one repo, or the repos the given filter shows.
func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	act := r.PathValue("action")
	if act != "sync" && act != "fetch" && act != "refresh" {
		http.Error(w, "unknown action "+act, http.StatusNotFound)
		return
	}
	var targets []model.Repo
	s.mu.Lock()
	if act == "refresh" {
		s.rediscoverLocked()
	}
	switch {
	case act == "refresh" || r.Form.Get("scope") == "all":
		targets = append(targets, s.repos...)
	case r.Form.Get("repo") != "":
		if path, ok := s.byID[r.Form.Get("repo")]; ok {
			targets = append(targets, s.results[path].Repo)
		}
	case r.Form.Get("ws") != "":
		for _, repo := range s.repos {
			if repo.Workspace == r.Form.Get("ws") {
				targets = append(targets, repo)
			}
		}
	case r.Form.Get("scope") == "shown":
		f, err := s.parseQuery(r.Form.Get("q"))
		if err != nil {
			s.mu.Unlock()
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, repo := range s.repos {
			if f.Match(repo, s.results[repo.Path].Status) {
				targets = append(targets, repo)
			}
		}
	}
	s.mu.Unlock()
	if len(targets) == 0 {
		http.Error(w, "no repos to "+act, http.StatusBadRequest)
		return
	}
	if err := s.start(act, targets, false); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "%s: %d repos\n", act, len(targets))
}

// Operations.

var errBusy = errors.New("busy")

func (s *Server) start(act string, repos []model.Repo, fetch bool) error {
	op := map[string]engine.Op{"sync": engine.OpSync, "fetch": engine.OpFetch, "refresh": engine.OpStatus, "status": engine.OpStatus}[act]
	label := map[string]string{"sync": "syncing", "fetch": "fetching", "refresh": "reading", "status": "reading"}[act]
	s.mu.Lock()
	if s.busy != "" {
		busy := s.busy
		s.mu.Unlock()
		return fmt.Errorf("%w: %s is still running", errBusy, busy)
	}
	s.busy, s.progress = label, [2]int{0, len(repos)}
	for _, r := range repos {
		s.running[r.Path] = true
	}
	s.mu.Unlock()

	err := s.run.Start(s.ctx, op, repos, fetch, s.onResult, func(results []model.Result) { s.finish(op, results) })
	if err != nil {
		s.mu.Lock()
		s.busy = ""
		for _, r := range repos {
			delete(s.running, r.Path)
		}
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	busy := s.busyLocked(false)
	rows := make([]map[string]string, 0, len(repos))
	for _, r := range repos {
		vm := s.rowLocked(s.results[r.Path])
		rows = append(rows, map[string]string{"id": vm.ID, "row": s.render("row", vm), "card": s.render("card", vm)})
	}
	panels := s.panelsLocked()
	s.mu.Unlock()
	s.hub.publish("busy", busy)
	for _, row := range rows {
		s.hub.publish("row", row)
	}
	s.hub.publish("panels", panels)
	return nil
}

func (s *Server) onResult(res model.Result) {
	s.mu.Lock()
	delete(s.running, res.Repo.Path)
	s.results[res.Repo.Path] = res
	s.progress[0]++
	vm := s.rowLocked(res)
	row := map[string]string{"id": vm.ID, "row": s.render("row", vm), "card": s.render("card", vm)}
	busy := s.busyLocked(false)
	panels := s.panelsLocked()
	s.mu.Unlock()
	s.hub.publish("row", row)
	s.hub.publish("busy", busy)
	s.hub.publish("panels", panels)
}

func (s *Server) finish(op engine.Op, results []model.Result) {
	s.mu.Lock()
	s.busy = ""
	if op == engine.OpSync {
		s.lastSync = time.Now()
	}
	s.cacheSeen = modTime(s.opts.CachePath)
	n := map[model.Outcome]int{}
	for _, r := range results {
		n[r.Outcome]++
	}
	busy := s.busyLocked(true)
	panels := s.panelsLocked()
	s.mu.Unlock()

	s.hub.publish("busy", busy)
	s.hub.publish("panels", panels)
	s.refreshForge(results)
	if op == engine.OpStatus {
		return
	}
	var parts []string
	for _, o := range []model.Outcome{model.OutcomeSynced, model.OutcomeFetched, model.OutcomeCurrent, model.OutcomeExcluded, model.OutcomeSkipped, model.OutcomeFailed} {
		if n[o] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n[o], o))
		}
	}
	attention := n[model.OutcomeSkipped]+n[model.OutcomeFailed] > 0
	msg := fmt.Sprintf("%s %s: %s", op, plural(len(results), "repo", "repos"), strings.Join(parts, ", "))
	if attention {
		msg += " - see Needs attention"
	}
	s.hub.publish("toast", map[string]any{"msg": msg, "warn": attention})
}

// refreshForge asks the forges about the repos an operation just read, in
// the background, and pushes each changed row.
func (s *Server) refreshForge(results []model.Result) {
	fc := s.opts.Forge
	if fc == nil || !fc.Any() || !s.forgeBusy.CompareAndSwap(false, true) {
		return
	}
	targets := make([]forgeapi.Target, 0, len(results))
	for _, r := range results {
		targets = append(targets, forgeTarget(r))
	}
	go func() {
		defer s.forgeBusy.Store(false)
		fc.Refresh(s.ctx, targets, s.cfg.Concurrency, func(path string, i forgeapi.Info) {
			s.mu.Lock()
			s.forge[path] = i
			vm := s.rowLocked(s.results[path])
			row := map[string]string{"id": vm.ID, "row": s.render("row", vm), "card": s.render("card", vm)}
			s.mu.Unlock()
			s.hub.publish("row", row)
		})
	}()
}

func forgeTarget(r model.Result) forgeapi.Target {
	return forgeapi.Target{Path: r.Repo.Path, RemoteURL: r.Status.RemoteURL, DefaultBranch: r.Status.DefaultBranch}
}

// loadCache fills results from the status cache, keeping last outcomes.
func (s *Server) loadCache() {
	cache := store.LoadCache(s.opts.CachePath)
	for _, r := range s.repos {
		res := s.results[r.Path]
		res.Repo = r
		if st, ok := cache.Repos[r.Path]; ok {
			res.Status = st
		}
		s.results[r.Path] = res
	}
	s.cacheSeen = modTime(s.opts.CachePath)
}

// watch notices other mrm processes: a sync lock held by another process
// shows in the top bar, and when another run rewrites the status cache the
// page reloads its rows from it.
func (s *Server) watch() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
		}
		s.mu.Lock()
		ours := s.busy != ""
		wss := runner.Workspaces(s.repos)
		s.mu.Unlock()
		ext := ""
		if !ours {
			for _, ws := range wss {
				if pid, held := engine.LockHolder(s.opts.LockDir, ws); held {
					ext = "sync running in another mrm (pid " + pid + ")"
					break
				}
			}
		}
		mod := modTime(s.opts.CachePath)
		s.mu.Lock()
		changed := ext != s.external
		s.external = ext
		reload := !ours && mod.After(s.cacheSeen)
		if reload {
			s.loadCache()
			if run, ok := store.LastRun(s.opts.HistPath, "sync"); ok {
				s.lastSync = run.Started.Add(time.Duration(run.DurationMS) * time.Millisecond)
			}
		}
		busy := s.busyLocked(reload)
		var panels map[string]string
		if changed || reload {
			panels = s.panelsLocked()
		}
		s.mu.Unlock()
		if changed || reload {
			s.hub.publish("busy", busy)
			s.hub.publish("panels", panels)
		}
		if reload {
			s.hub.publish("reload", map[string]bool{"reload": true})
		}
	}
}

func modTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
