// Package browser opens a URL: browser_cmd if set, else wslview on WSL,
// else xdg-open (Linux) or open (macOS).
package browser

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// IsWSL reports whether mrm is running under WSL.
func IsWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// Command returns the opener argv for a URL.
func Command(browserCmd, url string) ([]string, error) {
	if browserCmd != "" {
		return append(strings.Fields(browserCmd), url), nil
	}
	var candidates []string
	switch {
	case runtime.GOOS == "darwin":
		candidates = []string{"open"}
	case IsWSL():
		candidates = []string{"wslview", "xdg-open"}
	default:
		candidates = []string{"xdg-open"}
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return []string{c, url}, nil
		}
	}
	return nil, fmt.Errorf("no browser opener found (tried %s); set browser_cmd in the config", strings.Join(candidates, ", "))
}

// Open opens url without waiting for the browser.
func Open(browserCmd, url string) error {
	argv, err := Command(browserCmd, url)
	if err != nil {
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck // openers exit at once; nothing to report
	return nil
}
