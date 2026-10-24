package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/logger"
	"github.com/adegoodyer/mrm/internal/store"
	"go.uber.org/zap"
)

var (
	flagPruneDry   bool
	flagPruneForce bool
	flagPruneYes   bool
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete local branches already merged, after confirmation",
	Long: `List local branches that are merged into the remote default branch, or
whose upstream is gone, and delete the merged ones after you confirm.

A gone-upstream branch that isn't merged (often a squash-merged one) may hold
commits that exist nowhere else: it is listed and kept unless --force. The
current branch and the default branch are never touched, and "merged" is
checked again just before each delete. Fetch first (mrm fetch) so "gone"
and "merged" are up to date.`,
	Example: `  mrm prune --dry-run
  mrm prune -w work
  mrm prune -f "is:stale-branches" --force`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPrune(cmd.Context())
	},
}

func init() {
	f := pruneCmd.Flags()
	f.BoolVarP(&flagPruneDry, "dry-run", "n", false, "list what would be deleted; delete nothing")
	f.BoolVar(&flagPruneForce, "force", false, "also delete gone-upstream branches that aren't merged")
	f.BoolVarP(&flagPruneYes, "yes", "y", false, "don't ask first")
	rootCmd.AddCommand(pruneCmd)
}

func runPrune(ctx context.Context) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	repos, _, err := a.repos(ctx)
	if err != nil {
		return err
	}
	s := a.style
	cands := a.eng.PruneCandidates(ctx, repos)
	if len(cands) == 0 {
		fmt.Println("Nothing to prune: no merged or gone-upstream branches in " + repoCount(len(repos)) + ".")
		return nil
	}

	var del []engine.PruneCandidate
	last := ""
	for _, c := range cands {
		if c.Repo.Path != last {
			last = c.Repo.Path
			fmt.Println(s.Bold(c.Repo.Display()))
		}
		mark, note := s.Green("✓"), s.Dim(c.Reason())
		switch {
		case c.Merged || flagPruneForce:
			del = append(del, c)
			if !c.Merged {
				mark, note = s.Red("✗"), s.Red(c.Reason()+" - deleting (--force)")
			}
		default:
			mark, note = s.Yellow("!"), s.Yellow(c.Reason()+" - kept (--force deletes it)")
		}
		fmt.Printf("  %s %s  %s\n", mark, c.Branch, note)
	}
	repoSet := map[string]bool{}
	for _, c := range del {
		repoSet[c.Repo.Path] = true
	}
	fmt.Println()
	if len(del) == 0 {
		fmt.Println("Nothing merged to delete.")
		return nil
	}
	if flagPruneDry {
		fmt.Printf("Would delete %d branches in %s.\n", len(del), repoCount(len(repoSet)))
		return nil
	}
	if err := confirm(flagPruneYes, fmt.Sprintf("Delete %d branches in %s?", len(del), repoCount(len(repoSet)))); err != nil {
		return err
	}
	started := time.Now()
	deleted, failed := 0, 0
	var hist []store.RunResult
	for _, c := range del {
		if err := a.eng.Prune(ctx, c, flagPruneForce); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "%s %s %s: %v\n", s.Red("✗"), c.Repo.Display(), c.Branch, err)
			hist = append(hist, store.RepoResult(c.Repo, store.OutcomeFailed, c.Branch+": "+err.Error()))
			continue
		}
		deleted++
		logger.L.Info("branch deleted", zap.String("repo", c.Repo.Path), zap.String("branch", c.Branch), zap.Bool("merged", c.Merged))
		hist = append(hist, store.RepoResult(c.Repo, store.OutcomeDeleted, "", "deleted branch "+c.Branch+" ("+c.Reason()+")"))
	}
	detail := ""
	if flagPruneForce {
		detail = "--force"
	}
	record(store.NewEvent(store.OpPrune, detail, started, hist))
	fmt.Printf("Deleted %d branches", deleted)
	if failed > 0 {
		fmt.Printf(", %d kept", failed)
		fmt.Println()
		return &exitError{code: 2}
	}
	fmt.Println(".")
	return nil
}
