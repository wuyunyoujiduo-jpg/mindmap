package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/egoist/mygo/ui/editor"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/automate"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/mcp"
)

// Automations: Python programs in the folder of maps' 自动化, run by hand
// or on their schedules, which ask the app (the mymind module) for a
// model, a notification, a confirmation or the library (internal/automate).

// autoFolder is the automations' folder in the folder of maps.
const autoFolder = "自动化"

// autoState is the automations' part of the app.
type autoState struct {
	open    bool   // the main area shows the automations
	sel     string // the automation shown
	list    []*automate.Automation
	metas   automate.Metas
	listed  time.Time
	gen     int                          // counts the listings, which files shown are read again on
	loaded  int                          // the listing the file edited was read on
	procs   map[string]*automate.Process // the runs going on, by automation
	sigs    map[string]string            // what the folders watched held
	waiting map[string]bool              // told it waits for an approval
	tab     int                          // 0 the code, 1 the runs
	run     string                       // the run shown, by its ID

	// The code edited: the automation's file, its buffer, and whether its
	// code was approved before it was edited.
	file     string
	fileOf   string
	code     *editor.State
	edited   time.Time
	approved bool

	confirms []*autoConfirm // what programs ask, the first shown
	approve  string         // the automation whose changes show to approve
	diffs    []autoDiff
	diffSel  int
	runAfter bool // run once approved

	// Asking the AI to write or change one.
	newOpen bool
	newText string
	fixOpen bool
	fixText string
	busy    string // what the AI is doing, shown while it does
	aiErr   string

	installing bool
	installLog string
	logOpen    bool

	dirty atomic.Bool // a run printed: draw again
}

// autoConfirm is a program's mymind.confirm, waiting for an answer.
type autoConfirm struct {
	name, message, detail string
	answer                chan bool
}

// autoDiff is a file of the changes shown to approve.
type autoDiff struct {
	change automate.Change
	view   *editor.DiffState
}

func (a *app) autoRoot() string { return filepath.Join(a.dir, autoFolder) }

// autoEnv is what runs need, the answers to programs included.
func (a *app) autoEnv() automate.Env {
	return automate.Env{Root: a.autoRoot(), Handler: a.autoHandle, Changed: func() {
		if !a.auto.dirty.Swap(true) {
			time.AfterFunc(150*time.Millisecond, func() {
				a.post(func() { a.auto.dirty.Store(false) })
			})
		}
	}}
}

// autoRefresh reads the automations again, every two seconds.
func (a *app) autoRefresh(force bool) {
	now := a.now()
	if !force && now.Sub(a.auto.listed) < 2*time.Second {
		return
	}
	a.auto.listed = now
	a.auto.gen++
	a.auto.list, _ = automate.List(a.autoRoot())
	if a.auto.metas == nil || force {
		a.auto.metas = automate.LoadMetas(a.autoRoot())
	}
}

func (a *app) autoByID(id string) *automate.Automation {
	for _, x := range a.auto.list {
		if x.ID == id {
			return x
		}
	}
	return nil
}

// autoTick runs the automations whose time has come. It runs every two
// seconds, with tick.
func (a *app) autoTick() {
	if a.dir == "" {
		return
	}
	a.autoRefresh(false)
	now := a.now()
	for _, x := range a.auto.list {
		m := a.auto.metas[x.ID]
		if m == nil || !m.Enabled || x.Err != "" || a.auto.procs[x.ID] != nil {
			continue
		}
		tr, err := automate.ParseTrigger(x.Trigger)
		if err != nil {
			continue
		}
		if tr.Watch != "" {
			// Run as what is in the folder changes; what was there when
			// the schedule went on, or the last run ended, is not news.
			sig := automate.Signature(tr.WatchDir(x.Dir()))
			if a.auto.sigs == nil {
				a.auto.sigs = map[string]string{}
			}
			old, seen := a.auto.sigs[x.ID]
			if !seen || old == sig {
				a.auto.sigs[x.ID] = sig
				continue
			}
		} else if !tr.Due(m.From(), now) {
			continue
		}
		if m.NeedsApproval(x.Code()) {
			if !a.auto.waiting[x.ID] {
				if a.auto.waiting == nil {
					a.auto.waiting = map[string]bool{}
				}
				a.auto.waiting[x.ID] = true
				a.plat.Notify("自动化等待确认", "「"+x.Name+"」的代码改过了，确认后才会定时运行")
			}
			continue
		}
		by := "定时"
		if tr.Watch != "" {
			by = "文件变化"
		}
		a.autoStart(x, by)
	}
}

// autoStart runs an automation; one not approved asks for the approval
// first.
func (a *app) autoStart(x *automate.Automation, by string) {
	if a.auto.procs[x.ID] != nil {
		return
	}
	a.autoSaveCode()
	if x.Err != "" {
		a.tell(x.Err)
		return
	}
	m := a.auto.metas.Of(x.ID)
	code := x.Code()
	if m.NeedsApproval(code) {
		a.openApproval(x, true)
		return
	}
	p, err := automate.Start(a.autoEnv(), x, by, a.now())
	if err != nil {
		m.Status, m.LastRun = automate.Failed, a.now()
		_ = a.auto.metas.Save(a.autoRoot())
		a.tell("运行失败：" + err.Error())
		if by != "手动" {
			a.plat.Notify("自动化没能运行", x.Name+"："+err.Error())
		}
		return
	}
	if a.auto.procs == nil {
		a.auto.procs = map[string]*automate.Process{}
	}
	a.auto.procs[x.ID] = p
	delete(a.auto.waiting, x.ID)
	m.LastRun, m.Status = a.now(), automate.Running
	_ = a.auto.metas.Save(a.autoRoot())
	if a.auto.sel == x.ID {
		a.auto.tab, a.auto.run = 1, ""
	}
	id, name := x.ID, x.Name
	go func() {
		<-p.Done()
		a.post(func() { a.autoEnded(id, name, p) })
	}()
}

func (a *app) autoEnded(id, name string, p *automate.Process) {
	if a.auto.procs[id] == p {
		delete(a.auto.procs, id)
	}
	r := p.Snapshot()
	if x := a.autoByID(id); x != nil && a.auto.sigs != nil {
		if tr, err := automate.ParseTrigger(x.Trigger); err == nil && tr.Watch != "" {
			a.auto.sigs[id] = automate.Signature(tr.WatchDir(x.Dir())) // what it wrote there itself
		}
	}
	m := a.auto.metas.Of(id)
	m.Status = r.Status
	_ = a.auto.metas.Save(a.autoRoot())
	switch r.Status {
	case automate.Failed, automate.TimeOut:
		msg := cmpOr(r.Error, lastLine(r.Log))
		a.plat.Notify("自动化出错："+name, msg)
	case automate.OK:
		if r.By != "手动" && r.Output != "" {
			a.plat.Notify(name, firstSentence(r.Output))
		}
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func (a *app) autoStop(id string) {
	if p := a.auto.procs[id]; p != nil {
		p.Stop()
	}
}

// autoStopAll stops the runs going on, as the app quits.
func (a *app) autoStopAll() {
	for _, p := range a.auto.procs {
		p.Stop()
	}
}

// autoSetEnabled turns an automation's schedule on or off; on, it counts
// from now.
func (a *app) autoSetEnabled(id string, on bool) {
	m := a.auto.metas.Of(id)
	m.Enabled = on
	if on {
		m.Since = a.now()
	}
	_ = a.auto.metas.Save(a.autoRoot())
}

// onMain runs fn on the main thread and waits for it.
func (a *app) onMain(fn func()) {
	done := make(chan struct{})
	a.post(func() {
		defer close(done)
		fn()
	})
	<-done
}

// autoHandle answers a program's call to mymind, off the main thread.
func (a *app) autoHandle(ctx context.Context, x *automate.Automation, out io.Writer, method string, params json.RawMessage) (any, error) {
	switch method {
	case "agent":
		var p struct{ Task, Repo, Agent string }
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return runAgent(ctx, cmpOr(p.Agent, x.Agent), cmpOr(p.Repo, x.Dir()), p.Task, out)
	case "maps.list", "maps.read", "maps.tasks", "maps.edit":
		return a.autoMaps(method, params)
	case "llm":
		var p struct {
			Prompt, Input, Model, System string
			JSON                         bool
			MaxTokens                    int `json:"max_tokens"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		var cfg config
		a.onMain(func() { cfg = a.cfg })
		return a.autoLLM(ctx, cfg, cmpOr(p.Model, x.Model), p.System, p.Prompt, p.Input, p.JSON, p.MaxTokens)
	case "notify":
		var p struct{ Title, Body string }
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		a.onMain(func() { a.plat.Notify(cmpOr(p.Title, x.Name), p.Body) })
		return true, nil
	case "confirm":
		var p struct{ Message, Detail string }
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		q := &autoConfirm{name: x.Name, message: p.Message, detail: p.Detail, answer: make(chan bool, 1)}
		a.onMain(func() {
			a.auto.confirms = append(a.auto.confirms, q)
			a.plat.Notify("「"+x.Name+"」等你确认", p.Message)
		})
		select {
		case ok := <-q.answer:
			return ok, nil
		case <-ctx.Done():
			a.post(func() { a.dropConfirm(q) })
			return false, ctx.Err()
		}
	case "clip":
		var p struct {
			Text, Title, URL, Note string
			Tags                   []string
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		var id string
		var err error
		a.onMain(func() {
			c := &library.Clip{Kind: library.KindText, Title: p.Title, Text: p.Text, URL: p.URL, Note: p.Note, Tags: p.Tags,
				Source: "自动化：" + x.Name}
			if strings.TrimSpace(p.Text) == "" && p.URL != "" {
				c.Kind = library.KindLink
			}
			err = a.lib.Add(c, a.now())
			var dup *library.DuplicateError
			switch {
			case errors.As(err, &dup):
				id, err = dup.Clip.ID, nil
			case err == nil:
				id = c.ID
				a.noteLibraryWrite()
				a.enqueueClip(c.ID, !a.cfg.LibraryManual)
			}
		})
		if err != nil {
			return nil, err
		}
		return id, nil
	case "search":
		var p struct {
			Query string
			Limit int
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if p.Limit <= 0 {
			p.Limit = 5
		}
		var out []map[string]string
		a.onMain(func() {
			for _, h := range a.lib.Search(p.Query, nil) {
				if len(out) == p.Limit {
					break
				}
				out = append(out, map[string]string{"id": h.Clip.ID, "title": h.Clip.Label(), "text": h.Clip.Text, "url": h.Clip.URL})
			}
		})
		return out, nil
	}
	return nil, fmt.Errorf("mymind 没有 %s", method)
}

func (a *app) dropConfirm(q *autoConfirm) {
	for i, c := range a.auto.confirms {
		if c == q {
			a.auto.confirms = append(a.auto.confirms[:i], a.auto.confirms[i+1:]...)
			return
		}
	}
}

// autoNoModel is the error of a program that asks a model without naming
// one: there is no default, the program says which.
const autoNoModel = "这个自动化没有配置模型：在 automation.toml 写 model = \"deepseek-chat\"（或 claude-…、cli:claude -p），单文件在开头写 # 模型: …，也可以 mymind.llm(..., model=\"…\")"

// autoLLM asks the model a program names: DeepSeek's or Claude's API with
// the keys of the settings, or a command line, given the prompt on its
// input.
func (a *app) autoLLM(ctx context.Context, cfg config, model, system, prompt, input string, asJSON bool, maxTokens int) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", errors.New(autoNoModel)
	}
	if maxTokens <= 0 {
		maxTokens = 8000
	}
	user := prompt
	if input != "" {
		user += "\n\n" + input
	}
	if asJSON {
		system = strings.TrimSpace(system + "\n只回答一个 JSON 对象（json），不要别的文字。")
	}
	var text string
	switch {
	case strings.HasPrefix(model, "cli:"):
		cmdline := strings.TrimSpace(strings.TrimPrefix(model, "cli:"))
		if cmdline == "" {
			return "", errors.New("cli: 后面要写命令，如 cli:claude -p")
		}
		stdin := user
		if system != "" {
			stdin = system + "\n\n" + user
		}
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cmdline)
		cmd.Stdin = strings.NewReader(stdin)
		cmd.Env = append(os.Environ(), "PATH="+automate.SearchPath())
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("%s：%v %s", cmdline, err, strings.TrimSpace(errOut.String()))
		}
		text = out.String()
	case strings.HasPrefix(model, "deepseek"):
		if deepseekKey(cfg) == "" {
			return "", errors.New("没有 DeepSeek 的 API Key：在设置的 AI 里填上")
		}
		req := dsRequest{Model: model, MaxTokens: maxTokens, Messages: []dsMessage{{Role: "user", Content: user}}}
		if system != "" {
			req.Messages = append([]dsMessage{{Role: "system", Content: system}}, req.Messages...)
		}
		if asJSON {
			req.ResponseFormat = &dsFormat{Type: "json_object"}
		}
		resp, err := a.sendDeepSeek(ctx, cfg, req, nil)
		if err != nil {
			return "", errors.New(deepseekError(err))
		}
		text = resp.Choices[0].Message.Content
		if resp.Choices[0].FinishReason == "length" {
			return "", errors.New("回答超出 max_tokens，没有写完")
		}
	case strings.HasPrefix(model, "claude"):
		if aiKey(cfg) == "" {
			return "", errors.New("没有 Claude 的 API Key：在设置的 AI 里填上")
		}
		params := anthropic.BetaMessageNewParams{
			Model: anthropic.Model(model), MaxTokens: int64(maxTokens),
			Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user))},
		}
		if system != "" {
			params.System = []anthropic.BetaTextBlockParam{{Text: system}}
		}
		resp, err := a.sendAI(ctx, cfg, params, nil)
		if err != nil {
			return "", err
		}
		for _, block := range resp.Content {
			if b, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
				text += b.Text
			}
		}
		if resp.StopReason == anthropic.BetaStopReasonMaxTokens {
			return "", errors.New("回答超出 max_tokens，没有写完")
		}
	default:
		return "", fmt.Errorf("不认识的模型「%s」：可以写 deepseek-…、claude-…，或 cli:命令", model)
	}
	text = strings.TrimSpace(text)
	if asJSON {
		return jsonPart(text), nil
	}
	return text, nil
}

// jsonPart is the JSON of an answer, around which a model may have put
// words or a code fence.
func jsonPart(text string) string {
	i := strings.IndexAny(text, "{[")
	if i < 0 {
		return text
	}
	end := "}"
	if text[i] == '[' {
		end = "]"
	}
	j := strings.LastIndex(text, end)
	if j < i {
		return text
	}
	return text[i : j+1]
}

// autoNoAgent is the error of a program that asks for the coding tool
// without naming one.
const autoNoAgent = "这个自动化没有配置编码工具：在 automation.toml 写 agent = \"claude -p\"（或 codex exec、aider --yes --message {prompt} 等），单文件在开头写 # 编码工具: …，也可以 mymind.agent(..., agent=\"…\")"

// runAgent runs a coding tool in a repository, given the task in place of
// {prompt} in its command or else on its input, its printing going into
// the run's log; it answers what the tool printed, the last of it.
func runAgent(ctx context.Context, command, repo, task string, log io.Writer) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New(autoNoAgent)
	}
	if strings.TrimSpace(task) == "" {
		return "", errors.New("没有要做的任务")
	}
	if rest, ok := strings.CutPrefix(repo, "~"); ok {
		home, _ := os.UserHomeDir()
		repo = filepath.Join(home, rest)
	}
	if st, err := os.Stat(repo); err != nil || !st.IsDir() {
		return "", fmt.Errorf("找不到仓库文件夹：%s", repo)
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	if strings.Contains(command, "{prompt}") {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", strings.ReplaceAll(command, "{prompt}", shellQuote(task)))
	} else {
		cmd.Stdin = strings.NewReader(task)
	}
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+automate.SearchPath())
	var answer tailBuffer
	w := io.MultiWriter(&answer, log)
	cmd.Stdout, cmd.Stderr = w, w
	fmt.Fprintf(log, "$ (%s) %s\n", repo, command)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s：%v", command, err)
	}
	return strings.TrimSpace(answer.String()), nil
}

// shellQuote quotes s for the shell, as one word.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// tailBuffer keeps the last of what is written to it.
type tailBuffer struct{ b []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > 200<<10 {
		t.b = t.b[len(t.b)-200<<10:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return string(t.b) }

// autoMaps answers mymind.maps: the maps read as data, and changed as an
// agent's tools change them, keeping the versions before and after. The
// map open is saved first, and shows the change as the tools' do.
func (a *app) autoMaps(method string, params json.RawMessage) (any, error) {
	var s *mcp.Server
	a.onMain(func() {
		a.flush()
		s = &mcp.Server{Dir: a.dir, Versions: a.versions, Version: version, Now: time.Now}
	})
	var p struct {
		Map, Status string
		DoneSince   string          `json:"done_since"`
		Tool        string          `json:"tool"`
		Args        json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	switch method {
	case "maps.list":
		return s.Maps()
	case "maps.read":
		return s.Map(p.Map)
	case "maps.tasks":
		var since time.Time
		if p.DoneSince != "" {
			var err error
			if since, err = parseSince(p.DoneSince); err != nil {
				return nil, err
			}
		}
		return s.Tasks(p.Map, p.Status, since)
	}
	switch p.Tool {
	case "add_nodes", "update_node", "move_node", "delete_node", "create_map":
	default:
		return nil, fmt.Errorf("maps 不能 %s", p.Tool)
	}
	msg, err := s.Call(p.Tool, p.Args)
	if err == nil {
		a.post(func() { a.refreshFiles() })
	}
	return msg, err
}

// parseSince reads a date or a time, as Python writes them.
func parseSince(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339Nano, "2006-01-02T15:04:05.999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("看不懂的日期：%s", s)
}
