package fleet

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/adegoodyer/mrm/internal/model"
)

func repos(t *testing.T, names ...string) []model.Repo {
	var out []model.Repo
	for _, n := range names {
		out = append(out, model.Repo{Name: n, Path: t.TempDir()})
	}
	return out
}

// 4.1: parallel, every line prefixed, nothing interleaved mid-line.
func TestExecParallelPrefixes(t *testing.T) {
	var out, errs bytes.Buffer
	rs := repos(t, "api", "web", "docs")
	res := Exec(context.Background(), rs, Options{
		Argv:        []string{"sh", "-c", "for i in 1 2 3; do echo line$i; done; echo oops >&2; printf tail"},
		Concurrency: 3,
		Stdout:      &out, Stderr: &errs,
		Label: func(r model.Repo) string { return r.Name + " | " },
	})
	for _, r := range res {
		if !r.OK() {
			t.Fatalf("%s: %+v", r.Repo.Name, r)
		}
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 12 { // 3 lines + an unterminated "tail", per repo
		t.Fatalf("%d lines:\n%s", len(lines), out.String())
	}
	for _, l := range lines {
		if !(strings.HasPrefix(l, "api | ") || strings.HasPrefix(l, "web | ") || strings.HasPrefix(l, "docs | ")) {
			t.Fatalf("unprefixed line %q", l)
		}
	}
	if strings.Count(errs.String(), "| oops") != 3 {
		t.Fatalf("stderr:\n%s", errs.String())
	}
}

// Failures keep their exit code; a missing command is an error, not a panic.
func TestExecFailures(t *testing.T) {
	var out bytes.Buffer
	res := Exec(context.Background(), repos(t, "a"), Options{Argv: []string{"sh", "-c", "exit 7"}, Concurrency: 1, Stdout: &out, Stderr: &out})
	if res[0].Code != 7 || res[0].OK() {
		t.Fatalf("%+v", res[0])
	}
	res = Exec(context.Background(), repos(t, "a"), Options{Argv: []string{"no-such-command-mrm"}, Concurrency: 1, Stdout: &out, Stderr: &out})
	if res[0].Err == nil || res[0].Code != -1 {
		t.Fatalf("%+v", res[0])
	}
}

// The command runs in each repo's folder.
func TestExecRunsInRepo(t *testing.T) {
	var out bytes.Buffer
	rs := repos(t, "a")
	Exec(context.Background(), rs, Options{Argv: []string{"pwd"}, Concurrency: 1, Stdout: &out, Stderr: &out, Serial: true,
		Header: func(r model.Repo) string { return "== " + r.Name }})
	if !strings.Contains(out.String(), "== a\n"+rs[0].Path) {
		t.Fatalf("%q", out.String())
	}
}

// Cancelling stops commands still running.
func TestExecCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	var out bytes.Buffer
	res := Exec(ctx, repos(t, "a", "b"), Options{Argv: []string{"sleep", "10"}, Concurrency: 2, Stdout: &out, Stderr: &out})
	if time.Since(start) > 5*time.Second || res[0].OK() {
		t.Fatalf("took %s, %+v", time.Since(start), res[0])
	}
}
