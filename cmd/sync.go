package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/logger"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/notify"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/store"
	"github.com/adegoodyer/mrm/internal/term"
)

var flagDryRun bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch every repo and fast-forward what is safe to",
	Long: `Fetch every repo, then fast-forward the default branch where that is
safe: the tree is clean and the branch is strictly behind. On another
branch, the default branch is updated in place and the current branch is
left alone. mrm never stashes, resets, rebases, merges or forces.

Exit code 0: every repo synced or already current. 2: some repos were
skipped or failed and need attention. 1: mrm itself failed.`,
	Example: `  mrm sync -w work
  mrm sync --dry-run
  mrm sync -t terraform`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSync(cmd.Context(), engine.OpSync, flagDryRun)
	},
}

var fetchCmd = &cobra.Command{
	Use:     "fetch",
	Short:   "Fetch every repo (no fast-forward)",
	Example: "  mrm fetch -w work",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSync(cmd.Context(), engine.OpFetch, false)
	},
}

func init() {
	syncCmd.Flags().BoolVarP(&flagDryRun, "dry-run", "n", false, "say what would change; change nothing")
	rootCmd.AddCommand(syncCmd, fetchCmd)
}

func runSync(ctx context.Context, op engine.Op, dryRun bool) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	if !dryRun {
		lock, err := engine.LockWorkspaces(paths.LockDir(), a.workspaceNames())
		if err != nil {
			return err
		}
		defer lock.Release()
	}
	repos, _, err := a.repos(ctx)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return noRepos(a)
	}

	started := time.Now()
	logger.L.Info("run started", zap.String("op", string(op)), zap.Bool("dry_run", dryRun),
		zap.Strings("workspaces", a.workspaceNames()), zap.Int("repos", len(repos)))

	nameW := 0
	for _, r := range repos {
		nameW = max(nameW, min(len(r.Display()), nameCap()))
	}
	verb := map[engine.Op]string{engine.OpSync: "syncing", engine.OpFetch: "fetching"}[op]
	if dryRun {
		verb = "checking"
	}
	prog := term.NewProgress(os.Stdout, os.Stderr, !flagJSON)
	results := make([]model.Result, len(repos))
	running := map[string]bool{}
	done := 0
	last := store.LoadCache(paths.StatusCache()).Repos
	for ev := range a.eng.Run(ctx, repos, engine.Options{Op: op, DryRun: dryRun, Last: last}) {
		switch ev.Kind {
		case engine.EventStarted:
			running[ev.Repo.Display()] = true
		case engine.EventDone:
			delete(running, ev.Repo.Display())
			results[ev.Index] = ev.Result
			done++
			if !flagJSON {
				prog.Println(a.outcomeLine(ev.Result, nameW))
			}
			if ev.Result.Outcome == model.OutcomeFailed {
				logger.L.Warn("repo failed", zap.String("repo", ev.Repo.Path), zap.String("reason", ev.Result.Reason))
			}
		}
		prog.Status(fmt.Sprintf("%s %d/%d  %s", verb, done, len(repos), names(running)))
	}
	prog.Done()
	took := time.Since(started)

	_ = store.UpdateCache(paths.StatusCache(), results)
	if err := store.AppendRun(paths.History(), store.NewRun(string(op), dryRun, started, a.workspaceNames(), results)); err != nil {
		logger.L.Warn("history not written", zap.Error(err))
	}
	if op == engine.OpSync && !dryRun {
		if err := a.notify(ctx, results); err != nil {
			fmt.Fprintln(os.Stderr, a.style.Yellow("warning: ")+"notify: "+err.Error())
			logger.L.Warn("notify failed", zap.Error(err))
		}
	}
	logger.L.Info("run finished", zap.String("op", string(op)), zap.Duration("took", took))

	if flagJSON {
		if err := a.printJSON(results); err != nil {
			return err
		}
	} else {
		label := map[engine.Op]string{engine.OpSync: "Synced", engine.OpFetch: "Fetched"}[op]
		if dryRun {
			label = "Checked"
		}
		a.printSummary(os.Stdout, label, results, took)
	}
	for _, r := range results {
		if r.Outcome.NeedsAttention() {
			return &exitError{code: 2}
		}
	}
	return nil
}

// names lists up to three running repos for the progress line.
func names(running map[string]bool) string {
	var n []string
	for k := range running {
		n = append(n, k)
	}
	sort.Strings(n)
	if len(n) > 3 {
		return strings.Join(n[:3], ", ") + fmt.Sprintf(" +%d", len(n)-3)
	}
	return strings.Join(n, ", ")
}

// notify sends the run summary per notify.on.
func (a *app) notify(ctx context.Context, results []model.Result) error {
	n := a.cfg.Notify
	if n.On == "never" || (n.Gotify == nil && n.Ntfy == nil) {
		return nil
	}
	counts := map[model.Outcome]int{}
	var attention []string
	for _, r := range results {
		counts[r.Outcome]++
		if r.Outcome.NeedsAttention() {
			attention = append(attention, fmt.Sprintf("%s %s - %s", r.Repo.Display(), r.Outcome, r.Reason))
		}
	}
	if n.On == "attention" && len(attention) == 0 {
		return nil
	}
	body := fmt.Sprintf("%s: %d synced, %d current, %d skipped, %d failed",
		repoCount(len(results)), counts[model.OutcomeSynced], counts[model.OutcomeCurrent],
		counts[model.OutcomeSkipped], counts[model.OutcomeFailed])
	if len(attention) > 0 {
		body += "\n\n" + strings.Join(attention, "\n")
	}
	prio := 2
	if len(attention) > 0 {
		prio = 5
	}
	return notify.Send(ctx, n, notify.Message{
		Title:    "mrm sync: " + strings.Join(a.workspaceNames(), ", "),
		Body:     body,
		Priority: prio,
	})
}
