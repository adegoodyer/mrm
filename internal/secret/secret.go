// Package secret resolves a secret from a command or an environment
// variable - the only two places mrm takes secrets from. It never returns a
// literal from the config, and never echoes a failing command's output (it
// may be the secret).
package secret

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Resolve runs cmd (with sh -c) or reads env, whichever is set; neither set
// gives "".
func Resolve(ctx context.Context, cmd, env string) (string, error) {
	switch {
	case env != "":
		v := os.Getenv(env)
		if v == "" {
			return "", fmt.Errorf("$%s is empty", env)
		}
		return v, nil
	case cmd != "":
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "sh", "-c", cmd).Output()
		if err != nil {
			return "", fmt.Errorf("token_cmd failed: %v", err)
		}
		return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), nil
	}
	return "", nil
}
