package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/store"
)

var flagInitForce bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Write, edit and locate the config",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a commented default config",
	Long: `Write a commented default config with one workspace per root found
(~/src, ~/code, ~/projects, ~/repos, ~/work, ~/dev, ~/git), or the
current directory if none exists.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := configPath()
		started := time.Now()
		if err := config.WriteInit(path, flagInitForce); err != nil {
			return err
		}
		record(store.NewEvent(store.OpConfig, "wrote the default config to "+paths.Tilde(path), started, nil))
		fmt.Println("wrote " + paths.Tilde(path))
		return nil
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config in $VISUAL / $EDITOR (writing the default first if missing)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := configPath()
		if _, err := os.Stat(path); os.IsNotExist(err) {
			started := time.Now()
			if err := config.WriteInit(path, false); err != nil {
				return err
			}
			record(store.NewEvent(store.OpConfig, "wrote the default config to "+paths.Tilde(path), started, nil))
		}
		editor := os.Getenv("VISUAL")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			editor = "vi"
		}
		argv := append(strings.Fields(editor), path)
		c := exec.Command(argv[0], argv[1:]...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return err
		}
		if _, err := config.Load(path); err != nil {
			return fmt.Errorf("saved, but the config is invalid:\n%w", err)
		}
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the config, state, cache and lock paths",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		rows := [][2]string{
			{"config", configPath()},
			{"history", paths.History()},
			{"log", paths.Log()},
			{"cache", paths.StatusCache()},
			{"pr/ci", paths.ForgeCache()},
			{"locks", paths.LockDir()},
		}
		for _, r := range rows {
			fmt.Printf("%-8s %s\n", r[0], r[1])
		}
		return nil
	},
}

func init() {
	configInitCmd.Flags().BoolVar(&flagInitForce, "force", false, "overwrite an existing config")
	configCmd.AddCommand(configInitCmd, configEditCmd, configPathCmd)
	rootCmd.AddCommand(configCmd)
}
