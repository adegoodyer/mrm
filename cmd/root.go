// Package cmd is mrm's command line.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	flagWorkspaces []string
	flagTags       []string
	flagFilter     string
	flagJSON       bool
	flagNoColor    bool
	flagConfig     string
	flagVerbose    bool

	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

// SetBuildInfo injects version metadata from main (GoReleaser ldflags).
func SetBuildInfo(version, commit, date string) {
	buildVersion, buildCommit, buildDate = version, commit, date
}

var rootCmd = &cobra.Command{
	Use:   "mrm",
	Short: "Mass repo manager - status and safe sync for every git repo you have",
	Long: `mrm finds every git repo under your workspace roots, keeps them current
safely and in parallel (fetch + fast-forward only, never touching work in
progress), and shows the state of all of them at once.`,
	Example: `  mrm status -w work            # every repo in the work workspace
  mrm status -f "is:dirty"      # only repos with uncommitted changes
  mrm sync -w work              # fetch + fast-forward what is safe to
  mrm sync --dry-run            # say what sync would do
  mrm open api --pr             # open api's pull requests
  mrm config init               # write a commented config`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// The TUI lands in phase 2; until then the bare command is status.
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(cmd.Context(), false)
	},
}

// exitError carries an exit code; err may be nil for a silent exit.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit %d", e.code)
	}
	return e.err.Error()
}

// Execute runs the command line and exits with its code: 0 ok, 1 mrm
// failed, 2 some repos need attention.
func Execute() {
	err := rootCmd.Execute()
	if err == nil {
		return
	}
	code := 1
	var ee *exitError
	if errors.As(err, &ee) {
		code = ee.code
		if ee.err == nil {
			os.Exit(code)
		}
	}
	fmt.Fprintln(os.Stderr, "mrm: "+strings.TrimSpace(err.Error()))
	os.Exit(code)
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringSliceVarP(&flagWorkspaces, "workspace", "w", nil, "only these workspaces (repeatable or comma-separated)")
	pf.StringSliceVarP(&flagTags, "tag", "t", nil, "only repos with these tags")
	pf.StringVarP(&flagFilter, "filter", "f", "", `filter query, e.g. "is:dirty -tag:archived"`)
	pf.BoolVar(&flagJSON, "json", false, "JSON output")
	pf.BoolVar(&flagNoColor, "no-color", false, "no colour (also NO_COLOR)")
	pf.StringVarP(&flagConfig, "config", "c", "", "config file (default $XDG_CONFIG_HOME/mrm/config.yaml, or $MRM_CONFIG)")
	pf.BoolVarP(&flagVerbose, "verbose", "v", false, "debug logging to the log file")
}
