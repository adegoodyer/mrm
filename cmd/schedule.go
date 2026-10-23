package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/schedule"
)

var (
	flagScheduleOn  string
	flagScheduleYes bool
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Run mrm sync on a schedule (systemd user timer)",
	Long: `Install, list and remove systemd user timers that run mrm sync. A missed
run (machine off, WSL stopped) happens as soon as it is back. The sync uses
your notify settings, so problems still reach you.

Runs need ssh keys that work without an agent prompt (or an agent the timer
can reach); a rejected key shows as "ssh key rejected" in the run.`,
}

var scheduleInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install a timer for the selected workspaces (-w), or all",
	Example: `  mrm schedule install -w work --on "Mon..Fri 08:30"
  mrm schedule install --on daily`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := newApp()
		if err != nil {
			return err
		}
		if flagScheduleOn == "" {
			return fmt.Errorf("--on is required, e.g. --on \"Mon..Fri 08:30\"")
		}
		if err := schedule.Available(); err != nil {
			return err
		}
		next, err := schedule.CheckCalendar(flagScheduleOn)
		if err != nil {
			return err
		}
		bin, err := os.Executable()
		if err != nil {
			return err
		}
		if resolved, err := filepath.EvalSymlinks(bin); err == nil {
			bin = resolved
		}
		spec := schedule.Spec{Workspaces: flagWorkspaces, Calendar: flagScheduleOn, Binary: bin, Path: os.Getenv("PATH")}
		if a.cfg.Path != "" && a.cfg.Path != paths.Config() {
			spec.Config = a.cfg.Path
		}
		what := "every workspace"
		if len(flagWorkspaces) > 0 {
			what = fmt.Sprint(flagWorkspaces)
		}
		fmt.Printf("%s: mrm sync for %s, %s (next: %s)\nunits in %s\n", spec.Name(), what, flagScheduleOn, next, paths.Tilde(schedule.UnitsDir()))
		if err := confirm(flagScheduleYes, "Install and start this timer?"); err != nil {
			return err
		}
		if err := schedule.Install(spec); err != nil {
			return err
		}
		fmt.Println("installed - mrm schedule list shows it; journalctl --user -u " + spec.Name() + " shows its runs")
		if on, ok := schedule.Lingering(); ok && !on {
			fmt.Fprintln(os.Stderr, "warning: "+schedule.LingerHint)
		}
		return nil
	},
}

var scheduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List mrm timers and when they run next",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		timers, err := schedule.List()
		if err != nil {
			return err
		}
		if len(timers) == 0 {
			fmt.Println("No schedules. mrm schedule install --on \"Mon..Fri 08:30\" adds one.")
			return nil
		}
		for _, t := range timers {
			fmt.Printf("%s  %s  [%s]\n  next: %s\n  last: %s\n", t.Name, t.Calendar, t.State, orDash(t.Next), orDash(t.Last))
		}
		return nil
	},
}

var scheduleRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Short:   "Stop a timer and delete its units",
	Example: "  mrm schedule remove work\n  mrm schedule remove mrm-sync-all",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := confirm(flagScheduleYes, "Remove schedule "+args[0]+"?"); err != nil {
			return err
		}
		if err := schedule.Remove(args[0]); err != nil {
			return err
		}
		fmt.Println("removed")
		return nil
	},
}

func orDash(s string) string {
	if s == "" || s == "n/a" {
		return "-"
	}
	return s
}

func init() {
	scheduleInstallCmd.Flags().StringVar(&flagScheduleOn, "on", "", `when, as a systemd calendar expression ("Mon..Fri 08:30", "daily")`)
	for _, c := range []*cobra.Command{scheduleInstallCmd, scheduleRemoveCmd} {
		c.Flags().BoolVarP(&flagScheduleYes, "yes", "y", false, "don't ask first")
	}
	scheduleCmd.AddCommand(scheduleInstallCmd, scheduleListCmd, scheduleRemoveCmd)
	rootCmd.AddCommand(scheduleCmd)
}
