package cmd

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/forgeapi"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/store"
	"github.com/adegoodyer/mrm/internal/term"
)

var (
	flagStatusFetch bool
	flagNoForge     bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "One line per repo: state, branch, ahead/behind, changes, age, flags",
	Example: `  mrm status
  mrm status -w work --fetch
  mrm status -f "is:dirty"
  mrm status --json | jq '.[] | select(.state != "clean") | .name'`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(cmd.Context(), flagStatusFetch)
	},
}

func init() {
	statusCmd.Flags().BoolVar(&flagStatusFetch, "fetch", false, "fetch every repo first")
	statusCmd.Flags().BoolVar(&flagNoForge, "no-prs", false, "skip asking GitHub, GitLab and the rest for open PRs and CI")
	statusCmd.Flags().BoolVar(&flagNoForge, "no-forge", false, "the name before 0.3.0 for --no-prs")
	_ = statusCmd.Flags().MarkHidden("no-forge")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(ctx context.Context, fetch bool) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	repos, _, err := a.repos(ctx)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repos found under %v", rootsOf(a))
	}
	verb := "reading"
	if fetch {
		verb = "fetching"
	}
	prog := term.NewProgress(os.Stdout, os.Stderr, !flagJSON)
	results := make([]model.Result, len(repos))
	done := 0
	last := store.LoadCache(paths.StatusCache()).Repos
	for ev := range a.eng.Run(ctx, repos, engine.Options{Op: engine.OpStatus, Fetch: fetch, Last: last}) {
		if ev.Kind == engine.EventDone {
			results[ev.Index] = ev.Result
			done++
			prog.Status(fmt.Sprintf("%s %d/%d", verb, done, len(repos)))
		}
	}
	_ = store.UpdateCache(paths.StatusCache(), results)
	a.loadForge(ctx, results, prog)
	prog.Done()
	if flagJSON {
		return a.printJSON(results)
	}
	a.printStatus(os.Stdout, results)
	return nil
}

func rootsOf(a *app) []string {
	var roots []string
	for _, ws := range a.workspaces {
		for _, r := range ws.Roots {
			roots = append(roots, paths.Tilde(r))
		}
	}
	return roots
}

// loadForge asks the git hosts that have a token about open PRs and CI
// (cached), unless --no-prs.
func (a *app) loadForge(ctx context.Context, results []model.Result, prog *term.Progress) {
	fc := forgeapi.New(a.cfg, paths.ForgeCache())
	if !fc.Any() || flagNoForge {
		return
	}
	var targets []forgeapi.Target
	for _, r := range results {
		targets = append(targets, forgeapi.Target{Path: r.Repo.Path, RemoteURL: r.Status.RemoteURL, DefaultBranch: r.Status.DefaultBranch})
	}
	var mu sync.Mutex
	a.forge = map[string]forgeapi.Info{}
	n := 0
	fc.Refresh(ctx, targets, a.cfg.Concurrency, func(path string, i forgeapi.Info) {
		mu.Lock()
		a.forge[path] = i
		n++
		if prog != nil {
			prog.Status(fmt.Sprintf("asking for PRs and CI %d", n))
		}
		mu.Unlock()
	})
}
