//go:build !windows

package automate

import (
	"os/exec"
	"syscall"
)

// setGroup runs the program in a group of its own, so that stopping it
// stops what it started too.
func setGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
