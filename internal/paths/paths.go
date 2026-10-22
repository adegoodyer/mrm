// Package paths resolves mrm's XDG locations.
package paths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
)

const App = "mrm"

// Reload re-reads the XDG environment variables (tests set them).
func Reload() { xdg.Reload() }

// Config is the config file: $XDG_CONFIG_HOME/mrm/config.yaml.
func Config() string { return filepath.Join(xdg.ConfigHome, App, "config.yaml") }

// StateDir holds the run history and log: $XDG_STATE_HOME/mrm.
func StateDir() string { return filepath.Join(xdg.StateHome, App) }

// History is the run history file.
func History() string { return filepath.Join(StateDir(), "runs.jsonl") }

// Log is the log file.
func Log() string { return filepath.Join(StateDir(), "mrm.log") }

// CacheDir is $XDG_CACHE_HOME/mrm.
func CacheDir() string { return filepath.Join(xdg.CacheHome, App) }

// StatusCache is the last known status of every repo.
func StatusCache() string { return filepath.Join(CacheDir(), "status.json") }

// ForgeCache holds the last answers from forge APIs.
func ForgeCache() string { return filepath.Join(CacheDir(), "forge.json") }

// TUIState is the interactive view's last workspace, groups and panes.
func TUIState() string { return filepath.Join(StateDir(), "tui.json") }

// LockDir holds the per-workspace sync locks: $XDG_RUNTIME_DIR/mrm, or the
// state dir when there is no runtime dir.
func LockDir() string {
	if os.Getenv("XDG_RUNTIME_DIR") != "" && xdg.RuntimeDir != "" {
		return filepath.Join(xdg.RuntimeDir, App)
	}
	return filepath.Join(StateDir(), "locks")
}

// ExpandHome turns a leading ~ into the home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// Tilde shortens a path under the home directory to ~/...
func Tilde(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
