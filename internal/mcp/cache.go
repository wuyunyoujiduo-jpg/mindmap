package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// An agent's session keeps one server running and calls it many times.
// A call that names no map finds the project's among all the maps, a
// search and a list of tasks read them all, and each tool of the library
// opened the whole of it: every file was read and parsed again at every
// call. What was read is kept here for the next call, with the time and
// size of the file it was read from, and read again when they changed:
// the app, another agent and the sync write the same files.
//
// Kept for looking only. What a call changes and writes is read anew
// (model.Load, library.Open), under the folder's lock: a file written
// twice in one tick of its system's clock, to the same size, would be
// kept as it was before, which a search can live with and a write that
// starts from it cannot.

// kept is a map as read, and what its file was then.
type kept struct {
	mod  time.Time
	size int64
	f    *model.File
}

// peek is the map at path to look at, not to change: as kept if its file
// is as it was, else read.
func (s *Server) peek(path string) (*model.File, error) {
	fi, err := os.Stat(path)
	if err != nil {
		s.mu.Lock()
		delete(s.maps, path)
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Lock()
	k, ok := s.maps[path]
	s.mu.Unlock()
	if ok && k.mod.Equal(fi.ModTime()) && k.size == fi.Size() {
		return k.f, nil
	}
	f, err := model.Load(path)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.maps == nil {
		s.maps = map[string]kept{}
	}
	s.maps[path] = kept{fi.ModTime(), fi.Size(), f}
	s.mu.Unlock()
	return f, nil
}

// lookLibrary is the library to look at, not to write in: as kept if its
// files are as they were, else opened.
func (s *Server) lookLibrary() (*library.Library, error) {
	dir := library.In(s.Dir)
	stamp := libraryStamp(dir)
	s.mu.Lock()
	l, was := s.lib, s.libStamp
	s.mu.Unlock()
	if l != nil && was == stamp {
		return l, nil
	}
	l, err := library.Open(dir)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.lib, s.libStamp = l, stamp
	s.mu.Unlock()
	return l, nil
}

// libraryStamp is a word that changes when the library's files did: the
// clips, a file each, by their names, times and sizes (a clip is written
// beside its file and put in its place, so the folder's own time moves
// too); the keys of the vault, by which what is written is sealed; and
// the vectors the search by meaning reads. No file is read.
func libraryStamp(dir string) string {
	var b strings.Builder
	for _, sub := range []string{"clips", "vault"} {
		p := filepath.Join(dir, sub)
		if fi, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%s %d\n", sub, fi.ModTime().UnixNano())
		}
		entries, _ := os.ReadDir(p)
		for _, e := range entries {
			if fi, err := e.Info(); err == nil && !e.IsDir() {
				fmt.Fprintf(&b, "%s %d %d\n", e.Name(), fi.ModTime().UnixNano(), fi.Size())
			}
		}
	}
	if fi, err := os.Stat(filepath.Join(dir, "vectors.json")); err == nil {
		fmt.Fprintf(&b, "vectors %d %d\n", fi.ModTime().UnixNano(), fi.Size())
	}
	return b.String()
}

// embedder is what finds clips by meaning, set up at the first search
// that asks for it: the local model is tens of megabytes to read, and
// most sessions never search the library.
func (s *Server) embedder() library.Embedder {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.LoadEmbedder != nil {
		s.Embedder, s.LoadEmbedder = s.LoadEmbedder(), nil
	}
	return s.Embedder
}
