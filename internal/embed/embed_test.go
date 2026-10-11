package embed

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"slices"
	"testing"
)

// build writes a model's file of pieces, with their scores and vectors.
func build(t *testing.T, pieces []string, scores []float32, vecs [][]float32, normalizer, pre string, charmap map[string]string) []byte {
	t.Helper()
	h, _ := json.Marshal(map[string]any{"name": "test", "kind": "Unigram", "dim": len(vecs[0]), "unk": 0,
		"normalizer": json.RawMessage(normalizer), "pre_tokenizer": json.RawMessage(pre), "charmap": charmap})
	var b bytes.Buffer
	b.WriteString("MMEMB1")
	binary.Write(&b, binary.LittleEndian, uint32(len(h)))
	b.Write(h)
	binary.Write(&b, binary.LittleEndian, uint32(len(pieces)))
	for i, p := range pieces {
		binary.Write(&b, binary.LittleEndian, uint16(len(p)))
		b.WriteString(p)
		var mx float32
		for _, x := range vecs[i] {
			mx = max(mx, float32(math.Abs(float64(x))))
		}
		scale := mx / 127
		if scale == 0 {
			scale = 1
		}
		binary.Write(&b, binary.LittleEndian, scores[i])
		binary.Write(&b, binary.LittleEndian, scale)
		for _, x := range vecs[i] {
			b.WriteByte(byte(int8(math.Round(float64(x / scale)))))
		}
	}
	return b.Bytes()
}

func TestTokenizer(t *testing.T) {
	pieces := []string{"<unk>", "▁", "数据", "数据库", "库", "数", "据", "慢", "▁SQL", "▁S", "Q", "L", ":", "变"}
	scores := []float32{0, -2, -5, -6, -8, -9, -9, -8, -7, -12, -12, -12, -4, -8}
	vecs := make([][]float32, len(pieces))
	for i := range vecs {
		vecs[i] = []float32{float32(i), 1, -float32(i) / 2}
	}
	data := build(t, pieces, scores, vecs,
		`{"type":"Sequence","normalizers":[{"type":"Precompiled"},{"type":"Replace","pattern":{"Regex":" {2,}"},"content":" "}]}`,
		`{"type":"Metaspace","replacement":"▁","prepend_scheme":"always","split":false}`,
		map[string]string{"：": ":", "\t": " "})
	m, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Normalize("数据库：\t 慢"); got != "数据库: 慢" {
		t.Fatalf("normalized %q", got)
	}
	// "数据库" is one piece, scoring higher than 数据 + 库; 爆 is unknown.
	if got := m.Pieces("数据库变慢"); !slices.Equal(got, []string{"▁", "数据库", "变", "慢"}) {
		t.Fatalf("pieces %q", got)
	}
	if got := m.Pieces("SQL爆爆慢"); !slices.Equal(got, []string{"▁SQL", "<unk>", "慢"}) {
		t.Fatalf("unknown %q", got)
	}
	v := m.Embed("数据库")
	var n float64
	for _, x := range v {
		n += float64(x * x)
	}
	if len(v) != 3 || math.Abs(n-1) > 1e-5 {
		t.Fatalf("vector %v", v)
	}
	if m.Embed("") != nil {
		t.Fatal("empty text has a vector")
	}
}

// TestReference compares with what Model2Vec gives, when the model built
// by tools/embedmodel is at hand (MINDMAP_EMBED_MODEL and _REF).
func TestReference(t *testing.T) {
	path, refPath := os.Getenv("MINDMAP_EMBED_MODEL"), os.Getenv("MINDMAP_EMBED_REF")
	if path == "" || refPath == "" {
		t.Skip("no model")
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Samples []struct {
			Text       string    `json:"text"`
			Normalized string    `json:"normalized"`
			Tokens     []string  `json:"tokens"`
			Vector     []float32 `json:"vector"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	same := 0
	for _, s := range ref.Samples {
		if got := m.Normalize(s.Text); got != s.Normalized {
			t.Errorf("%q normalized to %q, want %q", s.Text, got, s.Normalized)
		}
		got := m.Pieces(s.Text)
		if slices.Equal(got, s.Tokens) {
			same++
		} else {
			t.Logf("%q: %q, want %q", s.Text, got, s.Tokens)
		}
		v := m.Embed(s.Text)
		var cos float64
		for i := range v {
			cos += float64(v[i]) * float64(s.Vector[i])
		}
		if cos < 0.99 {
			t.Errorf("%q: cosine %.4f with Model2Vec's vector", s.Text, cos)
		}
	}
	if same < len(ref.Samples)-2 {
		t.Errorf("tokens differ for %d of %d texts", len(ref.Samples)-same, len(ref.Samples))
	}
}
