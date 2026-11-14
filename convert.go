package main

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A thought and a task are kept each as what it is, a clip of 收集 and a
// topic of a map, and one is made of the other by a link: the task made
// of a thought links to it (clip:id), and the thought stays, with its
// tags, its picture and what the AI made of it; a task kept as a thought
// has its name and its note written down, and links to that.

// clipToTask makes a task of a clip, in the inbox: its first line names
// it, and it links to the clip, which stays in 收集.
func (a *app) clipToTask(cl *library.Clip) {
	title := cl.Label()
	if isMemo(cl) {
		title = cl.Text
	}
	title, _, _ = strings.Cut(strings.TrimSpace(refToken.ReplaceAllString(title, "")), "\n")
	title = strings.NewReplacer("**", "", "- ", "").Replace(title)
	if bare := strings.TrimSpace(tagInTitle.ReplaceAllString(title, "")); bare != "" {
		title = bare // its tags are the task's own
	}
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		title = cl.Label()
	}
	if r := []rune(title); len(r) > 60 {
		title = string(r[:60]) + "…"
	}
	n := model.NewNode(title)
	n.SetStatus(model.StatusTodo, a.now())
	n.Links = []string{clipScheme + cl.ID}
	if cl.URL != "" {
		n.Links = append(n.Links, cl.URL)
	}
	n.Tags = slices.Clone(cl.Tags)
	err := a.ensureInbox()
	if err == nil {
		err = a.editMapAt(a.inboxPath(), "灵感转成任务", func(f *model.File) error {
			f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, n)
			return nil
		})
	}
	if err != nil {
		a.tell("没有转成：" + err.Error())
		return
	}
	a.tasks.read = time.Time{}
	a.tell("已转成任务，在收件箱：" + title)
}

// taskToMemo keeps a task's name and note as a thought of 收集. With
// keep, the task stays and links to it; else the task is no more.
func (a *app) taskToMemo(it taskItem, keep bool) {
	a.saveTaskDrafts()
	var cl *library.Clip
	if had := a.taskClips(it.node); len(had) > 0 && !keep {
		cl = had[0] // it has its thought already: only the task goes
	} else {
		text := it.node.Title
		if note := strings.TrimSpace(it.node.Note); note != "" {
			text += "\n\n" + note
		}
		var err error
		if cl, err = a.addMemo(text, "任务"); err != nil {
			a.tell("没有存下：" + err.Error())
			return
		}
		if len(it.node.Tags) > 0 {
			for _, tag := range it.node.Tags {
				if !slices.Contains(cl.Tags, tag) {
					cl.Tags = append(cl.Tags, tag)
				}
			}
			a.saveClip(cl)
		}
	}
	if keep {
		ref := clipScheme + cl.ID
		a.editTask(it, "任务存为灵感", func(n *model.Node) {
			if !slices.Contains(n.Links, ref) {
				n.Links = append(n.Links, ref)
			}
		})
		a.tell("已存为灵感，任务上有它的链接")
		return
	}
	err := a.editMapAt(it.path, "任务改成灵感", func(f *model.File) error {
		for _, d := range f.Sheets {
			if dropNode(d.Root, it.node.ID) {
				break
			}
		}
		return nil
	})
	if err != nil {
		a.tell("灵感存下了，任务没有去掉：" + err.Error())
		return
	}
	if a.tasks.sel == it.key() {
		a.tasks.sel, a.tasks.draftOf = "", ""
	}
	a.tasks.read = time.Time{}
	a.tell("已改成灵感，在收集里")
}

// tagInTitle is a tag written in a thought's words.
var tagInTitle = regexp.MustCompile(`#[^\s#]+`)

// dropNode removes the node of an id from below n.
func dropNode(n *model.Node, id string) bool {
	for i, ch := range n.Children {
		if ch.ID == id {
			n.Children = slices.Delete(n.Children, i, i+1)
			return true
		}
		if dropNode(ch, id) {
			return true
		}
	}
	return false
}

// taskClips are the clips a task links to.
func (a *app) taskClips(n *model.Node) []*library.Clip {
	var out []*library.Clip
	for _, l := range n.Links {
		if id, ok := strings.CutPrefix(l, clipScheme); ok && a.lib != nil {
			if cl := a.lib.Find(id); cl != nil {
				out = append(out, cl)
			}
		}
	}
	return out
}

// taskThoughts shows, in a task's details, the thoughts it links to, each
// read there and opened in 收集 by a click; and the key that keeps the
// task as one.
func (a *app) taskThoughts(c *ui.Context, it taskItem) {
	t := c.Theme()
	clips := a.taskClips(it.node)
	section(c, iconPencil, "灵感", func() {
		if len(clips) == 0 && ghostButton(c, "存为灵感", "把任务名和备注存成收集里的一条灵感，任务留着并链接过去").Clicked() {
			a.later(func() { a.taskToMemo(it, true) })
		}
	}, func() {
		if len(clips) == 0 {
			ui.Text(c, "这个任务还没有连着的灵感。").FontSize(textSmall).TextColor(t.TextMuted)
		}
		for _, cl := range clips {
			card := paper(ui.Column(c.Key("thought:"+cl.ID)).Padding(10, 12).Gap(4).Radius(radiusRow).FillWidth().
				Cursor(ui.CursorPointer).Label("打开灵感 "+refName(cl)).Tooltip("在收集里打开"), t)
			if card.Clicked() {
				a.later(func() {
					a.saveTaskDrafts()
					a.openList(kindAll, "")
					a.libSel = cl.ID
				})
			}
			card.Children(func() {
				body := cl.Text
				if !isMemo(cl) || strings.TrimSpace(body) == "" {
					body = cl.Label()
				}
				a.noteText(c, body, textSmall+1, 6)
			})
		}
		label, tip := "改成灵感", "存成灵感，并把这个任务去掉：它不再是要做的事"
		if len(clips) > 0 {
			label, tip = "不做了，只留灵感", "把这个任务去掉，灵感留在收集里"
		}
		if ghostButton(c, label, tip).Margin(0, 0, 0, -8).Clicked() {
			a.later(func() { a.taskToMemo(it, false) })
		}
	})
}
