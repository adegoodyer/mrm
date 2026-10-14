package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var flagShortVersion bool

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print the version, commit and build date",
	Example: "  mrm version\n  mrm version --short",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		v, c, d := buildVersion, buildCommit, buildDate
		// `go install` builds carry module and VCS info instead of ldflags.
		if v == "dev" {
			if bi, ok := debug.ReadBuildInfo(); ok {
				if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
					v = bi.Main.Version
				}
				for _, s := range bi.Settings {
					switch s.Key {
					case "vcs.revision":
						c = s.Value
					case "vcs.time":
						d = s.Value
					}
				}
			}
		}
		if flagShortVersion {
			fmt.Println(v)
			return
		}
		fmt.Printf("mrm %s (commit %s, built %s)\n", v, shortSHA(c), d)
	},
}

func init() {
	versionCmd.Flags().BoolVarP(&flagShortVersion, "short", "s", false, "version only")
	rootCmd.AddCommand(versionCmd)
}
