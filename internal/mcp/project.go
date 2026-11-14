package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/gitbase"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// A project has one map. An agent need not name it: the map is the one
// whose project folder is the repository the agent works in (the folder
// the server runs in, or project_dir), or whose repository has the same
// origin, as on the user's other computer, and is made the first time
// something is written down for a repository that has none. What agents
// write without being told where goes to a branch of that map, 待整理,
// and to its log (model.File.Log); where things belong in the map stays
// the user's to say.

// SortBranch is the branch of a project's map where agents put the tasks
// they write down, for the user to place.
const SortBranch = "待整理"

// InboxName is the map of what is written down for no project, the app's
// inbox.
const InboxName = "收件箱"

// DecisionTag is the tag of the clips that are decisions.
const DecisionTag = "决策"

// agentSource is where a clip an agent kept says it came from.
const agentSource = "AI 工具"

// errNoProject says the agent works in no repository the server can tell.
var errNoProject = errors.New("不知道是哪个项目：MyMind 的 MCP 服务不在项目的代码目录里运行。请传 project_dir（代码仓库的绝对路径），或传 map")

// root is the repository an agent works in: the one dir is in, or the
// server's own folder is; dir itself when given and no repository.
func (s *Server) root(dir string) string {
	given := strings.TrimSpace(dir) != ""
	if !given {
		dir = s.Cwd
	}
	if dir == "" {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if r, err := gitbase.Main(dir); err == nil && r != "" {
		return r
	}
	if fi, err := os.Stat(dir); given && err == nil && fi.IsDir() {
		return filepath.Clean(dir)
	}
	return ""
}

// samePath tells whether two paths name one folder.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	real := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	return real(a) == real(b)
}

// projectMap finds the map of the project in dir, the oldest when the
// user bound several to it. With create, a project that has none gets
// one, named as its folder is: a map of that name with no project folder
// yet is taken for it rather than a second made beside it.
func (s *Server) projectMap(dir string, create bool) (path string, f *model.File, err error) {
	root := s.root(dir)
	if root == "" {
		return "", nil, errNoProject
	}
	files, err := model.AllFilesIn(s.Dir)
	if err != nil {
		return "", nil, err
	}
	name := filepath.Base(root)
	remote := gitbase.Remote(root)
	var unbound, byRepo string
	var unboundFile, byRepoFile *model.File
	for _, p := range files {
		g, err := model.Load(p)
		if err != nil {
			continue
		}
		if samePath(g.Project, root) {
			if g.Repo == "" {
				g.Repo = remote // kept with what is written next
			}
			return p, g, nil
		}
		if remote != "" && g.Repo == remote && byRepo == "" {
			byRepo, byRepoFile = p, g
		}
		if g.Project == "" && unbound == "" && strings.TrimSuffix(filepath.Base(p), model.Ext) == name {
			unbound, unboundFile = p, g
		}
	}
	if byRepo != "" { // the project's, made on another computer: its folder there is left as it is
		return byRepo, byRepoFile, nil
	}
	if !create {
		return "", nil, fmt.Errorf("项目 %s 还没有导图。用 capture、log_decision 或 log_session 记下第一条时会自动建一张", root)
	}
	if unbound != "" {
		unboundFile.Project, unboundFile.Repo = root, remote
		return unbound, unboundFile, nil
	}
	path = filepath.Join(s.Dir, model.FileName(name))
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		path = filepath.Join(s.Dir, model.FileName(fmt.Sprintf("%s %d", name, i)))
	}
	f = model.NewFile(name)
	f.Project, f.Repo = root, remote
	return path, f, nil
}

// inbox is the map of what belongs to no project, made when missing.
func (s *Server) inbox() (string, *model.File, error) {
	path := filepath.Join(s.Dir, model.FileName(InboxName))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f := model.NewFile(InboxName)
		f.Sheets[0].Root.Note = "随手记下的想法和待办，之后整理到对应的导图里（右键节点「移到其他导图…」）。"
		return path, f, nil
	}
	f, err := model.Load(path)
	return path, f, err
}

// sortBranch is the branch tasks written down go under, made when missing.
func sortBranch(f *model.File) *model.Node {
	root := f.Sheets[0].Root
	for _, n := range root.Children {
		if n.Title == SortBranch {
			return n
		}
	}
	n := &model.Node{ID: model.NewID(), Title: SortBranch,
		Note: "AI 记下的待办先放在这里，由你拖到它该在的地方。"}
	root.Children = append(root.Children, n)
	return n
}

// log adds an entry to a map's log, with who reported it and where the
// code was.
func (s *Server) log(f *model.File, kind, text string, e model.LogEntry) model.LogEntry {
	e.Kind, e.Text, e.At, e.Agent = kind, text, s.Now(), s.Agent
	if e.Commit == "" {
		dir := f.Project
		if r := s.root(""); r != "" && (samePath(r, f.Project) || f.Repo != "" && gitbase.Remote(r) == f.Repo) {
			dir = s.Cwd // the worktree the agent is in, and its branch
		}
		if h, err := gitbase.HeadOf(dir); dir != "" && err == nil {
			e.Commit, e.Branch = h.Commit, h.Branch
		}
	}
	return f.AddLog(e)
}

// clean is what an agent wrote, with what looks like a secret in dots: a
// map's words are shown as they are.
func clean(s string) string { return library.Mask(strings.TrimSpace(s)) }

// taskNodes makes tasks to do of lines.
func taskNodes(lines []string) []*model.Node {
	var out []*model.Node
	for _, l := range lines {
		l = clean(l)
		for _, p := range []string{"- [ ]", "- ", "* "} {
			l = strings.TrimSpace(strings.TrimPrefix(l, p))
		}
		if l == "" {
			continue
		}
		title, note, _ := strings.Cut(l, "\n")
		out = append(out, &model.Node{ID: model.NewID(), Title: strings.TrimSpace(title), Note: strings.TrimSpace(note),
			Task: &model.Task{Status: model.StatusTodo}})
	}
	return out
}

func ids(nodes []*model.Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.ID)
	}
	return out
}

// target is the map a thing is written down in: the one named, else the
// project's, else, with orInbox, the inbox.
func (s *Server) target(name, dir string, orInbox bool) (path string, f *model.File, isInbox bool, err error) {
	if strings.TrimSpace(name) != "" {
		path, f, err = s.find(name)
		return path, f, false, err
	}
	path, f, err = s.projectMap(dir, true)
	if err == errNoProject && orInbox {
		path, f, err = s.inbox()
		return path, f, true, err
	}
	return path, f, false, err
}

// save writes a map an agent wrote in: a new one as it is, another with
// its versions.
func (s *Server) save(path string, before, f *model.File, summary string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := f.Save(path); err != nil {
			return err
		}
		_, _ = s.Versions.Save(path, f, versions.AI, "为项目建的导图", s.Now())
		return nil
	}
	return s.write(path, before, f, summary)
}

// capture writes down a task, in the project's map, or a thought, in the
// library.
func (s *Server) capture(args map[string]any) (string, error) {
	get := func(k string) string { v, _ := args[k].(string); return v }
	text := clean(get("text"))
	if text == "" {
		return "", errors.New("text 是空的")
	}
	kind := get("kind")
	if kind == "" {
		kind = "task"
	}
	path, f, isInbox, err := s.target(get("map"), get("project_dir"), true)
	if err != nil {
		return "", err
	}
	before := f.Clone()
	where := s.rel(path)
	switch kind {
	case "idea":
		l, err := library.Open(library.In(s.Dir))
		if err != nil {
			return "", err
		}
		c := &library.Clip{Kind: library.KindText, Text: text, Source: agentSource}
		if !isInbox {
			c.Tags = []string{filepath.Base(where)}
		}
		if err := l.Add(c, s.Now()); err != nil {
			var dup *library.DuplicateError
			if errors.As(err, &dup) {
				return "收集里已经有这一条了（" + dup.Clip.ID + "），没有再记。", nil
			}
			return "", err
		}
		if isInbox {
			return "已记到收集（" + c.ID + "）。", nil
		}
		s.log(f, model.LogIdea, firstLine(text), model.LogEntry{Clip: c.ID})
		if err := s.save(path, before, f, "记下一条想法"); err != nil {
			return "", err
		}
		return fmt.Sprintf("已记到收集（%s），并记在项目「%s」的记录里。", c.ID, where), nil
	case "task":
		nodes := taskNodes([]string{text})
		if len(nodes) == 0 {
			return "", errors.New("text 是空的")
		}
		n := nodes[0]
		for k, set := range map[string]func(time.Time){"due": func(t time.Time) { n.Task.Due = &t }, "start": func(t time.Time) { n.Task.Start = &t }} {
			if v := strings.TrimSpace(get(k)); v != "" {
				t, err := time.ParseInLocation("2006-01-02", v, time.Local)
				if err != nil {
					return "", fmt.Errorf("%s 的日期格式应为 YYYY-MM-DD", k)
				}
				set(t)
			}
		}
		if v := get("priority"); v != "" {
			p, ok := priorityValues[v]
			if !ok {
				return "", fmt.Errorf("未知的 priority %q", v)
			}
			n.Task.Priority = p
		}
		parent := f.Sheets[0].Root
		switch under := get("under"); {
		case under != "":
			_, p, _, err := findNode(f, under)
			if err != nil {
				return "", err
			}
			parent = p
		case !isInbox:
			parent = sortBranch(f)
		}
		parent.Children = append(parent.Children, n)
		parent.Collapsed = false
		if !isInbox {
			s.log(f, model.LogTask, n.Title, model.LogEntry{Nodes: []string{n.ID}})
		}
		if err := s.save(path, before, f, "记下待办："+n.Title); err != nil {
			return "", err
		}
		return fmt.Sprintf("已记到「%s」的「%s」下：%s", where, parent.Title, nodeLine(n)), nil
	}
	return "", fmt.Errorf("未知的 kind %q：task 或 idea", kind)
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(l)
}

// logDecision keeps a decision: whole as a clip of the library, in the
// project's log, and linked from the node it is about.
func (s *Server) logDecision(args map[string]any) (string, error) {
	get := func(k string) string { v, _ := args[k].(string); return v }
	title, why := firstLine(clean(get("title"))), clean(get("why"))
	if title == "" || why == "" {
		return "", errors.New("title 和 why 都要写：决定了什么，为什么")
	}
	path, f, _, err := s.target(get("map"), get("project_dir"), false)
	if err != nil {
		return "", err
	}
	before := f.Clone()
	var node *model.Node
	if id := get("node_id"); id != "" {
		if _, node, _, err = findNode(f, id); err != nil {
			return "", err
		}
	}
	where := s.rel(path)
	text := "决定：" + title + "\n\n原因：" + why
	if alt := clean(get("alternatives")); alt != "" {
		text += "\n\n没有选的方案：" + alt
	}
	text += "\n\n项目：" + filepath.Base(where)
	l, err := library.Open(library.In(s.Dir))
	if err != nil {
		return "", err
	}
	c := &library.Clip{Kind: library.KindText, Title: title, Text: text, Source: agentSource,
		Tags: []string{DecisionTag, filepath.Base(where)}}
	if err := l.Add(c, s.Now()); err != nil {
		var dup *library.DuplicateError
		if errors.As(err, &dup) {
			return "这条决策已经记过了（" + dup.Clip.ID + "）。", nil
		}
		return "", err
	}
	e := model.LogEntry{Clip: c.ID}
	if node != nil {
		if link := "clip:" + c.ID; !slices.Contains(node.Links, link) {
			node.Links = append(node.Links, link)
		}
		e.Nodes = []string{node.ID}
	}
	s.log(f, model.LogDecision, title, e)
	if err := s.save(path, before, f, "记下决策："+title); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("决策已记下（收集 %s），在项目「%s」的记录里", c.ID, where)
	if node != nil {
		msg += "，并链接到「" + node.Title + "」"
	}
	return msg + "。", nil
}

// logSession keeps what a session of work did, and the tasks it left.
func (s *Server) logSession(args map[string]any) (string, error) {
	get := func(k string) string { v, _ := args[k].(string); return v }
	summary := clean(get("summary"))
	if summary == "" {
		return "", errors.New("summary 是空的：用一两句话说这次做了什么")
	}
	path, f, _, err := s.target(get("map"), get("project_dir"), false)
	if err != nil {
		return "", err
	}
	before := f.Clone()
	var lines []string
	if vs, ok := args["todos"].([]any); ok {
		for _, v := range vs {
			if t, ok := v.(string); ok {
				lines = append(lines, t)
			}
		}
	}
	nodes := taskNodes(lines)
	if len(nodes) > 0 {
		b := sortBranch(f)
		b.Children = append(b.Children, nodes...)
		b.Collapsed = false
	}
	s.log(f, model.LogSession, summary, model.LogEntry{Nodes: ids(nodes)})
	if err := s.save(path, before, f, firstLine(summary)); err != nil {
		return "", err
	}
	s.reported(get("project_dir"))
	msg := fmt.Sprintf("已记在项目「%s」的记录里。", s.rel(path))
	if len(nodes) > 0 {
		msg += fmt.Sprintf("留下的 %d 条待办在「%s」下：\n", len(nodes), SortBranch)
		for _, n := range nodes {
			msg += "- " + nodeLine(n) + "\n"
		}
	}
	return msg, nil
}

// brief is what an agent reads before it works on a project: what is in
// hand and to do, what was decided and done lately, the map's first
// levels to place things by, and what changed in the code since the map
// was brought up to date.
func (s *Server) brief(name, dir string) (string, error) {
	var path string
	var f *model.File
	var err error
	if strings.TrimSpace(name) != "" {
		path, f, err = s.find(name)
	} else {
		path, f, err = s.projectMap(dir, false)
	}
	if err != nil {
		return "", err
	}
	return s.briefOf(path, f), nil
}

// The most of each part a brief lists.
const (
	briefTasks     = 30
	briefDecisions = 8
	briefSessions  = 5
)

func (s *Server) briefOf(path string, f *model.File) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 项目「%s」（MyMind 导图 %s）\n", f.Sheets[0].Root.Title, s.rel(path))
	if f.Project != "" {
		fmt.Fprintf(&b, "代码目录：%s\n", f.Project)
	}
	var doing, todo []model.TaskNode
	for _, t := range f.Tasks() {
		switch t.Node.Task.Status {
		case model.StatusDoing:
			doing = append(doing, t)
		case model.StatusTodo:
			todo = append(todo, t)
		}
	}
	slices.SortStableFunc(todo, func(x, y model.TaskNode) int {
		if d := int(y.Node.Task.Priority) - int(x.Node.Task.Priority); d != 0 {
			return d
		}
		xd, yd := x.Node.Task.Due, y.Node.Task.Due
		switch {
		case xd != nil && yd != nil:
			return xd.Compare(*yd)
		case xd != nil:
			return -1
		case yd != nil:
			return 1
		}
		return 0
	})
	list := func(title string, tasks []model.TaskNode, max int) {
		if len(tasks) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n## %s（%d）\n", title, len(tasks))
		for i, t := range tasks {
			if i == max {
				fmt.Fprintf(&b, "- ……还有 %d 条，用 list_tasks 看全部\n", len(tasks)-max)
				break
			}
			line := nodeLine(t.Node)
			if w := where(t.Sheet, t.Node.ID); w != "" {
				line = w + " › " + line
			}
			b.WriteString("- " + line + "\n")
		}
	}
	list("进行中", doing, briefTasks)
	list("待办", todo, briefTasks)
	if len(doing)+len(todo) == 0 {
		b.WriteString("\n没有未完成的任务。\n")
	}
	var decisions, sessions []model.LogEntry
	for i := len(f.Log) - 1; i >= 0; i-- {
		switch e := f.Log[i]; e.Kind {
		case model.LogDecision:
			if len(decisions) < briefDecisions {
				decisions = append(decisions, e)
			}
		case model.LogSession:
			if len(sessions) < briefSessions {
				sessions = append(sessions, e)
			}
		}
	}
	if len(decisions) > 0 {
		b.WriteString("\n## 最近的决策（原因用 read_clip 看）\n")
		for _, e := range decisions {
			fmt.Fprintf(&b, "- %s %s（clip %s）\n", e.At.Format("01-02"), e.Text, e.Clip)
		}
	}
	if len(sessions) > 0 {
		b.WriteString("\n## 最近几次做了什么\n")
		for _, e := range sessions {
			fmt.Fprintf(&b, "- %s %s\n", e.At.Format("01-02 15:04"), strings.ReplaceAll(e.Text, "\n", " "))
		}
	}
	b.WriteString("\n## 导图的前两层（放东西时用这些 id；全图用 read_map）\n")
	for _, d := range f.Sheets {
		b.WriteString("- " + d.Root.Title + " {id:" + d.Root.ID + "}\n")
		for _, n := range d.Root.Children {
			fmt.Fprintf(&b, "  - %s {id:%s}", n.Title, n.ID)
			if k := len(n.Children); k > 0 {
				fmt.Fprintf(&b, "（下有 %d 个）", k)
			}
			b.WriteString("\n")
		}
	}
	if f.Baseline != nil {
		b.WriteString(BaselineText(path, f))
	}
	return b.String()
}
