package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/embed"
)

// Semantic search: each clip is turned into a vector by an embedding
// model, through any OpenAI-compatible /embeddings API (Ollama on this
// machine, SiliconFlow, OpenAI…), and a query finds the clips whose
// vectors are nearest to its own, by what they mean rather than the words
// they use. The vectors are kept in vectors.json, each with a hash of the
// text it was made of, so that only clips that changed are made again.

// Embedder makes vectors of texts: by an API when its address is set, else
// by the built-in model, Local, when there is one.
type Embedder struct {
	URL   string // the API's base, as http://localhost:11434/v1
	Key   string
	Model string
	Local *embed.Model
}

// On tells whether an embedding model is set up.
func (e Embedder) On() bool { return e.URL != "" && e.Model != "" || e.URL == "" && e.Local != nil }

// Builtin tells whether the built-in model makes the vectors.
func (e Embedder) Builtin() bool { return e.URL == "" && e.Local != nil }

// Name names the model whose vectors are kept, so that vectors of another
// are made again.
func (e Embedder) Name() string {
	if e.Builtin() {
		return fmt.Sprintf("builtin:%s:%d", e.Local.Name, e.Local.Dim())
	}
	return e.Model
}

// Embed makes the vectors of texts.
func (e Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.Builtin() {
		out := make([][]float32, len(texts))
		for i, t := range texts {
			if out[i] = e.Local.Embed(t); out[i] == nil {
				out[i] = make([]float32, e.Local.Dim())
			}
		}
		return out, nil
	}
	body, _ := json.Marshal(map[string]any{"model": e.Model, "input": texts})
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(e.URL, "/")+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.Key != "" {
		req.Header.Set("Authorization", "Bearer "+e.Key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		var e struct {
			Error any `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != nil {
			if m, ok := e.Error.(map[string]any); ok {
				msg = fmt.Sprint(m["message"])
			} else {
				msg = fmt.Sprint(e.Error)
			}
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("向量模型返回 %d：%s", resp.StatusCode, msg)
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("无法解析向量模型的回复：%w", err)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("向量模型返回了 %d 个向量，应为 %d 个", len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vecs) {
			return nil, fmt.Errorf("向量模型的回复有误")
		}
		vecs[d.Index] = normalize(d.Embedding)
	}
	return vecs, nil
}

func normalize(v []float32) []float32 {
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	if n == 0 {
		return v
	}
	k := float32(1 / math.Sqrt(n))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x * k
	}
	return out
}

// Cosine is how alike two normalized vectors are, from -1 to 1.
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// EmbedText is what of a clip its vector is made of.
func EmbedText(c *Clip) string {
	var b strings.Builder
	for _, s := range []string{c.Title, c.Desc, c.Summary, c.Category, strings.Join(c.Tags, " "), strings.Join(c.Keywords, " "), strings.Join(c.Synonyms, " "), c.Analysis, c.Note} {
		if s = strings.TrimSpace(s); s != "" {
			b.WriteString(s + "\n")
		}
	}
	text := []rune(strings.TrimSpace(c.Text))
	if len(text) > 1500 {
		text = text[:1500]
	}
	b.WriteString(string(text))
	if b.Len() == 0 {
		b.WriteString(c.Label())
	}
	return b.String()
}

// vector is a clip's vector, with the hash of what it was made of.
type vector struct {
	Hash string    `json:"hash"`
	V    []float32 `json:"-"`
	Data string    `json:"v"` // V as little-endian float32, in base64
}

type vectorFile struct {
	Model   string             `json:"model"`
	Vectors map[string]*vector `json:"vectors"`
}

func (l *Library) vectorsPath() string { return filepath.Join(l.Dir, "vectors.json") }

// loadVectors reads the vectors kept, of the model named.
func (l *Library) loadVectors(model string) {
	if l.vecs != nil && l.vecs.Model == model {
		return
	}
	l.vecs = &vectorFile{Model: model, Vectors: map[string]*vector{}}
	data, err := os.ReadFile(l.vectorsPath())
	if err != nil {
		return
	}
	var f vectorFile
	if json.Unmarshal(data, &f) != nil || f.Model != model {
		return
	}
	for id, v := range f.Vectors {
		raw, err := base64.StdEncoding.DecodeString(v.Data)
		if err != nil || len(raw)%4 != 0 {
			continue
		}
		v.V = make([]float32, len(raw)/4)
		for i := range v.V {
			v.V[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		}
		v.Data = ""
		l.vecs.Vectors[id] = v
	}
}

func (l *Library) saveVectors() error {
	f := vectorFile{Model: l.vecs.Model, Vectors: map[string]*vector{}}
	for id, v := range l.vecs.Vectors {
		if l.Find(id) == nil {
			continue
		}
		raw := make([]byte, len(v.V)*4)
		for i, x := range v.V {
			binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(x))
		}
		f.Vectors[id] = &vector{Hash: v.Hash, Data: base64.StdEncoding.EncodeToString(raw)}
	}
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return err
	}
	tmp := l.vectorsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.vectorsPath())
}

// embedHash is the hash of a clip's EmbedText, kept until it changes.
func (l *Library) embedHash(c *Clip) string {
	if l.hashes == nil {
		l.hashes = map[string]cachedHash{}
	}
	if h, ok := l.hashes[c.ID]; ok && h.updated.Equal(c.Updated) {
		return h.hash
	}
	h := textHash(EmbedText(c))
	l.hashes[c.ID] = cachedHash{c.Updated, h}
	return h
}

type cachedHash struct {
	updated time.Time
	hash    string
}

func textHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// Vector is a clip's vector of the model, nil when it has none or its
// text changed since.
func (l *Library) Vector(c *Clip, model string) []float32 {
	l.loadVectors(model)
	v := l.vecs.Vectors[c.ID]
	if v == nil || v.Hash != l.embedHash(c) {
		return nil
	}
	return v.V
}

// Unembedded are the clips whose vector of the model is missing or out of
// date, with the texts to make them of.
func (l *Library) Unembedded(model string) (ids, texts []string) {
	for _, c := range l.Clips {
		if l.Vector(c, model) == nil {
			ids = append(ids, c.ID)
			texts = append(texts, EmbedText(c))
		}
	}
	return ids, texts
}

// SetVectors keeps the vectors of clips, made of the texts given, and
// writes them.
func (l *Library) SetVectors(model string, ids, texts []string, vecs [][]float32) error {
	l.loadVectors(model)
	for i, id := range ids {
		l.vecs.Vectors[id] = &vector{Hash: textHash(texts[i]), V: vecs[i]}
	}
	return l.saveVectors()
}

// SearchMeaning ranks the clips kept by how near their vectors are to the
// query's, the nearest first, those at least min alike.
func (l *Library) SearchMeaning(qv []float32, model string, keep func(*Clip) bool, min float64) []Hit {
	var hits []Hit
	for _, c := range l.Clips {
		if keep != nil && !keep(c) {
			continue
		}
		if v := l.Vector(c, model); v != nil {
			if s := Cosine(qv, v); s >= min {
				hits = append(hits, Hit{Clip: c, Score: s})
			}
		}
	}
	slices.SortStableFunc(hits, func(a, b Hit) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		}
		return 0
	})
	return hits
}

// Fuse merges rankings, by reciprocal rank: a clip high in any ranks
// high, and one high in both higher still.
func Fuse(lists ...[]Hit) []Hit {
	const k = 60
	score := map[*Clip]float64{}
	var order []*Clip
	for _, list := range lists {
		for rank, h := range list {
			if _, ok := score[h.Clip]; !ok {
				order = append(order, h.Clip)
			}
			score[h.Clip] += 1 / float64(k+rank+1)
		}
	}
	out := make([]Hit, len(order))
	for i, c := range order {
		out[i] = Hit{Clip: c, Score: score[c]}
	}
	slices.SortStableFunc(out, func(a, b Hit) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		}
		return 0
	})
	return out
}

// SearchBoth finds clips by their words and, given the query's vector, by
// what they mean.
func (l *Library) SearchBoth(query string, qv []float32, model string, keep func(*Clip) bool) []Hit {
	words := l.Search(query, keep)
	if qv == nil || strings.TrimSpace(query) == "" {
		return words
	}
	return Fuse(words, l.SearchMeaning(qv, model, keep, 0.35))
}

// Related are the clips most like c: by meaning when vectors are made,
// else by the words of its title, description, tags and keywords.
func (l *Library) Related(c *Clip, model string, n int) []Hit {
	other := func(o *Clip) bool { return o != c }
	var hits []Hit
	if v := l.Vector(c, model); v != nil {
		hits = l.SearchMeaning(v, model, other, 0.5)
	} else {
		q := strings.Join([]string{c.Title, c.Desc, strings.Join(c.Tags, " "), strings.Join(c.Keywords, " "), strings.Join(c.Synonyms, " ")}, " ")
		for _, h := range l.Search(q, other) {
			if h.Coverage >= 0.3 {
				hits = append(hits, h)
			}
		}
	}
	if len(hits) > n {
		hits = hits[:n]
	}
	return hits
}
