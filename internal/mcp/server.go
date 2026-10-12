// Package mcp serves the maps of a folder to AI coding agents (Claude Code,
// Cursor and others) over the Model Context Protocol: JSON-RPC 2.0 on
// stdin and stdout, a message a line. Agents read maps, search them, list
// tasks, and add, change, move and delete nodes. Every change is written
// to the map's file, which the app reloads, and kept as a version before
// and after, with what the agent said it did.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// Server answers an agent's requests about the maps of Dir.
type Server struct {
	Dir      string
	Versions versions.Store
	Version  string // the app's, as the server's
	Now      func() time.Time
	// Embedder, when set up, finds clips by meaning too.
	Embedder library.Embedder
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// supported are the protocol versions the server speaks, the newest first.
var supported = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Serve answers the requests read from r on w until r ends.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 1<<20), 64<<20)
	enc := json.NewEncoder(w)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		if len(req.ID) == 0 { // a notification: no answer
			continue
		}
		result, err := s.handle(req)
		resp := response{JSONRPC: "2.0", ID: req.ID, Result: result}
		if err != nil {
			resp.Result, resp.Error = nil, err
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return in.Err()
}

func (s *Server) handle(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := supported[0]
		if slices.Contains(supported, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "mymind", "title": "MyMind", "version": s.Version},
			"instructions":    instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		text, err := s.call(p.Name, p.Arguments)
		if err != nil {
			return map[string]any{"content": []map[string]string{{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}}, nil
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

const instructions = `MyMind holds the user's project maps: the architecture, features, implementation notes, tasks and the next optimizations, as trees of nodes. Read the relevant map before working on a project, and keep it current as you work: mark tasks done or in progress, add notes on how things were implemented and decisions made, link nodes to the files (path:line) and PRs that implement them, and add nodes for follow-ups and optimizations you notice. Node IDs appear as {id:…} in read_map's output. Always pass a short "summary" of what you changed; the user reviews changes in the app's history. The user also keeps a library of collected notes, links, pictures and files: search it with search_library when the task may build on what they collected.`

// tool is a tool as tools/list describes it.
type tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

func schema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var (
	mapProp     = str("The map: its name (as list_maps shows it) or its path relative to the maps folder.")
	summaryProp = str("A short description of the change, in the user's language, shown in the map's history.")
	statusProp  = map[string]any{"type": "string", "enum": []string{"idea", "todo", "doing", "done", "none"},
		"description": "The task status; none makes the node no longer a task."}
	readOnly = map[string]any{"readOnlyHint": true}
)

var tools = []tool{
	{"list_maps", "List maps", "List the maps in the maps folder, with their canvases and how many tasks are open.",
		schema(nil, map[string]any{}), readOnly},
	{"read_map", "Read a map", "Read a map as an indented outline: every node with its {id:…}, task status as [ ] / [~] / [x] / [?] (todo, doing, done, idea), priority, dates, #tags and its note below it.",
		schema([]string{"map"}, map[string]any{"map": mapProp, "sheet": str("A canvas's title; all canvases when left out.")}), readOnly},
	{"search", "Search maps", "Find nodes whose title, note or tags contain the query, in one map or all.",
		schema([]string{"query"}, map[string]any{"query": str("Text to find, case-insensitive."), "map": mapProp}), readOnly},
	{"list_tasks", "List tasks", "List task nodes, optionally of one map and one status, with their ids, dates and where they are.",
		schema(nil, map[string]any{"map": mapProp, "status": map[string]any{"type": "string", "enum": []string{"idea", "todo", "doing", "done", "open"},
			"description": "Only tasks with this status; open means not done."}}), readOnly},
	{"add_nodes", "Add nodes", "Add nodes under a node, written as a Markdown outline: a line a node, nested by two-space indentation; '- [ ] text' makes a task to do and '- [x] text' a done one.",
		schema([]string{"map", "parent_id", "outline", "summary"}, map[string]any{"map": mapProp,
			"parent_id": str("The id of the node to add under; use the root's id for top-level branches."),
			"outline":   str("The nodes, as a Markdown list."), "summary": summaryProp}), nil},
	{"update_node", "Update a node", "Change a node: its title, note (replace or append), tags, and task status, priority and dates. Fields left out stay as they are.",
		schema([]string{"map", "id", "summary"}, map[string]any{"map": mapProp, "id": str("The node's id."),
			"title": str("A new title."), "note": str("A new note, replacing the old one (Markdown)."),
			"append_note": str("Text to add at the end of the note, as a new paragraph."),
			"add_tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"remove_tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"add_links": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
				"description": "Links to add: web pages (PRs, issues, docs) or files of the code as path or path:line, relative to the project folder or absolute."},
			"remove_links": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"status":       statusProp,
			"priority":     map[string]any{"type": "string", "enum": []string{"none", "low", "medium", "high"}},
			"start":        str("Start date as YYYY-MM-DD, or empty to clear."), "due": str("Due date as YYYY-MM-DD, or empty to clear."),
			"estimate": map[string]any{"type": "number", "description": "The work it takes, in hours."},
			"add_after": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
				"description": "Ids of tasks to finish before this one."},
			"remove_after": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"summary":      summaryProp}), nil},
	{"move_node", "Move a node", "Move a node, with all below it, under another node.",
		schema([]string{"map", "id", "parent_id", "summary"}, map[string]any{"map": mapProp, "id": str("The node to move."),
			"parent_id": str("The node to move it under."),
			"index":     map[string]any{"type": "integer", "description": "Its place among the new parent's children; at the end when left out."},
			"summary":   summaryProp}), nil},
	{"delete_node", "Delete a node", "Delete a node and all below it. The user can restore it from the map's history.",
		schema([]string{"map", "id", "summary"}, map[string]any{"map": mapProp, "id": str("The node to delete."), "summary": summaryProp}),
		map[string]any{"destructiveHint": true}},
	{"search_library", "Search the library", "Search the user's library of collected notes, links (with the pages' text), pictures and files, which the app's AI sorted into categories and tagged. Returns the best matches with their ids, summaries and matching excerpts.",
		schema([]string{"query"}, map[string]any{"query": str("Keywords, separated by spaces; Chinese or English."),
			"category": str("Only clips of this category."), "limit": map[string]any{"type": "integer", "description": "How many clips at most; 10 when left out."}}), readOnly},
	{"read_clip", "Read a clip", "Read a clip of the library in full: its text, the page's text for a link, notes, tags.",
		schema([]string{"id"}, map[string]any{"id": str("The clip's id, as search_library shows it.")}), readOnly},
	{"add_clip", "Add to the library", "Keep a note or a link in the user's library, where the app's AI sorts it out. A lone URL as text is kept as a link, whose page the app reads.",
		schema([]string{"text"}, map[string]any{"text": str("The note's text, or a URL."), "title": str("A title."),
			"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "category": str("A category.")}), nil},
	{"create_map", "Create a map", "Create a new map in the maps folder, optionally with nodes from a Markdown outline under its root.",
		schema([]string{"name"}, map[string]any{"name": str("The map's name, also its file's."), "root_title": str("The central topic; the name when left out."),
			"outline": str("Nodes under the root, as a Markdown list."), "folder": str("A folder within the maps folder to put it in.")}), nil},
}

func (s *Server) call(name string, raw json.RawMessage) (string, error) {
	var args map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("参数无法解析：%v", err)
		}
	}
	get := func(k string) string {
		v, _ := args[k].(string)
		return v
	}
	switch name {
	case "list_maps":
		return s.listMaps()
	case "read_map":
		path, f, err := s.find(get("map"))
		if err != nil {
			return "", err
		}
		return readMap(s.rel(path), f, get("sheet")), nil
	case "search":
		return s.search(get("query"), get("map"))
	case "list_tasks":
		return s.listTasks(get("map"), get("status"))
	case "add_nodes", "update_node", "move_node", "delete_node":
		path, f, err := s.find(get("map"))
		if err != nil {
			return "", err
		}
		before := f.Clone()
		var msg string
		switch name {
		case "add_nodes":
			msg, err = addNodes(f, get("parent_id"), get("outline"))
		case "update_node":
			msg, err = updateNode(f, get("id"), args)
		case "move_node":
			index := -1
			if v, ok := args["index"].(float64); ok {
				index = int(v)
			}
			msg, err = moveNode(f, get("id"), get("parent_id"), index)
		case "delete_node":
			msg, err = deleteNode(f, get("id"))
		}
		if err != nil {
			return "", err
		}
		if err := s.write(path, before, f, get("summary")); err != nil {
			return "", err
		}
		return msg, nil
	case "create_map":
		return s.createMap(get("name"), get("root_title"), get("outline"), get("folder"))
	case "search_library", "read_clip", "add_clip":
		l, err := library.Open(library.In(s.Dir))
		if err != nil {
			return "", err
		}
		switch name {
		case "search_library":
			limit := 10
			if v, ok := args["limit"].(float64); ok && v > 0 {
				limit = min(int(v), 50)
			}
			var qv []float32
			if s.Embedder.On() && strings.TrimSpace(get("query")) != "" {
				if vs, err := s.Embedder.Embed(context.Background(), []string{get("query")}); err == nil {
					qv = vs[0]
				}
			}
			text, _ := library.SearchText(l, get("query"), get("category"), limit, qv, s.Embedder.Name())
			return text, nil
		case "read_clip":
			c := l.Find(get("id"))
			if c == nil {
				return "", fmt.Errorf("no clip %q", get("id"))
			}
			return library.Full(c, 50000), nil
		}
		text := strings.TrimSpace(get("text"))
		if text == "" {
			return "", errors.New("text is empty")
		}
		c := &library.Clip{Kind: library.KindText, Text: text, Title: get("title"), Category: get("category"), Source: "AI 工具"}
		if library.IsURL(text) {
			c.Kind, c.Text, c.URL = library.KindLink, "", text
		}
		if tags, ok := args["tags"].([]any); ok {
			for _, t := range tags {
				if t, ok := t.(string); ok && strings.TrimSpace(t) != "" {
					c.Tags = append(c.Tags, strings.TrimSpace(t))
				}
			}
		}
		if err := l.Add(c, s.Now()); err != nil {
			return "", err
		}
		return "Kept in the library as " + c.ID + ".", nil
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

// write saves a map an agent changed, keeping the versions before and after.
func (s *Server) write(path string, before, after *model.File, summary string) error {
	now := s.Now()
	_, _ = s.Versions.Save(path, before, versions.BeforeAI, "", now)
	if err := after.Save(path); err != nil {
		return err
	}
	_, _ = s.Versions.Save(path, after, versions.AI, summary, now.Add(time.Millisecond))
	return nil
}

func (s *Server) rel(path string) string {
	rel, err := filepath.Rel(s.Dir, path)
	if err != nil {
		return path
	}
	return strings.TrimSuffix(rel, model.Ext)
}

// find reads the map a name or a path names.
func (s *Server) find(name string) (string, *model.File, error) {
	if strings.TrimSpace(name) == "" {
		return "", nil, errors.New("缺少 map 参数，先用 list_maps 查看导图")
	}
	files, err := model.AllFilesIn(s.Dir)
	if err != nil {
		return "", nil, err
	}
	name = strings.TrimSuffix(strings.TrimSpace(name), model.Ext)
	var matches []string
	for _, p := range files {
		rel := s.rel(p)
		base := strings.TrimSuffix(filepath.Base(p), model.Ext)
		if rel == name || base == name {
			matches = []string{p}
			break
		}
		if strings.Contains(strings.ToLower(rel), strings.ToLower(name)) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 0:
		return "", nil, fmt.Errorf("找不到导图 %q，可用 list_maps 查看", name)
	case 1:
		f, err := model.Load(matches[0])
		return matches[0], f, err
	}
	var names []string
	for _, p := range matches {
		names = append(names, s.rel(p))
	}
	return "", nil, fmt.Errorf("%q 匹配到多个导图：%s", name, strings.Join(names, "、"))
}

func (s *Server) listMaps() (string, error) {
	files, err := model.AllFilesIn(s.Dir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "导图文件夹 " + s.Dir + " 里还没有导图。", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "导图文件夹：%s\n\n", s.Dir)
	for _, p := range files {
		f, err := model.Load(p)
		if err != nil {
			continue
		}
		open, total := 0, 0
		for _, t := range f.Tasks() {
			total++
			if t.Node.Task.Status != model.StatusDone {
				open++
			}
		}
		var sheets []string
		for _, d := range f.Sheets {
			sheets = append(sheets, d.Title)
		}
		fmt.Fprintf(&b, "- %s — 中心主题「%s」；画布：%s；任务 %d（未完成 %d）\n", s.rel(p), f.Sheets[0].Root.Title,
			strings.Join(sheets, "、"), total, open)
	}
	return b.String(), nil
}

var statusMarks = map[model.Status]string{model.StatusTodo: "[ ]", model.StatusDoing: "[~]", model.StatusDone: "[x]", model.StatusIdea: "[?]"}

// readMap writes a map as an outline with the ids of its nodes.
func readMap(name string, f *model.File, sheet string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", name)
	for _, d := range f.Sheets {
		if sheet != "" && d.Title != sheet {
			continue
		}
		fmt.Fprintf(&b, "\n## 画布：%s\n\n", d.Title)
		var walk func(n *model.Node, depth int)
		walk = func(n *model.Node, depth int) {
			indent := strings.Repeat("  ", depth)
			b.WriteString(indent + "- " + nodeLine(n) + "\n")
			for _, line := range strings.Split(strings.TrimSpace(n.Note), "\n") {
				if line != "" {
					b.WriteString(indent + "  > " + line + "\n")
				}
			}
			for _, c := range n.Children {
				walk(c, depth+1)
			}
		}
		walk(d.Root, 0)
	}
	return b.String()
}

func nodeLine(n *model.Node) string {
	var b strings.Builder
	if n.Task != nil {
		b.WriteString(statusMarks[n.Task.Status] + " ")
	}
	b.WriteString(n.Title)
	if n.Task != nil {
		if n.Task.Priority != model.PriorityNone {
			b.WriteString(" !" + priorityNames[n.Task.Priority])
		}
		if n.Task.Start != nil {
			b.WriteString(" start:" + n.Task.Start.Format("2006-01-02"))
		}
		if n.Task.Due != nil {
			b.WriteString(" due:" + n.Task.Due.Format("2006-01-02"))
		}
		if n.Task.Estimate > 0 {
			b.WriteString(" est:" + strconv.FormatFloat(n.Task.Estimate, 'f', -1, 64) + "h")
		}
		for _, id := range n.Task.After {
			b.WriteString(" after:" + id)
		}
	}
	for _, t := range n.Tags {
		b.WriteString(" #" + t)
	}
	for _, l := range n.Links {
		b.WriteString(" <" + l + ">")
	}
	b.WriteString(" {id:" + n.ID + "}")
	return b.String()
}

var priorityNames = map[model.Priority]string{model.PriorityLow: "low", model.PriorityMedium: "medium", model.PriorityHigh: "high"}

func (s *Server) search(query, only string) (string, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return "", errors.New("缺少 query")
	}
	paths, err := s.paths(only)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	n := 0
	for _, p := range paths {
		f, err := model.Load(p)
		if err != nil {
			continue
		}
		for _, d := range f.Sheets {
			d.Walk(func(node, _ *model.Node, _ int) bool {
				if n < 100 && node.Matches(q) {
					n++
					fmt.Fprintf(&b, "- %s › %s：%s\n", s.rel(p), where(d, node.ID), nodeLine(node))
				}
				return true
			})
		}
	}
	if n == 0 {
		return "没有找到 " + query, nil
	}
	return b.String(), nil
}

func where(d *model.Document, id string) string {
	var titles []string
	for _, n := range d.Path(id) {
		titles = append(titles, n.Title)
	}
	if len(titles) > 1 {
		titles = titles[:len(titles)-1]
	}
	return strings.Join(titles, " › ")
}

func (s *Server) paths(only string) ([]string, error) {
	if only != "" {
		p, _, err := s.find(only)
		if err != nil {
			return nil, err
		}
		return []string{p}, nil
	}
	return model.AllFilesIn(s.Dir)
}

func (s *Server) listTasks(only, status string) (string, error) {
	paths, err := s.paths(only)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	n := 0
	for _, p := range paths {
		f, err := model.Load(p)
		if err != nil {
			continue
		}
		for _, t := range f.Tasks() {
			st := t.Node.Task.Status
			if status == "open" && st == model.StatusDone || status != "" && status != "open" && string(st) != status {
				continue
			}
			n++
			fmt.Fprintf(&b, "- %s › %s：%s\n", s.rel(p), where(t.Sheet, t.Node.ID), nodeLine(t.Node))
		}
	}
	if n == 0 {
		return "没有符合的任务。", nil
	}
	return b.String(), nil
}

func findNode(f *model.File, id string) (*model.Document, *model.Node, *model.Node, error) {
	d := f.SheetOf(id)
	if d == nil {
		return nil, nil, nil, fmt.Errorf("找不到节点 %q，先用 read_map 查看节点 id", id)
	}
	n, p := d.Find(id)
	return d, n, p, nil
}

func addNodes(f *model.File, parentID, outline string) (string, error) {
	_, parent, _, err := findNode(f, parentID)
	if err != nil {
		return "", err
	}
	nodes := model.ParseOutline(outline)
	if len(nodes) == 0 {
		return "", errors.New("outline 是空的")
	}
	parent.Children = append(parent.Children, nodes...)
	parent.Collapsed = false
	var b strings.Builder
	fmt.Fprintf(&b, "已在「%s」下添加：\n", parent.Title)
	var walk func(n *model.Node, depth int)
	walk = func(n *model.Node, depth int) {
		b.WriteString(strings.Repeat("  ", depth) + "- " + nodeLine(n) + "\n")
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	for _, n := range nodes {
		walk(n, 0)
	}
	return b.String(), nil
}

var statusNames = map[string]model.Status{"idea": model.StatusIdea, "todo": model.StatusTodo, "doing": model.StatusDoing, "done": model.StatusDone}
var priorityValues = map[string]model.Priority{"none": model.PriorityNone, "low": model.PriorityLow, "medium": model.PriorityMedium, "high": model.PriorityHigh}

func updateNode(f *model.File, id string, args map[string]any) (string, error) {
	_, n, _, err := findNode(f, id)
	if err != nil {
		return "", err
	}
	text := func(k string) (string, bool) {
		v, ok := args[k].(string)
		return v, ok
	}
	list := func(k string) []string {
		var out []string
		if vs, ok := args[k].([]any); ok {
			for _, v := range vs {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimPrefix(strings.TrimSpace(s), "#"))
				}
			}
		}
		return out
	}
	if v, ok := text("title"); ok && strings.TrimSpace(v) != "" {
		n.Title = strings.TrimSpace(v)
	}
	if v, ok := text("note"); ok {
		n.Note = v
	}
	if v, ok := text("append_note"); ok && strings.TrimSpace(v) != "" {
		if strings.TrimSpace(n.Note) == "" {
			n.Note = v
		} else {
			n.Note = strings.TrimRight(n.Note, "\n") + "\n\n" + v
		}
	}
	for _, t := range list("add_tags") {
		if !slices.Contains(n.Tags, t) {
			n.Tags = append(n.Tags, t)
		}
	}
	for _, t := range list("remove_tags") {
		n.Tags = slices.DeleteFunc(n.Tags, func(x string) bool { return x == t })
	}
	for _, l := range links(args["add_links"]) {
		if !slices.Contains(n.Links, l) {
			n.Links = append(n.Links, l)
		}
	}
	for _, l := range links(args["remove_links"]) {
		n.Links = slices.DeleteFunc(n.Links, func(x string) bool { return x == l })
	}
	if len(n.Links) == 0 {
		n.Links = nil
	}
	if len(n.Tags) == 0 {
		n.Tags = nil
	}
	task := func() *model.Task {
		if n.Task == nil {
			n.Task = &model.Task{Status: model.StatusTodo}
		}
		return n.Task
	}
	if v, ok := text("status"); ok && v != "" {
		if v == "none" {
			n.Task = nil
		} else if st, ok := statusNames[v]; ok {
			task().SetStatus(st, time.Now())
		} else {
			return "", fmt.Errorf("未知的 status %q", v)
		}
	}
	if v, ok := text("priority"); ok && v != "" {
		p, ok := priorityValues[v]
		if !ok {
			return "", fmt.Errorf("未知的 priority %q", v)
		}
		task().Priority = p
	}
	if v, ok := args["estimate"].(float64); ok && v >= 0 {
		task().Estimate = v
	}
	for _, id := range links(args["add_after"]) {
		if f.Node(id) == nil || id == n.ID {
			return "", fmt.Errorf("前置任务 %q 不存在", id)
		}
		if !slices.Contains(task().After, id) {
			task().After = append(task().After, id)
		}
	}
	for _, id := range links(args["remove_after"]) {
		if n.Task != nil {
			n.Task.After = slices.DeleteFunc(n.Task.After, func(x string) bool { return x == id })
		}
	}
	for _, k := range []string{"start", "due"} {
		v, ok := text(k)
		if !ok {
			continue
		}
		var day *time.Time
		if strings.TrimSpace(v) != "" {
			t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(v), time.Local)
			if err != nil {
				return "", fmt.Errorf("%s 的日期格式应为 YYYY-MM-DD", k)
			}
			day = &t
		}
		if k == "start" {
			task().Start = day
		} else {
			task().Due = day
		}
	}
	return "已更新：" + nodeLine(n), nil
}

// links reads a list of links, kept as given.
func links(v any) []string {
	var out []string
	if vs, ok := v.([]any); ok {
		for _, x := range vs {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	}
	return out
}

func moveNode(f *model.File, id, parentID string, index int) (string, error) {
	d, n, _, err := findNode(f, id)
	if err != nil {
		return "", err
	}
	if f.SheetOf(parentID) != d {
		return "", errors.New("只能在同一个画布内移动")
	}
	if err := d.Move(id, parentID, index); err != nil {
		return "", err
	}
	p, _ := d.Find(parentID)
	return fmt.Sprintf("已把「%s」移到「%s」下", n.Title, p.Title), nil
}

func deleteNode(f *model.File, id string) (string, error) {
	d, n, p, err := findNode(f, id)
	if err != nil {
		return "", err
	}
	if p == nil {
		return "", errors.New("不能删除中心主题")
	}
	if _, err := d.Delete(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("已删除「%s」（用户可在历史版本里恢复）", n.Title), nil
}

func (s *Server) createMap(name, rootTitle, outline, folder string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\`) {
		return "", errors.New("name 不能为空，也不能包含斜杠")
	}
	dir := s.Dir
	if folder != "" {
		dir = filepath.Join(s.Dir, filepath.Clean("/" + folder)[1:])
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, model.FileName(name))
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("导图 %q 已经存在", s.rel(path))
	}
	if rootTitle == "" {
		rootTitle = name
	}
	f := model.NewFile(rootTitle)
	if outline != "" {
		f.Sheets[0].Root.Children = model.ParseOutline(outline)
	}
	if err := f.Save(path); err != nil {
		return "", err
	}
	_, _ = s.Versions.Save(path, f, versions.AI, "创建导图", s.Now())
	return "已创建导图 " + s.rel(path) + "\n\n" + readMap(s.rel(path), f, ""), nil
}
