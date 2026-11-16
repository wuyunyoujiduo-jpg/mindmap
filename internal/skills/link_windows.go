//go:build windows

package skills

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// makeLink puts at a link to the folder target: a link if Windows lets
// one be made (in developer mode), else a junction, which needs no right.
func makeLink(target, at string) error {
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	if os.Symlink(target, at) == nil {
		return nil
	}
	cmd := exec.Command("cmd", "/c", "mklink", "/J", at, target)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
