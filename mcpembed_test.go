package main

import (
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/embed"
)

// The server reads the local model when it first searches the library,
// not as it starts; and never with the search by meaning turned off.
func TestMCPEmbedderIsLate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	read := 0
	local := func() *embed.Model {
		read++
		return nil
	}
	s := mcpServer(t.TempDir())
	s.LoadEmbedder = mcpEmbedder(config{EmbedURL: "https://example.com/v1", EmbedModel: "m"}, local)
	if read != 0 {
		t.Fatal("the model read as the server was made")
	}
	if _, err := s.Call("list_maps", nil); err != nil || read != 0 {
		t.Fatalf("the model read for a call that searches nothing: %v, %d", err, read)
	}
	if _, err := s.Call("search_library", []byte(`{"query": ""}`)); err != nil || read != 1 {
		t.Fatalf("at the first search: %v, read %d times", err, read)
	}
	if e := s.Embedder; e.URL != "https://example.com/v1" || e.Model != "m" || s.LoadEmbedder != nil {
		t.Fatalf("embedder %+v", e)
	}
	if _, err := s.Call("search_library", []byte(`{"query": ""}`)); err != nil || read != 1 {
		t.Fatalf("at the second: %v, read %d times", err, read)
	}
	off := mcpEmbedder(config{NoEmbed: true, EmbedURL: "https://example.com/v1"}, local)()
	if off.On() || read != 1 {
		t.Fatalf("turned off: %+v, read %d times", off, read)
	}
}
