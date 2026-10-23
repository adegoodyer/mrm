// Package engine is the only thing in mrm that runs git. Front ends hand it
// repos and an operation; it works through them with a bounded worker pool
// and streams an event per repo as it starts and finishes.
package engine

import (
	"context"
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/adegoodyer/mrm/internal/git"
	"github.com/adegoodyer/mrm/internal/model"
)

// Op is an operation on repos.
type Op string

const (
	OpStatus Op = "status"
	OpFetch  Op = "fetch"
	OpSync   Op = "sync"
)

// Options for a run.
type Options struct {
	Op     Op
	DryRun bool // sync: say what would happen, change nothing
	Fetch  bool // status: fetch first
	// Last is each repo's previous status, by path (from the status
	// cache). A status read keeps its fetch error, and fetches start
	// with the repos that were slowest last time.
	Last map[string]model.Status
}

// EventKind says what happened.
type EventKind string

const (
	EventStarted EventKind = "started"
	EventDone    EventKind = "done"
)

// Event reports progress on one repo.
type Event struct {
	Kind   EventKind
	Index  int // position in the input slice
	Total  int
	Repo   model.Repo
	Result model.Result // set on EventDone
}

// RemoteHeadMaxAge is how often a fetch re-checks the remote's default
// branch (tests shorten it).
var RemoteHeadMaxAge = 24 * time.Hour

// Engine runs operations.
type Engine struct {
	Git         *git.Runner
	Concurrency int
	Hooks       []Hook // run after a sync moves a repo's checked-out branch
}

// New returns an engine.
func New(timeout time.Duration, concurrency int) *Engine {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Engine{Git: &git.Runner{Timeout: timeout}, Concurrency: concurrency}
}

// Run works through repos and streams events. The channel closes when every
// repo is done or ctx is cancelled.
func (e *Engine) Run(ctx context.Context, repos []model.Repo, opts Options) <-chan Event {
	events := make(chan Event, len(repos)*2+1)
	go func() {
		defer close(events)
		jobs := make(chan int)
		var wg sync.WaitGroup
		workers := e.Concurrency
		if opts.Op == OpStatus && !opts.Fetch {
			// Status is local and CPU-bound, not network-bound.
			workers = max(workers, 2*runtime.NumCPU())
		}
		for w := 0; w < workers && w < len(repos); w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					r := repos[i]
					events <- Event{Kind: EventStarted, Index: i, Total: len(repos), Repo: r}
					start := time.Now()
					res := e.one(ctx, r, opts)
					res.Duration = time.Since(start)
					stampFetch(&res, opts.Last)
					events <- Event{Kind: EventDone, Index: i, Total: len(repos), Repo: r, Result: res}
				}
			}()
		}
		order := make([]int, len(repos))
		for i := range order {
			order[i] = i
		}
		if opts.Op != OpStatus || opts.Fetch {
			slowestFirst(order, repos, opts.Last)
		}
	feed:
		for _, i := range order {
			select {
			case jobs <- i:
			case <-ctx.Done():
				break feed
			}
		}
		close(jobs)
		wg.Wait()
	}()
	return events
}

// slowestFirst orders fetches by how long each repo's last fetch took,
// longest first, so a slow host overlaps the quick ones instead of
// finishing alone at the end. Repos never fetched go first: they may be
// slow too.
func slowestFirst(order []int, repos []model.Repo, last map[string]model.Status) {
	took := func(i int) int64 {
		if st, ok := last[repos[i].Path]; ok && st.FetchMS > 0 {
			return st.FetchMS
		}
		return math.MaxInt64
	}
	sort.SliceStable(order, func(a, b int) bool { return took(order[a]) > took(order[b]) })
}

// stampFetch records how the fetch went in the status, or carries the last
// fetch's error and time over when this operation didn't fetch.
func stampFetch(res *model.Result, last map[string]model.Status) {
	if f := res.Fetch; f != nil {
		res.Status.FetchError, res.Status.FetchMS = f.Err, f.Took.Milliseconds()
		return
	}
	if prev, ok := last[res.Repo.Path]; ok {
		res.Status.FetchError, res.Status.FetchMS = prev.FetchError, prev.FetchMS
	}
}

// Collect runs and returns the results in input order.
func (e *Engine) Collect(ctx context.Context, repos []model.Repo, opts Options) []model.Result {
	out := make([]model.Result, len(repos))
	for ev := range e.Run(ctx, repos, opts) {
		if ev.Kind == EventDone {
			out[ev.Index] = ev.Result
		}
	}
	return out
}

func (e *Engine) one(ctx context.Context, r model.Repo, opts Options) model.Result {
	switch opts.Op {
	case OpFetch:
		return e.fetch(ctx, r)
	case OpSync:
		return e.sync(ctx, r, opts.DryRun)
	default:
		if opts.Fetch && !r.NoSync {
			if res := e.fetch(ctx, r); res.Outcome == model.OutcomeFailed {
				return res
			}
		}
		return model.Result{Repo: r, Status: e.status(ctx, r)}
	}
}

func (e *Engine) status(ctx context.Context, r model.Repo) model.Status {
	return e.Git.ReadStatus(ctx, r.Path, remote(r), r.Branch)
}

func remote(r model.Repo) string {
	if r.Remote == "" {
		return "origin"
	}
	return r.Remote
}

func (e *Engine) fetch(ctx context.Context, r model.Repo) model.Result {
	st := e.status(ctx, r)
	res := model.Result{Repo: r, Status: st}
	switch {
	case st.Error != "":
		res.Outcome, res.Reason = model.OutcomeFailed, st.Error
		return res
	case !st.HasRemote:
		res.Outcome, res.Reason = model.OutcomeExcluded, "no remote"
		return res
	}
	start := time.Now()
	err := e.Git.Fetch(ctx, r.Path, remote(r))
	res.Fetch = &model.FetchAttempt{Took: time.Since(start)}
	if err != nil {
		res.Fetch.Err = err.Error()
		res.Outcome, res.Reason = model.OutcomeFailed, "fetch: "+err.Error()
		res.Status = e.status(ctx, r)
		return res
	}
	// Refresh <remote>/HEAD at most once a day: it is a second network round
	// trip per repo. Not with a branch override - the default comes from
	// config then. A failure here is not worth failing the fetch for.
	if r.Branch == "" {
		if age, ok := git.RemoteHeadAge(st.GitCommonDir, remote(r)); !ok || age > RemoteHeadMaxAge {
			_ = e.Git.RefreshRemoteHead(ctx, r.Path, st.GitCommonDir, remote(r))
		}
	}
	res.Outcome = model.OutcomeFetched
	res.Status = e.status(ctx, r)
	return res
}
