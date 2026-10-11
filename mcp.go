package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/mcp"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// serveMCP runs the MCP server on the maps folder of the settings, or the
// one --dir names.
func serveMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	dir := fs.String("dir", "", "the maps folder (default: the app's)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		*dir = loadConfig().Dir
	}
	if *dir == "" {
		*dir = defaultDir()
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("导图文件夹不存在：%s", abs)
	}
	cfg := loadConfig()
	s := &mcp.Server{Dir: abs, Versions: versions.Store{Dir: versionsDir()}, Version: version, Now: time.Now,
		Embedder: library.Embedder{URL: cfg.EmbedURL, Key: cfg.EmbedKey, Model: cfg.EmbedModel, Local: loadLocalModel()}}
	if cfg.NoEmbed {
		s.Embedder = library.Embedder{}
	}
	return s.Serve(os.Stdin, os.Stdout)
}

// executable is the path of the app's program, which agents run.
func executable() string {
	p, err := os.Executable()
	if err != nil {
		return "/Applications/MindMap.app/Contents/MacOS/MindMap"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// mcpView explains how to connect an AI coding agent to the maps, with
// what to paste where.
func (a *app) mcpView(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c.Key("mcp"), &a.mcpOpen, func() {
		exe := executable()
		claude := fmt.Sprintf("claude mcp add mindmap --scope user -- %q mcp", exe)
		json := fmt.Sprintf("{\n  \"mcpServers\": {\n    \"mindmap\": {\n      \"command\": %q,\n      \"args\": [\"mcp\"]\n    }\n  }\n}", exe)
		rule := "项目的架构、功能、任务和待优化点记录在 MindMap 导图「" + mapName(a.path) + "」里（通过 mindmap MCP 工具访问）。\n" +
			"- 开始工作前先用 read_map 读一遍，了解结构和约定；\n" +
			"- 完成或开始一个任务时用 update_node 更新状态，并用 append_note 记下实现要点和决策；\n" +
			"- 用 update_node 的 add_links 把实现它的代码文件（路径:行号）和 PR 链接到节点上；\n" +
			"- 发现的问题和可优化点，用 add_nodes 加到对应模块下（作为 [ ] 任务）；\n" +
			"- 每次修改都写清楚 summary。"
		w, _ := c.Size()
		ui.Column(c).Gap(14).Width(min(680, w-80)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "连接 AI 编程工具（MCP）").FontSize(17).Bold().Grow(1)
				if iconButton(c, iconClose, "关闭", "关闭（Esc）", 28, true).Clicked() {
					a.mcpOpen = false
				}
			})
			ui.Text(c, "让 Claude Code、Cursor 等 AI 编程工具直接读写你的导图：动手前读架构，完成后更新任务状态、补实现备注、记录待优化点。AI 的每次修改都会存进历史版本（标注为「AI 修改」），可以随时对比和恢复。").
				FontSize(13).Wrap()
			step := func(title, body, copyLabel string) {
				ui.Column(c.Key(title)).Gap(6).Children(func() {
					ui.Text(c, title).FontSize(13).Bold()
					ui.Box(c).Padding(8, 10).Radius(6).Background(t.Surface).Border(1, t.Border).FillWidth().Children(func() {
						ui.Text(c, body).Font("monospace").FontSize(12).Wrap().Selectable()
					})
					if ui.Button(c, copyLabel).Clicked() {
						a.plat.CopyText(body)
						a.tell("已复制")
					}
				})
			}
			step("Claude Code：在终端运行", claude, "复制命令")
			step("Cursor / Claude Desktop 等：加到 MCP 配置文件", json, "复制配置")
			step("建议：加到项目的 CLAUDE.md（或 Cursor 规则）里", rule, "复制说明")
			ui.Text(c, "AI 能用的工具：list_maps、read_map、search、list_tasks、add_nodes、update_node、move_node、delete_node、create_map。导图文件夹："+a.dir).
				FontSize(11).TextColor(t.TextMuted).Wrap()
		})
	})
}
