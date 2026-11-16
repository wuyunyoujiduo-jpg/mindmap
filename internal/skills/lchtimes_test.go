//go:build !windows

package skills

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// lchtimes sets a link's own time, not its target's.
func lchtimes(t *testing.T, p string, at time.Time) {
	t.Helper()
	tv := unix.NsecToTimeval(at.UnixNano())
	if err := unix.Lutimes(p, []unix.Timeval{tv, tv}); err != nil {
		t.Fatal(err)
	}
}
