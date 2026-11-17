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
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
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

// EmbedText is what of a clip its vector is made of: without what is
// sealed, and with what looks like a secret and is not sealed in dots,
// for the model may be one asked over the network.
func EmbedText(c *Clip) string {
	var b strings.Builder
	for _, s := range []string{c.embedPart(PartTitle), c.embedPart(PartDesc), c.embedPart(PartSummary), Mask(c.Category), Mask(strings.Join(c.Tags, " ")),
		Mask(strings.Join(c.Keywords, " ")), Mask(strings.Join(c.Synonyms, " ")), c.embedPart(PartAnalysis), c.embedPart(PartNote)} {
		if s = strings.TrimSpace(s); s != "" {
			b.WriteString(s + "\n")
		}
	}
	// The start of its text, cut where it is: the whole of a long page
	// was made runes to take the first 1,500 of them.
	b.WriteString(firstRunes(strings.TrimSpace(c.embedPart(PartText)), 1500))
	if b.Len() == 0 {
		b.WriteString(c.Label())
	}
	return b.String()
}

// embedPart is a part of a clip as a vector is made of it: without what
// is sealed, the rest with its secrets in dots, which the clip keeps
// worked out (Masked).
func (c *Clip) embedPart(p Part) string {
	if s := c.Of(p); vault.Has(s) {
		return Mask(noSealed(s))
	}
	return c.Masked(p)
}

// firstRunes is the first n characters of s.
func firstRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	i := 0
	for at := range s {
		if i == n {
			return s[:at]
		}
		i++
	}
	return s
}

// vector is a clip's vector, with the hash of what it was made of. Once
// kept it is not changed: a new one takes its place.
type vector struct {
	Hash string    `json:"hash"`
	V    []float32 `json:"-"`
	Data string    `json:"v"` // V as little-endian float32, in base64
}

type vectorFile struct {
	Model   string             `json:"model"`
	Vectors map[string]*vector `json:"vectors"`
}

func vectorsPath(dir string) string { return filepath.Join(dir, "vectors.json") }

// Vectors are the clips' vectors of a model, as read from a library's
// folder.
type Vectors struct{ file *vectorFile }

// ReadVectors reads the vectors kept in a library's folder, those of the
// model named: none when they are of another. It is the file read and
// every vector of it decoded, some megabytes for a library of thousands,
// and is done away from the window by who sets Library.VectorsLater: it
// was done in a frame, each time the library was read again.
func ReadVectors(dir, model string) *Vectors {
	out := &vectorFile{Model: model, Vectors: map[string]*vector{}}
	data, err := os.ReadFile(vectorsPath(dir))
	if err != nil {
		return &Vectors{out}
	}
	var f vectorFile
	if json.Unmarshal(data, &f) != nil || f.Model != model {
		return &Vectors{out}
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
		out.Vectors[id] = v
	}
	return &Vectors{out}
}

// UseVectors gives the library the vectors read for it. Those it has of
// the same model already, made meanwhile, stay.
func (l *Library) UseVectors(v *Vectors) {
	if v == nil || v.file == nil {
		return
	}
	if l.vecs != nil && l.vecs.Model == v.file.Model {
		for id, made := range l.vecs.Vectors {
			v.file.Vectors[id] = made
		}
	}
	l.vecs = v.file
	l.Version++ // what is found by meaning is found now
}

// HasVectors tells whether the library has the vectors of a model: read,
// or given.
func (l *Library) HasVectors(model string) bool { return l.vecs != nil && l.vecs.Model == model }

// loadVectors reads the vectors kept, of the model named.
func (l *Library) loadVectors(model string) {
	if !l.HasVectors(model) {
		l.vecs = ReadVectors(l.Dir, model).file
	}
}

// VectorsWriter is the work of writing the vectors as they are now, to
// be done away from the window: the whole file is written, which for a
// library of thousands is some megabytes made and written, and was done
// in the window's turn after every sixteen clips of a first indexing.
// One at a time: two would write the same file.
func (l *Library) VectorsWriter() func() error {
	if l.vecs == nil {
		return func() error { return nil }
	}
	model, dir := l.vecs.Model, l.Dir
	keep := make(map[string]*vector, len(l.vecs.Vectors))
	for id, v := range l.vecs.Vectors {
		if l.Find(id) != nil {
			keep[id] = v
		}
	}
	return func() error {
		f := vectorFile{Model: model, Vectors: make(map[string]*vector, len(keep))}
		for id, v := range keep {
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
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		tmp := vectorsPath(dir) + ".tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, vectorsPath(dir))
	}
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
	if !l.HasVectors(model) {
		if l.VectorsLater {
			return nil // not read yet
		}
		l.loadVectors(model)
	}
	v := l.vecs.Vectors[c.ID]
	if v == nil || v.Hash != l.embedHash(c) {
		return nil
	}
	return v.V
}

// ToEmbed are the clips whose vector of the model is missing or out of
// date, by their IDs: what a run of making vectors has to do, taken once
// for the run (asked again after each sixteen of them, every clip was
// looked at, and the text of each one left made, some two hundred times
// over for a library of three thousand). None while the vectors are yet
// to be read (VectorsLater): what is kept is not made again.
func (l *Library) ToEmbed(model string) []string {
	if l.VectorsLater && !l.HasVectors(model) {
		return nil
	}
	var ids []string
	for _, c := range l.Clips {
		if l.Vector(c, model) == nil {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// EmbedTexts are the texts to make the vectors of clips of, those of
// them that are there still.
func (l *Library) EmbedTexts(ids []string) (there, texts []string) {
	for _, id := range ids {
		if c := l.Find(id); c != nil {
			there = append(there, id)
			texts = append(texts, EmbedText(c))
		}
	}
	return there, texts
}

// Unembedded are the clips whose vector of the model is missing or out of
// date, with the texts to make them of.
func (l *Library) Unembedded(model string) (ids, texts []string) {
	return l.EmbedTexts(l.ToEmbed(model))
}

// KeepVectors keeps the vectors of clips, made of the texts given, in
// memory: VectorsWriter writes them.
func (l *Library) KeepVectors(model string, ids, texts []string, vecs [][]float32) {
	l.loadVectors(model)
	for i, id := range ids {
		l.vecs.Vectors[id] = &vector{Hash: textHash(texts[i]), V: vecs[i]}
	}
}

// SetVectors keeps the vectors of clips, made of the texts given, and
// writes them.
func (l *Library) SetVectors(model string, ids, texts []string, vecs [][]float32) error {
	l.KeepVectors(model, ids, texts, vecs)
	return l.VectorsWriter()()
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
