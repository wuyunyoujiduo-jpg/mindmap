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
