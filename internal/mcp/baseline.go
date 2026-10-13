package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/gitbase"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A map of a project's code keeps the commit it was brought up to date
// with (model.File.Baseline). read_map shows what changed since and which
// nodes those files are linked to, so that the agent updates those nodes
// alone; set_baseline then moves the baseline to the commit it read.

// maxChanges is how many changed files read_map lists.
const maxChanges = 120

// projectDir is where a map's relative links are: its project folder, or
// the map's own.
func projectDir(path string, f *model.File) string {
	if f.Project != "" {
		return f.Project
	}
	return filepath.Dir(path)
}

// BaselineText is read_map's part on the code: the baseline, and what
// changed since it, by node.
func BaselineText(path string, f *model.File) string {
	var b strings.Builder
	b.WriteString("\n## 代码基线\n\n")
	bl := f.Baseline
	dir := projectDir(path, f)
	if f.Project != "" {
		fmt.Fprintf(&b, "项目代码目录：%s\n", f.Project)
	}
	if bl == nil {
		b.WriteString("还没有记录代码基线。分析完代码、把导图更新到和代码一致后，用 set_baseline 记下当前 commit；以后更新导图只需看这之后的改动，不用重新扫描整个项目。\n")
		return b.String()
	}
	branch := ""
	if bl.Branch != "" {
		branch = "分支 " + bl.Branch + " · "
	}
	fmt.Fprintf(&b, "导图对应的代码：%scommit %s（%s）· 记录于 %s\n", branch, gitbase.Short(bl.Commit), bl.Subject, bl.At.Local().Format("2006-01-02 15:04"))
	if !gitbase.IsRepo(dir) {
		fmt.Fprintf(&b, "找不到代码仓库（%s 不是 git 仓库）：请用 set_baseline 的 project_dir 设置项目代码目录。\n", dir)
		return b.String()
	}
	commits, changes, err := gitbase.Since(dir, bl.Commit)
	if err != nil {
		fmt.Fprintf(&b, "读不出基线之后的改动：%v\n", err)
		return b.String()
	}
	if len(changes) == 0 {
		b.WriteString("基线之后代码没有改动：导图是最新的。\n")
		return b.String()
	}
	fmt.Fprintf(&b, "基线之后：%d 个提交，%d 个文件有改动（含未提交的）。只需按这些改动更新对应的节点（用 git diff %s 看具体内容），新模块加节点、删掉的模块删节点；更新完用 set_baseline 记下新的 commit。\n",
		len(commits), len(changes), gitbase.Short(bl.Commit))
	if len(commits) > 0 {
		b.WriteString("\n提交：\n")
		for i, c := range commits {
			if i == 30 {
				fmt.Fprintf(&b, "- …还有 %d 个\n", len(commits)-30)
				break
			}
			b.WriteString("- " + c + "\n")
		}
	}
	idx := linkIndex(dir, f)
	b.WriteString("\n改动的文件 → 链接了它的节点：\n")
	for i, c := range changes {
		if i == maxChanges {
			fmt.Fprintf(&b, "- …还有 %d 个文件\n", len(changes)-maxChanges)
			break
		}
		name := c.Path
		if c.From != "" {
			name = c.From + " → " + c.Path
		}
		nodes := idx.nodesOf(c.Path)
		if c.From != "" {
			nodes = append(nodes, idx.nodesOf(c.From)...)
		}
		where := "（没有节点链接它）"
		if len(nodes) > 0 {
			var parts []string
			for _, n := range nodes {
				parts = append(parts, "「"+n.Title+"」{id:"+n.ID+"}")
			}
			where = strings.Join(parts, "、")
		}
		fmt.Fprintf(&b, "- %s %s → %s\n", c.Status, name, where)
	}
	return b.String()
}

// linkSet are a map's links to files of the code, by their path relative to
// the repository's root.
type linkSet struct {
	paths []string // longest first, so that the closest wins
	nodes map[string][]*model.Node
}

var lineSuffix = regexp.MustCompile(`(:\d+(-\d+)?|#L\d+(-L?\d+)?)$`)

func linkIndex(dir string, f *model.File) linkSet {
	l := linkSet{nodes: map[string][]*model.Node{}}
	root, err := gitbase.Root(dir)
	if err != nil {
		return l
	}
	root, _ = filepath.EvalSymlinks(root)
	f.Walk(func(n *model.Node) {
		for _, link := range n.Links {
			if strings.Contains(link, "://") {
				continue
			}
			p := lineSuffix.ReplaceAllString(link, "")
			if strings.HasPrefix(p, "~/") {
				if home, err := os.UserHomeDir(); err == nil {
					p = filepath.Join(home, p[2:])
				}
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			if r, err := filepath.EvalSymlinks(p); err == nil {
				p = r
			}
			rel, err := filepath.Rel(root, p)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			rel = filepath.ToSlash(rel)
			if !contains(l.nodes[rel], n) {
				l.nodes[rel] = append(l.nodes[rel], n)
			}
		}
	})
	for p := range l.nodes {
		l.paths = append(l.paths, p)
	}
	sort.Slice(l.paths, func(i, j int) bool { return len(l.paths[i]) > len(l.paths[j]) })
	return l
}

// nodesOf are the nodes linked to a file, or else to the closest folder
// it is in.
func (l linkSet) nodesOf(path string) []*model.Node {
	if ns := l.nodes[path]; len(ns) > 0 {
		return ns
	}
	for _, p := range l.paths {
		if p == "." || strings.HasPrefix(path, p+"/") {
			return l.nodes[p]
		}
	}
	return nil
}

func contains(ns []*model.Node, n *model.Node) bool {
	for _, m := range ns {
		if m == n {
			return true
		}
	}
	return false
}

// setBaseline records the commit a map is up to date with: commit, or the
// one checked out in the project's folder.
func setBaseline(path string, f *model.File, projectArg, commit string, now time.Time) (string, error) {
	if projectArg != "" {
		abs, err := filepath.Abs(projectArg)
		if err != nil {
			return "", err
		}
		if !gitbase.IsRepo(abs) {
			return "", fmt.Errorf("%s 不是 git 仓库", abs)
		}
		f.Project = abs
	}
	dir := projectDir(path, f)
	if !gitbase.IsRepo(dir) {
		return "", errors.New("这张导图还没有项目代码目录：请传 project_dir（项目代码仓库的绝对路径）")
	}
	head, err := gitbase.HeadOf(dir)
	if err != nil {
		return "", err
	}
	bl := &model.Baseline{Commit: head.Commit, Branch: head.Branch, Subject: head.Subject, At: now}
	if commit = strings.TrimSpace(commit); commit != "" {
		full, err := gitbase.Resolve(dir, commit)
		if err != nil {
			return "", fmt.Errorf("仓库里没有 commit %s", commit)
		}
		if full != head.Commit {
			bl = &model.Baseline{Commit: full, At: now}
		}
	}
	f.Baseline = bl
	return fmt.Sprintf("代码基线已记为 commit %s%s。", gitbase.Short(bl.Commit), map[bool]string{true: "（分支 " + bl.Branch + "）", false: ""}[bl.Branch != ""]), nil
}
