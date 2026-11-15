package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/diagram"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// 图解: a topic's branch drawn by the AI to be understood by (a flow, the
// states of a thing, a table of what is decided when, the nouns of the
// matter), read three ways: told step by step, walked with a case, asked
// back. A tree holds what is under what, and not what comes after what,
// what turns into what, or what is decided when: those are drawn from it.
//
// A set is a view of the map, never a thing of its own: it is drawn from
// a topic, says for each box the topics it came from, tells when they
// have changed since, and is drawn again, not edited. It is kept beside
// the library, in .library/diagrams/, a file a topic, named by the
// topic's ID, which is the same on every canvas of a map and wherever the
// map is moved: writing one touches no map. As the analyses of the maps,
// the newer of two wins in the sync (ghsync's newerClip).

// diaFile is the file of a topic's set.
type diaFile struct {
	Path    string    `json:"path"` // of the map, relative to the maps folder, with slashes
	Updated time.Time `json:"updated"`
	diagram.Set
}

// The ways a set is read.
const (
	diaRead = iota
	diaTell // told step by step
	diaCase // walked with a case
	diaQuiz // asked back: the table's results covered
)

// diaState is what of a set shows.
type diaState struct {
	node string // the topic whose set shows
	kind string // "flow", "state", "table" or "objects"
	// pick is what is chosen in it: a node of the flow or of the states by
	// its ID, or "row:2", "note:1", "obj:3", "gap:1" (from 1).
	pick string
	way  int
	step int          // of the telling
	cas  int          // the case walked
	open map[int]bool // the rows shown, asked back (from 1)
	got  map[int]bool // those answered, and whether rightly
	cur  int          // the row asked about
	told string       // the line to the AI
}

func (a *app) diaDir() string { return filepath.Join(a.dir, ".library", "diagrams") }

// loadDias reads the sets again when their folder changed: when one was
// written, or the sync brought some.
func (a *app) loadDias() {
	// Looked at once a second, not at every topic of every frame: what
	// the app writes itself it knows of at once (writeDia).
	if a.dias != nil && a.diaFor == a.dir && time.Since(a.diaLooked) < time.Second {
		return
	}
	a.diaLooked = time.Now()
	dir := a.diaDir()
	mt := modTime(dir)
	if a.dias != nil && mt.Equal(a.diaMod) && a.diaFor == a.dir {
		return
	}
	a.diaMod, a.diaFor = mt, a.dir
	a.dias = map[string]*diaFile{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var f diaFile
		if json.Unmarshal(data, &f) != nil || f.Node == "" || f.Path == "" {
			continue
		}
		// As an answer is: a file the sync brought, or of another
		// version, is not drawn as it is.
		f.Clean(func(id string) (string, bool) { return id, id != "" })
		if !f.Empty() {
			a.dias[f.Node] = &f
		}
	}
	a.diaVersion++
}

// diaName is the file of a topic's set: named by the topic's ID, which is
// a few letters and digits; one of a map written elsewhere that is not,
// by a hash of it, so that no ID names a file outside the folder.
func diaName(node string) string {
	if safeID.MatchString(node) {
		return node + ".json"
	}
	h := sha1.Sum([]byte(node))
	return "x" + hex.EncodeToString(h[:10]) + ".json"
}

var safeID = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)

// writeDia keeps a topic's set.
func (a *app) writeDia(f *diaFile) error {
	dir := a.diaDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f.Updated = a.now()
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	file := filepath.Join(dir, diaName(f.Node))
	if err := os.WriteFile(file+".tmp", data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(file+".tmp", file); err != nil {
		return err
	}
	a.loadDias()
	a.dias[f.Node] = f
	a.diaMod = modTime(dir)
	a.diaVersion++
	a.syncChange()
	return nil
}

// dropDia forgets a topic's set.
func (a *app) dropDia(node string) {
	if err := os.Remove(filepath.Join(a.diaDir(), diaName(node))); err == nil {
		a.loadDias()
		delete(a.dias, node)
		a.diaMod = modTime(a.diaDir())
		a.diaVersion++
		a.syncChange()
	}
}

// diaIDs are the topics of the open map that have a set, worked out again
// when the sets or the map change.
func (a *app) diaIDs() map[string]*diaFile {
	a.loadDias()
	if a.file == nil {
		return nil
	}
	if key := fmt.Sprint(a.path, a.diaVersion, a.edits); key != a.diaIDsKey {
		a.diaIDsKey, a.diaIDsOf = key, map[string]*diaFile{}
		mk := a.mapKey(a.path)
		a.file.Walk(func(n *model.Node) {
			if f := a.dias[n.ID]; f != nil && f.Path == mk {
				a.diaIDsOf[n.ID] = f
			}
		})
	}
	return a.diaIDsOf
}

// diaOf is the set of a topic of the open map, or nil.
func (a *app) diaOf(node string) *diaFile { return a.diaIDs()[node] }

// mapDias are the open map's sets, by canvas and then as their topics
// come.
func (a *app) mapDias() []*diaFile {
	ids := a.diaIDs()
	var out []*diaFile
	if a.file != nil && len(ids) > 0 {
		a.file.Walk(func(n *model.Node) {
			if f := ids[n.ID]; f != nil {
				out = append(out, f)
			}
		})
	}
	return out
}

// adoptDias gives the map just opened the sets of its topics that a map
// no longer there left: the map renamed or moved outside the app, or a
// branch moved from a map since deleted.
func (a *app) adoptDias() {
	a.loadDias()
	if a.file == nil || len(a.dias) == 0 {
		return
	}
	key := a.mapKey(a.path)
	a.file.Walk(func(n *model.Node) {
		f := a.dias[n.ID]
		if f == nil || f.Path == key {
			return
		}
		if _, err := os.Stat(filepath.Join(a.dir, filepath.FromSlash(f.Path))); err == nil {
			return // still there: a copy, which keeps its own
		}
		f.Path = key
		_ = a.writeDia(f)
	})
}

// moveDias has the sets follow the maps moved: follow gives a path's new
// place.
func (a *app) moveDias(follow func(string) string) {
	a.loadDias()
	for _, f := range a.dias {
		from := filepath.Join(a.dir, filepath.FromSlash(f.Path))
		if to := follow(from); to != from {
			f.Path = a.mapKey(to)
			_ = a.writeDia(f)
		}
	}
}

// dropDiasOf forgets the sets of a map deleted.
func (a *app) dropDiasOf(path string) {
	a.loadDias()
	key := a.mapKey(path)
	for id, f := range a.dias {
		if f.Path == key {
			a.dropDia(id)
		}
	}
}

// branchSeen is a hash of each topic of a branch, by its ID: of where it
// is (under what, and which of its children), its title and its note; and
// one of the lines drawn between its topics. A flow follows the order of
// the topics, so a topic moved counts as changed.
func branchSeen(d *model.Document, root *model.Node) map[string]string {
	seen := map[string]string{}
	sum := func(parts ...string) string {
		h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
		return hex.EncodeToString(h[:6])
	}
	var walk func(n *model.Node, parent string, i int)
	walk = func(n *model.Node, parent string, i int) {
		seen[n.ID] = sum(parent, strconv.Itoa(i), n.Title, n.Note)
		for k, c := range n.Children {
			walk(c, n.ID, k)
		}
		if t := n.SummaryTopic(); t != nil {
			walk(t, n.ID, -1)
		}
	}
	walk(root, "", 0)
	var rels []string
	if d != nil {
		for _, r := range d.Relations {
			if seen[r.From] != "" && seen[r.To] != "" {
				rels = append(rels, r.From+">"+r.To+":"+r.Label)
			}
		}
	}
	if len(rels) > 0 {
		sort.Strings(rels)
		seen["~relations"] = sum(rels...)
	}
	return seen
}

// diaChange is what the map says otherwise than when a set was drawn.
type diaChange struct {
	changed map[string]bool // the topics changed or gone
	n       int             // how many changed, came or went
	first   string          // the title of one of them
}

// diaChanges tells what of a set's branch changed since it was drawn.
func (a *app) diaChanges(f *diaFile) diaChange {
	key := fmt.Sprintf("%q %d %d %q", f.Node, a.diaVersion, a.edits, a.path) // a set drawn again is another
	if key == a.diaChangeKey {
		return a.diaChanged
	}
	ch := diaChange{changed: map[string]bool{}}
	if n := a.file.Node(f.Node); n != nil && len(f.Seen) > 0 {
		now := branchSeen(a.file.SheetOf(f.Node), n)
		name := func(id string) string {
			if m := a.file.Node(id); m != nil {
				return m.Title
			}
			return ""
		}
		for id, h := range f.Seen {
			if now[id] != h {
				ch.changed[id] = true
				ch.n++
				if t := name(id); ch.first == "" && t != "" {
					ch.first = t
				}
			}
		}
		for id := range now {
			if _, was := f.Seen[id]; !was {
				ch.n++
				if t := name(id); ch.first == "" && t != "" {
					ch.first = t
				}
			}
		}
	}
	a.diaChangeKey, a.diaChanged = key, ch
	return ch
}

// linkToken is a link as a thought or a note keeps it.
var linkToken = regexp.MustCompile(`<(clip|mindmap):[^>\s]*>`)

// diaOutline is a branch as the AI is given it: a line a topic, led by a
// number the answer names it by, its note under it. What looks like a key
// or a password in it is sent as dots.
func diaOutline(root *model.Node) (string, map[string]string) {
	alias := map[string]string{}
	var b strings.Builder
	i := 0
	flat := func(s string) string { return strings.Join(strings.Fields(linkToken.ReplaceAllString(s, "")), " ") }
	var walk func(n *model.Node, depth int)
	walk = func(n *model.Node, depth int) {
		if b.Len() > 30000 {
			return
		}
		i++
		alias[strconv.Itoa(i)] = n.ID
		fmt.Fprintf(&b, "%s[%d] %s", strings.Repeat("  ", depth), i, flat(n.Title))
		if n.Task != nil {
			b.WriteString("［" + n.Task.Status.Label() + "］")
		}
		if len(n.Tags) > 0 {
			b.WriteString(" #" + strings.Join(n.Tags, " #"))
		}
		b.WriteString("\n")
		if note := flat(n.Note); note != "" {
			if r := []rune(note); len(r) > 1500 {
				note = string(r[:1500]) + "（后面还有，没给你）"
			}
			b.WriteString(strings.Repeat("  ", depth+1) + "备注：" + note + "\n")
		}
		for _, c := range n.Children {
			walk(c, depth+1)
		}
		if t := n.SummaryTopic(); t != nil {
			walk(t, depth+1)
		}
	}
	walk(root, 0)
	return library.Mask(b.String()), alias
}

const diaSystem = `你把思维导图里的一支内容画成帮人理解的图解。只根据给你的内容画，原文没说的不要补；发现没说清、或互相矛盾的地方，写进 gaps，不要替用户决定。
只回答一个 JSON 对象，不要任何别的文字。`

const diaAsk = `回答是一个对象，键是 flow、state、table、objects、cases、gaps 这六个里有内容的那些：哪样内容里没有，就省略那个键，不要硬凑，也不要给空值。

1. flow：流程。内容里有先后步骤时才画。
{"lanes":["角色或部门"],"nodes":[{"id":"n1","kind":"start","title":"…","sub":"…","lane":0,"src":[1]}],"edges":[{"from":"n1","to":"n2","label":"…"}],"tell":[{"nodes":["n1"],"say":"…"}]}
lanes 最多 5 个，没有角色之分就给空数组，这时每个节点的 lane 都写 0；lane 是 lanes 的下标。kind：start 起点（只一个）、step 步骤、decision 判断（title 用问句，出口的 label 写 是/否 或条件）、end 中途结束、ok 正常结束。title 不超过 8 个字，sub 可省，是不超过 12 个字的补充；edges 的 label 不超过 6 个字。最多 14 个节点：放不下时，把几个被拒绝的出口并成一个 end，把细节留给 table。可以有往回走的边（比如驳回后重来）。tell 按顺序把流程讲一遍，每步一句话，nodes 是这一步讲到的节点。

2. state：一个东西（单据、任务、账号……）有哪些状态、怎么变。内容里有状态时才画。
{"nodes":[{"id":"s0","kind":"start"},{"id":"s1","title":"状态名","kind":"","src":[2]}],"edges":[{"from":"s0","to":"s1","label":"什么事让它变"}]}
s0 是开始之前的圆点。kind：空是普通状态，ok 是圆满结束，bad 是被拒绝，end 是其他结束。label 不超过 6 个字。最多 9 个状态。不要画从一个状态回到它自己的边。

3. table：判定表。内容里有“什么情况下怎么办”的规则时才画。
{"conds":["条件名"],"results":["结果名"],"rows":[{"when":["每个条件的取值"],"then":["每个结果"],"src":[3],"gap":false,"why":""}],"notes":[{"text":"表里放不下的规则","src":[4]}]}
conds 最多 4 个，results 最多 2 个，rows 最多 10 行；每格不超过 12 个字。不限的条件写“不限”。只列结果不同的组合，用“不限”把结果相同的并成一行，不必列出每一种排列。导图没说怎么办、而又确实会遇到的组合也列出来，gap 设为 true，then 留空，why 用一句话说哪里没说清。条件放不进 4 个的规则写进 notes。

4. objects：这一支里的名词：角色、单据、状态、系统、关键的量。
[{"name":"名词","kind":"角色","what":"一句话：它是什么，在这里起什么作用","aliases":["导图里对它的其他叫法"]}]
kind 从 角色、单据、数据、状态、系统、规则、概念 里选最贴近的。最多 12 个，只收导图里真的出现的词，至少两个字。

5. cases：3 到 5 个具体例子，用来把上面的图走一遍：每条主要规则一个，再加规则碰到一起或没说清的地方。
[{"title":"不超过 10 个字","facts":["这个例子的具体情况，一项一条"],"flow":["依次经过的 flow 节点 id"],"states":["依次经过的 state 节点 id，不含 s0"],"row":1,"result":"一句话结果","gap":false}]
row 是命中 table 的第几行，从 1 数，没有就写 0；没画 state 时 states 给空数组；flow 里可以重复经过同一个节点。走到导图没说清的地方走不下去的例子，gap 设为 true。

6. gaps：导图里没说清、或互相矛盾的地方。
[{"say":"一句话","src":[5],"row":0}]
没说清的和互相矛盾的都写在这里，矛盾的在句子里说明是哪两处矛盾。row 是对应 table 里 gap 为 true 的那一行，没有就写 0。备注末尾写着“（后面还有，没给你）”是内容太长被截掉了，不算没说清。

src 是这个节点、这一行依据的主题的编号（上面方括号里的数字），只能用出现过的编号。`

// diaPrompt is what the AI is given to draw a branch from.
func (a *app) diaPrompt(n *model.Node, told string) (string, map[string]string) {
	text, alias := diaOutline(n)
	var b strings.Builder
	fmt.Fprintf(&b, "下面是导图「%s」里「%s」这一支。每行开头方括号里的数字是这个主题的编号，备注跟在主题下面。\n\n%s\n", mapName(a.path), n.Title, text)
	if told = strings.TrimSpace(told); told != "" {
		b.WriteString("用户对这次画法的要求：" + told + "\n\n")
	}
	b.WriteString(diaAsk)
	return b.String(), alias
}

// askLongJSON asks the provider chosen for a long answer that is a JSON
// object, and returns its text: askJSON with room for a whole set, and
// told when the answer was cut short.
func askLongJSON(ctx context.Context, cfg config, send aiSender, sendDS deepseekSender, system, prompt string) (string, error) {
	if cfg.AIProvider == providerDeepSeek {
		resp, err := sendDS(ctx, cfg, dsRequest{Model: deepseekModel(cfg), MaxTokens: 16000, Messages: []dsMessage{
			{Role: "system", Content: system}, {Role: "user", Content: prompt},
		}, ResponseFormat: &dsFormat{Type: "json_object"}}, nil)
		if err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", plainError("AI 没有回答")
		}
		if resp.Choices[0].FinishReason == "length" {
			return "", plainError("回答超出长度，没有写完：换一个小一些的分支再试")
		}
		return resp.Choices[0].Message.Content, nil
	}
	resp, err := send(ctx, cfg, anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(aiModel(cfg)),
		MaxTokens: 12000,
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(prompt))},
	}, nil)
	if err != nil {
		return "", err
	}
	switch resp.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return "", errors.New("请求被模型拒绝")
	case anthropic.BetaStopReasonMaxTokens:
		return "", plainError("回答超出长度，没有写完：换一个小一些的分支再试")
	}
	text := ""
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text += b.Text
		}
	}
	return text, nil
}

// diaWorth tells whether a topic has enough under it to be drawn.
func diaWorth(n *model.Node) bool {
	return n != nil && (len(n.Children) > 0 || len([]rune(n.Note)) >= 80)
}

// drawDia asks the AI to draw a topic's branch, again when it has a set:
// told is what the user asks of the drawing.
func (a *app) drawDia(node, told string) {
	n := a.file.Node(node)
	if n == nil {
		return
	}
	if a.diaBusy != "" {
		a.tell("正在画另一组图解，等它画完再试")
		return
	}
	if providerKey(a.cfg) == "" {
		a.tell("图解要用 AI：先在 AI 设置里选好模型")
		a.openKeySettings()
		return
	}
	prev := a.diaOf(node)
	if prev != nil && strings.TrimSpace(told) == "" && a.diaChanges(prev).n == 0 {
		a.tell("导图没变，图也不用重画") // the same would come of it
		return
	}
	doc := a.file.SheetOf(node)
	prompt, alias := a.diaPrompt(n, told)
	seen := branchSeen(doc, n)
	key, title := a.mapKey(a.path), n.Title
	dir, path := a.dir, a.path
	a.diaBusy, a.diaSince, a.diaErr, a.diaErrFor = node, a.now(), "", node
	cfg := a.cfg
	send, sendDS := a.sendAI, a.sendDeepSeek
	a.async(func() (any, error) {
		text, err := askLongJSON(context.Background(), cfg, send, sendDS, diaSystem, prompt)
		if err != nil {
			return nil, err
		}
		set, err := diagram.Parse(text)
		if err != nil {
			return nil, plainError("AI 没有按格式回答，请重试")
		}
		set.Clean(func(v string) (string, bool) { id, ok := alias[v]; return id, ok })
		if set.Empty() {
			return nil, plainError("这一支里没找到可以画的先后、状态或规则")
		}
		return set, nil
	}, func(v any, err error) {
		a.diaBusy = ""
		stageOver("diagram")
		if a.dir != dir { // another folder was turned to: its sets are not this one's
			return
		}
		set, _ := v.(*diagram.Set)
		if err != nil || set == nil {
			if err == nil {
				err = plainError("AI 没有回答")
			}
			a.diaErr = "没画成：" + aiError(err)
			return
		}
		set.Node, set.Title, set.Drawn, set.Seen, set.Told = node, title, a.now(), seen, strings.TrimSpace(told)
		f := &diaFile{Path: key, Set: *set}
		if err := a.writeDia(f); err != nil {
			a.diaErr = "保存图解失败：" + err.Error()
			return
		}
		// Shown at once only where the user waited for it: on its page,
		// or on its topic with nothing being written. Turned elsewhere
		// meanwhile, they are told, and go when they will.
		here := a.path == path && !a.libOpen && (a.mode == viewDiagram && a.dia.node == node ||
			a.mode == viewMap && a.selected == node && a.editing == "")
		switch {
		case here && prev != nil:
			a.openDia(node, "")
			was := *prev
			a.tellUndo("照现在的导图重画了", func() {
				if a.writeDia(&was) == nil && a.dia.node == node {
					a.openDia(node, "")
				}
			})
		case here:
			a.openDia(node, "")
		default:
			a.tellAct("「"+title+"」的图解画好了", "看图解", func() {
				if a.path != path && a.open(path) != nil {
					return
				}
				a.openDia(node, "")
			})
		}
	})
}

// openDia shows a topic's set, on the kind named or its first.
func (a *app) openDia(node, kind string) {
	f := a.diaOf(node)
	if f == nil {
		return
	}
	kinds := f.Kinds()
	if !slices.Contains(kinds, kind) {
		kind = kinds[0]
	}
	if a.editing != "" {
		a.commitEdit()
	}
	if d := a.file.SheetOf(node); d != nil && d != a.doc {
		a.showSheet(d.ID)
	}
	a.libOpen, a.mode = false, viewDiagram
	a.dia = diaState{node: node, kind: kind}
	a.diaFocus = true
}

// closeDia goes back to the map, on the set's topic.
func (a *app) closeDia() {
	node := a.dia.node
	a.dia = diaState{}
	if a.mode == viewDiagram {
		a.mode = viewMap
	}
	if a.doc != nil {
		if n, _ := a.doc.Find(node); n != nil {
			a.selectNode(node)
			a.reveal = true
		}
	}
}

// deleteDia forgets a topic's set, with the way back.
func (a *app) deleteDia(node string) {
	f := a.diaOf(node)
	if f == nil {
		return
	}
	was := *f
	if a.dia.node == node {
		a.closeDia()
	}
	a.dropDia(node)
	a.tellUndo("图解删掉了", func() { _ = a.writeDia(&was) })
}

// diaSource goes to a topic a set was drawn from, on whichever canvas of
// the map it is.
func (a *app) diaSource(id string) {
	d := a.file.SheetOf(id)
	if d == nil {
		a.tell("这个主题已经不在导图里了")
		return
	}
	a.dia = diaState{}
	a.goToNode(a.path, d.ID, id)
	a.reveal = true
}

// setDiaWay turns to a way of reading the set shown.
func (a *app) setDiaWay(way int) {
	f := a.diaOf(a.dia.node)
	if f == nil {
		return
	}
	a.dia.way, a.dia.pick, a.dia.step = way, "", 0
	switch way {
	case diaTell:
		a.dia.kind = "flow"
	case diaCase:
		if a.dia.kind == "objects" {
			a.dia.kind = f.Kinds()[0]
		}
		a.dia.cas = min(a.dia.cas, max(len(f.Cases)-1, 0))
	case diaQuiz:
		a.dia.kind, a.dia.open, a.dia.got, a.dia.cur = "table", map[int]bool{}, map[int]bool{}, 0
	}
}

// quizRows are the rows of a table asked back: those the map decides.
func quizRows(t *diagram.Table) []int {
	var rows []int
	if t != nil {
		for i, r := range t.Rows {
			if !r.Gap {
				rows = append(rows, i+1)
			}
		}
	}
	return rows
}

// The days after which a table is asked back again: sooner when a row was
// missed.
const (
	quizAgainMissed = 3
	quizAgainRight  = 7
)

// answerQuiz notes whether the row asked about was recalled, and, the
// last one answered, keeps how it went and when to ask again.
func (a *app) answerQuiz(right bool) {
	f := a.diaOf(a.dia.node)
	if f == nil || a.dia.cur == 0 {
		return
	}
	a.dia.got[a.dia.cur] = right
	rows := quizRows(f.Table)
	if len(a.dia.got) < len(rows) {
		return
	}
	q := &diagram.Quiz{At: a.now()}
	for _, r := range rows {
		if !a.dia.got[r] {
			q.Missed = append(q.Missed, r)
		}
	}
	days := quizAgainRight
	if len(q.Missed) > 0 {
		days = quizAgainMissed
	}
	q.Next = today(a.now()).AddDate(0, 0, days)
	f.Quiz = q
	_ = a.writeDia(f)
}

// quizDue tells whether a set's table is to be asked back again.
func (a *app) quizDue(f *diaFile) bool {
	return f != nil && f.Table != nil && f.Quiz != nil && !f.Quiz.Next.After(a.now())
}

// diasDue are the sets of every map whose table is to be asked back, the
// longest due first.
func (a *app) diasDue() []*diaFile {
	a.loadDias()
	key := fmt.Sprint(a.diaVersion, a.dir, len(a.files), today(a.now()).Unix())
	if key == a.diaDueKey {
		return a.diaDue
	}
	maps := map[string]bool{} // the maps there are, by their key
	for _, m := range a.files {
		maps[a.mapKey(m.Path)] = true
	}
	var out []*diaFile
	for _, f := range a.dias {
		if a.quizDue(f) && maps[f.Path] {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Quiz.Next.Equal(out[j].Quiz.Next) {
			return out[i].Quiz.Next.Before(out[j].Quiz.Next)
		}
		return out[i].Node < out[j].Node
	})
	a.diaDueKey, a.diaDue = key, out
	return out
}

// moveDiasTo has the sets of topics moved to another map follow them:
// the topics keep their IDs there.
func (a *app) moveDiasTo(nodes []*model.Node, path string) {
	a.loadDias()
	key := a.mapKey(path)
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if f := a.dias[n.ID]; f != nil && f.Path != key {
			f.Path = key
			_ = a.writeDia(f)
		}
		for _, c := range n.Children {
			walk(c)
		}
		if t := n.SummaryTopic(); t != nil {
			walk(t)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
}

// askBack opens a set of any map on its table, asked back.
func (a *app) askBack(f *diaFile) {
	path := filepath.Join(a.dir, filepath.FromSlash(f.Path))
	if path != a.path {
		if err := a.open(path); err != nil {
			return
		}
	}
	if a.diaOf(f.Node) == nil {
		a.tell("这组图解的主题已经不在导图里了")
		return
	}
	a.openDia(f.Node, "table")
	a.setDiaWay(diaQuiz)
}

// mention is a topic that speaks of a noun.
type mention struct {
	sheet *model.Document
	node  *model.Node
	path  string // the topics over it, as "退货退款 › 审核"
}

// minNoun is the least a noun is looked for by: a single character is in
// most topics.
const minNoun = 2

// mentions are the topics of every canvas of the open map that speak of a
// noun, by its name or another of its names, in their title or their
// note. They are looked up as the map is, so they are never out of date.
func (a *app) mentions(o diagram.Object) []mention {
	key := fmt.Sprintf("%q %d %q %q", a.path, a.edits, o.Name, o.Aliases)
	if m, ok := a.mentionOf[key]; ok {
		return m
	}
	var names []string
	for _, s := range append([]string{o.Name}, o.Aliases...) {
		if len([]rune(s)) >= minNoun {
			names = append(names, strings.ToLower(s))
		}
	}
	// Another name that most of the map's topics hold names nothing: only
	// the noun's own name is looked for then.
	topics := 0
	hits := make([]int, len(names))
	a.file.Walk(func(n *model.Node) {
		topics++
		text := strings.ToLower(n.Title + "\n" + n.Note)
		for i, s := range names {
			if strings.Contains(text, s) {
				hits[i]++
			}
		}
	})
	for i := len(names) - 1; i > 0; i-- {
		if topics > 8 && hits[i]*2 > topics {
			names = slices.Delete(names, i, i+1)
		}
	}
	var out []mention
	for _, d := range a.file.Sheets {
		var trail []string
		d.Walk(func(n, _ *model.Node, depth int) bool {
			trail = append(trail[:min(depth, len(trail))], n.Title)
			text := strings.ToLower(n.Title + "\n" + n.Note)
			if slices.ContainsFunc(names, func(s string) bool { return strings.Contains(text, s) }) {
				out = append(out, mention{d, n, strings.Join(trail[:len(trail)-1], " › ")})
			}
			return true
		})
	}
	if a.mentionOf == nil || len(a.mentionOf) > 64 {
		a.mentionOf = map[string][]mention{}
	}
	a.mentionOf[key] = out
	return out
}
