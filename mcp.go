package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	s := mcpServer(abs)
	s.Embedder = library.Embedder{URL: cfg.EmbedURL, Key: cfg.EmbedKey, Model: cfg.EmbedModel, Local: loadLocalModel()}
	if cfg.NoEmbed {
		s.Embedder = library.Embedder{}
	}
	return s.Serve(os.Stdin, os.Stdout)
}

// mcpServer is the server of the maps in dir, for an agent working in the
// folder the program runs in.
func mcpServer(dir string) *mcp.Server {
	s := &mcp.Server{Dir: dir, Versions: versions.Store{Dir: versionsDir()}, Version: version, Now: time.Now}
	s.Cwd, _ = os.Getwd()
	if v := versionsDir(); v != "" {
		s.StateDir = filepath.Join(filepath.Dir(v), "agents")
	}
	return s
}

// serveHook answers a hook of Claude Code (internal/mcp/hook.go): "start"
// prints the brief of the project the session is in, "stop" has the agent
// record the session before it stops when the code changed. What Claude
// Code says of the session is JSON on stdin. A hook never fails: an agent
// is not to be held up by its record.
func serveHook(args []string) {
	if len(args) == 0 {
		return
	}
	var in struct {
		Cwd    string `json:"cwd"`
		Active bool   `json:"stop_hook_active"`
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		if data, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20)); err == nil {
			_ = json.Unmarshal(data, &in)
		}
	}
	dir := loadConfig().Dir
	if dir == "" {
		dir = defaultDir()
	}
	if _, err := os.Stat(dir); err != nil {
		return
	}
	s := mcpServer(dir)
	s.Agent = "claude-code"
	if in.Cwd == "" {
		in.Cwd = s.Cwd
	}
	s.Cwd = in.Cwd
	switch args[0] {
	case "start":
		fmt.Print(s.HookStart(in.Cwd))
	case "stop":
		fmt.Print(s.HookStop(in.Cwd, in.Active))
	}
}

// hookCommands are the hooks of Claude Code and the command of each.
func hookCommands(exe string) map[string]string {
	return map[string]string{"SessionStart": fmt.Sprintf("%q hook start", exe), "Stop": fmt.Sprintf("%q hook stop", exe)}
}

// hooksConfig is the hooks as Claude Code's settings.json holds them.
func hooksConfig(exe string) string {
	hooks := map[string]any{}
	for event, cmd := range hookCommands(exe) {
		hooks[event] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": cmd}}}}
	}
	out, _ := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	return string(out)
}

// claudeSettings is Claude Code's settings file of the user.
func claudeSettings() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// installHooks adds the hooks to Claude Code's settings at path, leaving
// what else is there, and replaces those of the app put there before (the
// app may have moved). It reports whether the file changed.
func installHooks(path, exe string) (bool, error) {
	settings := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("读不懂 %s：%v", path, err)
		}
	case !os.IsNotExist(err):
		return false, err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	changed := false
	for event, cmd := range hookCommands(exe) {
		groups, _ := hooks[event].([]any)
		var kept []any
		have := false
		for _, g := range groups {
			ours, same := false, false
			if gm, ok := g.(map[string]any); ok {
				hs, _ := gm["hooks"].([]any)
				for _, h := range hs {
					if hm, ok := h.(map[string]any); ok {
						c, _ := hm["command"].(string)
						if strings.HasSuffix(c, " hook "+strings.ToLower(strings.TrimPrefix(event, "Session"))) && strings.Contains(c, "MyMind") {
							ours, same = true, c == cmd
						}
					}
				}
			}
			if ours && (!same || have) {
				changed = true
				continue
			}
			have = have || ours
			kept = append(kept, g)
		}
		if !have {
			kept = append(kept, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": cmd}}})
			changed = true
		}
		hooks[event] = kept
	}
	if !changed {
		return false, nil
	}
	settings["hooks"] = hooks
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if len(data) > 0 { // what was there, should it be wanted back
		_ = os.WriteFile(path+".mymind-backup", data, 0o600)
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o644)
}

// executable is the path of the app's program, which agents run.
func executable() string {
	p, err := os.Executable()
	if err != nil {
		return "/Applications/MyMind.app/Contents/MacOS/MyMind"
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
	modal(c, "mcp", &a.mcpOpen, func() {
		exe := executable()
		claude := fmt.Sprintf("claude mcp add mymind --scope user -- %q mcp", exe)
		json := fmt.Sprintf("{\n  \"mcpServers\": {\n    \"mymind\": {\n      \"command\": %q,\n      \"args\": [\"mcp\"]\n    }\n  }\n}", exe)
		hooks := hooksConfig(exe)
		rule := "这个项目在 MyMind 里有一张导图（通过 mymind MCP 工具访问，不用传导图名）。\n" +
			"- 开始工作前先调 project_brief，了解在做什么、定过什么；\n" +
			"- 开始或完成任务时用 update_node 改状态；新想到的待办和发现的问题用 capture 记下，不要自己调整导图结构；\n" +
			"- 在几个方案里做了取舍，用 log_decision 记下决定、原因和没选的方案；\n" +
			"- 结束前用 log_session 用一两句话记下这次做了什么，留下的事放进 todos。"
		w, _ := c.Size()
		ui.Column(c).Gap(14).Width(min(680, w-80)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "连接 AI 编程工具（MCP）").FontSize(17).Bold().Grow(1)
				if iconButton(c, iconClose, "关闭", "关闭（Esc）", 28, true).Clicked() {
					a.mcpOpen = false
				}
			})
			ui.Text(c, "让 Claude Code、Cursor 等 AI 编程工具把做了什么记到 MyMind：一个项目一张导图，按它所在的代码仓库自动对上，没有就新建。动手前读简报，过程中改任务状态、记下待办和决策，收工记一笔。AI 记下的待办放在导图的「待整理」下，结构由你来定；每次修改都存进历史版本，可以对比和恢复。").
				FontSize(13).Wrap()
			step := func(title, body, copyLabel string) {
				ui.Column(c.Key(title)).Gap(6).Children(func() {
					// Its name, and at the row's end the key that copies what is under it.
					ui.Row(c).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
						ui.Text(c, title).FontSize(textSmall).FontWeight(700).Grow(1).Shrink(1).MinWidth(0)
						if textKey(c, copyLabel, copyLabel, "复制到剪贴板").Clicked() {
							a.plat.CopyText(body)
							a.tell("已复制")
						}
					})
					ui.Box(c).Padding(10, 12).Radius(radiusRow).Background(blockFill(t)).FillWidth().Children(func() {
						ui.Text(c, body).Font("monospace").FontSize(12).LineHeight(1.5).Wrap().Selectable()
					})
				})
			}
			step("1. Claude Code：在终端运行", claude, "复制命令")
			// The hooks: what makes the record not depend on the agent's memory.
			ui.Row(c.Key("hooks")).Gap(10).AlignItems(ui.Center).Padding(10, 12).Radius(radiusRow).Background(blockFill(t)).FillWidth().Children(func() {
				ui.Column(c).Gap(2).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					ui.Text(c, "2. 让 Claude Code 自动读和记").FontSize(textSmall).FontWeight(700)
					ui.Text(c, "会话开始时自动读到项目简报；改了代码要收工时，自动记下做了什么和留下的待办。不靠 AI 自己记得。").
						FontSize(textCaption).TextColor(t.TextMuted).Wrap()
				})
				if textKey(c, "复制配置", "复制 hooks 配置", "自己加到 ~/.claude/settings.json").Clicked() {
					a.plat.CopyText(hooks)
					a.tell("已复制")
				}
				if accentButton(c, "装上", "加到 Claude Code 的设置（~/.claude/settings.json）里").Clicked() {
					a.later(func() {
						switch changed, err := installHooks(claudeSettings(), exe); {
						case err != nil:
							a.tell("没有装上：" + err.Error())
						case changed:
							a.tell("已装到 Claude Code，新开的会话生效")
						default:
							a.tell("已经装过了")
						}
					})
				}
			})
			step("3. Cursor / Claude Desktop 等：加到 MCP 配置文件", json, "复制配置")
			step("4. 没有 hook 的工具：把这段加到项目的规则里", rule, "复制说明")
			ui.Text(c, "导图文件夹："+a.dir).FontSize(11).TextColor(t.TextMuted).Wrap()
		})
	})
}
