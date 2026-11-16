//go:build !windows

package skills

import (
	"os"
	"path/filepath"
)

// makeLink puts at a link to the folder target.
func makeLink(target, at string) error {
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	return os.Symlink(target, at)
}

// makeRelLink puts at a link to target from where at is: a link that
// still holds when the folder both are in is moved or cloned.
func makeRelLink(target, at string) error {
	rel, err := filepath.Rel(filepath.Dir(at), target)
	if err != nil {
		return makeLink(target, at)
	}
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	return os.Symlink(rel, at)
}
