package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// Actions of the AI assistant beyond chatting: a report of the week, a
// task broken down, the risks of the map, a map from text, the
// architecture of a code repository. Each asks with what it needs besides
// the map; edits come back as proposals to confirm.

// aiAction sends a request as the user would, with context added.
func (a *app) aiAction(prompt string, scope int, extra string) {
	a.aiOpen = true
	a.ai.scope = scope
	a.ai.draft = prompt
	a.ai.extra = extra
	a.aiSend()
}

// changesSince describes what changed in the map from the version kept
// nearest before a time to now, and the tasks done since.
func (a *app) changesSince(since time.Time) string {
	var b strings.Builder
	vs := a.versions.List(a.path)
	var base *model.File
	for _, v := range vs { // newest first: the first older than since
		if !v.Time.After(since) {
			base, _ = a.versions.Load(v)
			break
		}
	}
	if base == nil && len(vs) > 0 {
		base, _ = a.versions.Load(vs[len(vs)-1]) // the oldest kept
	}
	if base != nil {
		changes := model.Diff(base, a.file)
		fmt.Fprintf(&b, "从 %s 到现在导图的变化（%s）：\n", since.Format("2006-01-02"), model.Summary(changes))
		for i, ch := range changes {
			if i == 150 {
				fmt.Fprintf(&b, "…还有 %d 项\n", len(changes)-i)
				break
			}
			fmt.Fprintf(&b, "- %s：%s", ch.Kind, ch.Title)
			if ch.Where != "" {
				fmt.Fprintf(&b, "（在 %s）", ch.Where)
			}
			if ch.Detail != "" {
				b.WriteString("，" + ch.Detail)
			}
			b.WriteString("\n")
		}
	}
	var done []string
	for _, tn := range a.file.Tasks() {
		if t := tn.Node.Task; t.Status == model.StatusDone && t.DoneAt != nil && t.DoneAt.After(since) {
			done = append(done, fmt.Sprintf("- %s（%s 完成）", tn.Node.Title, t.DoneAt.Format("1/2")))
		}
	}
	if len(done) > 0 {
		b.WriteString("\n期间完成的任务：\n" + strings.Join(done, "\n") + "\n")
	}
	if b.Len() == 0 {
		return "（没有找到这段时间的历史版本和完成记录。）"
	}
	return b.String()
}

// aiReport asks for a report of the last days.
func (a *app) aiReport(days int) {
	since := today(a.now()).AddDate(0, 0, -days)
	kind := "周报"
	if days <= 1 {
		kind = "日报"
	}
	prompt := fmt.Sprintf("根据导图和下面「最近 %d 天的变化」，写一份%s（Markdown）：本期完成、进行中、风险与阻塞、下期计划。简洁，用条目，引用节点标题。不要修改导图。", days, kind)
	a.aiAction(prompt, scopeFile, a.changesSince(since))
}

// aiBreakDown asks to break the node selected down into tasks.
func (a *app) aiBreakDown() {
	n, _ := a.selectedNode()
	if n == nil {
		return
	}
	prompt := fmt.Sprintf("把选中的「%s」拆解成 3–8 个可执行的子任务（必要时再分一层），用 propose_edits 添加为它的子节点并设为待做任务，每个给一句备注说明要做什么、怎样算完成。", n.Title)
	a.aiAction(prompt, scopeSubtree, "")
}

// riskFacts lists what in the map may need attention, for the risk scan.
func (a *app) riskFacts() string {
	now := a.now()
	var b strings.Builder
	var overdue, noDue, ideas, blocked []string
	for _, tn := range a.file.Tasks() {
		n, t := tn.Node, tn.Node.Task
		switch {
		case t.Status == model.StatusDone:
			continue
		case t.Overdue(now):
			overdue = append(overdue, n.Title+"（截止 "+t.Due.Format("1/2")+"）")
		case t.Due == nil && t.Status != model.StatusIdea:
			noDue = append(noDue, n.Title)
		}
		if t.Status == model.StatusIdea {
			ideas = append(ideas, n.Title)
		}
		if len(a.file.Blockers(n)) > 0 {
			blocked = append(blocked, n.Title)
		}
	}
	var bare []string
	a.file.Walk(func(n *model.Node) {
		if len(n.Children) >= 3 && strings.TrimSpace(n.Note) == "" {
			bare = append(bare, n.Title)
		}
	})
	list := func(title string, items []string) {
		if len(items) > 0 {
			fmt.Fprintf(&b, "%s（%d）：%s\n", title, len(items), strings.Join(items[:min(len(items), 30)], "、"))
		}
	}
	list("已逾期", overdue)
	list("被阻塞", blocked)
	list("没有截止日期的待做/进行中任务", noDue)
	list("停留在「想法」的任务", ideas)
	list("有 3 个以上下级却没有备注的节点", bare)
	if b.Len() == 0 {
		return "（没有发现明显的问题。）"
	}
	return "自动检查发现：\n" + b.String()
}

// aiRiskScan asks for the risks of the map.
func (a *app) aiRiskScan() {
	prompt := "检查这个导图的风险：结合下面的自动检查结果和导图内容，列出最该处理的 5 个问题（按严重程度），每个给一句原因和建议。需要改的地方（比如补截止日期、拆分过大的任务、补备注）可以用 propose_edits 提出，由我确认。"
	a.aiAction(prompt, scopeFile, a.riskFacts())
}

// aiFromText asks for nodes made of the text pasted in the window.
func (a *app) aiFromText(text string) {
	prompt := "根据下面「待整理的文本」生成结构化的导图节点，挂在当前画布的中心主题下：用 propose_edits 一次添加，层级不超过 4 层，标题简短、细节写进备注，可执行的事项设为待做任务。"
	a.aiAction(prompt, scopeCanvas, "待整理的文本：\n"+text)
}

// skipDirs are folders a repository scan leaves out.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "target": true, ".next": true,
	"__pycache__": true, ".venv": true, "venv": true, ".idea": true, ".vscode": true, "coverage": true, ".cache": true,
}

// scanRepo describes a code repository: its tree of folders and files to
// a depth, and the start of its README and manifest files.
func scanRepo(dir string, depth int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "代码仓库：%s\n\n目录结构：\n", filepath.Base(dir))
	count := 0
	var walk func(path string, level int)
	walk = func(path string, level int) {
		entries, err := os.ReadDir(path)
		if err != nil {
			return
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].IsDir() && !entries[j].IsDir() })
		files := 0
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") && name != ".github" || skipDirs[name] {
				continue
			}
			if count > 600 {
				return
			}
			if e.IsDir() {
				count++
				fmt.Fprintf(&b, "%s%s/\n", strings.Repeat("  ", level), name)
				if level+1 < depth {
					walk(filepath.Join(path, name), level+1)
				}
				continue
			}
			if files++; files > 25 {
				if files == 26 {
					fmt.Fprintf(&b, "%s…\n", strings.Repeat("  ", level))
				}
				continue
			}
			count++
			fmt.Fprintf(&b, "%s%s\n", strings.Repeat("  ", level), name)
		}
	}
	walk(dir, 0)
	for _, name := range []string{"README.md", "README", "readme.md", "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "pom.xml", "build.gradle"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		text := string(data)
		if r := []rune(text); len(r) > 3000 {
			text = string(r[:3000]) + "\n…"
		}
		fmt.Fprintf(&b, "\n%s：\n%s\n", name, text)
	}
	return b.String()
}

// aiFromRepo asks for the architecture of a code repository as nodes,
// linked to its files, and makes it the map's project folder.
func (a *app) aiFromRepo(dir string) {
	if a.file.Project == "" {
		a.mutate("", func() { a.file.Project = dir })
	}
	prompt := "根据下面这个代码仓库的目录结构和说明，生成项目架构导图，挂在当前画布的中心主题下：按模块 / 层次组织（不超过 4 层），每个模块写一句职责备注，关键文件作为节点的 links（相对仓库根目录的路径）。用 propose_edits 一次添加。"
	a.ai.nextSync = headBaseline(dir, a.now()) // applied, the map matches this code
	a.aiAction(prompt, scopeCanvas, scanRepo(dir, 3))
}

// mapFromFolders adds a branch per folder of a repository, without the AI:
// its folders to two levels, each linked to its path.
func (a *app) mapFromFolders(dir string) {
	var nodes []*model.Node
	var walk func(path, rel string, level int) []*model.Node
	walk = func(path, rel string, level int) []*model.Node {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		var out []*model.Node
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() || strings.HasPrefix(name, ".") || skipDirs[name] {
				continue
			}
			r := filepath.ToSlash(filepath.Join(rel, name))
			n := model.NewNode(name)
			n.Links = []string{r}
			if level < 1 {
				n.Children = walk(filepath.Join(path, name), r, level+1)
			}
			out = append(out, n)
		}
		return out
	}
	nodes = walk(dir, "", 0)
	if len(nodes) == 0 {
		a.saveErr = "这个文件夹里没有子目录"
		return
	}
	a.mutate("", func() {
		if a.file.Project == "" {
			a.file.Project = dir
		}
		root := a.shownRoot()
		root.Children = append(root.Children, nodes...)
	})
	a.tell(fmt.Sprintf("已按目录添加 %d 个模块", len(nodes)))
}

// aiMenu lists the actions, for the panel's menu and the node's.
func (a *app) aiMenu(m *ui.Menu) {
	if m.Item("生成周报").Chosen() {
		a.later(func() { a.aiReport(7) })
	}
	if m.Item("生成日报").Chosen() {
		a.later(func() { a.aiReport(1) })
	}
	if m.Item("风险扫描").Chosen() {
		a.later(a.aiRiskScan)
	}
	if m.Item("拆解所选任务").Disabled(a.selected == "").Chosen() {
		a.later(a.aiBreakDown)
	}
	m.Separator()
	if m.Item("从文本生成导图…").Chosen() {
		a.fromTextOpen = true
	}
	if m.Item("从代码仓库生成架构…").Chosen() {
		a.later(a.askRepo)
	}
}

// askRepo asks for a repository's folder and maps it.
func (a *app) askRepo() {
	a.plat.PickFolder(func(dir string) {
		if dir == "" {
			return
		}
		if providerKey(a.cfg) == "" {
			a.mapFromFolders(dir) // without a key: the folders alone
			return
		}
		a.aiFromRepo(dir)
	})
}

// fromTextView is the window to paste text into, to make nodes of.
func (a *app) fromTextView(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c.Key("from-text"), &a.fromTextOpen, func() {
		w, h := c.Size()
		ui.Column(c).Gap(12).Width(min(640, w-80)).Children(func() {
			ui.Text(c, "从文本生成导图").FontSize(17).Bold()
			ui.Text(c, "粘贴需求文档、会议记录或和 AI 的对话，AI 会整理成节点挂在当前画布的中心主题下，你确认后才添加。没有设置 AI 时，按缩进 / 列表直接转成节点。").
				FontSize(12).TextColor(t.TextMuted).Wrap()
			ui.TextArea(c.Key("text"), &a.fromText).Lines(10, int(max(10, (h-320)/18))).Placeholder("粘贴文本…").AutoFocus().FillWidth()
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					a.fromTextOpen = false
				}
				if ui.Button(c, "按缩进直接转换").Disabled(strings.TrimSpace(a.fromText) == "").Clicked() {
					text := a.fromText
					a.fromTextOpen, a.fromText = false, ""
					a.later(func() {
						a.selected = a.shownRoot().ID
						a.plat.CopyText(text)
						a.clip = nil
						a.paste()
					})
				}
				if ui.PrimaryButton(c, "用 AI 整理").Disabled(strings.TrimSpace(a.fromText) == "" || providerKey(a.cfg) == "").Clicked() {
					text := a.fromText
					a.fromTextOpen, a.fromText = false, ""
					a.later(func() { a.aiFromText(text) })
				}
			})
		})
	})
}
