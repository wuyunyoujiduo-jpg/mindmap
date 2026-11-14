package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Sorting out the inbox: what is written down quickly lands there, and
// the AI says, for each task, the topic of a map it belongs under. Each
// suggestion is shown on its task, to take or to leave: nothing moves by
// itself.

// taskPlace is where the AI would put a task: a topic of a map.
type taskPlace struct {
	path, sheet, node string
	label             string // 导图 › 主题 › 主题
}

const placeSystem = "你帮用户把收件箱里的任务归到合适的导图主题下。只根据给出的主题列表选择，拿不准的任务不要归。"

// suggestPlaces asks the AI where the open tasks of the inbox belong.
func (a *app) suggestPlaces() {
	if a.tasks.placing {
		return
	}
	if providerKey(a.cfg) == "" {
		a.tell("先在 AI 设置里选好模型")
		a.openKeySettings()
		return
	}
	var tasks []taskItem
	for _, it := range a.taskItems() {
		if it.path == a.inboxPath() && !it.done() {
			tasks = append(tasks, it)
		}
	}
	if len(tasks) == 0 {
		return
	}
	topics := a.placeTopics()
	if len(topics) == 0 {
		a.tell("还没有可以归位的导图")
		return
	}
	titles := make([]string, len(tasks))
	for i, it := range tasks {
		titles[i] = it.node.Title
	}
	a.tasks.placing, a.tasks.placeErr = true, ""
	a.askPlaces("任务", titles, topics, func(got map[int]taskPlace, err error) {
		a.tasks.placing = false
		if err != nil {
			a.tasks.placeErr = err.Error()
			return
		}
		a.tasks.places = map[string]taskPlace{}
		for i, p := range got {
			a.tasks.places[tasks[i].node.ID] = p
		}
		if len(got) == 0 {
			a.tasks.placeErr = "AI 觉得这些任务都不属于现有的主题，可以右键「移到导图…」自己放"
		}
	})
}

// placeTopics are the topics the AI chooses from: of every map but the
// inbox, its central topic and the two levels below it.
func (a *app) placeTopics() []taskPlace {
	var topics []taskPlace
	for _, fi := range a.files {
		if fi.Path == a.inboxPath() {
			continue
		}
		f := a.file
		if fi.Path != a.path {
			f = a.indexed(fi.Path)
		}
		if f == nil {
			continue
		}
		for _, d := range f.Sheets {
			var walk func(n *model.Node, trail string, depth int)
			walk = func(n *model.Node, trail string, depth int) {
				if len(topics) >= 400 {
					return
				}
				label := trail + n.Title
				topics = append(topics, taskPlace{fi.Path, d.ID, n.ID, label})
				if depth < 2 {
					for _, ch := range n.Children {
						walk(ch, label+" › ", depth+1)
					}
				}
			}
			if d.Root != nil {
				walk(d.Root, mapName(fi.Path)+" › ", 0)
			}
		}
	}
	return topics
}

// askPlaces asks the AI which of the topics each of the items, of a kind
// said in what, belongs under; done gets the places by the items' indexes,
// those it was sure of only.
func (a *app) askPlaces(what string, items []string, topics []taskPlace, done func(map[int]taskPlace, error)) {
	var b strings.Builder
	b.WriteString(what + "：\n")
	for i, it := range items {
		fmt.Fprintf(&b, "%d. %s\n", i+1, it)
	}
	b.WriteString("\n主题（编号 路径）：\n")
	for i, p := range topics {
		fmt.Fprintf(&b, "t%d %s\n", i+1, p.label)
	}
	b.WriteString("\n回答 JSON：{\"places\":[{\"task\":序号,\"topic\":\"主题编号\"}]}，拿不准的不要列出。")
	cfg, send, sendDS, prompt := a.cfg, a.sendAI, a.sendDeepSeek, b.String()
	system := strings.ReplaceAll(placeSystem, "任务", what)
	a.async(func() (any, error) {
		return askJSON(context.Background(), cfg, send, sendDS, system, prompt)
	}, func(v any, err error) {
		if err != nil {
			done(nil, errors.New("AI 没有给出建议："+err.Error()))
			return
		}
		text, _ := v.(string)
		var out struct {
			Places []struct {
				Task  int    `json:"task"`
				Topic string `json:"topic"`
			} `json:"places"`
		}
		i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
		if i < 0 || j < i || json.Unmarshal([]byte(text[i:j+1]), &out) != nil {
			done(nil, errors.New("AI 没有按格式回答，请重试"))
			return
		}
		got := map[int]taskPlace{}
		for _, p := range out.Places {
			var k int
			if _, err := fmt.Sscanf(p.Topic, "t%d", &k); err != nil || k < 1 || k > len(topics) || p.Task < 1 || p.Task > len(items) {
				continue // made up
			}
			got[p.Task-1] = topics[k-1]
		}
		done(got, nil)
	})
}

// takePlace moves a task where the AI suggested.
func (a *app) takePlace(it taskItem) {
	p, ok := a.tasks.places[it.node.ID]
	if !ok {
		return
	}
	delete(a.tasks.places, it.node.ID)
	a.moveTaskUndoably(it, p.path, p.node, p.label)
}

// moveTaskUndoably moves a task under a topic of a map and says so, with
// the way to take it back: under the topic it was under before.
func (a *app) moveTaskUndoably(it taskItem, toPath, toID, label string) {
	was := ""
	src := a.file
	if it.path != a.path {
		src = a.indexed(it.path)
	}
	if src != nil {
		if d := src.SheetOf(it.node.ID); d != nil {
			if _, parent := d.Find(it.node.ID); parent != nil {
				was = parent.ID
			}
		}
	}
	if err := a.moveNodeAcross(it.path, it.node.ID, toPath, toID); err != nil {
		a.tell("没有移过去：" + err.Error())
		return
	}
	said := "已放到「" + label + "」下"
	if was == "" {
		a.tell(said)
		return
	}
	from, id := it.path, it.node.ID
	a.tellUndo(said, func() {
		if err := a.moveNodeAcross(toPath, id, from, was); err != nil {
			a.tell("没有放回去：" + err.Error())
		}
	})
}

// takeAllPlaces moves every task with a suggestion.
func (a *app) takeAllPlaces() {
	n := 0
	for _, it := range a.taskItems() {
		if p, ok := a.tasks.places[it.node.ID]; ok {
			if a.moveNodeAcross(it.path, it.node.ID, p.path, p.node) == nil {
				n++
			}
		}
	}
	a.tasks.places = nil
	a.tasks.read = a.tasks.read.AddDate(-1, 0, 0)
	a.tell(fmt.Sprintf("已归位 %d 个任务", n))
}

// moveNodeAcross moves a node, with what is below it, under a node of
// another map, or of its own, each map saved as an edit of it. The node
// keeps its ID, so that what links to it follows.
func (a *app) moveNodeAcross(fromPath, id, toPath, toID string) error {
	src := a.file
	if fromPath != a.path {
		src = a.indexed(fromPath)
	}
	if src == nil || src.Node(id) == nil {
		return errors.New("任务已不存在")
	}
	n := src.Node(id)
	if n.Contains(toID) {
		return errors.New("不能放到它自己下面")
	}
	c := n.Clone()
	if fromPath != toPath {
		copyAssets(c, model.AssetsDir(fromPath), model.AssetsDir(toPath))
	}
	take := func(f *model.File) error {
		d := f.SheetOf(id)
		if d == nil {
			return errors.New("任务已不存在")
		}
		_, err := d.Delete(id)
		return err
	}
	put := func(f *model.File) error {
		d := f.SheetOf(toID)
		if d == nil {
			return errors.New("目标主题已不存在")
		}
		target, _ := d.Find(toID)
		target.Children = append(target.Children, c)
		target.Collapsed = false
		return nil
	}
	defer func() { a.tasks.read = a.tasks.read.AddDate(-1, 0, 0) }()
	if fromPath == toPath {
		return a.editMapAt(fromPath, "移动任务", func(f *model.File) error {
			if err := take(f); err != nil {
				return err
			}
			return put(f)
		})
	}
	if err := a.editMapAt(toPath, "从「"+mapName(fromPath)+"」移来", put); err != nil {
		return err
	}
	return a.editMapAt(fromPath, "移到「"+mapName(toPath)+"」", take)
}

// placeBar is the inbox's line of the AI: a button that asks it where
// the tasks belong, and, once it answered, one that takes all it said.
func (a *app) placeBar(c *ui.Context, open int) {
	if open == 0 {
		return
	}
	t := c.Theme()
	if !a.tasks.placing {
		stageOver("place-tasks")
	}
	ui.Row(c.Key("place-bar")).Padding(6, 4, 2).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
		switch {
		case a.tasks.placing:
			// What it is doing, by its steps: a wait that cannot be counted.
			stageLines(c, "place-tasks", []string{"读了收件箱的任务和各导图的主题"}, "AI 正在给每个任务找去处…")
		case len(a.tasks.places) > 0:
			ui.Text(c, fmt.Sprintf("AI 建议了 %d 个任务的去处，逐个确认，或", len(a.tasks.places))).FontSize(textSmall).TextColor(t.TextMuted)
			if ghostButton(c, "全部放过去", "按建议把这些任务移到对应的主题下").Clicked() {
				a.later(a.takeAllPlaces)
			}
			if addButton(c, "不用了", "不采纳这些建议").Clicked() {
				a.tasks.places = nil
			}
		default:
			again := "AI 归位"
			if a.tasks.placeErr != "" {
				col := t.TextMuted
				if placeFailed(a.tasks.placeErr) { // it failed: said here, with the way to try again
					col, again = t.Danger, "重试"
				}
				ui.Text(c, a.tasks.placeErr).FontSize(textSmall).TextColor(col).Shrink(1).MinWidth(0).Wrap()
			} else {
				ui.Text(c, "记下的任务先在这里，再放进导图。").FontSize(textSmall).TextColor(t.TextMuted)
			}
			ui.Spacer(c)
			if sparkKey(c, again, "让 AI 建议每个任务该放到哪张导图的哪个主题下，你确认后才移动").Clicked() {
				a.later(a.suggestPlaces)
			}
		}
	})
}

// askJSON asks the configured model a question to answer in JSON, and
// gives its text.
func askJSON(ctx context.Context, cfg config, send aiSender, sendDS deepseekSender, system, prompt string) (string, error) {
	if cfg.AIProvider == providerDeepSeek {
		resp, err := sendDS(ctx, cfg, dsRequest{Model: deepseekModel(cfg), MaxTokens: 16000, Messages: []dsMessage{
			{Role: "system", Content: system}, {Role: "user", Content: prompt},
		}, ResponseFormat: &dsFormat{Type: "json_object"}}, nil)
		if err != nil {
			return "", err
		}
		return resp.Choices[0].Message.Content, nil
	}
	resp, err := send(ctx, cfg, anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(aiModel(cfg)),
		MaxTokens: 4000,
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(prompt))},
	}, nil)
	if err != nil {
		return "", err
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return "", errors.New("请求被模型拒绝")
	}
	var text string
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text += b.Text
		}
	}
	return text, nil
}

// The same for 灵感 and the library: the AI says the topic each note or
// clip not yet on a map belongs under; taken, a topic of it is added
// there, linked to it.

// suggestClipPlaces asks where the clips belong: those of them no topic
// links to yet, the first 40.
func (a *app) suggestClipPlaces(clips []*library.Clip) {
	if a.clipPlacing {
		return
	}
	if providerKey(a.cfg) == "" {
		a.tell("先在 AI 设置里选好模型")
		a.openKeySettings()
		return
	}
	var loose []*library.Clip
	for _, cl := range clips {
		if len(a.clipBacklinks(cl.ID)) == 0 && len(loose) < 40 {
			loose = append(loose, cl)
		}
	}
	if len(loose) == 0 {
		a.clipPlaceErr = "这些都已经在导图里了"
		return
	}
	topics := a.placeTopics()
	if len(topics) == 0 {
		a.tell("还没有可以归位的导图")
		return
	}
	items := make([]string, len(loose))
	for i, cl := range loose {
		text := cl.Label()
		for _, more := range []string{cl.Summary, cl.Desc, cl.Text} {
			if more = strings.TrimSpace(more); more != "" && more != text {
				if r := []rune(more); len(r) > 120 {
					more = string(r[:120])
				}
				text += "（" + strings.ReplaceAll(more, "\n", " ") + "）"
				break
			}
		}
		items[i] = text
	}
	a.clipPlacing, a.clipPlaceErr = true, ""
	a.askPlaces("资料", items, topics, func(got map[int]taskPlace, err error) {
		a.clipPlacing = false
		if err != nil {
			a.clipPlaceErr = err.Error()
			return
		}
		a.clipPlaces = map[string]taskPlace{}
		for i, p := range got {
			a.clipPlaces[loose[i].ID] = p
		}
		if len(got) == 0 {
			a.clipPlaceErr = "AI 觉得这些都不属于现有的主题"
		}
	})
}

// takeClipPlace adds a topic of the clip where the AI suggested, linked
// to it; the clip stays in the library.
func (a *app) takeClipPlace(cl *library.Clip) bool {
	p, ok := a.clipPlaces[cl.ID]
	if !ok {
		return false
	}
	delete(a.clipPlaces, cl.ID)
	n := model.NewNode(firstSentence(cl.Label()))
	var note []string
	for _, s := range []string{cl.Desc, cl.Summary, cl.Note} {
		if s = strings.TrimSpace(s); s != "" {
			note = append(note, s)
		}
	}
	if cl.Kind == library.KindText && strings.TrimSpace(cl.Text) != n.Title {
		note = append(note, cl.Text)
	}
	n.Note = strings.Join(note, "\n\n")
	n.Links = []string{clipScheme + cl.ID}
	if cl.URL != "" {
		n.Links = append(n.Links, cl.URL)
	}
	err := a.editMapAt(p.path, "放进资料", func(f *model.File) error {
		d := f.SheetOf(p.node)
		if d == nil {
			return errors.New("目标主题已不存在")
		}
		target, _ := d.Find(p.node)
		target.Children = append(target.Children, n)
		target.Collapsed = false
		return nil
	})
	if err != nil {
		a.tell("没有放过去：" + err.Error())
		return false
	}
	// The way back: the topic made goes; the clip was never moved.
	path, id := p.path, n.ID
	a.tellUndo("已放到「"+p.label+"」下", func() {
		err := a.editMapAt(path, "撤回放进的资料", func(f *model.File) error {
			d := f.SheetOf(id)
			if d == nil {
				return errors.New("那个主题已不存在")
			}
			_, err := d.Delete(id)
			return err
		})
		if err != nil {
			a.tell("没有撤回：" + err.Error())
		}
	})
	return true
}

// clipPlaceBar is the line of the AI's suggestions over a list of clips:
// while it is asked, what it said, and the way to take all of it.
func (a *app) clipPlaceBar(c *ui.Context) {
	t := c.Theme()
	if !a.clipPlacing && len(a.clipPlaces) == 0 && a.clipPlaceErr == "" {
		return
	}
	if !a.clipPlacing {
		stageOver("place-clips")
	}
	ui.Row(c.Key("clip-place-bar")).Padding(4, 4).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
		switch {
		case a.clipPlacing:
			stageLines(c, "place-clips", []string{"读了这些资料和各导图的主题"}, "AI 正在给每条找去处…")
		case len(a.clipPlaces) > 0:
			ui.Text(c, fmt.Sprintf("AI 建议了 %d 条的去处，逐条确认，或", len(a.clipPlaces))).FontSize(textSmall).TextColor(t.TextMuted)
			if ghostButton(c, "全部放过去", "按建议在对应的主题下各加一个主题，链接到它").Clicked() {
				a.later(func() {
					n := 0
					for id := range a.clipPlaces {
						if cl := a.lib.Find(id); cl != nil && a.takeClipPlace(cl) {
							n++
						}
					}
					a.clipPlaces = nil
					a.tell(fmt.Sprintf("已归位 %d 条", n))
				})
			}
			if addButton(c, "不用了", "不采纳这些建议").Clicked() {
				a.clipPlaces = nil
			}
		default:
			col := t.TextMuted
			if placeFailed(a.clipPlaceErr) {
				col = t.Danger
			}
			ui.Text(c, a.clipPlaceErr).FontSize(textSmall).TextColor(col).Shrink(1).MinWidth(0).Wrap()
			if placeFailed(a.clipPlaceErr) && sparkKey(c, "重试", "再让 AI 看一遍").Clicked() {
				a.later(a.askAllPlaces)
			}
			if addButton(c, "知道了", "").Clicked() {
				a.clipPlaceErr = ""
			}
		}
	})
}

// clipPlaceLine is the AI's suggestion on a clip: where, and the buttons
// to take it or leave it.
func (a *app) clipPlaceLine(c *ui.Context, cl *library.Clip) {
	p, ok := a.clipPlaces[cl.ID]
	if !ok {
		return
	}
	t := c.Theme()
	ui.Row(c.Key("clip-place:" + cl.ID)).Gap(6).AlignItems(ui.Center).FillWidth().Children(func() {
		ui.Text(c, "→ "+p.label).FontSize(textSmall).TextColor(t.Accent).SingleLine().Shrink(1).MinWidth(0)
		if ghostButton(c.Key("take-clip:"+cl.ID), "放过去", "在「"+p.label+"」下加一个主题，链接到它").Label("放过去 " + cl.Label()).Clicked() {
			a.later(func() { a.takeClipPlace(cl) })
		}
		if iconButton(c.Key("skip-clip:"+cl.ID), iconClose, "不放 "+cl.Label(), "不采纳这个建议", 22, true).Clicked() {
			delete(a.clipPlaces, cl.ID)
		}
	})
}

// placeFailed tells whether what is said of the AI's places is a failure
// (the AI did not answer, or not as asked) and not an answer with nothing
// to place: a failure is said in the danger's color, with 重试.
func placeFailed(msg string) bool {
	return strings.HasPrefix(msg, "AI 没有")
}
