// Package embed runs the built-in embedding model of the library's
// semantic search: a Model2Vec model, a table of token vectors distilled
// from a sentence transformer, so that a text's vector is the mean of its
// tokens' vectors. No neural network runs; cutting the text into tokens is
// the work, done as the model's Unigram tokenizer does. tools/embedmodel
// makes the model's file.
package embed

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// FileName is the model's file, among the app's resources.
const FileName = "mindmap-embed.bin"

// MaxTokens is how many tokens of a text count, as Model2Vec's encode.
const MaxTokens = 512

// Model is a loaded model.
type Model struct {
	Name   string
	dim    int
	unk    int
	ids    map[string]int32 // pieces' ids
	scores []float32
	scales []float32
	vecs   []int8 // dim a piece
	maxLen int    // the longest piece, in runes
	minS   float32

	normalize func(string) string
	metaspace bool   // spaces become ▁, which also starts the text
	split     bool   // and the text is cut at them before tokenizing
	space     string // the replacement of spaces
}

type header struct {
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	Dim          int               `json:"dim"`
	Unk          int               `json:"unk"`
	Normalizer   json.RawMessage   `json:"normalizer"`
	PreTokenizer json.RawMessage   `json:"pre_tokenizer"`
	Charmap      map[string]string `json:"charmap"`
}

// Load reads a model's file.
func Load(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse reads a model from its file's bytes.
func Parse(data []byte) (*Model, error) {
	if !bytes.HasPrefix(data, []byte("MMEMB1")) || len(data) < 14 {
		return nil, errors.New("不是 MindMap 的向量模型文件")
	}
	r := data[6:]
	n := int(binary.LittleEndian.Uint32(r))
	r = r[4:]
	if n > len(r) {
		return nil, errors.New("模型文件不完整")
	}
	var h header
	if err := json.Unmarshal(r[:n], &h); err != nil {
		return nil, fmt.Errorf("模型文件的头部有误：%w", err)
	}
	r = r[n:]
	if h.Kind != "Unigram" || h.Dim <= 0 {
		return nil, fmt.Errorf("不支持的模型：%s", h.Kind)
	}
	m := &Model{Name: h.Name, dim: h.Dim, unk: h.Unk}
	var err error
	if m.normalize, err = normalizer(h.Normalizer, h.Charmap); err != nil {
		return nil, err
	}
	if err := m.preTokenizer(h.PreTokenizer); err != nil {
		return nil, err
	}
	if len(r) < 4 {
		return nil, errors.New("模型文件不完整")
	}
	count := int(binary.LittleEndian.Uint32(r))
	r = r[4:]
	m.ids = make(map[string]int32, count)
	m.scores = make([]float32, count)
	m.scales = make([]float32, count)
	m.vecs = make([]int8, count*h.Dim)
	m.minS = float32(math.Inf(1))
	for i := range count {
		if len(r) < 2 {
			return nil, errors.New("模型文件不完整")
		}
		l := int(binary.LittleEndian.Uint16(r))
		if len(r) < 2+l+8+h.Dim {
			return nil, errors.New("模型文件不完整")
		}
		piece := string(r[2 : 2+l])
		r = r[2+l:]
		m.ids[piece] = int32(i)
		m.scores[i] = math.Float32frombits(binary.LittleEndian.Uint32(r))
		m.scales[i] = math.Float32frombits(binary.LittleEndian.Uint32(r[4:]))
		for d := range h.Dim {
			m.vecs[i*h.Dim+d] = int8(r[8+d])
		}
		r = r[8+h.Dim:]
		m.maxLen = max(m.maxLen, utf8.RuneCountInString(piece))
		if i != h.Unk {
			m.minS = min(m.minS, m.scores[i])
		}
	}
	return m, nil
}

// Dim is the length of the vectors.
func (m *Model) Dim() int { return m.dim }

// normalizer builds the normalizer the tokenizer describes. Precompiled
// (SentencePiece's charsmap) is the table of characters the file carries.
func normalizer(raw json.RawMessage, charmap map[string]string) (func(string) string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return func(s string) string { return s }, nil
	}
	var n struct {
		Type        string            `json:"type"`
		Normalizers []json.RawMessage `json:"normalizers"`
		Pattern     struct {
			String *string `json:"String"`
			Regex  *string `json:"Regex"`
		} `json:"pattern"`
		Content string `json:"content"`
		Left    bool   `json:"strip_left"`
		Right   bool   `json:"strip_right"`
		Prepend string `json:"prepend"`
	}
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, err
	}
	switch n.Type {
	case "Sequence":
		var fns []func(string) string
		for _, sub := range n.Normalizers {
			fn, err := normalizer(sub, charmap)
			if err != nil {
				return nil, err
			}
			fns = append(fns, fn)
		}
		return func(s string) string {
			for _, fn := range fns {
				s = fn(s)
			}
			return s
		}, nil
	case "Precompiled":
		cm := map[rune]string{}
		for k, v := range charmap {
			if r, size := utf8.DecodeRuneInString(k); size == len(k) {
				cm[r] = v
			}
		}
		return func(s string) string {
			var b strings.Builder
			for _, r := range s {
				if v, ok := cm[r]; ok {
					b.WriteString(v)
				} else {
					b.WriteRune(r)
				}
			}
			return b.String()
		}, nil
	case "Replace":
		if n.Pattern.Regex != nil {
			// \s is Unicode's white space, as in Rust's regex.
			re, err := regexp.Compile(strings.ReplaceAll(*n.Pattern.Regex, `\s`, `[\s\p{Z}\x{85}]`))
			if err != nil {
				return nil, err
			}
			return func(s string) string { return re.ReplaceAllLiteralString(s, n.Content) }, nil
		}
		if n.Pattern.String != nil {
			old := *n.Pattern.String
			return func(s string) string { return strings.ReplaceAll(s, old, n.Content) }, nil
		}
	case "Strip":
		return func(s string) string {
			if n.Left {
				s = strings.TrimLeftFunc(s, isSpace)
			}
			if n.Right {
				s = strings.TrimRightFunc(s, isSpace)
			}
			return s
		}, nil
	case "NFKC":
		return norm.NFKC.String, nil
	case "NFC":
		return norm.NFC.String, nil
	case "NFKD":
		return norm.NFKD.String, nil
	case "NFD":
		return norm.NFD.String, nil
	case "Lowercase":
		return strings.ToLower, nil
	case "Prepend":
		return func(s string) string {
			if s == "" {
				return s
			}
			return n.Prepend + s
		}, nil
	}
	return nil, fmt.Errorf("不支持的文字规整：%s", n.Type)
}

func isSpace(r rune) bool { return unicode.IsSpace(r) }

func (m *Model) preTokenizer(raw json.RawMessage) error {
	var p struct {
		Type          string            `json:"type"`
		Replacement   string            `json:"replacement"`
		PrependScheme string            `json:"prepend_scheme"`
		AddPrefix     *bool             `json:"add_prefix_space"`
		Split         *bool             `json:"split"`
		PreTokenizers []json.RawMessage `json:"pretokenizers"`
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if p.Type != "Metaspace" {
		return fmt.Errorf("不支持的预切分：%s", p.Type)
	}
	m.metaspace = true
	m.space = p.Replacement
	m.split = p.Split == nil || *p.Split
	return nil
}

// Normalize is the text as the tokenizer sees it.
func (m *Model) Normalize(s string) string { return m.normalize(s) }

// words are the parts of a normalized text the tokenizer cuts.
func (m *Model) words(s string) []string {
	if s == "" {
		return nil
	}
	if !m.metaspace {
		return []string{s}
	}
	s = strings.ReplaceAll(s, " ", m.space)
	if !strings.HasPrefix(s, m.space) {
		s = m.space + s
	}
	if !m.split {
		return []string{s}
	}
	var out []string
	start := 0
	for i := range s {
		if i > start && strings.HasPrefix(s[i:], m.space) {
			out = append(out, s[start:i])
			start = i
		}
	}
	return append(out, s[start:])
}

// Tokens cuts a text into the model's tokens, -1 for what none covers.
func (m *Model) Tokens(text string) []int {
	var out []int
	for _, w := range m.words(m.normalize(text)) {
		out = append(out, m.viterbi(w)...)
		if len(out) >= MaxTokens {
			return out[:MaxTokens]
		}
	}
	return out
}

// Pieces are the tokens of a text, as strings, "<unk>" for unknown ones.
func (m *Model) Pieces(text string) []string {
	inv := make(map[int32]string, len(m.ids))
	for p, id := range m.ids {
		inv[id] = p
	}
	var out []string
	for _, id := range m.Tokens(text) {
		if id < 0 || id == m.unk {
			out = append(out, "<unk>")
		} else {
			out = append(out, inv[int32(id)])
		}
	}
	return out
}

// viterbi cuts a word into the pieces whose scores sum highest, as the
// Unigram model does. A character no piece covers is an unknown token of
// a low score, and runs of them are one.
func (m *Model) viterbi(w string) []int {
	runes := []rune(w)
	n := len(runes)
	// Byte offsets of the runes, to cut the word without allocating runes.
	offs := make([]int, n+1)
	o := 0
	for i, r := range runes {
		offs[i] = o
		o += utf8.RuneLen(r)
	}
	offs[n] = o
	unkScore := m.minS - 10
	best := make([]float32, n+1)
	from := make([]int, n+1)
	id := make([]int, n+1)
	for i := 1; i <= n; i++ {
		best[i] = float32(math.Inf(-1))
	}
	for i := 0; i < n; i++ {
		if math.IsInf(float64(best[i]), -1) {
			continue
		}
		found := false
		for l := 1; l <= m.maxLen && i+l <= n; l++ {
			p, ok := m.ids[w[offs[i]:offs[i+l]]]
			if !ok {
				continue
			}
			if l == 1 {
				found = true
			}
			if s := best[i] + m.scores[p]; s > best[i+l] {
				best[i+l], from[i+l], id[i+l] = s, i, int(p)
			}
		}
		if !found { // the character as unknown
			if s := best[i] + unkScore; s > best[i+1] {
				best[i+1], from[i+1], id[i+1] = s, i, -1
			}
		}
	}
	var out []int
	for i := n; i > 0; i = from[i] {
		out = append(out, id[i])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	// Runs of unknown characters are one token.
	merged := out[:0]
	for _, t := range out {
		if t == -1 && len(merged) > 0 && merged[len(merged)-1] == -1 {
			continue
		}
		merged = append(merged, t)
	}
	return merged
}

// Embed is a text's vector: the mean of its known tokens' vectors, of
// length 1; nil when no token is known.
func (m *Model) Embed(text string) []float32 {
	v := make([]float32, m.dim)
	count := 0
	for _, t := range m.Tokens(text) {
		if t < 0 || t == m.unk {
			continue
		}
		count++
		s := m.scales[t]
		row := m.vecs[t*m.dim : (t+1)*m.dim]
		for d, q := range row {
			v[d] += float32(q) * s
		}
	}
	if count == 0 {
		return nil
	}
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return nil
	}
	k := float32(1 / math.Sqrt(sum))
	for d := range v {
		v[d] *= k
	}
	return v
}
