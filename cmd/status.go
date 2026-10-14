package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/store"
	"github.com/adegoodyer/mrm/internal/term"
)

var flagStatusFetch bool

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
	for ev := range a.eng.Run(ctx, repos, engine.Options{Op: engine.OpStatus, Fetch: fetch}) {
		if ev.Kind == engine.EventDone {
			results[ev.Index] = ev.Result
			done++
			prog.Status(fmt.Sprintf("%s %d/%d", verb, done, len(repos)))
		}
	}
	prog.Done()
	_ = store.UpdateCache(paths.StatusCache(), results)
	if flagJSON {
		return printJSON(results)
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
