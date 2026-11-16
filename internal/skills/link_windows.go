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

// makeRelLink puts at a link to target from where at is, if Windows lets
// a link be made; a junction, which is whole, else.
func makeRelLink(target, at string) error {
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	if rel, err := filepath.Rel(filepath.Dir(at), target); err == nil && os.Symlink(rel, at) == nil {
		return nil
	}
	return makeLink(target, at)
}
