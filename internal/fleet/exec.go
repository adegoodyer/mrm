// Package fleet runs a user's command in many repos: in parallel with each
// output line prefixed by the repo, or one repo at a time on the terminal.
// It runs the command itself, never through a shell - pass `sh -c '...'`
// for one.
package fleet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/adegoodyer/mrm/internal/model"
)

// Options for Exec.
type Options struct {
	Argv        []string
	Concurrency int
	Serial      bool // one repo at a time, with the terminal attached
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	// Label renders a repo's prefix ("api │ "); Header the line before a
	// repo in serial mode.
	Label  func(model.Repo) string
	Header func(model.Repo) string
}

// Result of the command in one repo.
type Result struct {
	Repo     model.Repo
	Code     int   // exit code; -1 when it couldn't start
	Err      error // start or wait error other than a non-zero exit
	Duration time.Duration
}

// OK reports whether the command ran and exited 0.
func (r Result) OK() bool { return r.Err == nil && r.Code == 0 }

// Exec runs opts.Argv in every repo and returns the results in input order.
// Cancelling ctx kills the commands still running.
func Exec(ctx context.Context, repos []model.Repo, opts Options) []Result {
	out := make([]Result, len(repos))
	if opts.Serial {
		for i, r := range repos {
			if ctx.Err() != nil {
				out[i] = Result{Repo: r, Code: -1, Err: ctx.Err()}
				continue
			}
			if opts.Header != nil {
				fmt.Fprintln(opts.Stdout, opts.Header(r))
			}
			out[i] = run(ctx, r, opts.Argv, opts.Stdin, opts.Stdout, opts.Stderr)
		}
		return out
	}

	var mu sync.Mutex // one line at a time on the shared outputs
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < max(opts.Concurrency, 1) && w < len(repos); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				r := repos[i]
				label := ""
				if opts.Label != nil {
					label = opts.Label(r)
				}
				so := &lineWriter{mu: &mu, w: opts.Stdout, prefix: label}
				se := &lineWriter{mu: &mu, w: opts.Stderr, prefix: label}
				out[i] = run(ctx, r, opts.Argv, nil, so, se)
				so.Flush()
				se.Flush()
			}
		}()
	}
	for i := range repos {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

func run(ctx context.Context, r model.Repo, argv []string, stdin io.Reader, stdout, stderr io.Writer) Result {
	start := time.Now()
	res := Result{Repo: r}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = r.Path
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	res.Duration = time.Since(start)
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		res.Code = ee.ExitCode()
	default:
		res.Code, res.Err = -1, err
	}
	return res
}

// lineWriter prefixes every complete line and writes it under a shared
// lock, so lines from repos running at once never interleave mid-line.
type lineWriter struct {
	mu     *sync.Mutex
	w      io.Writer
	prefix string
	buf    []byte
}

func (l *lineWriter) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		l.emit(l.buf[:i+1])
		l.buf = l.buf[i+1:]
	}
}

// Flush writes a last line that had no newline.
func (l *lineWriter) Flush() {
	if len(l.buf) > 0 {
		l.emit(append(l.buf, '\n'))
		l.buf = nil
	}
}

func (l *lineWriter) emit(line []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = io.WriteString(l.w, l.prefix)
	_, _ = l.w.Write(line)
}
