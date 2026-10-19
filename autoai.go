package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/automate"
)

// Automations written or changed by the AI of the settings, and the
// templates to start from.

type autoTemplate struct {
	name  string
	id    string // the file's or folder's name: a folder unless it ends in .py
	files map[string]string
}

var autoTemplates = []autoTemplate{
	{name: "网页监控", id: "网页监控", files: map[string]string{
		automate.Settings: `name = "网页监控"
trigger = "每 30 分钟"
timeout = "5m"
# 有变化时用来概括的模型：deepseek-chat、claude-…，或 cli:claude -p。
# 留空就不概括，只列出变化的行。
model = ""
`,
		"pages.txt": `# 要监控的网页，一行一个
https://example.com
`,
		"main.py": `"""网页监控：网页的文字有变化时通知，并列出变化。"""

import difflib
import hashlib
import os
import re
import urllib.request

import mymind


def page_text(url):
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0 MyMind"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        html = resp.read().decode(resp.headers.get_content_charset() or "utf-8", "replace")
    html = re.sub(r"(?is)<(script|style|noscript)[^>]*>.*?</\1>", " ", html)
    text = re.sub(r"(?s)<[^>]+>", "\n", html)
    lines = [re.sub(r"\s+", " ", line).strip() for line in text.splitlines()]
    return "\n".join(line for line in lines if line)


def main():
    with open("pages.txt", encoding="utf-8") as f:
        urls = [u.strip() for u in f if u.strip() and not u.startswith("#")]
    seen = mymind.state.get("pages", {})
    report = []
    for url in urls:
        try:
            text = page_text(url)
        except Exception as e:
            print("打不开", url, e)
            continue
        digest = hashlib.sha256(text.encode()).hexdigest()
        old = seen.get(url)
        seen[url] = {"hash": digest, "text": text[:200000]}
        if old is None:
            print("开始监控", url)
            continue
        if old["hash"] == digest:
            print("没有变化", url)
            continue
        diff = [l for l in difflib.unified_diff(old["text"].splitlines(), text.splitlines(), lineterm="", n=0)
                if l[:1] in "+-" and not l.startswith(("+++", "---"))]
        change = "\n".join(diff[:80])
        if os.environ.get("MYMIND_MODEL"):
            change = mymind.llm("这是一个网页前后两次的文字差异（- 删掉的，+ 新增的），用中文简要说明变了什么：", input=change)
        report.append("## " + url + "\n\n" + change)
        mymind.notify("网页有变化", url)
    mymind.state["pages"] = seen
    if report:
        mymind.output("\n\n".join(report))
    else:
        mymind.output("没有变化")


main()
`,
	}},
	{name: "季度绩效", id: "季度绩效", files: map[string]string{
		automate.Settings: `name = "季度绩效"
trigger = "每季度末 18:00"
timeout = "10m"
# 写绩效总结用的模型，必须配置：deepseek-chat、claude-…，或 cli:claude -p
model = ""
`,
		"要求.md": `按公司的绩效模板写：
1. 本季度完成的主要工作（按项目，写清结果和数据）
2. 亮点与影响
3. 不足与改进
4. 下季度计划
`,
		"素材/README.txt": `把这个季度的周报、会议记录、需求文档等放进这个文件夹（.md / .txt）。
程序还会读导图里本季度完成的任务，并在资料库里搜 main.py 里的关键词。
`,
		"main.py": `"""季度绩效：把这个季度导图里完成的任务、素材和资料库里的相关内容，整理成绩效总结。"""

import datetime
import pathlib

import mymind

KEYWORDS = ["周报", "上线", "完成", "绩效"]


def quarter_start(today):
    return datetime.date(today.year, (today.month - 1) // 3 * 3 + 1, 1)


def main():
    today = datetime.date.today()
    start = quarter_start(today)
    title = f"{today.year} 年第 {(today.month - 1) // 3 + 1} 季度绩效总结"
    parts = []
    done = mymind.maps.tasks(status="done", done_since=start)
    if done:
        lines = [f"- [{t['map']}] {t['where']} › {t['title']}（{t['done_at'][:10]}）" for t in done]
        parts.append("### 导图里本季度完成的任务\n" + "\n".join(lines))
    for path in sorted(pathlib.Path("素材").rglob("*")):
        if path.suffix in (".md", ".txt") and path.name != "README.txt":
            parts.append(f"### {path.name}\n{path.read_text(encoding='utf-8')[:20000]}")
    for word in KEYWORDS:
        for hit in mymind.search(word, limit=5):
            parts.append(f"### 资料：{hit['title']}\n{hit['text'][:4000]}")
    if not parts:
        mymind.output("没有素材：导图里没有本季度完成的任务，「素材」文件夹和资料库里也没有")
        return
    rules = pathlib.Path("要求.md").read_text(encoding="utf-8")
    text = mymind.llm(f"根据下面的素材写「{title}」。要求：\n{rules}\n只用素材里有的事实，不要编造。", input="\n\n".join(parts))
    mymind.output(text)
    mymind.clip(text, title=title, tags=["绩效"])
    mymind.notify(title + "写好了", "已存进资料库")


main()
`,
	}},
	{name: "需求编码", id: "需求编码", files: map[string]string{
		automate.Settings: `name = "需求编码"
trigger = "手动"
timeout = "60m"
# 写代码用的工具（必须配置），在仓库目录里运行，需求从输入传给它，或替换命令里的 {prompt}：
#   agent = "claude -p --permission-mode acceptEdits"
#   agent = "codex exec --full-auto"
#   agent = "aider --yes --message {prompt}"
agent = ""
# 代码仓库的文件夹
repo = "~/code/项目"
# 从哪张导图取需求：带「编码」标签、状态是待做的任务；留空就用 需求.md
map = ""
`,
		"需求.md": `# 需求

在这里写要实现的需求，越具体越好：要改哪个页面、期望的行为、验收标准。
`,
		"main.py": `"""需求编码：把需求交给配置好的编码工具，在仓库里实现，完成后汇报改动。"""

import pathlib
import subprocess

import mymind

REPO = mymind.settings.get("repo", "")
MAP = mymind.settings.get("map", "")
TAG = "编码"
FENCE = chr(96) * 3


def git(*args):
    r = subprocess.run(["git", *args], cwd=pathlib.Path(REPO).expanduser(), capture_output=True, text=True)
    return r.stdout.strip()


def implement(title, detail):
    task = f"""在这个仓库里实现下面的需求。完成后用中文简要总结：改了哪些文件、怎么验证。

需求：{title}

{detail}"""
    if not mymind.confirm(f"让编码工具实现「{title}」？", detail=f"仓库：{REPO}\n\n{detail}"):
        return None
    answer = mymind.agent(task, repo=REPO)
    stat = git("diff", "--stat")
    return f"## {title}\n\n{answer[-3000:]}\n\n{FENCE}\n{stat or '（工作区没有改动）'}\n{FENCE}"


def main():
    reports = []
    if MAP:
        tasks = [t for t in mymind.maps.tasks(map=MAP, status="todo") if TAG in (t.get("tags") or [])]
        if not tasks:
            mymind.output(f"「{MAP}」里没有带「{TAG}」标签的待做任务")
            return
        for t in tasks:
            mymind.maps.update(MAP, t["id"], status="doing")
            report = implement(t["title"], t.get("note", ""))
            if report is None:
                mymind.maps.update(MAP, t["id"], status="todo")
                continue
            mymind.maps.update(MAP, t["id"], status="done", append_note="自动编码：\n" + report[:2000])
            reports.append(report)
    else:
        text = pathlib.Path("需求.md").read_text(encoding="utf-8").strip()
        title = text.splitlines()[0].lstrip("# ").strip() if text else "需求"
        report = implement(title, text)
        if report:
            reports.append(report)
    if reports:
        mymind.output("\n\n".join(reports))
        mymind.notify("需求编码完成", f"{len(reports)} 个需求")


main()
`,
	}},
	{name: "页面测试", id: "页面测试", files: map[string]string{
		automate.Settings: `name = "页面测试"
trigger = "每天 09:00"
timeout = "10m"
# 要测试的网址，见 cases.py
# 失败时用来分析原因的模型（可选）：deepseek-chat、claude-…，或 cli:claude -p
model = ""
`,
		"requirements.txt": "playwright\n",
		"cases.py": `"""测试用例：每个用例打开一个页面，做些操作，检查结果。page 是 Playwright 的页面。"""


def 首页能打开(page):
    page.goto("https://example.com")
    assert "Example" in page.title(), "标题不对：" + page.title()


def 链接可以点击(page):
    page.goto("https://example.com")
    page.get_by_role("link").first.click()
    page.wait_for_load_state()


CASES = [首页能打开, 链接可以点击]
`,
		"main.py": `"""页面测试：用 Playwright 跑 cases.py 里的用例，失败的截图，汇总结果。

先点「安装依赖」装好 Playwright 和浏览器。"""

import os
import pathlib
import traceback

import mymind
from cases import CASES

try:
    from playwright.sync_api import sync_playwright
except ImportError:
    raise SystemExit("没有 Playwright：先点「安装依赖」")


def main():
    shots = pathlib.Path("截图")
    shots.mkdir(exist_ok=True)
    results = []
    with sync_playwright() as p:
        browser = p.chromium.launch()
        for case in CASES:
            page = browser.new_page()
            errors = []
            page.on("pageerror", lambda e: errors.append(str(e)))
            try:
                case(page)
                results.append((case.__name__, True, "；".join(errors)))
                print("通过", case.__name__)
            except Exception:
                path = shots / f"{case.__name__}.png"
                page.screenshot(path=str(path), full_page=True)
                results.append((case.__name__, False, traceback.format_exc(limit=3)))
                print("失败", case.__name__, f"截图：{path.resolve()}")
            finally:
                page.close()
        browser.close()
    failed = [r for r in results if not r[1]]
    lines = [f"- {'✅' if ok else '❌'} {name}" for name, ok, _ in results]
    report = f"**{len(results) - len(failed)}/{len(results)} 通过**\n\n" + "\n".join(lines)
    if failed:
        detail = "\n\n".join(f"### {name}\n{why}" for name, _, why in failed)
        if os.environ.get("MYMIND_MODEL"):
            detail = mymind.llm("这些页面自动化测试失败了，用中文分析可能的原因：", input=detail)
        report += "\n\n" + detail
        mymind.notify("页面测试失败", f"{len(failed)} 个用例失败")
    mymind.output(report)
    if failed:
        raise SystemExit(1)


main()
`,
	}},
}

var autoBlankFile = autoTemplate{name: "新程序", id: "新程序.py", files: map[string]string{"": `# 名称: 新程序
# 触发: 手动
# 模型:
# 触发可以写：` + automate.TriggerHelp + `
# 模型是程序自己的：deepseek-chat、claude-…，或 cli:claude -p；不用大模型就留空

import mymind

print("你好，", mymind.name)
mymind.output("运行完成")
`}}

var autoBlankFolder = autoTemplate{name: "新程序", id: "新程序", files: map[string]string{
	automate.Settings: `name = "新程序"
# 触发可以写：` + automate.TriggerHelp + `
trigger = "手动"
# 程序自己的模型：deepseek-chat、claude-…，或 cli:claude -p；不用大模型就留空
model = ""
# entry = "main.py"
# timeout = "30m"
`,
	"main.py": `import mymind

print("你好，", mymind.name)
mymind.output("运行完成")
`,
	"requirements.txt": "",
}}

// autoFromTemplate makes an automation from a template and shows it.
func (a *app) autoFromTemplate(tpl autoTemplate) {
	root := a.autoRoot()
	ext := ""
	if strings.HasSuffix(tpl.id, ".py") {
		ext = ".py"
	}
	id := automate.FreeID(root, strings.TrimSuffix(tpl.id, ".py"), ext)
	files := tpl.files
	if ext == ".py" {
		files = map[string]string{id: tpl.files[""]}
	}
	if _, err := automate.Write(root, id, files); err != nil {
		a.tell("没有新建：" + err.Error())
		return
	}
	a.autoCreated(id)
}

// autoCreated shows an automation just made. The app made it, from a
// template or the user's words: it runs once seen, as any new code.
func (a *app) autoCreated(id string) {
	a.openAutomations()
	a.autoSelect(id)
	a.auto.tab = 0
}

// The instructions of the AI that writes automations.
const autoSystem = `你为「MyMind」写自动化程序：在用户的 Mac 上运行的 Python 3 程序，由 MyMind 手动或定时运行。

一个自动化是一个文件夹：
- automation.toml：name = "名称"、trigger = "触发"、model = "模型"、agent = "编码工具"、timeout = "10m"（可选）、entry = "main.py"（可选），还可以加程序自己的配置项（如 repo = "…"），程序里用 mymind.settings 读
- main.py：入口；可以拆成多个模块，同文件夹里互相 import
- requirements.txt：用到的第三方包（标准库够用就留空）；数据和配置文件也可以放在文件夹里
程序运行时的当前目录就是这个文件夹。

触发的写法：` + automate.TriggerHelp + `。

程序用 mymind 模块和 MyMind 交互：
- mymind.llm(prompt, input=None, model=None, system=None, json=False) -> str（json=True 时返回解析好的对象）
- mymind.notify(title, body="")：系统通知
- mymind.confirm(message, detail="") -> bool：在 MyMind 里弹窗等用户允许，做有风险或对外的操作前要先问
- mymind.output(text)：这次运行的结果（Markdown），显示在运行记录里
- mymind.clip(text, title="", url="", tags=None)：存进资料库
- mymind.search(query, limit=5) -> [{"id","title","text","url"}]：搜资料库
- mymind.state：跨运行保存的 dict（赋值即保存）
- mymind.settings：automation.toml 里的配置项（dict）
- mymind.agent(task, repo=None) -> str：让配置的编码工具（agent）在仓库目录里完成编码任务，返回它的回答
- mymind.maps：用户的思维导图。maps.list()；maps.read(map) 返回整张图的节点树；maps.tasks(map=None, status=None, done_since=None) 返回任务（status 为 idea/todo/doing/done/open，节点含 id/title/note/tags/status/due/done_at/map/where）；maps.add(map, parent_id, outline)；maps.update(map, id, title=, note=, append_note=, status=, priority=, due=, add_tags=)；maps.move / maps.delete / maps.create(name, outline=)
- print 的内容会显示在运行记录里

模型：程序用的大模型由程序自己配置在 automation.toml 的 model 里（如 deepseek-chat、claude-…，或 cli:claude -p 这样的命令行）。
用户没有指定用哪个模型时，model 留空 ""，不要替用户选，在 summary 里提醒用户去填。程序不用大模型就不要调用 mymind.llm。
编码工具同样由程序自己配置在 agent 里（如 "claude -p"、"codex exec"、"aider --yes --message {prompt}"）；用户没指定就留空 ""，在 summary 里提醒用户去填。

要求：代码简洁可靠，出错时打印清楚的原因；不要硬编码密钥；中文注释和输出。
只回答一个 JSON 对象：{"name": "名称", "files": {"automation.toml": "…", "main.py": "…"}, "summary": "一两句话说明做了什么、用户还要配置什么"}`

const autoModifySystem = autoSystem + `

现在是修改已有的程序：files 里只放要新建或改动的文件，每个都写完整内容；要删掉的文件放在 "delete": ["文件名"] 里。不要改 name，除非用户要求。`

// autoAsk asks the AI of the settings, which writes the code, for a JSON
// object.
func (a *app) autoAsk(ctx context.Context, cfg config, system, prompt string) (string, error) {
	if cfg.AIProvider == providerDeepSeek {
		resp, err := a.sendDeepSeek(ctx, cfg, dsRequest{Model: deepseekModel(cfg), MaxTokens: 32000, Messages: []dsMessage{
			{Role: "system", Content: system}, {Role: "user", Content: prompt},
		}, ResponseFormat: &dsFormat{Type: "json_object"}}, nil)
		if err != nil {
			return "", errors.New(deepseekError(err))
		}
		if resp.Choices[0].FinishReason == "length" {
			return "", plainError("回答超出长度，没有写完，请把要求说得更具体些再试")
		}
		return resp.Choices[0].Message.Content, nil
	}
	resp, err := a.sendAI(ctx, cfg, anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(aiModel(cfg)),
		MaxTokens: 32000,
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(prompt))},
	}, nil)
	if err != nil {
		return "", err
	}
	var text string
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text += b.Text
		}
	}
	if resp.StopReason == anthropic.BetaStopReasonMaxTokens {
		return "", plainError("回答超出长度，没有写完，请把要求说得更具体些再试")
	}
	return text, nil
}

// autoWritten is what the AI answers: the files of the program.
type autoWritten struct {
	Name    string            `json:"name"`
	Files   map[string]string `json:"files"`
	Delete  []string          `json:"delete"`
	Summary string            `json:"summary"`
}

func parseWritten(text string) (*autoWritten, error) {
	var w autoWritten
	if err := json.Unmarshal([]byte(jsonPart(text)), &w); err != nil {
		return nil, fmt.Errorf("AI 的回答不是要的格式：%w", err)
	}
	return &w, nil
}

// autoGenerate has the AI write an automation from a description.
func (a *app) autoGenerate(desc string) {
	if providerKey(a.cfg) == "" {
		a.auto.aiErr = "先在设置里配置 AI（写程序用的是设置里的 AI）"
		return
	}
	cfg, root := a.cfg, a.autoRoot()
	a.auto.newOpen, a.auto.busy, a.auto.aiErr = false, "AI 正在写程序…", ""
	a.async(func() (any, error) {
		text, err := a.autoAsk(context.Background(), cfg, autoSystem, "要做的自动化：\n"+desc)
		if err != nil {
			return nil, err
		}
		return parseWritten(text)
	}, func(v any, err error) {
		a.auto.busy = ""
		if err != nil {
			a.auto.aiErr, a.auto.newOpen, a.auto.newText = "没有写成："+aiErrorText(err), true, desc
			return
		}
		w := v.(*autoWritten)
		if len(w.Files) == 0 {
			a.auto.aiErr, a.auto.newOpen, a.auto.newText = "AI 没有给出文件", true, desc
			return
		}
		id := automate.FreeID(root, cmpOr(w.Name, firstSentence(desc)), "")
		if _, err := automate.Write(root, id, w.Files); err != nil {
			a.auto.aiErr, a.auto.newOpen, a.auto.newText = "没有写成："+err.Error(), true, desc
			return
		}
		a.autoCreated(id)
		a.tell(cmpOr(w.Summary, "已写好，看过代码后运行"))
	})
}

// autoModify has the AI change an automation as asked; log is what a
// run printed, when it is to be fixed.
func (a *app) autoModify(x *automate.Automation, ask, log string) {
	if providerKey(a.cfg) == "" {
		a.tell("先在设置里配置 AI（改程序用的是设置里的 AI）")
		return
	}
	a.autoSaveCode()
	var b strings.Builder
	b.WriteString("要求：" + ask + "\n\n程序现在的文件：\n")
	code := x.Code()
	if !x.Folder {
		// One file: shown as the main.py of a folder, which it becomes
		// when the AI adds files.
		code = map[string]string{"main.py": code[x.ID]}
	}
	names := make([]string, 0, len(code))
	for f := range code {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		fmt.Fprintf(&b, "\n=== %s ===\n%s\n", f, code[f])
	}
	if log != "" {
		if len(log) > 12000 {
			log = log[len(log)-12000:]
		}
		b.WriteString("\n上次运行的输出：\n" + log + "\n")
	}
	cfg, root, id := a.cfg, a.autoRoot(), x.ID
	single := !x.Folder
	a.auto.fixOpen, a.auto.busy, a.auto.aiErr = false, "AI 正在修改「"+x.Name+"」…", ""
	a.async(func() (any, error) {
		text, err := a.autoAsk(context.Background(), cfg, autoModifySystem, b.String())
		if err != nil {
			return nil, err
		}
		return parseWritten(text)
	}, func(v any, err error) {
		a.auto.busy = ""
		if err != nil {
			a.tell("没有改成：" + aiErrorText(err))
			return
		}
		w := v.(*autoWritten)
		if err := autoApply(root, id, single, w); err != nil {
			a.tell("没有改成：" + err.Error())
			return
		}
		a.autoRefresh(true)
		a.auto.fileOf, a.auto.tab = "", 0
		a.tell(cmpOr(w.Summary, "已改好，确认改动后运行"))
		if x := a.autoByID(id); x != nil {
			a.openApproval(x, false)
		}
	})
}

// autoApply writes the files the AI changed. A program of one file stays
// one while the AI changes only its main.py.
func autoApply(root, id string, single bool, w *autoWritten) error {
	if single {
		if src, ok := w.Files["main.py"]; ok && len(w.Files) == 1 && len(w.Delete) == 0 {
			return os.WriteFile(filepath.Join(root, id), []byte(src), 0o644)
		}
		if len(w.Files) > 0 {
			return errors.New("单文件程序要拆成多个文件：请先把它改成文件夹")
		}
		return nil
	}
	if len(w.Files) > 0 {
		if _, err := automate.Write(root, id, w.Files); err != nil {
			return err
		}
	}
	for _, f := range w.Delete {
		clean := filepath.Clean(filepath.FromSlash(f))
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			continue
		}
		_ = os.Remove(filepath.Join(root, id, clean))
	}
	return nil
}

// aiErrorText is an error of the AI as shown.
func aiErrorText(err error) string {
	var plain plainError
	if errors.As(err, &plain) {
		return string(plain)
	}
	return err.Error()
}

// autoNewView asks what automation to write.
func (a *app) autoNewView(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c.Key("auto-new"), &a.auto.newOpen, func() {
		w, _ := c.Size()
		ui.Column(c).Gap(12).Width(min(600, w-80)).Children(func() {
			ui.Text(c, "新建自动化").FontSize(17).Bold()
			ui.Text(c, "用一两句话说要自动做什么、多久做一次。AI（设置里的）写成 Python 程序，你看过代码再运行。\n"+
				"程序里要用大模型的话，用哪个模型写进描述（如「用 deepseek-chat 总结」），不写就留给你之后配置。").
				FontSize(textSmall).TextColor(t.TextMuted).Wrap()
			ui.TextArea(c.Key("auto-new-text"), &a.auto.newText).Lines(5, 12).
				Placeholder("例如：每天 9 点检查 https://… 的价格，低于 100 就通知我").AutoFocus().FillWidth()
			if a.auto.aiErr != "" {
				ui.Text(c, a.auto.aiErr).FontSize(textSmall).TextColor(t.Danger).Wrap()
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					a.auto.newOpen = false
				}
				if ui.PrimaryButton(c, "让 AI 写").Disabled(strings.TrimSpace(a.auto.newText) == "").Clicked() {
					desc := strings.TrimSpace(a.auto.newText)
					a.auto.newText = ""
					a.later(func() { a.autoGenerate(desc) })
				}
			})
		})
	})
}

// autoFixView asks how to change the automation shown.
func (a *app) autoFixView(c *ui.Context) {
	x := a.autoByID(a.auto.sel)
	if x == nil {
		a.auto.fixOpen = false
		return
	}
	t := c.Theme()
	ui.Modal(c.Key("auto-fix"), &a.auto.fixOpen, func() {
		w, _ := c.Size()
		ui.Column(c).Gap(12).Width(min(600, w-80)).Children(func() {
			ui.Text(c, "修改「"+x.Name+"」").FontSize(17).Bold()
			ui.Text(c, "说要怎么改，AI 改好后会列出改动，你确认后才会运行。").FontSize(textSmall).TextColor(t.TextMuted).Wrap()
			ui.TextArea(c.Key("auto-fix-text"), &a.auto.fixText).Lines(4, 12).
				Placeholder("例如：改成每周一早上 9 点运行，结果也存进资料库").AutoFocus().FillWidth()
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Clicked() {
					a.auto.fixOpen = false
				}
				if ui.PrimaryButton(c, "让 AI 改").Disabled(strings.TrimSpace(a.auto.fixText) == "").Clicked() {
					ask := strings.TrimSpace(a.auto.fixText)
					a.later(func() { a.autoModify(x, ask, "") })
				}
			})
		})
	})
}
