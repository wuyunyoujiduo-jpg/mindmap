package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
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
	// The topics to choose from: of every other map, its central topic
	// and the two levels below it.
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
	if len(topics) == 0 {
		a.tell("还没有可以归位的导图")
		return
	}
	var b strings.Builder
	b.WriteString("任务：\n")
	for i, it := range tasks {
		fmt.Fprintf(&b, "%d. %s\n", i+1, it.node.Title)
	}
	b.WriteString("\n主题（编号 路径）：\n")
	for i, p := range topics {
		fmt.Fprintf(&b, "t%d %s\n", i+1, p.label)
	}
	b.WriteString("\n回答 JSON：{\"places\":[{\"task\":任务序号,\"topic\":\"主题编号\"}]}，拿不准的任务不要列出。")
	a.tasks.placing, a.tasks.placeErr = true, ""
	cfg, send, sendDS, prompt := a.cfg, a.sendAI, a.sendDeepSeek, b.String()
	a.async(func() (any, error) {
		return askJSON(context.Background(), cfg, send, sendDS, placeSystem, prompt)
	}, func(v any, err error) {
		a.tasks.placing = false
		if err != nil {
			a.tasks.placeErr = "AI 没有给出建议：" + err.Error()
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
			a.tasks.placeErr = "AI 没有按格式回答，请重试"
			return
		}
		a.tasks.places = map[string]taskPlace{}
		for _, p := range out.Places {
			var k int
			if _, err := fmt.Sscanf(p.Topic, "t%d", &k); err != nil || k < 1 || k > len(topics) || p.Task < 1 || p.Task > len(tasks) {
				continue // made up
			}
			a.tasks.places[tasks[p.Task-1].node.ID] = topics[k-1]
		}
		if len(a.tasks.places) == 0 {
			a.tasks.placeErr = "AI 觉得这些任务都不属于现有的主题，可以右键「移到导图…」自己放"
		}
	})
}

// takePlace moves a task where the AI suggested.
func (a *app) takePlace(it taskItem) {
	p, ok := a.tasks.places[it.node.ID]
	if !ok {
		return
	}
	delete(a.tasks.places, it.node.ID)
	if err := a.moveNodeAcross(it.path, it.node.ID, p.path, p.node); err != nil {
		a.tell("没有移过去：" + err.Error())
		return
	}
	a.tell("已放到「" + p.label + "」下")
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
	ui.Row(c.Key("place-bar")).Padding(6, 4, 2).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
		switch {
		case a.tasks.placing:
			ui.Spinner(c)
			ui.Text(c, "AI 正在看这些任务该放到哪…").FontSize(textSmall).TextColor(t.TextMuted)
		case len(a.tasks.places) > 0:
			ui.Text(c, fmt.Sprintf("AI 建议了 %d 个任务的去处，逐个确认，或", len(a.tasks.places))).FontSize(textSmall).TextColor(t.TextMuted)
			if ghostButton(c, "全部放过去", "按建议把这些任务移到对应的主题下").Clicked() {
				a.later(a.takeAllPlaces)
			}
			if addButton(c, "不用了", "不采纳这些建议").Clicked() {
				a.tasks.places = nil
			}
		default:
			if a.tasks.placeErr != "" {
				ui.Text(c, a.tasks.placeErr).FontSize(textSmall).TextColor(t.TextMuted).Shrink(1).MinWidth(0).Wrap()
			} else {
				ui.Text(c, "记下的任务先在这里，再放进导图。").FontSize(textSmall).TextColor(t.TextMuted)
			}
			ui.Spacer(c)
			if iconTextButton(c, iconSparkle, "AI 归位", "AI 归位", "让 AI 建议每个任务该放到哪张导图的哪个主题下，你确认后才移动", false).Clicked() {
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
