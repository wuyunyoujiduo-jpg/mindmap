// Package xmind converts mind maps to and from XMind's .xmind files.
//
// An .xmind file is a zip archive. XMind 2020 and later keep the sheets
// in content.json, with manifest.json and metadata.json beside it and
// pictures under resources/; XMind 8 and earlier kept them in content.xml,
// pictures under attachments/. Export writes the former; Import reads
// either.
//
// What XMind has an equivalent for maps onto it: titles, notes, labels
// (tags), markers, pictures, folding and the structure. What it has not
// (a task's status and dates, node styles, the canvas's style) travels in
// an extension of the topic, so that a map going out and coming back
// keeps it; XMind shows the task's progress as a marker.
package xmind

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // pictures' sizes
	_ "image/jpeg" // pictures' sizes
	_ "image/png"  // pictures' sizes
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	_ "golang.org/x/image/bmp"  // pictures' sizes
	_ "golang.org/x/image/webp" // pictures' sizes

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// provider names this app's extension of topics and sheets.
const provider = "com.wuyunyoujiduo.mindmap"

// sheet and topic are XMind's content.json.
type sheet struct {
	ID            string         `json:"id"`
	Class         string         `json:"class"`
	Title         string         `json:"title"`
	RootTopic     *topic         `json:"rootTopic"`
	Relationships []relationship `json:"relationships,omitempty"`
	Extensions    []extension    `json:"extensions,omitempty"`
}

type relationship struct {
	ID     string `json:"id"`
	End1ID string `json:"end1Id"`
	End2ID string `json:"end2Id"`
	Title  string `json:"title,omitempty"`
}

type position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type topic struct {
	ID             string      `json:"id"`
	Class          string      `json:"class"`
	Title          string      `json:"title"`
	StructureClass string      `json:"structureClass,omitempty"`
	Branch         string      `json:"branch,omitempty"` // "folded"
	Notes          *notes      `json:"notes,omitempty"`
	Labels         []string    `json:"labels,omitempty"`
	Markers        []marker    `json:"markers,omitempty"`
	Image          *topicImage `json:"image,omitempty"`
	Href           string      `json:"href,omitempty"`
	Position       *position   `json:"position,omitempty"`
	Children       *children   `json:"children,omitempty"`
	Extensions     []extension `json:"extensions,omitempty"`
}

type children struct {
	Attached []*topic `json:"attached,omitempty"`
	Detached []*topic `json:"detached,omitempty"`
}

type notes struct {
	Plain *struct {
		Content string `json:"content"`
	} `json:"plain,omitempty"`
}

type marker struct {
	MarkerID string `json:"markerId"`
}

type topicImage struct {
	Src    string `json:"src"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type extension struct {
	Provider string          `json:"provider"`
	Content  json.RawMessage `json:"content"`
}

// topicExtra is what a topic carries that XMind has no place for.
type topicExtra struct {
	Task     *model.Task      `json:"task,omitempty"`
	Markers  []string         `json:"markers,omitempty"`
	Style    *model.NodeStyle `json:"style,omitempty"`
	Links    []string         `json:"links,omitempty"`
	Summary  *model.Mark      `json:"summary,omitempty"`
	Boundary *model.Mark      `json:"boundary,omitempty"`
}

// sheetExtra is what a sheet carries that XMind has no place for.
type sheetExtra struct {
	Structure model.Structure   `json:"structure,omitempty"`
	Style     model.CanvasStyle `json:"style,omitzero"`
	Present   model.Present     `json:"present,omitzero"`
}

var structureClasses = map[model.Structure]string{
	model.StructureMindMap: "org.xmind.ui.map.unbalanced",
	model.StructureLogic:   "org.xmind.ui.logic.right",
	model.StructureBracket: "org.xmind.ui.brace.right",
}

func structureOf(class string) model.Structure {
	switch {
	case strings.Contains(class, "logic"), strings.Contains(class, "tree"), strings.Contains(class, "org-chart"):
		return model.StructureLogic
	case strings.Contains(class, "brace"):
		return model.StructureBracket
	}
	return model.StructureMindMap
}

// The markers XMind has, by this app's: colors and the progress of a task.
var xmindColors = map[string]string{
	"red": "red", "orange": "orange", "yellow": "yellow", "green": "green",
	"blue": "blue", "purple": "purple", "gray": "gray",
}

// taskSteps are XMind's task progress markers, from start to done.
var taskSteps = []string{"task-start", "task-oct", "task-quarter", "task-3oct", "task-half",
	"task-5oct", "task-3quar", "task-7oct", "task-done"}

// toXMind returns the XMind marker of one of this app's, or "".
func toXMind(m string) string {
	group, value, _ := strings.Cut(m, ":")
	switch group {
	case "priority":
		return "priority-" + value
	case "task":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 6 {
			return ""
		}
		return taskSteps[int(math.Round(float64(n)/6*8))]
	case "flag", "star":
		if c, ok := xmindColors[value]; ok {
			return group + "-" + c
		}
	case "person":
		if c, ok := xmindColors[value]; ok {
			return "people-" + c
		}
	}
	return ""
}

// fromXMind returns this app's marker of one of XMind's, or "".
func fromXMind(id string) string {
	if i := indexOf(taskSteps, id); i >= 0 {
		return "task:" + strconv.Itoa(int(math.Round(float64(i)/8*6)))
	}
	group, value, ok := strings.Cut(id, "-")
	if !ok {
		return ""
	}
	switch group {
	case "priority":
		if n, err := strconv.Atoi(value); err == nil && n >= 1 && n <= 7 {
			return "priority:" + value
		}
	case "flag", "star", "people":
		if value == "black" || value == "dark-gray" {
			value = "gray"
		}
		if _, ok := xmindColors[value]; ok {
			if group == "people" {
				group = "person"
			}
			return group + ":" + value
		}
	}
	return ""
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// taskMarker is the progress marker XMind shows for a task's status.
func taskMarker(t *model.Task) string {
	switch t.Status {
	case model.StatusDone:
		return "task-done"
	case model.StatusDoing:
		return "task-half"
	case model.StatusTodo:
		return "task-start"
	}
	return ""
}

// Export writes the file as an .xmind archive, with the pictures of its
// nodes from assetsDir.
func Export(w io.Writer, f *model.File, assetsDir string) error {
	z := zip.NewWriter(w)
	entries := map[string]any{"content.json": struct{}{}, "metadata.json": struct{}{}}
	pictures := map[string]bool{}
	var sheets []sheet
	for _, d := range f.Sheets {
		extra, _ := json.Marshal(sheetExtra{Structure: d.Structure, Style: d.Style, Present: d.Present})
		root := exportTopic(d.Root, pictures)
		root.StructureClass = structureClasses[d.Structure]
		title := d.Title
		if title == "" {
			title = d.Root.Title
		}
		for _, f := range d.Floating {
			ft := exportTopic(f, pictures)
			if f.Pos != nil {
				ft.Position = &position{X: float64(f.Pos.X), Y: float64(f.Pos.Y)}
			}
			if root.Children == nil {
				root.Children = &children{}
			}
			root.Children.Detached = append(root.Children.Detached, ft)
		}
		var rels []relationship
		for _, r := range d.Relations {
			rels = append(rels, relationship{ID: r.ID, End1ID: r.From, End2ID: r.To, Title: r.Label})
		}
		sheets = append(sheets, sheet{ID: d.ID, Class: "sheet", Title: title, RootTopic: root, Relationships: rels,
			Extensions: []extension{{Provider: provider, Content: extra}}})
	}
	for name := range pictures {
		data, err := os.ReadFile(filepath.Join(assetsDir, name))
		if err != nil {
			continue // a picture missing is left out
		}
		fw, err := z.Create("resources/" + name)
		if err != nil {
			return err
		}
		if _, err := fw.Write(data); err != nil {
			return err
		}
		entries["resources/"+name] = struct{}{}
	}
	write := func(name string, v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		fw, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = fw.Write(b)
		return err
	}
	if err := write("content.json", sheets); err != nil {
		return err
	}
	if err := write("metadata.json", map[string]any{"creator": map[string]string{"name": "MyMind"}}); err != nil {
		return err
	}
	if err := write("manifest.json", map[string]any{"file-entries": entries}); err != nil {
		return err
	}
	return z.Close()
}

func exportTopic(n *model.Node, pictures map[string]bool) *topic {
	t := &topic{ID: n.ID, Class: "topic", Title: n.Title, Labels: n.Tags}
	if n.Note != "" {
		t.Notes = &notes{Plain: &struct {
			Content string `json:"content"`
		}{n.Note}}
	}
	hasTask := false
	for _, m := range n.Markers {
		if id := toXMind(m); id != "" {
			t.Markers = append(t.Markers, marker{id})
			hasTask = hasTask || strings.HasPrefix(id, "task-")
		}
	}
	if n.Task != nil && !hasTask {
		if id := taskMarker(n.Task); id != "" {
			t.Markers = append(t.Markers, marker{id})
		}
	}
	if n.Image != nil {
		pictures[n.Image.File] = true
		t.Image = &topicImage{Src: "xap:resources/" + n.Image.File, Width: n.Image.W, Height: n.Image.H}
	}
	if n.Collapsed && len(n.Children) > 0 {
		t.Branch = "folded"
	}
	for _, l := range n.Links { // XMind takes one link: the first page's
		if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
			t.Href = l
			break
		}
	}
	for _, c := range n.Children {
		if t.Children == nil {
			t.Children = &children{}
		}
		t.Children.Attached = append(t.Children.Attached, exportTopic(c, pictures))
	}
	if n.Task != nil || len(n.Markers) > 0 || n.Style != nil || len(n.Links) > 0 || n.Summary != nil || n.Boundary != nil {
		extra, _ := json.Marshal(topicExtra{Task: n.Task, Markers: n.Markers, Style: n.Style, Links: n.Links,
			Summary: n.Summary, Boundary: n.Boundary})
		t.Extensions = []extension{{Provider: provider, Content: extra}}
	}
	return t
}

// ErrNotXMind tells that a file is no .xmind archive.
var ErrNotXMind = errors.New("不是 XMind 文件")

// Import reads an .xmind archive, writing the pictures of its topics into
// assetsDir.
func Import(r io.ReaderAt, size int64, assetsDir string) (*model.File, error) {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, ErrNotXMind
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		files[f.Name] = f
	}
	read := func(name string) ([]byte, error) {
		f := files[name]
		if f == nil {
			return nil, os.ErrNotExist
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, 256<<20))
	}
	im := importer{read: read, assetsDir: assetsDir, saved: map[string]*model.Image{}, ids: map[string]string{}}
	var out *model.File
	if b, err := read("content.json"); err == nil {
		out, err = im.fromJSON(b)
		if err != nil {
			return nil, err
		}
	} else if b, err := read("content.xml"); err == nil {
		out, err = im.fromXML(b)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, ErrNotXMind
	}
	if len(out.Sheets) == 0 {
		return nil, fmt.Errorf("XMind 文件里没有画布")
	}
	out.Version = model.FormatVersion
	return out, nil
}

type importer struct {
	read      func(name string) ([]byte, error)
	assetsDir string
	saved     map[string]*model.Image // by the picture's path in the archive
	ids       map[string]string       // the new IDs of topics, by XMind's
}

func (im *importer) fromJSON(b []byte) (*model.File, error) {
	var sheets []sheet
	if err := json.Unmarshal(b, &sheets); err != nil {
		return nil, fmt.Errorf("无法读取 XMind 内容：%w", err)
	}
	f := &model.File{}
	for _, s := range sheets {
		if s.RootTopic == nil {
			continue
		}
		d := &model.Document{ID: model.NewID(), Title: s.Title, Root: im.topic(s.RootTopic)}
		if s.RootTopic.Children != nil { // floating topics where they were
			for _, t := range s.RootTopic.Children.Detached {
				f := im.topic(t)
				f.Pos = &model.Point{X: 220, Y: -120}
				if t.Position != nil {
					f.Pos = &model.Point{X: float32(t.Position.X), Y: float32(t.Position.Y)}
				}
				d.Floating = append(d.Floating, f)
			}
		}
		for _, r := range s.Relationships {
			from, to := im.ids[r.End1ID], im.ids[r.End2ID]
			if from != "" && to != "" {
				d.Relations = append(d.Relations, model.Relation{ID: model.NewID(), From: from, To: to, Label: r.Title})
			}
		}
		d.Structure = structureOf(s.RootTopic.StructureClass)
		for _, e := range s.Extensions {
			var extra sheetExtra
			if e.Provider == provider && json.Unmarshal(e.Content, &extra) == nil {
				d.Structure, d.Style, d.Present = extra.Structure, extra.Style, extra.Present
			}
		}
		f.Sheets = append(f.Sheets, d)
	}
	return f, nil
}

func (im *importer) topic(t *topic) *model.Node {
	n := &model.Node{ID: model.NewID(), Title: t.Title, Tags: t.Labels, Collapsed: t.Branch == "folded"}
	im.ids[t.ID] = n.ID
	if t.Notes != nil && t.Notes.Plain != nil {
		n.Note = strings.TrimSpace(t.Notes.Plain.Content)
	}
	for _, m := range t.Markers {
		if id := fromXMind(m.MarkerID); id != "" {
			n.Markers = addMarker(n.Markers, id)
		}
	}
	if t.Image != nil {
		n.Image = im.picture(t.Image.Src, t.Image.Width, t.Image.Height)
	}
	if t.Href != "" && !strings.HasPrefix(t.Href, "xap:") && !strings.HasPrefix(t.Href, "xmind:") {
		n.Links = []string{t.Href}
	}
	if t.Children != nil {
		for _, c := range t.Children.Attached {
			n.Children = append(n.Children, im.topic(c))
		}
	}
	for _, e := range t.Extensions {
		var extra topicExtra
		if e.Provider == provider && json.Unmarshal(e.Content, &extra) == nil {
			n.Task, n.Style, n.Summary, n.Boundary = extra.Task, extra.Style, extra.Summary, extra.Boundary
			if extra.Links != nil {
				n.Links = extra.Links
			}
			if extra.Markers != nil {
				n.Markers = extra.Markers
			}
		}
	}
	if len(n.Children) == 0 {
		n.Collapsed = false
	}
	return n
}

// addMarker adds a marker, in place of one of its group.
func addMarker(markers []string, m string) []string {
	group, _, _ := strings.Cut(m, ":")
	for i, x := range markers {
		if g, _, _ := strings.Cut(x, ":"); g == group {
			markers[i] = m
			return markers
		}
	}
	return append(markers, m)
}

// picture copies a topic's picture into the assets folder.
func (im *importer) picture(src string, w, h int) *model.Image {
	name := strings.TrimPrefix(src, "xap:")
	if img, ok := im.saved[name]; ok {
		return img
	}
	data, err := im.read(name)
	if err != nil || im.assetsDir == "" {
		return nil
	}
	ext := strings.ToLower(path.Ext(name))
	if ext == "" {
		ext = ".png"
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		w, h = cfg.Width, cfg.Height
	}
	if w <= 0 || h <= 0 {
		return nil // not a picture this app shows
	}
	if err := os.MkdirAll(im.assetsDir, 0o755); err != nil {
		return nil
	}
	file := model.NewID() + ext
	if err := os.WriteFile(filepath.Join(im.assetsDir, file), data, 0o644); err != nil {
		return nil
	}
	img := &model.Image{File: file, W: w, H: h}
	im.saved[name] = img
	return img
}

// XMind 8's content.xml.
type xmlContent struct {
	Sheets []struct {
		Title string    `xml:"title"`
		Topic *xmlTopic `xml:"topic"`
	} `xml:"sheet"`
}

type xmlTopic struct {
	StructureClass string `xml:"structure-class,attr"`
	Branch         string `xml:"branch,attr"`
	Title          string `xml:"title"`
	Notes          struct {
		Plain string `xml:"plain"`
	} `xml:"notes"`
	Labels  []string `xml:"labels>label"`
	Markers []struct {
		ID string `xml:"marker-id,attr"`
	} `xml:"marker-refs>marker-ref"`
	Image *struct {
		Src    string `xml:"src,attr"`
		Width  string `xml:"width,attr"`
		Height string `xml:"height,attr"`
	} `xml:"img"`
	Children []struct {
		Type   string      `xml:"type,attr"`
		Topics []*xmlTopic `xml:"topic"`
	} `xml:"children>topics"`
}

func (im *importer) fromXML(b []byte) (*model.File, error) {
	var c xmlContent
	if err := xml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("无法读取 XMind 8 内容：%w", err)
	}
	f := &model.File{}
	for _, s := range c.Sheets {
		if s.Topic == nil {
			continue
		}
		d := &model.Document{ID: model.NewID(), Title: s.Title, Root: im.xmlTopic(s.Topic),
			Structure: structureOf(s.Topic.StructureClass)}
		f.Sheets = append(f.Sheets, d)
	}
	return f, nil
}

func (im *importer) xmlTopic(t *xmlTopic) *model.Node {
	n := &model.Node{ID: model.NewID(), Title: t.Title, Tags: t.Labels, Collapsed: t.Branch == "folded",
		Note: strings.TrimSpace(t.Notes.Plain)}
	for _, m := range t.Markers {
		if id := fromXMind(m.ID); id != "" {
			n.Markers = addMarker(n.Markers, id)
		}
	}
	if t.Image != nil {
		w, _ := strconv.Atoi(t.Image.Width)
		h, _ := strconv.Atoi(t.Image.Height)
		n.Image = im.picture(t.Image.Src, w, h)
	}
	for _, group := range t.Children {
		for _, c := range group.Topics {
			n.Children = append(n.Children, im.xmlTopic(c))
		}
	}
	if len(n.Children) == 0 {
		n.Collapsed = false
	}
	return n
}
