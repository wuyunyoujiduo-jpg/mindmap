package mcp

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"
)

// Several agents write to the maps at once, each through a server of its
// own, and a change is a map read, changed and written whole: two at the
// same moment would lose one's. So a server changes the maps folder only
// while it holds the folder's lock, a file made with O_EXCL beside the
// versions; one left by a server that died is taken over.

const (
	lockWait  = 10 * time.Second
	lockStale = 30 * time.Second
)

// lock takes the maps folder's lock and returns what gives it back.
func (s *Server) lock() func() {
	if s.StateDir == "" {
		return func() {}
	}
	h := sha1.Sum([]byte(filepath.Clean(s.Dir)))
	path := filepath.Join(s.StateDir, hex.EncodeToString(h[:8])+".lock")
	_ = os.MkdirAll(s.StateDir, 0o755)
	for start := time.Now(); ; {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return func() { _ = os.Remove(path) }
		}
		if fi, serr := os.Stat(path); serr == nil && time.Since(fi.ModTime()) > lockStale {
			_ = os.Remove(path)
			continue
		}
		if time.Since(start) > lockWait { // better to write than to hang the agent
			return func() {}
		}
		time.Sleep(20 * time.Millisecond)
	}
}
