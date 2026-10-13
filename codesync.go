package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/gitbase"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/mcp"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A map of a project's code keeps the commit it matches, its code
// baseline. Only the AI moves it: asked to bring the map up to date, it
// reads what changed in the code since and proposes the edits, and
// applying them records the commit it read, in the same step, which undo
// takes back. Nobody marks a map up to date by hand, which would hide the
// changes it has not caught up with.

// maxPatch is how much of the diff the AI is given.
const maxPatch = 80 << 10

// headBaseline is the commit checked out in dir, as a baseline, or nil.
func headBaseline(dir string, now time.Time) *model.Baseline {
	h, err := gitbase.HeadOf(dir)
	if err != nil {
		return nil
	}
	return &model.Baseline{Commit: h.Commit, Branch: h.Branch, Subject: h.Subject, At: now}
}

// baselineLabel says which commit a baseline is, as "master · 3e252a4（…）".
func baselineLabel(b *model.Baseline) string {
	s := "commit " + gitbase.Short(b.Commit)
	if b.Branch != "" {
		s = b.Branch + " · " + s
	}
	if b.Subject != "" {
		s += "（" + b.Subject + "）"
	}
	return s
}

// codeStatus is how far the code has gone since the map's baseline, as
// last looked at.
type codeStatus struct {
	key     string // the map and its baseline
	at      time.Time
	commits int
	files   int
	err     string
}

// codeStatusOf looks at the code again when what it knows is old.
func (a *app) codeStatusOf(dir string) codeStatus {
	key := a.path + "\x00" + dir + "\x00"
	if b := a.file.Baseline; b != nil {
		key += b.Commit
	}
	if a.codeStat.key == key && a.now().Sub(a.codeStat.at) < 20*time.Second {
		return a.codeStat
	}
	st := codeStatus{key: key, at: a.now()}
	if b := a.file.Baseline; b != nil {
		commits, changes, err := gitbase.Since(dir, b.Commit)
		if err != nil {
			st.err = err.Error()
		}
		st.commits, st.files = len(commits), len(changes)
	}
	a.codeStat = st
	return st
}

// codeSyncField shows the map's code baseline, how far the code has gone
// since, and the button that has the AI bring the map up to date.
func (a *app) codeSyncField(c *ui.Context) {
	t := c.Theme()
	ui.Field(c, "代码基线", func() {
		ui.Column(c).Gap(6).Children(func() {
			dir := a.projectDir()
			repo := gitbase.IsRepo(dir)
			bl := a.file.Baseline
			var lines []string
			label := "按代码更新导图"
			ready := repo
			switch {
			case !repo:
				lines = append(lines, "先选择项目代码目录（一个 git 仓库）")
			case bl == nil:
				lines = append(lines, "还没有：让 AI 对照代码检查一遍导图，应用后记下导图对应的 commit，之后只需看新的改动")
				label = "用 AI 对照代码检查导图"
			default:
				lines = append(lines, "导图对应 "+baselineLabel(bl), "由 AI 更新于 "+bl.At.Local().Format("2006-01-02 15:04"))
				switch st := a.codeStatusOf(dir); {
				case st.err != "":
					lines = append(lines, st.err)
				case st.files == 0:
					lines = append(lines, "✓ 代码之后没有改动，导图是最新的")
					ready = false
				default:
					lines = append(lines, fmt.Sprintf("之后有 %d 个提交、%d 个文件改动还没同步到导图", st.commits, st.files))
				}
			}
			for i, l := range lines {
				col := t.TextMuted
				if i == len(lines)-1 && bl != nil && ready {
					col = t.Warning
				}
				ui.Text(c.Key(i), l).FontSize(12).TextColor(col).Wrap()
			}
			if ui.Button(c.Key("code-sync"), label).FontSize(12).AlignSelf(ui.Start).Disabled(!ready).
				Tooltip("AI 读基线之后的代码改动，提出对导图的修改；你应用后才更新代码基线").Clicked() {
				a.later(a.syncWithCode)
			}
		})
	})
}

// syncWithCode has the AI bring the map up to date with the code: from
// what changed since the baseline, or, without one, by checking the map
// against the code's layout. The proposals it makes carry the commit read.
func (a *app) syncWithCode() {
	if a.file == nil {
		return
	}
	dir := a.projectDir()
	head := headBaseline(dir, a.now())
	if head == nil {
		a.tell("读不出项目代码的当前 commit")
		return
	}
	var prompt, extra string
	if bl := a.file.Baseline; bl == nil {
		prompt = "对照下面代码仓库的目录结构和说明，检查这张导图（整个文件）和代码是否一致：补上缺的模块、改正过时的描述、删掉已经不存在的部分，给模块节点补上对应代码的 links（相对仓库根目录的路径）。没变化的部分不要动。用 propose_edits 一次提出所有修改；如果导图已经和代码一致，也调用 propose_edits，operations 为空数组，summary 写「导图已与代码一致」。"
		extra = scanRepo(dir, 3)
	} else {
		patch, err := gitbase.Patch(dir, bl.Commit, maxPatch)
		if err != nil {
			a.tell("读不出代码改动：" + err.Error())
			return
		}
		prompt = "根据下面「代码基线」之后的代码改动更新这张导图（整个文件），让它和当前代码一致：只改受影响的节点——改备注、补 links、为新模块加节点、删掉已删除模块的节点、按需要更新任务状态；没变化的部分不要动，不要重写整张图。用 propose_edits 一次提出所有修改；如果导图不需要改，也调用 propose_edits，operations 为空数组，summary 写「导图已与代码一致」。"
		extra = mcp.BaselineText(a.path, a.file) + "\n## 代码改动（git diff " + gitbase.Short(bl.Commit) + "）\n\n```diff\n" + patch + "\n```\n"
	}
	a.ai.nextSync = head // applied, the map matches this code
	a.aiAction(prompt, scopeFile, extra)
}
