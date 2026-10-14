//go:build unix

package git

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid signals the whole group (git and its ssh child).
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
