package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/fleet"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/term"
)

var (
	flagExecSerial bool
	flagExecYes    bool
)

var execCmd = &cobra.Command{
	Use:   "exec [flags] -- <command> [args...]",
	Short: "Run a command in every matching repo",
	Long: `Run a command in every matching repo, in parallel, each output line
prefixed by the repo. The command runs directly, not through a shell - use
sh -c '...' for pipes and globs. --serial runs one repo at a time with the
terminal attached, for interactive commands.

Asks before running on a terminal (--yes skips it; scripts need --yes).
Exit code 0 when it succeeded everywhere, 2 when it failed in any repo.`,
	Example: `  mrm exec -t terraform -- terraform fmt -check
  mrm exec -f "is:dirty" -- git status --short
  mrm exec -w work --serial -- lazygit
  mrm exec --yes -- sh -c 'git log -1 --format=%cs'`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExec(cmd.Context(), args)
	},
}

func init() {
	execCmd.Flags().BoolVar(&flagExecSerial, "serial", false, "one repo at a time, with the terminal attached")
	execCmd.Flags().BoolVarP(&flagExecYes, "yes", "y", false, "don't ask first")
	rootCmd.AddCommand(execCmd)
}

func runExec(ctx context.Context, argv []string) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	repos, _, err := a.repos(ctx)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repos match")
	}
	if err := confirm(flagExecYes, fmt.Sprintf("Run `%s` in %s?", strings.Join(argv, " "), repoCount(len(repos)))); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	nameW := 0
	for _, r := range repos {
		nameW = max(nameW, min(len(r.Display()), nameCap()))
	}
	s := a.style
	started := time.Now()
	results := fleet.Exec(ctx, repos, fleet.Options{
		Argv:        argv,
		Concurrency: a.cfg.Concurrency,
		Serial:      flagExecSerial,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Label:       func(r model.Repo) string { return s.Cyan(term.Pad(r.Display(), nameW)) + s.Dim(" │ ") },
		Header:      func(r model.Repo) string { return "\n" + s.Bold("── "+r.Display()+" ") + s.Dim(r.Path) },
	})

	var failed []string
	for _, r := range results {
		if !r.OK() {
			why := fmt.Sprintf("exit %d", r.Code)
			if r.Err != nil {
				why = r.Err.Error()
			}
			failed = append(failed, r.Repo.Display()+" ("+why+")")
		}
	}
	ok := len(results) - len(failed)
	fmt.Fprintf(os.Stderr, "\nRan in %s in %s: %d ok", repoCount(len(results)), time.Since(started).Round(100*time.Millisecond), ok)
	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, ", %s\n", s.Red(fmt.Sprintf("%d failed: %s", len(failed), strings.Join(failed, ", "))))
		return &exitError{code: 2}
	}
	fmt.Fprintln(os.Stderr)
	return nil
}
