package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// Past versions of the open map: snapshots taken as it opens, every few
// minutes while it is edited, and around changes another program (an AI
// agent) made; a window lists them, shows what changed since each, and
// restores one.

// snapshotEvery is how often editing keeps a version.
const snapshotEvery = 10 * time.Minute

func versionsDir() string {
	p := configPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "versions")
}

// keepVersion keeps the open map as a version.
func (a *app) keepVersion(reason string) {
	a.keepVersionOf(a.file, reason)
}

func (a *app) keepVersionOf(f *model.File, reason string) {
	if f == nil || a.path == "" {
		return
	}
	if ok, err := a.versions.Save(a.path, f, reason, "", a.now()); err == nil && ok {
		if a.lastSnap == nil {
			a.lastSnap = map[string]time.Time{}
		}
		a.lastSnap[a.path] = a.now()
	}
}

// keepVersionIfDue keeps a version when the last is old enough.
func (a *app) keepVersionIfDue() {
	if a.now().Sub(a.lastSnap[a.path]) >= snapshotEvery {
		a.keepVersion(versions.Auto)
	}
}

// conflictBar asks which version to keep when another program changed the
// file while the map had changes not yet saved.
func (a *app) conflictBar(c *ui.Context) {
	if !a.conflict {
		return
	}
	t := c.Theme()
	ui.Row(c.Key("conflict")).Padding(8, 12).Gap(10).AlignItems(ui.Center).Background(t.Warning.Alpha(0.15)).Children(func() {
		ui.Text(c, "这个导图同时被其他程序（比如 AI）修改了，你也有未保存的修改。选择保留哪一份，另一份会存进历史版本。").
			FontSize(12).Wrap().Grow(1)
		if ui.Button(c, "载入外部版本").Clicked() {
			a.resolveConflict(false)
		}
		if ui.Button(c, "保留我的版本").Clicked() {
			a.resolveConflict(true)
		}
	})
}

// resolveConflict keeps the app's version (mine) or the file's, keeping the
// other as a version.
func (a *app) resolveConflict(mine bool) {
	a.conflict = false
	if mine {
		if f, err := model.Load(a.path); err == nil {
			a.keepVersionOf(f, versions.Conflict)
		}
		a.modTime = modTime(a.path) // saved over, knowingly
		a.save()
		return
	}
	a.keepVersion(versions.Conflict)
	a.dirty = false
	a.reload()
	a.keepVersion(versions.External)
}

// openHistory shows the versions of the open map.
func (a *app) openHistory() {
	if a.file == nil {
		return
	}
	a.commitEditIfAny()
	a.flush()
	a.historyList = a.versions.WithNotes(a.versions.List(a.path))
	a.historySel = 0
	a.historyFile, a.historyDiff = nil, nil
	a.historyOpen = true
}

// historyPick shows what changed from a version to now.
func (a *app) historyPick(i int) {
	a.historySel = i
	a.historyFile, a.historyDiff = nil, nil
	if i < 0 || i >= len(a.historyList) {
		return
	}
	f, err := a.versions.Load(a.historyList[i])
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	a.historyFile = f
	a.historyDiff = model.Diff(f, a.file)
}

// restoreVersion brings back a version, as a step that undo takes back,
// keeping the map as it was as a version too.
func (a *app) restoreVersion(f *model.File) {
	a.keepVersion(versions.BeforeRestore)
	a.hist.Record(a.state())
	sheet := a.doc.ID
	a.restore(model.State{File: f.Clone(), Sheet: sheet, Selected: a.selected})
	a.dirty, a.changed = true, a.now()
	a.historyOpen = false
	a.tell("已恢复到所选版本，可以撤销")
}

// whenLabel says when, as "今天 14:32" or "10月3日 09:15".
func whenLabel(t, now time.Time) string {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "今天 " + t.Format("15:04")
	case y1 == y2 && m1 == m2 && d1 == d2-1:
		return "昨天 " + t.Format("15:04")
	case y1 == y2:
		return fmt.Sprintf("%d月%d日 %s", m1, d1, t.Format("15:04"))
	}
	return t.Format("2006-01-02 15:04")
}

// historyView is the window of versions: the list on the left, what changed
// since the one chosen on the right.
func (a *app) historyView(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c.Key("history"), &a.historyOpen, func() {
		w, h := c.Size()
		if a.historyFile == nil && len(a.historyList) > 0 {
			a.historyPick(a.historySel)
		}
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "历史版本 · "+mapName(a.path)).FontSize(17).Bold().Grow(1)
			if ui.Button(c, "关闭").Clicked() {
				a.historyOpen = false
			}
		})
		if len(a.historyList) == 0 {
			ui.Text(c, "还没有历史版本。编辑、或者其他程序修改这个导图后，这里会自动保留版本。").TextColor(t.TextMuted).Wrap()
			return
		}
		ui.Row(c).Gap(16).Width(min(900, w-80)).Height(min(560, h-160)).AlignItems(ui.Stretch).Children(func() {
			ui.Scroll(c.Key("versions")).Width(240).Shrink(0).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for i, v := range a.historyList {
						row := ui.Column(c.Key(fmt.Sprint("v", i))).Padding(6, 10).Radius(6).Cursor(ui.CursorPointer).
							Label(whenLabel(v.Time, a.now()))
						if i == a.historySel {
							row.Background(t.Accent.Alpha(0.14))
						} else if row.Hovered() {
							row.Background(t.SurfaceHover)
						}
						if row.Clicked() {
							a.historyPick(i)
						}
						row.Children(func() {
							ui.Text(c, whenLabel(v.Time, a.now())).FontSize(13)
							label := versions.Label(v.Reason)
							col := t.TextMuted
							if v.Reason == versions.AI || v.Reason == versions.External {
								col = t.Accent
							}
							ui.Text(c, label).FontSize(11).TextColor(col)
							if v.Note != "" {
								ui.Text(c, v.Note).FontSize(11).TextColor(t.TextMuted).MaxLines(2)
							}
						})
					}
				})
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(10).Children(func() {
				if a.historyFile == nil {
					return
				}
				v := a.historyList[a.historySel]
				ui.Text(c, "从 "+whenLabel(v.Time, a.now())+" 到现在："+model.Summary(a.historyDiff)).FontSize(13).Bold().Wrap()
				ui.Scroll(c.Key("changes")).Grow(1).Children(func() {
					ui.Column(c).Gap(4).Children(func() {
						for i, ch := range a.historyDiff {
							a.changeRow(c.Key(fmt.Sprint("ch", i)), ch)
						}
					})
				})
				ui.Row(c).Gap(8).Children(func() {
					f := a.historyFile
					if ui.Button(c, "恢复到这个版本").Clicked() {
						a.later(func() { a.restoreVersion(f) })
					}
					ui.Text(c, "恢复后可以撤销；恢复前的内容也会存为一个版本。").FontSize(11).TextColor(t.TextMuted).Wrap().Grow(1)
				})
			})
		})
	})
}

// changeColors color the kinds of changes.
var changeColors = map[model.ChangeKind]string{
	model.Added: "#16a34a", model.SheetAdded: "#16a34a", model.Removed: "#dc2626", model.SheetGone: "#dc2626",
	model.StatusSet: "#2563eb", model.TaskSet: "#2563eb",
}

func (a *app) changeRow(c *ui.Context, ch model.Change) {
	t := c.Theme()
	col := t.TextMuted
	if hex, ok := changeColors[ch.Kind]; ok {
		col = ui.Hex(hex)
	}
	row := ui.Row(c).Padding(4, 8).Gap(8).Radius(6).AlignItems(ui.Start)
	if ch.ID != "" && a.file.SheetOf(ch.ID) != nil { // still in the map: a click shows it
		row.Cursor(ui.CursorPointer).Tooltip("在导图中查看")
		if row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		if row.Clicked() {
			id := ch.ID
			sheet := a.file.SheetOf(id).ID
			a.historyOpen = false
			a.later(func() { a.goToNode(a.path, sheet, id) })
		}
	}
	row.Children(func() {
		ui.Box(c).Padding(1, 6).Radius(4).Background(col.Alpha(0.14)).Shrink(0).Children(func() {
			ui.Text(c, string(ch.Kind)).FontSize(11).TextColor(col).NoWrap()
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			title := ch.Title
			if title == "" {
				title = "（无标题）"
			}
			ui.Text(c, title).FontSize(13).Wrap()
			info := ch.Where
			if ch.Detail != "" {
				if info != "" {
					info += " · "
				}
				info += ch.Detail
			}
			if info != "" {
				ui.Text(c, info).FontSize(11).TextColor(t.TextMuted).Wrap()
			}
		})
	})
}
