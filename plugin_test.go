package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// tinyModel is a model's file of a few pieces, as tools/embedmodel writes.
func tinyModel() []byte {
	pieces := []string{"<unk>", "▁", "数据", "库", "慢"}
	h, _ := json.Marshal(map[string]any{"name": "tiny", "kind": "Unigram", "dim": 2, "unk": 0,
		"normalizer": nil, "pre_tokenizer": json.RawMessage(`{"type":"Metaspace","replacement":"▁","prepend_scheme":"always","split":false}`)})
	var b bytes.Buffer
	b.WriteString("MMEMB1")
	binary.Write(&b, binary.LittleEndian, uint32(len(h)))
	b.Write(h)
	binary.Write(&b, binary.LittleEndian, uint32(len(pieces)))
	for i, p := range pieces {
		binary.Write(&b, binary.LittleEndian, uint16(len(p)))
		b.WriteString(p)
		binary.Write(&b, binary.LittleEndian, float32(-float32(i)))
		binary.Write(&b, binary.LittleEndian, float32(0.01))
		b.Write([]byte{byte(i * 10), 100})
	}
	return b.Bytes()
}

func TestPluginDownload(t *testing.T) {
	a, tt := newTestApp(t)
	t.Setenv("MINDMAP_EMBED_MODEL", "")
	a.noLocalModel = false
	reloadLocalModel()
	defer reloadLocalModel()
	if a.localModel() != nil {
		t.Skip("a model is among the resources")
	}
	model := tinyModel()
	half := len(model) / 2
	parts := [][]byte{model[:half], model[half:]}
	manifest := map[string]any{"name": "tiny", "version": 1, "size": len(model), "sha256": sha(model)}
	var list []map[string]any
	for i, p := range parts {
		list = append(list, map[string]any{"file": fmt.Sprintf("m.%03d", i), "size": len(p), "sha256": sha(p)})
	}
	manifest["parts"] = list
	mj, _ := json.Marshal(manifest)
	files := map[string][]byte{"manifest.json": mj, "m.000": parts[0], "m.001": parts[1]}
	var asked []string
	a.pluginGet = func(_ context.Context, url string) ([]byte, error) {
		asked = append(asked, url)
		if strings.Contains(url, "raw.githubusercontent.com") {
			return nil, errors.New("blocked") // GitHub out of reach: the mirror serves
		}
		name := url[strings.LastIndex(url, "/")+1:]
		if strings.HasSuffix(name, ".001") && strings.Contains(url, "cdn.jsdelivr") {
			return []byte("corrupt"), nil // a bad part: the next mirror's is taken
		}
		return files[name], nil
	}

	a.showLibrary()
	tt.Frame()
	if !tt.HasText("下载") {
		t.Fatalf("no offer: %q", tt.Texts())
	}
	snapshot(t, tt, "plugin-banner")
	if err := tt.Click("下载"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pluginErr != "" || !pluginInstalled() || a.localModel() == nil || !a.embedder().Builtin() {
		t.Fatalf("not installed: %q, asked %v", a.pluginErr, asked)
	}
	if m := installedManifest(); m == nil || m.Version != 1 {
		t.Fatal("manifest not kept")
	}
	if tt.HasText("不用了") {
		t.Fatal("offer still shown")
	}
	a.removePlugin()
	if pluginInstalled() || a.localModel() != nil {
		t.Fatal("not removed")
	}

	// A download that does not check out is not installed.
	files["m.001"] = []byte("tampered")
	a.pluginGet = func(_ context.Context, url string) ([]byte, error) {
		return files[url[strings.LastIndex(url, "/")+1:]], nil
	}
	a.downloadPlugin()
	if pluginInstalled() || !strings.Contains(a.pluginErr, "失败") {
		t.Fatalf("tampered plugin: %q", a.pluginErr)
	}
	// Declined: the offer goes.
	tt.Frame()
	tt.Click("不用了")
	tt.Frame()
	if tt.HasText("不用了") || !a.cfg.PluginDeclined {
		t.Fatal("offer not declined")
	}
}
