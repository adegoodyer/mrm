// Package git runs the git CLI in a repo. Nothing here can prompt: terminal
// prompts are off, ssh runs in batch mode, and every call has a timeout.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner runs git commands.
type Runner struct {
	Timeout time.Duration // per command; 0 means no limit
	Env     []string      // extra environment, appended last
}

// Error is a failed git command with its stderr.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
	TimedOut bool
}

func (e *Error) Error() string {
	if e.TimedOut {
		return "timed out: git " + strings.Join(e.Args, " ")
	}
	if msg := e.Message(); msg != "" {
		return msg
	}
	return fmt.Sprintf("git %s: exit %d", strings.Join(e.Args, " "), e.ExitCode)
}

// knownCauses turn the usual remote failures into one plain reason; git's
// own "fatal: Could not read from remote repository." hides them.
var knownCauses = []struct{ match, reason string }{
	{"Permission denied (publickey", "ssh key rejected (Permission denied (publickey))"},
	{"Host key verification failed", "unknown ssh host key (Host key verification failed)"},
	{"no longer exists", "remote repo not found, or no access"},
	{"Repository not found", "remote repo not found, or no access"},
	{"does not appear to be a git repository", "remote repo not found, or no access"},
	{"Could not resolve hostname", "can't resolve the remote host - offline?"},
	{"Could not resolve host", "can't resolve the remote host - offline?"},
	{"Connection timed out", "connection timed out"},
	{"Connection refused", "connection refused"},
	{"Network is unreachable", "network unreachable - offline?"},
}

// Message is the most useful part of git's stderr: a known cause, else the
// first "fatal:" or "error:" line, else the last non-empty line.
func (e *Error) Message() string {
	for _, c := range knownCauses {
		if strings.Contains(e.Stderr, c.match) {
			return c.reason
		}
	}
	var last string
	for _, line := range strings.Split(e.Stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "fatal:") || strings.HasPrefix(line, "error:") {
			return line
		}
		last = line
	}
	return last
}

// IsTimeout reports whether err is a git timeout.
func IsTimeout(err error) bool {
	var ge *Error
	return errors.As(err, &ge) && ge.TimedOut
}

// env is the environment every git command runs with.
func (r *Runner) env() []string {
	env := os.Environ()
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GCM_INTERACTIVE=never",
		"LC_ALL=C",
	)
	// Give up on a host that doesn't answer within 10 s, and on a connection
	// that stops answering for 15 s, instead of waiting for the timeout.
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=3")
	}
	// The same for https: under 1 KB/s for 15 s counts as stalled.
	if os.Getenv("GIT_HTTP_LOW_SPEED_LIMIT") == "" {
		env = append(env, "GIT_HTTP_LOW_SPEED_LIMIT=1000", "GIT_HTTP_LOW_SPEED_TIME=15")
	}
	return append(env, r.Env...)
}

// Run runs git in dir and returns stdout.
func (r *Runner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	full := append([]string{"-C", dir, "-c", "color.ui=false", "-c", "core.quotePath=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = r.env()
	cmd.Stdin = nil
	// Kill the whole process group on timeout so ssh children die too.
	setProcessGroup(cmd)
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	ge := &Error{Args: args, ExitCode: -1, Stderr: stderr.String()}
	if ctx.Err() == context.DeadlineExceeded {
		ge.TimedOut = true
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		ge.ExitCode = ee.ExitCode()
	} else if ge.Stderr == "" {
		ge.Stderr = err.Error()
	}
	return stdout.String(), ge
}

// Version returns the installed git version, e.g. "2.43.0".
func Version(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "--version").Output()
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "git version"))
	if i := strings.IndexByte(v, ' '); i > 0 {
		v = v[:i]
	}
	return v, nil
}
