//go:build !unix

package git

import "os/exec"

func setProcessGroup(*exec.Cmd) {}
