package model

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ChangeKind is what happened to a node between two versions of a map.
type ChangeKind string

const (
	Added      ChangeKind = "新增"
	Removed    ChangeKind = "删除"
	Renamed    ChangeKind = "改名"
	Moved      ChangeKind = "移动"
	StatusSet  ChangeKind = "状态"
	NoteSet    ChangeKind = "备注"
	TagsSet    ChangeKind = "标签"
	LinksSet   ChangeKind = "链接"
	Settings   ChangeKind = "设置" // of the map: its project folder, code baseline…
	Others     ChangeKind = "其他" // styles, markers, pictures… of a node or a canvas
	TaskSet    ChangeKind = "任务"
	SheetAdded ChangeKind = "新画布"
	SheetGone  ChangeKind = "删除画布"
)

// Change is a difference between two versions of a map.
type Change struct {
	Kind   ChangeKind
	ID     string // the node's
	Sheet  string // the canvas's title
	Title  string // the node's title, as it is in the newer version
	Where  string // the titles above it, "a › b"
	Detail string // what changed, as "旧 → 新"
}

// Diff lists what changed from one version of a map to another, matching
// nodes by their IDs: the map's settings, canvases and nodes added and
// removed, then the nodes renamed, moved, and whose task, note, tags or
// links changed. Nodes under one added or removed are not listed by
// themselves.
func Diff(old, new *File) []Change {
	type entry struct {
		node   *Node
		parent string
		where  string
		sheet  string
	}
	index := func(f *File) map[string]entry {
		m := map[string]entry{}
		for _, d := range f.Sheets {
			var walk func(n *Node, parent string, path []string)
			walk = func(n *Node, parent string, path []string) {
				m[n.ID] = entry{n, parent, strings.Join(path, " › "), d.Title}
				for _, c := range n.Children {
					walk(c, n.ID, append(slices.Clip(path), n.Title))
				}
			}
			walk(d.Root, "", nil)
		}
		return m
	}
	before, after := index(old), index(new)
	out := settingsDiff(old, new)
	sheets := func(f *File) map[string]string {
		m := map[string]string{}
		for _, d := range f.Sheets {
			m[d.ID] = d.Title
		}
		return m
	}
	oldSheets, newSheets := sheets(old), sheets(new)
	for _, d := range new.Sheets {
		if _, ok := oldSheets[d.ID]; !ok {
			out = append(out, Change{Kind: SheetAdded, Sheet: d.Title, Title: d.Title})
		}
	}
	for _, d := range old.Sheets {
		if _, ok := newSheets[d.ID]; !ok {
			out = append(out, Change{Kind: SheetGone, Sheet: d.Title, Title: d.Title})
		}
	}
	// Added and removed: only the top of each branch.
	for _, d := range new.Sheets {
		if _, ok := oldSheets[d.ID]; !ok {
			continue
		}
		d.Walk(func(n, p *Node, _ int) bool {
			if _, ok := before[n.ID]; ok {
				return true
			}
			e := after[n.ID]
			detail := ""
			if k := n.count() - 1; k > 0 {
				detail = fmt.Sprintf("连同 %d 个下级", k)
			}
			out = append(out, Change{Kind: Added, ID: n.ID, Sheet: e.sheet, Title: n.Title, Where: e.where, Detail: detail})
			return false
		})
	}
	for _, d := range old.Sheets {
		if _, ok := newSheets[d.ID]; !ok {
			continue
		}
		d.Walk(func(n, p *Node, _ int) bool {
			if _, ok := after[n.ID]; ok {
				return true
			}
			e := before[n.ID]
			detail := ""
			if k := n.count() - 1; k > 0 {
				detail = fmt.Sprintf("连同 %d 个下级", k)
			}
			out = append(out, Change{Kind: Removed, ID: n.ID, Sheet: e.sheet, Title: n.Title, Where: e.where, Detail: detail})
			return false
		})
	}
	// Changed: in the newer version's order.
	for _, d := range new.Sheets {
		d.Walk(func(n, _ *Node, _ int) bool {
			o, ok := before[n.ID]
			if !ok {
				return true
			}
			e := after[n.ID]
			add := func(kind ChangeKind, detail string) {
				out = append(out, Change{Kind: kind, ID: n.ID, Sheet: e.sheet, Title: n.Title, Where: e.where, Detail: detail})
			}
			if o.node.Title != n.Title {
				add(Renamed, fmt.Sprintf("「%s」→「%s」", o.node.Title, n.Title))
			}
			if o.parent != e.parent && e.parent != "" {
				add(Moved, fmt.Sprintf("从「%s」移到「%s」", lastStep(o.where), lastStep(e.where)))
			}
			switch {
			case o.node.Task == nil && n.Task != nil:
				add(TaskSet, "设为任务（"+n.Task.Status.Label()+"）")
			case o.node.Task != nil && n.Task == nil:
				add(TaskSet, "不再是任务")
			case o.node.Task != nil && n.Task != nil:
				if o.node.Task.Status != n.Task.Status {
					add(StatusSet, o.node.Task.Status.Label()+" → "+n.Task.Status.Label())
				}
				if d := taskDates(o.node.Task, n.Task); d != "" {
					add(TaskSet, d)
				}
			}
			if o.node.Note != n.Note {
				switch {
				case o.node.Note == "":
					add(NoteSet, "添加了备注")
				case n.Note == "":
					add(NoteSet, "删除了备注")
				default:
					add(NoteSet, "修改了备注")
				}
			}
			if !slices.Equal(o.node.Tags, n.Tags) {
				add(TagsSet, tagDiff(o.node.Tags, n.Tags))
			}
			if !slices.Equal(o.node.Links, n.Links) {
				add(LinksSet, tagDiff(o.node.Links, n.Links))
			}
			if d := otherFields(o.node, n); d != "" {
				add(Others, d)
			}
			return true
		})
	}
	// The canvases' own: their titles, structures, styles, lines.
	oldDocs := map[string]*Document{}
	for _, d := range old.Sheets {
		oldDocs[d.ID] = d
	}
	for _, d := range new.Sheets {
		o := oldDocs[d.ID]
		if o == nil {
			continue
		}
		var parts []string
		if o.Title != d.Title {
			parts = append(parts, fmt.Sprintf("改名「%s」→「%s」", o.Title, d.Title))
		}
		if o.Structure != d.Structure {
			parts = append(parts, "结构 "+o.Structure.Label()+" → "+d.Structure.Label())
		}
		if !sameJSON(o.Style, d.Style) {
			parts = append(parts, "画布样式")
		}
		if !sameJSON(o.Present, d.Present) {
			parts = append(parts, "演说设置")
		}
		if !sameJSON(o.Relations, d.Relations) {
			parts = append(parts, "关联线")
		}
		if len(parts) > 0 {
			out = append(out, Change{Kind: Others, Sheet: d.Title, Title: "画布「" + d.Title + "」", Detail: strings.Join(parts, "、")})
		}
	}
	// Anything else that differs, folding aside, is told of, as a version
	// is never kept with nothing to show.
	if len(out) == 0 && !SameContent(old, new) {
		out = append(out, Change{Kind: Others, Title: "其他调整", Detail: "导图的其他设置"})
	}
	return out
}

// otherFields names the node's own fields that changed beyond those Diff
// tells of by themselves; folding is how the map is seen, not a change.
func otherFields(o, n *Node) string {
	var parts []string
	for _, f := range []struct {
		name string
		a, b any
	}{
		{"标记", o.Markers, n.Markers}, {"样式", o.Style, n.Style}, {"图片", o.Image, n.Image},
		{"附件", o.Files, n.Files}, {"概要", o.Summary, n.Summary}, {"外框", o.Boundary, n.Boundary},
		{"度量", o.Values, n.Values}, {"位置", o.Pos, n.Pos},
	} {
		if !sameJSON(f.a, f.b) {
			parts = append(parts, f.name)
		}
	}
	if o.Task != nil && n.Task != nil && (o.Task.Estimate != n.Task.Estimate || !slices.Equal(o.Task.After, n.Task.After) || o.Task.Iteration != n.Task.Iteration) {
		parts = append(parts, "任务的估时、依赖或迭代")
	}
	return strings.Join(parts, "、")
}

func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// SameContent tells whether two versions of a map hold the same, folding
// aside: folding and unfolding change how it is seen, not what it says.
func SameContent(a, b *File) bool {
	unfold := func(f *File) []byte {
		c := f.Clone()
		for _, d := range c.Sheets {
			d.Walk(func(n, _ *Node, _ int) bool { n.Collapsed = false; return true })
			for _, fl := range d.Floating {
				var walk func(n *Node)
				walk = func(n *Node) {
					n.Collapsed = false
					for _, k := range n.Children {
						walk(k)
					}
				}
				walk(fl)
			}
		}
		c.Version = 0
		j, _ := json.Marshal(c)
		return j
	}
	return string(unfold(a)) == string(unfold(b))
}

// settingsDiff lists the changes of the map's own settings.
func settingsDiff(old, new *File) []Change {
	var out []Change
	set := func(what, o, n string) {
		if o == n {
			return
		}
		if o == "" {
			o = "无"
		}
		if n == "" {
			n = "无"
		}
		out = append(out, Change{Kind: Settings, Title: what, Detail: o + " → " + n})
	}
	set("项目代码目录", old.Project, new.Project)
	set("GitHub 仓库", old.GitHub, new.GitHub)
	commit := func(b *Baseline) string {
		if b == nil {
			return ""
		}
		c := b.Commit
		if len(c) > 7 {
			c = c[:7]
		}
		if b.Branch != "" {
			c = b.Branch + " " + c
		}
		return c
	}
	set("代码基线", commit(old.Baseline), commit(new.Baseline))
	set("度量", strings.Join(old.Measures, "、"), strings.Join(new.Measures, "、"))
	return out
}

func (n *Node) count() int {
	k := 1
	for _, c := range n.Children {
		k += c.count()
	}
	return k
}

func lastStep(where string) string {
	if i := strings.LastIndex(where, " › "); i >= 0 {
		return where[i+len(" › "):]
	}
	return where
}

func taskDates(o, n *Task) string {
	var parts []string
	if o.Priority != n.Priority {
		parts = append(parts, "优先级 "+o.Priority.Label()+" → "+n.Priority.Label())
	}
	day := func(t interface{ Format(string) string }, ok bool) string {
		if !ok {
			return "无"
		}
		return t.Format("1/2")
	}
	if (o.Due == nil) != (n.Due == nil) || o.Due != nil && !o.Due.Equal(*n.Due) {
		parts = append(parts, "截止 "+day(o.Due, o.Due != nil)+" → "+day(n.Due, n.Due != nil))
	}
	if (o.Start == nil) != (n.Start == nil) || o.Start != nil && !o.Start.Equal(*n.Start) {
		parts = append(parts, "开始 "+day(o.Start, o.Start != nil)+" → "+day(n.Start, n.Start != nil))
	}
	return strings.Join(parts, "，")
}

func tagDiff(old, new []string) string {
	var parts []string
	for _, t := range new {
		if !slices.Contains(old, t) {
			parts = append(parts, "+"+t)
		}
	}
	for _, t := range old {
		if !slices.Contains(new, t) {
			parts = append(parts, "−"+t)
		}
	}
	return strings.Join(parts, " ")
}

// Summary counts changes by kind, as "新增 3 · 删除 1 · 状态 2".
func Summary(changes []Change) string {
	order := []ChangeKind{Settings, SheetAdded, SheetGone, Added, Removed, Renamed, Moved, StatusSet, TaskSet, NoteSet, TagsSet, LinksSet, Others}
	count := map[ChangeKind]int{}
	for _, c := range changes {
		count[c.Kind]++
	}
	var parts []string
	for _, k := range order {
		if count[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, count[k]))
		}
	}
	if len(parts) == 0 {
		return "没有变化"
	}
	return strings.Join(parts, " · ")
}
