package embed

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestSpeed(t *testing.T) {
	path := os.Getenv("MINDMAP_EMBED_MODEL")
	if path == "" {
		t.Skip("no model")
	}
	start := time.Now()
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	load := time.Since(start)
	text := strings.Repeat("缓存雪崩是指大量缓存同时过期，请求直接打到数据库。Use random TTLs to avoid it. ", 30)
	start = time.Now()
	for range 100 {
		m.Embed(text)
	}
	t.Logf("load %v, embed a %d-character text %v", load, len([]rune(text)), time.Since(start)/100)
}
