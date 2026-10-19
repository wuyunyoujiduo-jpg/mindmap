package mcp

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// The maps as data, for programs (the automations' mymind.maps) rather
// than agents, which read them as text.

// Call runs a tool as an agent's call would: its answer is text.
func (s *Server) Call(name string, args json.RawMessage) (string, error) { return s.call(name, args) }

// MapInfo is a map of the folder.
type MapInfo struct {
	Name  string `json:"name"` // relative to the folder, without the extension
	Path  string `json:"path"`
	Tasks int    `json:"tasks"`
	Open  int    `json:"open"` // tasks not done
}

// Maps are the maps of the folder.
func (s *Server) Maps() ([]MapInfo, error) {
	paths, err := model.AllFilesIn(s.Dir)
	if err != nil {
		return nil, err
	}
	out := []MapInfo{}
	for _, p := range paths {
		info := MapInfo{Name: s.rel(p), Path: p}
		if f, err := model.Load(p); err == nil {
			for _, t := range f.Tasks() {
				info.Tasks++
				if t.Node.Task.Status != model.StatusDone {
					info.Open++
				}
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// NodeData is a node and those under it.
type NodeData struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Note     string     `json:"note,omitempty"`
	Tags     []string   `json:"tags,omitempty"`
	Links    []string   `json:"links,omitempty"`
	Status   string     `json:"status,omitempty"` // of a task: idea, todo, doing, done
	Priority string     `json:"priority,omitempty"`
	Start    string     `json:"start,omitempty"` // YYYY-MM-DD
	Due      string     `json:"due,omitempty"`
	DoneAt   string     `json:"done_at,omitempty"` // RFC 3339
	Children []NodeData `json:"children,omitempty"`
}

func nodeData(n *model.Node, deep bool) NodeData {
	d := NodeData{ID: n.ID, Title: n.Title, Note: n.Note, Tags: n.Tags, Links: n.Links}
	if t := n.Task; t != nil {
		d.Status, d.Priority = string(t.Status), priorityNames[t.Priority]
		if t.Start != nil {
			d.Start = t.Start.Format("2006-01-02")
		}
		if t.Due != nil {
			d.Due = t.Due.Format("2006-01-02")
		}
		if t.DoneAt != nil {
			d.DoneAt = t.DoneAt.Format(time.RFC3339)
		}
	}
	if deep {
		for _, c := range n.Children {
			d.Children = append(d.Children, nodeData(c, true))
		}
	}
	return d
}

// SheetData is a canvas of a map.
type SheetData struct {
	Title    string     `json:"title"`
	Root     NodeData   `json:"root"`
	Floating []NodeData `json:"floating,omitempty"`
}

// MapData is a map, whole.
type MapData struct {
	Name    string      `json:"name"`
	Path    string      `json:"path"`
	Project string      `json:"project,omitempty"`
	Sheets  []SheetData `json:"sheets"`
}

// Map reads a map by its name or path.
func (s *Server) Map(name string) (*MapData, error) {
	path, f, err := s.find(name)
	if err != nil {
		return nil, err
	}
	m := &MapData{Name: s.rel(path), Path: path, Project: f.Project}
	for _, d := range f.Sheets {
		sd := SheetData{Title: d.Title, Root: nodeData(d.Root, true)}
		for _, n := range d.Floating {
			sd.Floating = append(sd.Floating, nodeData(n, true))
		}
		m.Sheets = append(m.Sheets, sd)
	}
	return m, nil
}

// TaskData is a task of a map, with where it is.
type TaskData struct {
	NodeData
	Map   string `json:"map"`
	Sheet string `json:"sheet"`
	Where string `json:"where"` // the titles above it, "A › B"
}

// Tasks are the tasks of a map, or of all: of a status ("open" for those
// not done), done since a time when it is not zero.
func (s *Server) Tasks(only, status string, doneSince time.Time) ([]TaskData, error) {
	paths, err := s.paths(only)
	if err != nil {
		return nil, err
	}
	out := []TaskData{}
	for _, p := range paths {
		f, err := model.Load(p)
		if err != nil {
			continue
		}
		for _, t := range f.Tasks() {
			task := t.Node.Task
			if status == "open" && task.Status == model.StatusDone || status != "" && status != "open" && string(task.Status) != status {
				continue
			}
			if !doneSince.IsZero() && (task.DoneAt == nil || task.DoneAt.Before(doneSince)) {
				continue
			}
			out = append(out, TaskData{NodeData: nodeData(t.Node, false), Map: s.rel(p), Sheet: t.Sheet.Title,
				Where: strings.Join(t.Parents, " › ")})
		}
	}
	return out, nil
}
