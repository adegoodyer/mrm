// Package engine is the only thing in mrm that runs git. Front ends hand it
// repos and an operation; it works through them with a bounded worker pool
// and streams an event per repo as it starts and finishes.
package engine

import (
	"context"
	"runtime"
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

// Engine runs operations.
type Engine struct {
	Git         *git.Runner
	Concurrency int
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
					events <- Event{Kind: EventDone, Index: i, Total: len(repos), Repo: r, Result: res}
				}
			}()
		}
	feed:
		for i := range repos {
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
	if err := e.Git.Fetch(ctx, r.Path, remote(r)); err != nil {
		res.Outcome, res.Reason = model.OutcomeFailed, "fetch: "+err.Error()
		res.Status = e.status(ctx, r)
		return res
	}
	// Not with a branch override: the default comes from config then. A
	// failure here is not worth failing the fetch for.
	if r.Branch == "" {
		_ = e.Git.RefreshRemoteHead(ctx, r.Path, remote(r))
	}
	res.Outcome = model.OutcomeFetched
	res.Status = e.status(ctx, r)
	return res
}
