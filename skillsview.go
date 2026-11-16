package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/gitbase"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/skills"
)

// AI 技能 is the skills of the AI coding agents on this computer, a list
// of 今天's left column beside AI 动态 (internal/skills): which agents
// there are, which skills each has, every session's, a project's or a
// plugin's, and what is wrong with any. A skill taken in is kept once, in
// the maps folder's .skills, and the agents' folders hold links to it;
// a switch a tool links it or takes the link out, and the store goes
// with the maps to the other computers, which offer to link what this
// one linked. The agents are found by the table of internal/skills, a
// newer one of which is fetched from mindmap-models' release once a day, and an
// agent the table does not know by its folder of skills. Built to the
// draft https://claude.ai/artifact/SUEZAk4SLoJ76BxsM9LXDE.

// skillsTableURL is where a newer table of the agents is: an agent come
// out since the app was built is known by it, with no new version. The
// workflow agent-tools publishes internal/skills/tools.json there when
// it changes on main, beside the update files (mygo.json's updates.url).
const skillsTableURL = "https://github.com/wuyunyoujiduo-jpg/mindmap-models/releases/download/desktop-latest/agent-tools.json"

// skillsState is the page's.
type skillsState struct {
	report    *skills.Report
	env       skills.Env
	repoMap   map[string]string // a repository's map, by its root
	hooks     map[string]bool   // whether the app's hooks are in a tool it connects, by its ID
	busy      bool              // a scan goes on
	again     bool              // and another was asked for meanwhile
	scanned   time.Time
	counted   time.Time // when the uses were last counted
	tool      string    // the tool narrowed to
	side      string    // the part of the left column: "", needs, unused, global, plugin, others, repo:<root>
	moreTools bool      // the tools with no skill shown in the left column
	effTool   string    // the tool whose sessions are looked at
	find      string
	sel       string // the key of the skill chosen
	effRepo   string // the project whose sessions are looked at
	diff      *skillDiff
	diffOn    bool
}

// skillDiff is a tool's copy of a skill beside MyMind's.
type skillDiff struct {
	key, tool string
	lines     []skills.Line
}

// skillsStore is MyMind's folder of skills: in the maps folder, or, with
// none, the home's .mymind-skills.
func (a *app) skillsStore(home string) string {
	if a.dir != "" {
		return filepath.Join(a.dir, ".skills")
	}
	return filepath.Join(home, ".mymind-skills")
}

// machineName is this computer's, as the other computers see it.
func machineName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "computer"
	}
	return strings.TrimSuffix(h, ".local")
}

// skillsTable is the table of agents: the built one, with the newer one
// fetched, if any.
func skillsTable() []skills.Tool {
	base := skills.Builtin()
	if p := skillsTableCache(); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			if newer := skills.Table(data); newer != nil {
				return skills.Merge(base, newer)
			}
		}
	}
	return base
}

func skillsTableCache() string {
	if p := configPath(); p != "" {
		return filepath.Join(filepath.Dir(p), "agent-tools.json")
	}
	return ""
}

// skillsFetchTable fetches the newer table, once a day.
func (a *app) skillsFetchTable() {
	p := skillsTableCache()
	if p == "" {
		return
	}
	if fi, err := os.Stat(p); err == nil && a.now().Sub(fi.ModTime()) < 24*time.Hour {
		return
	}
	get := a.pluginGet
	a.async(func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		data, err := get(ctx, skillsTableURL)
		if err != nil || skills.Table(data) == nil {
			return nil, err
		}
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		return nil, os.WriteFile(p, data, 0o644)
	}, func(any, error) {})
}

// skillsScan is what a scan gives back.
type skillsScan struct {
	env     skills.Env
	report  *skills.Report
	repoMap map[string]string
	hooks   map[string]bool
}

// skillsRescan scans the skills again, away from the window's work.
func (a *app) skillsRescan() {
	if a.skills.busy {
		a.skills.again = true
		return
	}
	home, _ := os.UserHomeDir()
	// The projects of the maps, read here, where the maps are.
	repoMap := map[string]string{}
	for _, fi := range a.files {
		f := a.file
		if fi.Path != a.path {
			f = a.indexed(fi.Path)
		}
		if f != nil && f.Project != "" {
			if _, ok := repoMap[f.Project]; !ok {
				repoMap[filepath.Clean(f.Project)] = fi.Path
			}
		}
	}
	env := skills.Env{Home: home, Store: a.skillsStore(home), Machine: machineName(), Since: a.now().AddDate(0, 0, -30),
		Marked: maps.Clone(a.cfg.SkillsMarked)}
	// The uses are counted once an hour: a scan comes whenever the app is
	// come back to, and the records it reads are large.
	if r := a.skills.report; r != nil && a.now().Sub(a.skills.counted) < time.Hour {
		env.Used = r.Used
	} else {
		a.skills.counted = a.now()
	}
	a.skills.busy = true
	a.async(func() (any, error) {
		env.Table = skillsTable()
		env.Repos = skillRepos(home, repoMap)
		r := skills.Scan(env)
		if skills.Repair(env, r) > 0 {
			r = skills.Scan(env)
		}
		_ = skills.Remember(env, r)
		hooks := map[string]bool{}
		for _, t := range r.Tools {
			if in, takes := hooksIn(t.ID); takes {
				hooks[t.ID] = in
			}
		}
		return skillsScan{env, r, repoMap, hooks}, nil
	}, func(v any, err error) {
		a.skills.busy = false
		if s, ok := v.(skillsScan); ok {
			a.skills.env, a.skills.report, a.skills.repoMap, a.skills.hooks, a.skills.scanned = s.env, s.report, s.repoMap, s.hooks, a.now()
			a.skillsNoticeTools()
		}
		if a.skills.again {
			a.skills.again = false
			a.skillsRescan()
		}
	})
}

// skillsActive scans again when the app is come back to: a tool may have
// been installed, a skill written, meanwhile.
func (a *app) skillsActive() {
	if a.now().Sub(a.skills.scanned) > 10*time.Second {
		a.skillsRescan()
	}
}

// skillRepos is the repositories whose skills are looked at: the maps'
// projects, and those Claude Code worked in. Each has a key of its own,
// the same from a scan to the next (by their roots' order).
func skillRepos(home string, repoMap map[string]string) []skills.Repo {
	roots := map[string]bool{}
	for root := range repoMap {
		if isDirPath(root) {
			roots[root] = true
		}
	}
	var cj struct {
		Projects map[string]json.RawMessage `json:"projects"`
	}
	if data, err := os.ReadFile(filepath.Join(home, ".claude.json")); err == nil && json.Unmarshal(data, &cj) == nil {
		for p := range cj.Projects {
			p = filepath.Clean(p)
			if p != home && isDirPath(p) && (isDirPath(filepath.Join(p, ".git")) || fileThere(filepath.Join(p, ".git"))) {
				roots[p] = true
			}
		}
	}
	sorted := make([]string, 0, len(roots))
	for r := range roots {
		sorted = append(sorted, r)
	}
	slices.Sort(sorted)
	taken := map[string]bool{}
	var out []skills.Repo
	for _, root := range sorted {
		name := filepath.Base(root)
		if p, ok := repoMap[root]; ok {
			name = mapName(p)
		}
		key := skills.RepoKey(filepath.Base(root), taken)
		taken[key] = true
		out = append(out, skills.Repo{Root: root, Name: name, Key: key, Remote: gitbase.Remote(root)})
	}
	return out
}

func isDirPath(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func fileThere(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// skillsNoticeTools learns the tools found; those not seen before are
// new, and said so on the page. The first scan learns them all.
func (a *app) skillsNoticeTools() {
	r := a.skills.report
	if r == nil {
		return
	}
	if !a.cfg.SkillsScanned {
		a.cfg.SkillsScanned = true
		for _, t := range r.Tools {
			a.cfg.SkillsSeen = append(a.cfg.SkillsSeen, t.ID)
		}
		a.cfg.save()
	}
}

// skillsFresh is the tools found that were not before.
func (a *app) skillsFresh() []skills.Tool {
	var out []skills.Tool
	if r := a.skills.report; r != nil && a.cfg.SkillsScanned {
		for _, t := range r.Tools {
			if !slices.Contains(a.cfg.SkillsSeen, t.ID) {
				out = append(out, t)
			}
		}
	}
	return out
}

func (a *app) skillSeen(id string) {
	if !slices.Contains(a.cfg.SkillsSeen, id) {
		a.cfg.SkillsSeen = append(a.cfg.SkillsSeen, id)
		a.cfg.save()
	}
}

// skillState is what a skill's row says of it, and in what color: a
// fault in the danger's, what waits for the user in the accent's.
func skillState(t *ui.Theme, e *skills.Entry) (string, ui.Color) {
	linkedCommitted, differs, alike, gone := false, false, false, false
	for _, c := range e.Copies {
		switch {
		case c.State == skills.Linked && c.Tracked:
			linkedCommitted = true
		case c.State == skills.Diverged:
			differs = true
		case c.State == skills.Same:
			alike = true
		case c.State == skills.Dangling:
			gone = true
		}
	}
	switch {
	// Another's to keep: shown, never for the user to mend.
	case e.Scope == skills.Plugin:
		return "插件的，只读", faint(t)
	case e.Scope == skills.Bundled:
		return "工具自带，只读", faint(t)
	case len(e.Copies) == 0 && len(e.Others) > 0:
		return "在别的电脑上", t.Accent
	case len(e.Issues) > 0:
		return "有问题", t.Danger
	case e.Shadowed:
		return "被全局的盖住", t.Danger
	case linkedCommitted:
		return "链接被提交了", t.Danger
	case gone:
		return "链接指向的没了", t.Danger
	case differs: // a tool's own copy beside MyMind's, not alike: which, the user says
		return "有一份不一样的", t.Accent
	case alike:
		return "有一份重复的", t.Accent
	case !e.Stored:
		return "未收进", faint(t)
	case len(e.Copies) == 0:
		return "停用了", faint(t)
	}
	return "已链接", t.Success
}

// skillNeeds tells whether a skill is for the user to look at.
func skillNeeds(e *skills.Entry) bool {
	if e.Secret && !e.ReadOnly() {
		return true
	}
	switch s, _ := skillState(&ui.Theme{}, e); s {
	case "在别的电脑上", "有问题", "被全局的盖住", "链接被提交了", "链接指向的没了", "有一份不一样的", "有一份重复的":
		return true
	}
	return false
}

// skillsNeedCount is the count of the left column's row.
func (a *app) skillsNeedCount() int {
	n := 0
	if r := a.skills.report; r != nil {
		for _, e := range r.Entries {
			if skillNeeds(e) {
				n++
			}
		}
	}
	return n
}

// skillsShown is the skills the page shows, by what the left column
// narrows to (a part and a tool, together) and the words looked for.
func (a *app) skillsShown() []*skills.Entry {
	r := a.skills.report
	if r == nil {
		return nil
	}
	find := strings.ToLower(strings.TrimSpace(a.skills.find))
	var out []*skills.Entry
	for _, e := range r.Entries {
		if a.skills.tool != "" && e.Copies[a.skills.tool] == nil && !slices.Contains(othersTools(e), a.skills.tool) {
			continue
		}
		if !skillInPart(e, a.skills.side) {
			continue
		}
		if find != "" && !strings.Contains(strings.ToLower(e.Name+" "+e.Description), find) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// skillInPart tells whether a skill is of a part of the left column.
func skillInPart(e *skills.Entry, part string) bool {
	switch {
	case part == "":
		return true
	case part == "needs":
		return skillNeeds(e)
	case part == "unused":
		return e.Used == 0 && len(e.Copies) > 0 && !e.ReadOnly()
	case part == "global":
		return e.Scope == skills.Global && !skillElsewhere(e)
	case part == "plugin":
		return e.Scope == skills.Plugin
	case part == "bundled":
		return e.Scope == skills.Bundled
	case part == "others":
		return skillElsewhere(e)
	case strings.HasPrefix(part, "repo:"):
		return e.Scope == skills.Project && e.Repo != nil && e.Repo.Root == strings.TrimPrefix(part, "repo:") && !skillElsewhere(e)
	}
	return true
}

// skillElsewhere is a skill only another computer has linked.
func skillElsewhere(e *skills.Entry) bool { return len(e.Copies) == 0 && len(e.Others) > 0 }

func othersTools(e *skills.Entry) []string {
	var out []string
	for _, ts := range e.Others {
		out = append(out, ts...)
	}
	return out
}

// showSkills shows 技能, the AI agents' skills, looked at again when it
// was not lately.
func (a *app) showSkills() {
	a.showLibrary()
	a.skillsOpen = true
	if a.skills.report == nil || a.now().Sub(a.skills.scanned) > 10*time.Second {
		a.skillsRescan()
	}
}

// skillsToolCount is how many skills a tool has here.
func (a *app) skillsToolCount(id string) int {
	n := 0
	for _, e := range a.skills.report.Entries {
		if c := e.Copies[id]; c != nil && c.State != skills.Dangling {
			n++
		}
	}
	return n
}

// skillsSide is 技能's left column, the one way to narrow its page: what
// to look at, where the skills are loaded, and the tools, those with
// skills first and the others folded (sixteen tools as cards on the page
// were too many to take in, the user found).
func (a *app) skillsSide(c *ui.Context) {
	a.sidePane(c, "skills-side", func() {
		r := a.skills.report
		if r == nil {
			skeleton(c, "skills-side", 0.8, 0.6, 0.7)
			return
		}
		part := func(id, name, icon string, count int, danger bool) {
			on := a.skills.side == id
			n := ""
			if count > 0 {
				n = fmt.Sprint(count)
			}
			row := sideRowTrail(c, "part:"+id, name, n, on, sideIcon(c, icon), func(bool) {
				if n != "" {
					sideCount(c, n, on, danger && !on)
				}
			})
			if row.Clicked() {
				a.skills.side, a.skills.sel = id, ""
			}
		}
		count := func(id string) int {
			n := 0
			for _, e := range r.Entries {
				if skillInPart(e, id) {
					n++
				}
			}
			return n
		}
		part("", "全部", "puzzle", count(""), false)
		part("needs", "要处理", "alarm-clock", count("needs"), true)
		part("unused", "30 天没用过", "history", count("unused"), false)
		sideHead(c, "范围")
		part("global", "全局", "layers", count("global"), false)
		for _, rp := range a.skills.env.Repos {
			id := "repo:" + rp.Root
			n := count(id)
			if n == 0 {
				continue
			}
			on := a.skills.side == id
			col := faint(c.Theme())
			if p, ok := a.skills.repoMap[rp.Root]; ok {
				col = a.mapColor(p)
			}
			row := sideRow(c, "part:"+id, rp.Name, fmt.Sprint(n), on, func(bool) {
				ui.Box(c).Size(sideIconSize, 18).Shrink(0).PassThrough().Draw(func(p *ui.Painter, rc ui.Rect) {
					p.FillPath(new(ui.Path).Circle(rc.X+rc.W/2, rc.Y+rc.H/2, 4), col)
				})
			}).Tooltip("只在这个项目里加载：" + tildePath(rp.Root, a.skills.env.Home))
			if row.Clicked() {
				a.skills.side, a.skills.sel = id, ""
			}
		}
		if n := count("plugin"); n > 0 {
			part("plugin", "插件带的", "plug", n, false)
		}
		if n := count("bundled"); n > 0 {
			part("bundled", "工具自带的", "archive", n, false)
		}
		if n := count("others"); n > 0 {
			part("others", "别的电脑上的", "sync", n, false)
		}
		sideHead(c, "工具")
		tools := slices.Clone(r.Tools)
		counts := map[string]int{}
		for _, tl := range tools {
			counts[tl.ID] = a.skillsToolCount(tl.ID)
		}
		slices.SortStableFunc(tools, func(x, y skills.Tool) int { return counts[y.ID] - counts[x.ID] })
		var empty []skills.Tool
		toolRow := func(tl skills.Tool) {
			on := a.skills.tool == tl.ID
			tip := fmt.Sprintf("每次会话先读约 %d 字的技能说明", r.Load(tl.ID))
			if in, takes := a.skills.hooks[tl.ID]; takes {
				tip += map[bool]string{true: " · 已连接 MyMind", false: " · 还没连接 MyMind"}[in]
			}
			row := sideRow(c, "tool:"+tl.ID, tl.Name, fmt.Sprint(counts[tl.ID]), on, func(bool) {
				ui.Box(c).Size(sideIconSize, 22).Center().Shrink(0).PassThrough().Children(func() { toolMark(c, tl, 18) })
			}).Tooltip(tip)
			if row.Clicked() {
				a.skills.sel = ""
				if on {
					a.skills.tool = ""
				} else {
					a.skills.tool = tl.ID
				}
			}
		}
		for _, tl := range tools {
			if counts[tl.ID] == 0 && a.skills.tool != tl.ID {
				empty = append(empty, tl)
				continue
			}
			toolRow(tl)
		}
		if len(empty) > 0 {
			name := fmt.Sprintf("另外 %d 个没有技能", len(empty))
			if a.skills.moreTools {
				name = "收起没有技能的"
			}
			names := make([]string, len(empty))
			for i, tl := range empty {
				names[i] = tl.Name
			}
			more := sideRow(c, "tools-more", name, "", false, func(bool) {
				ui.Box(c).Size(sideIconSize, 22).Center().Shrink(0).PassThrough().Children(func() { chevron(c, faint(c.Theme())) })
			}).Tooltip(strings.Join(names, "、"))
			if more.Clicked() {
				a.skills.moreTools = !a.skills.moreTools
			}
			if a.skills.moreTools {
				for _, tl := range empty {
					toolRow(tl)
				}
			}
		}
	}, nil)
}

// skillsPage is 技能: its head, the page and, at the right, what is
// chosen.
func (a *app) skillsPage(c *ui.Context) {
	ui.Row(c.Key("skills-page")).Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Stretch).Label("技能").Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Scroll(c.Key("skills-scroll")).Grow(1).Basis(0).MinHeight(0).Children(func() {
				pageColumn(c, "skills-column", false).MaxWidth(1036).Padding(0, 28, 40).Gap(6).Children(func() {
					sub := ""
					var fs []figure
					if r := a.skills.report; r != nil {
						sub = fmt.Sprintf("%d 个编程工具", len(r.Tools))
						n := 0
						repos := map[string]bool{}
						for _, e := range r.Entries {
							if len(e.Copies) > 0 || e.Stored {
								n++
							}
							if e.Scope == skills.Project && e.Repo != nil {
								repos[e.Repo.Root] = true
							}
						}
						fs = []figure{{n, "个技能"}, {a.skillsNeedCount(), "个要处理"}}
						if len(repos) > 0 {
							fs = append(fs, figure{len(repos), "个项目有自己的"})
						}
						if tl, ok := r.Tool(a.skills.tool); ok {
							fs = append(fs, figure{r.Load(tl.ID), "字 · " + tl.Name + " 每次会话先读"})
						}
					}
					pageHead(c, "skills-head", "AI 技能", sub, fs, func() {
						if busyClicked(busyKey(c, "重新扫描", "再看一遍各个工具的技能目录", a.skills.busy, true, false), a.skills.busy, true) {
							a.later(func() {
								a.skillsRescan()
								a.tell("扫过了各个工具的技能目录")
							})
						}
					})
					a.skillsView(c)
				})
			})
		})
		a.skillsSheet(c)
	})
}

// skillsView is the page under its head.
func (a *app) skillsView(c *ui.Context) {
	t := c.Theme()
	r := a.skills.report
	if r == nil {
		if !a.skills.busy {
			a.later(a.skillsRescan)
		}
		skeleton(c, "skills", 0.7, 0.5, 0.9, 0.6, 0.8, 0.4)
		return
	}
	// Where the one copy is, a line.
	ui.Row(c.Key("skills-store")).Gap(4).AlignItems(ui.Center).FillWidth().Children(func() {
		ui.Text(c, "技能只存一份，在 "+tildePath(a.skills.env.Store, a.skills.env.Home)+"，各个工具里放的是指向它的链接。").
			FontSize(textCaption).TextColor(t.TextMuted).SingleLine().Shrink(1).MinWidth(0)
		if textLink(c, "在访达中显示", "在访达中显示技能的目录").Clicked() {
			if isDirPath(a.skills.env.Store) {
				a.plat.ShowInFolder(a.skills.env.Store)
			} else {
				a.tell("还没有收进来的技能")
			}
		}
	})
	a.skillsSuggest(c)
	// What narrows the page, each with its ✕, and the words looked for.
	ui.Row(c.Key("skills-filters")).Gap(6).GapY(6).Wrap().AlignItems(ui.Center).FillWidth().Margin(10, 0, 2).Children(func() {
		if a.skills.side != "" {
			name := map[string]string{"needs": "要处理", "unused": "30 天没用过", "global": "全局", "plugin": "插件带的", "others": "别的电脑上的"}[a.skills.side]
			for _, rp := range a.skills.env.Repos {
				if a.skills.side == "repo:"+rp.Root {
					name = "项目 " + rp.Name
				}
			}
			if filterChip(c, name) {
				a.skills.side = ""
			}
		}
		if tl, ok := r.Tool(a.skills.tool); ok && filterChip(c, tl.Name) {
			a.skills.tool = ""
		}
		ui.Spacer(c)
		searchBox(c, "skills-find", &a.skills.find, "找技能", "找技能", "按名字和说明找", 180)
	})
	shown := a.skillsShown()
	if len(shown) == 0 {
		switch {
		case strings.TrimSpace(a.skills.find) != "":
			emptyState(c, "skills-found", "search", "没有找到", "没有名字或说明里含「"+strings.TrimSpace(a.skills.find)+"」的技能。", func() {
				if textKey(c, "清除搜索", "清除搜索", "").Clicked() {
					a.skills.find = ""
				}
			})
		case len(r.Tools) == 0:
			emptyState(c, "skills-none", "puzzle", "没有找到 AI 编程工具", "装上 Claude Code、Codex、Grok Build 这类工具后，它们的技能会出现在这里。", nil)
		case a.skills.side == "" && a.skills.tool == "":
			emptyState(c, "skills-none", "puzzle", "还没有技能", "技能是一个带 SKILL.md 的文件夹，放在工具的 skills 目录里，比如 ~/.claude/skills。", nil)
		case a.skills.side == "needs":
			emptyState(c, "skills-none", "circle-check", "都好好的", "没有要处理的技能。", nil)
		default:
			emptyState(c, "skills-none", "puzzle", "这里没有技能", "换一个工具或范围看看。", nil)
		}
		return
	}
	a.skillsTable(c, shown)
}

// tildePath is p with the home as ~.
func tildePath(p, home string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// A skillSuggestion is what the page offers to do for many skills at once.
type skillSuggestion struct {
	key, title, sub string
	act, tip        string
	do              func()
	main            bool // the key in the accent: the first thing to do
}

// skillsSuggestions is what can be done in one click, worked out from the
// scan: as soon as it is installed, the user has only to click (the
// user's ask, 0.99.2: one by one was too slow, and the page too full to
// know where to begin).
func (a *app) skillsSuggestions() []skillSuggestion {
	r := a.skills.report
	env := a.skills.env
	var out []skillSuggestion
	// A tool new here: the skills linked to it.
	if fresh := a.skillsFresh(); len(fresh) > 0 {
		var names []string
		for _, tl := range fresh {
			names = append(names, tl.Name)
		}
		stored := 0
		for _, e := range r.Entries {
			if e.Stored && e.Scope == skills.Global {
				stored++
			}
		}
		s := skillSuggestion{key: "fresh", title: "发现新装的 " + strings.Join(names, "、"),
			sub: "按它们的技能目录认出来的。", act: "知道了", do: func() {
				for _, tl := range fresh {
					a.skillSeen(tl.ID)
				}
			}}
		if stored > 0 {
			s.sub = fmt.Sprintf("可以把 MyMind 里的 %d 个全局技能链接过去，新开的会话就能用。", stored)
			s.act, s.tip = "链接过去", "把 MyMind 里的全局技能链接到新装的工具"
			s.do = func() { a.skillsLinkTo(fresh) }
		}
		out = append(out, s)
	}
	// Skills each tool keeps a copy of: taken in, one copy and links.
	var take []*skills.Entry
	dup, secret := 0, 0
	for _, e := range r.Entries {
		if e.Stored || e.ReadOnly() || skillOwner(r, e) == "" {
			continue
		}
		if e.Secret {
			secret++
			continue
		}
		take = append(take, e)
		own := 0
		for _, cp := range e.Copies {
			if cp.State == skills.Own {
				own++
			}
		}
		if own > 1 {
			dup++
		}
	}
	if len(take) > 0 {
		sub := "收进来只留一份，各工具里换成指向它的链接，用起来和现在一样；以后改一处，处处都改了。"
		if dup > 0 {
			sub = fmt.Sprintf("其中 %d 个在好几个工具里各存了一份。", dup) + sub
		}
		if secret > 0 {
			sub += fmt.Sprintf("另有 %d 个像含密钥，先不收。", secret)
		}
		out = append(out, skillSuggestion{key: "take", title: fmt.Sprintf("%d 个技能还没收进 MyMind", len(take)), sub: sub,
			act: "全部收进", tip: "把它们都收进 MyMind，原处换成链接", main: true, do: func() {
				ops := make([]func() (skills.Undo, error), 0, len(take))
				for _, e := range take {
					owner := skillOwner(r, e)
					ops = append(ops, func() (skills.Undo, error) { return skills.Adopt(env, e, owner) })
				}
				a.skillsBatch("收进了 %d 个技能，各工具里换成了链接", ops)
			}})
	}
	// Copies alike that took a link's place, and links to nothing.
	var same, gone []func() (skills.Undo, error)
	for _, e := range r.Entries {
		if e.ReadOnly() {
			continue
		}
		for tool, cp := range e.Copies {
			switch cp.State {
			case skills.Same:
				same = append(same, func() (skills.Undo, error) { return skills.Resolve(env, e, tool, false) })
			case skills.Dangling:
				gone = append(gone, func() (skills.Undo, error) { return skills.Forget(e, tool) })
			}
		}
	}
	if len(same) > 0 {
		out = append(out, skillSuggestion{key: "same", title: fmt.Sprintf("%d 份复制和 MyMind 里的一模一样", len(same)),
			sub: "是工具重写技能时把链接换掉了。换回链接，以后改一处就行。", act: "都换成链接",
			do: func() { a.skillsBatch("%d 份复制换回了链接", same) }})
	}
	if len(gone) > 0 {
		out = append(out, skillSuggestion{key: "gone", title: fmt.Sprintf("%d 个链接指向的文件夹没了", len(gone)),
			sub: "技能在别处被删了，链接留了下来。", act: "都拿掉", do: func() { a.skillsBatch("拿掉了 %d 个断了的链接", gone) }})
	}
	// What another computer linked and this one has not.
	var here []func() (skills.Undo, error)
	for _, e := range r.Entries {
		if skillElsewhere(e) && (e.Scope != skills.Project || e.Repo != nil) {
			here = append(here, func() (skills.Undo, error) { return skills.InstallHere(env, e, r.Tools) })
		}
	}
	if len(here) > 0 {
		out = append(out, skillSuggestion{key: "here", title: fmt.Sprintf("%d 个技能在别的电脑上链接了，这台还没有", len(here)),
			sub: "随导图同步过来的，链接到和那台一样的工具。", act: "都链接上", do: func() { a.skillsBatch("在这台链接上了 %d 个技能", here) }})
	}
	// What is for the user to judge, one by one.
	look := 0
	for _, e := range r.Entries {
		if !skillNeeds(e) || skillElsewhere(e) {
			continue
		}
		judged := false
		for _, cp := range e.Copies {
			judged = judged || cp.State == skills.Diverged || cp.Tracked && cp.State == skills.Linked
		}
		if judged || len(e.Issues) > 0 || e.Shadowed || e.Secret {
			look++
		}
	}
	if look > 0 {
		out = append(out, skillSuggestion{key: "look", title: fmt.Sprintf("还有 %d 个要你看一眼", look),
			sub: "格式不对、和全局同名、内容和 MyMind 的不一样、像含密钥：这些得你来定。", act: "去看看",
			do: func() { a.skills.side, a.skills.tool, a.skills.sel = "needs", "", "" }})
	}
	return out
}

// skillOwner is the tool whose copy of its own a skill would be taken in
// from: the first of the table's that has one (Claude Code's first).
func skillOwner(r *skills.Report, e *skills.Entry) string {
	for _, tl := range r.Tools {
		if cp := e.Copies[tl.ID]; cp != nil && cp.State == skills.Own {
			return tl.ID
		}
	}
	return ""
}

// skillsSuggest shows the suggestions at the page's head, each with the
// key that does it.
func (a *app) skillsSuggest(c *ui.Context) {
	t := c.Theme()
	ss := a.skillsSuggestions()
	if len(ss) == 0 {
		return
	}
	raise(ui.Column(c.Key("skills-suggest")).Padding(4, 16).Radius(radiusCard).FillWidth().Margin(10, 0, 0), t).Children(func() {
		for i, s := range ss {
			if i > 0 {
				setLine(c)
			}
			ui.Row(c.Key("sugg:"+s.key)).MinHeight(56).Padding(10, 0).Gap(14).AlignItems(ui.Center).FillWidth().Children(func() {
				ui.Column(c).Gap(3).Grow(1).Shrink(1).Basis(0).MinWidth(0).Children(func() {
					ui.Text(c, s.title).FontSize(textBody).FontWeight(700).Wrap()
					ui.Text(c, s.sub).FontSize(textCaption).TextColor(t.TextMuted).LineHeight(1.45).Wrap()
				})
				var b ui.Element
				if s.main {
					b = accentButton(c, s.act, s.tip)
				} else {
					b = key(ui.Box(c.Key("act:"+s.key)).Height(keyHeight).Padding(0, 14).Radius(radiusPill).Center().Cursor(ui.CursorPointer).
						Label(s.act).Tooltip(s.tip).Shrink(0), t)
					b.Children(func() { ui.Text(c, s.act).FontSize(textSmall).NoWrap().PassThrough() })
				}
				if b.Clicked() {
					a.later(s.do)
				}
				if s.key == "fresh" && s.act != "知道了" && textKey(c, "知道了", "知道了，不链接", "").Clicked() {
					a.later(func() {
						for _, tl := range a.skillsFresh() {
							a.skillSeen(tl.ID)
						}
					})
				}
			})
		}
	})
}

// skillsBatch does many changes as one, says how many (done holds a %d)
// and how many failed, and takes them all back by one 撤销.
func (a *app) skillsBatch(done string, ops []func() (skills.Undo, error)) {
	var undos []skills.Undo
	failed := 0
	var first error
	for _, op := range ops {
		u, err := op()
		if err != nil {
			failed++
			if first == nil {
				first = err
			}
			continue
		}
		undos = append(undos, u)
	}
	a.skillsRescan()
	msg := fmt.Sprintf(done, len(undos))
	if failed > 0 {
		msg += fmt.Sprintf("；%d 个没做成：%v", failed, first)
	}
	if len(undos) == 0 {
		a.tell(msg)
		return
	}
	a.tellUndo(msg, func() {
		for i := len(undos) - 1; i >= 0; i-- {
			_ = undos[i]()
		}
		a.skillsRescan()
	})
}

// agentToolOf is the name, in agentTools, of the tool of id, which the
// app connects; "" for one it does not.
func agentToolOf(id string) string {
	return map[string]string{"claude": "Claude Code", "codex": "Codex", "grok": "Grok Build"}[id]
}

// hooksIn tells whether the app's hooks are in the tool's settings, and
// whether it is a tool the app connects; it reads them, in a scan.
func hooksIn(id string) (bool, bool) {
	name := agentToolOf(id)
	if name == "" {
		return false, false
	}
	for _, tl := range agentTools(executable()) {
		if tl.name == name {
			data, err := os.ReadFile(tl.hooks)
			return err == nil && strings.Contains(string(data), " hook start") && strings.Contains(string(data), "MyMind"), true
		}
	}
	return false, false
}

// skillsLinkTo links every global skill of MyMind to the tools, new here.
func (a *app) skillsLinkTo(tools []skills.Tool) {
	r := a.skills.report
	env := a.skills.env
	var ops []func() (skills.Undo, error)
	for _, tl := range tools {
		a.skillSeen(tl.ID)
		for _, e := range r.Entries {
			if e.Stored && e.Scope == skills.Global && e.Copies[tl.ID] == nil {
				ops = append(ops, func() (skills.Undo, error) { return skills.Link(env, e, tl) })
			}
		}
	}
	a.skillsBatch("链接了 %d 个技能过去，新开的会话生效", ops)
}

// toolMark is a tool's two letters on a small tile in the ink.
func toolMark(c *ui.Context, tl skills.Tool, size float32) {
	t := c.Theme()
	ui.Box(c.Key("mark:"+tl.ID)).Size(size, size).Radius(5).Center().Shrink(0).Background(t.Text).Children(func() {
		ui.Text(c, tl.Mark()).FontSize(size * 0.46).FontWeight(700).TextColor(cardColor(t)).NoWrap()
	})
}

// skillsGroup is a part of the list: a scope, a project, the other
// computers'.
type skillsGroup struct {
	name, word string
	color      *ui.Color
	items      []*skills.Entry
}

// skillsTable is the list of the skills shown, by where they are.
func (a *app) skillsTable(c *ui.Context, shown []*skills.Entry) {
	t := c.Theme()
	r := a.skills.report
	var groups []skillsGroup
	add := func(name, word string, col *ui.Color, keep func(e *skills.Entry) bool) {
		var items []*skills.Entry
		for _, e := range shown {
			if keep(e) {
				items = append(items, e)
			}
		}
		if len(items) > 0 {
			groups = append(groups, skillsGroup{name, word, col, items})
		}
	}
	elsewhere := func(e *skills.Entry) bool { return len(e.Copies) == 0 && len(e.Others) > 0 }
	add("全局", "每个会话都会加载", nil, func(e *skills.Entry) bool { return e.Scope == skills.Global && !elsewhere(e) })
	for _, rp := range a.skills.env.Repos {
		rp := rp
		var col *ui.Color
		if p, ok := a.skills.repoMap[rp.Root]; ok {
			cc := a.mapColor(p)
			col = &cc
		}
		add("项目 · "+rp.Name, tildePath(rp.Root, a.skills.env.Home), col, func(e *skills.Entry) bool {
			return e.Scope == skills.Project && e.Repo != nil && e.Repo.Root == rp.Root && !elsewhere(e)
		})
	}
	add("插件带的", "由插件管理，这里只能看", nil, func(e *skills.Entry) bool { return e.Scope == skills.Plugin })
	add("工具自带的", "装工具时带的，由工具自己更新，这里只能看", nil, func(e *skills.Entry) bool { return e.Scope == skills.Bundled })
	add("别的电脑上的", "随导图同步过来，这台还没链接", nil, elsewhere)
	marksW := float32(len(r.Tools))*28 + 4
	// In a narrow room the count of uses goes, before the names do.
	w, _ := c.Size()
	room := w
	if a.sideShown() {
		room -= sideWidthOf
	}
	if r.Entry(a.skills.sel) != nil {
		room -= 380
	} else if w >= 1180 {
		room -= 340
	}
	wide := room-56 >= marksW+64+118+340
	ui.Column(c.Key("skills-list")).FillWidth().Children(func() {
		// The columns, named once.
		ui.Row(c.Key("head")).Height(30).Padding(0, 6).Gap(12).AlignItems(ui.Center).FillWidth().BorderWidth(0, 0, 1, 0).BorderColor(hairline(t)).Children(func() {
			for _, col := range []struct {
				name string
				w    float32
			}{{"技能", 0}, {"在哪些工具", marksW}, {"30 天用过", 64}, {"状态", 118}} {
				if col.name == "30 天用过" && !wide {
					continue
				}
				txt := ui.Text(c, col.name).FontSize(textCaption).TextColor(t.TextMuted).NoWrap()
				if col.w == 0 {
					txt.Grow(1)
				} else {
					txt.Width(col.w).Shrink(0)
				}
				if col.name == "30 天用过" {
					txt.TextAlign(ui.End).Tooltip("按 Claude Code 的会话记录数")
				}
			}
		})
		for _, g := range groups {
			ui.Row(c.Key("group:"+g.name)).Height(38).Padding(10, 6, 0).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
				if g.color != nil {
					ui.Box(c).Size(7, 7).Radius(4).Background(*g.color).Shrink(0)
				}
				ui.Text(c, g.name).FontSize(textCaption).FontWeight(700).LetterSpacing(0.3).TextColor(t.TextMuted).NoWrap()
				ui.Text(c, fmt.Sprintf("%d · %s", len(g.items), g.word)).FontSize(textCaption).TextColor(t.TextMuted).SingleLine().Shrink(1).MinWidth(0)
			})
			for _, e := range g.items {
				a.skillRow(c, e, marksW, wide)
			}
		}
	})
}

// skillRow is a skill's row: its name and what it says, the tools it is
// in, how often it was used, how it stands.
func (a *app) skillRow(c *ui.Context, e *skills.Entry, marksW float32, wide bool) {
	t := c.Theme()
	r := a.skills.report
	on := a.skills.sel == e.Key
	row := listCardRow(c, "skill:"+e.Key).Cursor(ui.CursorPointer).Label(e.Name)
	if on {
		row.Background(t.Text.Alpha(0.06))
	}
	if row.Clicked() {
		if on {
			a.skills.sel = ""
		} else {
			a.skills.sel = e.Key
		}
	}
	row.ContextMenu(func(m *ui.Menu) { a.skillMenu(m, e) })
	row.Children(func() {
		ui.Column(c).Gap(2).Grow(1).Shrink(1).Basis(0).MinWidth(0).PassThrough().Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				name := ui.Text(c, e.Name).Font("monospace").FontSize(textSmall + 0.5).SingleLine().Shrink(1).MinWidth(0)
				if on {
					name.FontWeight(700)
				}
				if e.Script {
					pill(c, "含脚本", t.Text.Alpha(0.05), t.TextMuted)
				}
				if e.Secret {
					pill(c, "含密钥", t.Danger.Alpha(0.1), t.Danger)
				}
			})
			desc := cmpOr(library.Mask(e.Description), "没有说明")
			ui.Text(c, desc).FontSize(textCaption).TextColor(t.TextMuted).SingleLine()
		})
		ui.Row(c).Width(marksW).Gap(4).Shrink(0).PassThrough().Children(func() {
			for _, tl := range r.Tools {
				a.skillMark(c, e, tl)
			}
		})
		used := "—"
		ink := faint(t)
		if e.Used > 0 {
			used, ink = fmt.Sprint(e.Used), ink2(t)
		}
		if len(e.Copies) == 0 {
			used = ""
		}
		if wide {
			ui.Text(c, used).Width(64).Shrink(0).TextAlign(ui.End).FontSize(textSmall).FontFeatures("tnum").TextColor(ink).PassThrough()
		}
		ui.Row(c).Width(118).Shrink(0).PassThrough().Children(func() {
			s, col := skillState(t, e)
			statePill(c, s, col)
		})
	})
}

// statePill is a state: a pale pill, a dot of its color.
func statePill(c *ui.Context, text string, col ui.Color) {
	t := c.Theme()
	ui.Row(c.Key("state")).Height(22).Padding(0, 8).Gap(6).Radius(radiusSmall).AlignItems(ui.Center).Shrink(1).MinWidth(0).Background(t.Text.Alpha(0.05)).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(col).Shrink(0)
		ui.Text(c, text).FontSize(textCaption).TextColor(ink2(t)).SingleLine()
	})
}

// skillMark is a tool's mark on a skill's row: full where it has the
// skill, a dashed outline where it has not, a dot where it holds a copy
// of its own where a link was.
func (a *app) skillMark(c *ui.Context, e *skills.Entry, tl skills.Tool) {
	t := c.Theme()
	cp := e.Copies[tl.ID]
	tip := tl.Name + "：没有"
	switch {
	case cp == nil && slices.Contains(othersTools(e), tl.ID):
		tip = tl.Name + "：别的电脑上有"
	case cp == nil:
	case cp.State == skills.Linked:
		tip = tl.Name + "：链接到 MyMind 的那份"
	case cp.State == skills.Own:
		tip = tl.Name + "：它自己的一份"
	case cp.State == skills.Diverged:
		tip = tl.Name + "：有一份它自己的，和 MyMind 的不一样"
	case cp.State == skills.Same:
		tip = tl.Name + "：有一份它自己的，和 MyMind 的一样"
	case cp.State == skills.Elsewhere:
		tip = tl.Name + "：链接到 " + tildePath(cp.Target, a.skills.env.Home)
	case cp.State == skills.Dangling:
		tip = tl.Name + "：链接指向的文件夹没了"
	}
	has := cp != nil && cp.State != skills.Dangling
	ui.Box(c.Key("mk:"+tl.ID)).Size(24, 22).Shrink(0).Tooltip(tip).Draw(func(p *ui.Painter, r ui.Rect) {
		path := roundedRect(r.X+0.5, r.Y+0.5, r.W-1, r.H-1, 6)
		if has {
			p.FillPath(path, t.Text.Alpha(0.075))
		} else {
			p.StrokePath(path.Dash(3, 2), 1, hairline(t))
		}
		if cp != nil && (cp.State == skills.Diverged || cp.State == skills.Same) {
			p.FillPath(new(ui.Path).Circle(r.X+r.W-2, r.Y+2, 3.5), t.Accent)
		}
		if cp != nil && cp.State == skills.Dangling {
			p.FillPath(new(ui.Path).Circle(r.X+r.W-2, r.Y+2, 3.5), t.Danger)
		}
	}).Children(func() {
		ink := faint(t)
		if has {
			ink = t.Text
		}
		ui.Text(c, tl.Mark()).FontSize(9.5).FontWeight(700).TextColor(ink).NoWrap().PassThrough()
	}).Center()
}

// skillMenu is what can be done with a skill, under the pointer's other
// key and the ⋯ of its details.
func (a *app) skillMenu(m *ui.Menu, e *skills.Entry) {
	if m.Item("看详情").Chosen() {
		a.skills.sel = e.Key
	}
	if e.Dir != "" {
		if m.Item("在编辑器中打开").Chosen() {
			dir := e.Dir
			a.later(func() { a.plat.OpenPath(filepath.Join(dir, "SKILL.md")) })
		}
		if m.Item("在访达中显示").Chosen() {
			dir := e.Dir
			a.later(func() { a.plat.ShowInFolder(dir) })
		}
	}
	if mark, ok := skillMarkOffer(e); ok {
		m.Separator()
		if m.Item(mark).Chosen() {
			a.later(func() { a.skillMarkOwn(e, e.Scope != skills.Bundled) })
		}
	}
	if e.ReadOnly() {
		return
	}
	m.Separator()
	if e.Stored && len(e.Copies) > 0 && m.Item("停用").Chosen() {
		a.later(func() { a.skillDisable(e) })
	}
	if m.Item("删除").Chosen() {
		a.later(func() { a.skillDelete(e) })
	}
}

// skillDo does what changes the folders, scans again, and says it with
// the way back.
func (a *app) skillDo(done string, op func() (skills.Undo, error)) bool {
	undo, err := op()
	if err != nil {
		a.tell("没做成：" + err.Error())
		a.skillsRescan()
		return false
	}
	a.skillsRescan()
	a.tellUndo(done, func() {
		if err := undo(); err != nil {
			a.tell("没能撤回：" + err.Error())
		}
		a.skillsRescan()
	})
	return true
}

// skillMarkOffer is the words of the item that says a skill came with its
// tool, or that it did not, and whether it is offered: not for a skill
// MyMind keeps, a project's or a plugin's, nor one in the tool's own
// folder of them, which the table says.
func skillMarkOffer(e *skills.Entry) (string, bool) {
	switch {
	case e.Stored || e.Scope == skills.Plugin || e.Scope == skills.Project || len(e.Copies) == 0:
		return "", false
	case e.Scope == skills.Bundled && strings.HasPrefix(e.Key, "b/"):
		return "", false
	case e.Scope == skills.Bundled:
		return "不是工具自带的", true
	}
	return "标为工具自带", true
}

// skillMarkOwn keeps what the user said of a skill: that it came with its
// tool, and is to be left alone, or that it did not.
func (a *app) skillMarkOwn(e *skills.Entry, bundled bool) {
	key := e.Key
	was, had := a.cfg.SkillsMarked[key]
	if a.cfg.SkillsMarked == nil {
		a.cfg.SkillsMarked = map[string]bool{}
	}
	a.cfg.SkillsMarked[key] = bundled
	a.cfg.save()
	a.skillsRescan()
	say := e.Name + " 标为工具自带：只显示，不再提示要处理，也不会被收进"
	if !bundled {
		say = e.Name + " 不再当作工具自带，和你自己的技能一样处理"
	}
	a.tellUndo(say, func() {
		if had {
			a.cfg.SkillsMarked[key] = was
		} else {
			delete(a.cfg.SkillsMarked, key)
		}
		a.cfg.save()
		a.skillsRescan()
	})
}

func (a *app) skillDisable(e *skills.Entry) {
	a.skillDo("已停用 "+e.Name+"，拿掉了各处的链接，MyMind 里还留着", func() (skills.Undo, error) { return skills.Disable(a.skills.env, e) })
}

func (a *app) skillDelete(e *skills.Entry) {
	what := "已删除 " + e.Name + "，文件在 MyMind 的回收里"
	if e.Stored {
		what = "已删除 " + e.Name + "，各处的链接也拿掉了，文件在 MyMind 的回收里"
	}
	if a.skillDo(what, func() (skills.Undo, error) { return skills.Delete(a.skills.env, e) }) && a.skills.sel == e.Key {
		a.skills.sel = ""
	}
}

// skillsSheet is the column at the page's right: the skill chosen, or,
// with none and room for it, what a project's sessions load.
func (a *app) skillsSheet(c *ui.Context) {
	r := a.skills.report
	if r == nil {
		return
	}
	if e := r.Entry(a.skills.sel); e != nil {
		sheet(c, "skill-sheet", 380, func() { a.skillDetail(c, e) })
		return
	}
	if w, _ := c.Size(); w >= 1180 && len(a.skills.env.Repos) > 0 && len(r.Tools) > 0 {
		sheet(c, "skill-next", 340, func() {
			ui.Scroll(c.Key("skills-next")).Grow(1).Basis(0).MinHeight(0).FillWidth().Children(func() {
				ui.Column(c).Padding(22, 22, 30).Gap(6).FillWidth().Children(func() { a.skillsEffective(c) })
			})
		})
	}
}

// skillsEffective says which skills a session of a tool in a project
// uses: every session's, its plugins', the project's, and those shadowed
// faint.
func (a *app) skillsEffective(c *ui.Context) {
	t := c.Theme()
	r := a.skills.report
	repos := a.skills.env.Repos
	repo := repos[0]
	for _, rp := range repos {
		if rp.Root == a.skills.effRepo {
			repo = rp
		}
	}
	tl := r.Tools[0]
	if x, ok := r.Tool(a.skills.tool); ok {
		tl = x
	} else if x, ok := r.Tool(a.skills.effTool); ok {
		tl = x
	}
	on, off := r.Effective(tl.ID, repo.Root)
	ui.Text(c, "开一个会话，实际加载哪些").FontSize(textPage - 4).FontWeight(700).LetterSpacing(-0.3).Wrap()
	ui.Text(c, "全局的、插件带的，加上这个项目自己的；同名被盖住的不算。").FontSize(textCaption).TextColor(t.TextMuted).LineHeight(1.45).Wrap().Margin(0, 0, 8)
	raise(ui.Column(c.Key("eff")).Padding(10, 12).Gap(8).Radius(radiusCard).FillWidth(), t).Children(func() {
		ui.Row(c).Gap(6).GapY(6).Wrap().AlignItems(ui.Center).Children(func() {
			dropButton(c, "eff-tool", tl.Name, "看哪个工具", "看哪个工具的会话").Menu(func(m *ui.Menu) {
				for _, x := range r.Tools {
					if a.skillsToolCount(x.ID) == 0 {
						continue
					}
					if m.Item(x.Name).Checked(x.ID == tl.ID).Chosen() {
						a.skills.effTool, a.skills.tool = x.ID, ""
					}
				}
			})
			ui.Text(c, "在").FontSize(textSmall).TextColor(t.TextMuted).NoWrap()
			dropButton(c, "eff-repo", repo.Name, "看哪个项目", "看在哪个项目里开会话").Menu(func(m *ui.Menu) {
				for _, rp := range repos {
					if m.Item(rp.Name).Checked(rp.Root == repo.Root).Chosen() {
						a.skills.effRepo = rp.Root
					}
				}
			})
		})
		chars := 0
		for _, e := range on {
			chars += utf8.RuneCountInString(e.Name) + utf8.RuneCountInString(e.Description)
		}
		ui.Text(c, fmt.Sprintf("%d 个 · 先读约 %d 字的说明", len(on), chars)).FontSize(textCaption).TextColor(t.TextMuted)
		ui.Row(c.Key("names")).Gap(4).GapY(4).Wrap().FillWidth().Children(func() {
			chip := func(e *skills.Entry, shadowed bool) {
				tip := "全局"
				switch {
				case shadowed:
					tip = "被全局的同名技能盖住，不会被用上"
				case e.Scope == skills.Project:
					tip = "这个项目自己的"
				case e.Scope == skills.Plugin:
					tip = "插件 " + e.Plugin + " 的"
				case e.Scope == skills.Bundled:
					tip = "工具自带的"
				}
				b := ui.Box(c.Key("eff:"+e.Key)).Padding(2, 7).Radius(6).Background(cardColor(t)).InsetShadow(0, 0, 0, 1, hairline(t)).Tooltip(tip).Cursor(ui.CursorPointer)
				if b.Clicked() {
					a.skills.sel = e.Key
				}
				b.Children(func() {
					ink := ink2(t)
					if shadowed {
						ink = faint(t)
					}
					ui.Text(c, e.Name).Font("monospace").FontSize(11.5).TextColor(ink).NoWrap().PassThrough()
				})
			}
			for _, e := range on {
				chip(e, false)
			}
			for _, e := range off {
				chip(e, true)
			}
		})
	})
}

// skillDetail is a skill chosen: what it is, the tools it is in, where it
// stands, its SKILL.md and files, and what can be done with it.
func (a *app) skillDetail(c *ui.Context, e *skills.Entry) {
	t := c.Theme()
	r := a.skills.report
	env := a.skills.env
	if !a.barOpen && !a.skills.diffOn && c.Shortcut(0, ui.KeyEscape) {
		if c.FocusTakesText() {
			a.escLeave = true
		} else {
			a.skills.sel = ""
		}
	}
	a.escRest(c)
	ui.Scroll(c.Key("skill-detail:" + e.Key)).Grow(1).Basis(0).MinHeight(0).FillWidth().Children(func() {
		ui.Column(c).Padding(20, 22, 32).Gap(6).FillWidth().Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).FillWidth().Children(func() {
				ui.Text(c, e.Name).Font("monospace").FontSize(17).FontWeight(700).Wrap().Grow(1).Shrink(1).Basis(0).MinWidth(0).Selectable()
				more := iconButton(c, iconMore, "操作 "+e.Name, "在编辑器中打开、停用、删除…", 28, true)
				more.Menu(func(m *ui.Menu) { a.skillMenu(m, e) })
				if iconButton(c, iconClose, "关闭", "关闭（Esc）", 28, true).Clicked() {
					a.skills.sel = ""
				}
			})
			ui.Text(c, cmpOr(library.Mask(e.Description), "没有说明。")).FontSize(textSmall+0.5).TextColor(ink2(t)).LineHeight(1.5).Wrap().Margin(4, 0, 0)
			where := "全局"
			switch e.Scope {
			case skills.Project:
				where = "项目 " + repoName(e)
			case skills.Plugin:
				where = "插件 " + e.Plugin
			case skills.Bundled:
				where = "工具自带"
				if tl, ok := r.Tool(e.By); ok {
					where = tl.Name + " 自带"
				}
			}
			meta := where
			if len(e.Copies) > 0 {
				meta += fmt.Sprintf(" · 30 天用过 %d 次", e.Used)
			}
			meta += fmt.Sprintf(" · 说明 %d 字", utf8.RuneCountInString(e.Description))
			ui.Text(c, meta).FontSize(textCaption).TextColor(t.TextMuted).Margin(0, 0, 8)
			a.skillWarnings(c, e)
			a.skillNotes(c, e)
			if !(len(e.Copies) == 0 && len(e.Others) > 0) {
				a.skillCopies(c, e)
			}
			if e.Stored {
				a.skillScope(c, e)
				ui.Column(c.Key("other-computers")).Gap(8).FillWidth().Margin(14, 0, 0).Children(func() {
					ui.Text(c, "别的电脑").FontSize(textSmall).FontWeight(700)
					say := "在设置的「同步」里打开 GitHub 同步，技能就会跟着导图到别的电脑，那边一点就能链接上。"
					if a.cfg.SyncRepo != "" {
						say = "跟着导图同步到 GitHub。别的电脑上的链接由那台自己建，打开 AI 技能就能一点链接上。"
					}
					ui.Text(c, say).FontSize(textCaption).TextColor(t.TextMuted).LineHeight(1.5).Wrap()
				})
			}
			if e.Dir != "" {
				a.skillFile(c, e)
			}
			// What is done to it, at its foot.
			ui.Row(c.Key("acts")).Gap(6).GapY(6).Wrap().FillWidth().Margin(18, 0, 0).Padding(14, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(hairline(t)).Children(func() {
				if e.Dir != "" {
					if textKey(c, "在编辑器中打开", "在编辑器中打开 SKILL.md", "").Clicked() {
						a.plat.OpenPath(filepath.Join(e.Dir, "SKILL.md"))
					}
					if textKey(c, "在访达中显示", "在访达中显示这个技能", "").Clicked() {
						a.plat.ShowInFolder(e.Dir)
					}
				}
				if mark, ok := skillMarkOffer(e); ok && textKey(c, mark, mark+" "+e.Name, "").Clicked() {
					a.later(func() { a.skillMarkOwn(e, e.Scope != skills.Bundled) })
				}
				if e.Scope == skills.Plugin {
					if tl, ok := r.Tool(firstTool(e)); ok && textKey(c, "复制成自己的", "复制成自己的", "复制到 MyMind，作为全局技能链接到 "+tl.Name).Clicked() {
						a.later(func() {
							a.skillDo("已复制成自己的 "+e.Name, func() (skills.Undo, error) { return skills.CopyOwn(env, e, tl) })
						})
					}
				}
				if e.ReadOnly() {
					return
				}
				if e.Stored && len(e.Copies) > 0 && textKey(c, "停用", "停用 "+e.Name, "拿掉各处的链接，MyMind 里留着").Clicked() {
					a.later(func() { a.skillDisable(e) })
				}
				del := ui.Box(c.Key("del")).Height(28).Padding(0, 12).Radius(radiusPill).Center().Cursor(ui.CursorPointer).Label("删除 " + e.Name).
					Background(t.Danger.Alpha(0)).Transition(easeLook)
				if del.Hovered() {
					del.Background(t.Danger.Alpha(0.09))
				}
				if press(del, 0.96).Clicked() {
					a.later(func() { a.skillDelete(e) })
				}
				del.Children(func() { ui.Text(c, "删除").FontSize(textSmall).TextColor(t.Danger).NoWrap().PassThrough() })
			})
		})
	})
	if d := a.skills.diff; d != nil && d.key == e.Key {
		a.skillDiffView(c, e, d)
	}
}

func repoName(e *skills.Entry) string {
	if e.Repo != nil {
		return e.Repo.Name
	}
	return "（这台没有它的仓库）"
}

func firstTool(e *skills.Entry) string {
	for id := range e.Copies {
		return id
	}
	return ""
}

// warnBlock is what is wrong, on the danger's tint, and what mends it.
func warnBlock(c *ui.Context, key string, lines []string, act func()) {
	t := c.Theme()
	ui.Column(c.Key("warn:"+key)).Gap(8).FillWidth().Margin(0, 0, 8).Children(func() {
		ui.Column(c).Padding(10, 12).Gap(6).Radius(radiusCard).FillWidth().Background(t.Danger.Alpha(0.08)).Children(func() {
			for i, l := range lines {
				ui.Row(c.Key(i)).Gap(8).AlignItems(ui.Start).FillWidth().Children(func() {
					ui.Box(c).Size(6, 6).Radius(3).Background(t.Danger).Margin(7, 0, 0).Shrink(0)
					ui.Text(c, l).FontSize(textSmall).LineHeight(1.5).Wrap().Grow(1).Shrink(1).Basis(0).MinWidth(0)
				})
			}
		})
		if act != nil {
			ui.Row(c).Gap(6).Children(act)
		}
	})
}

// skillWarnings is what is wrong with a skill.
func (a *app) skillWarnings(c *ui.Context, e *skills.Entry) {
	env := a.skills.env
	r := a.skills.report
	if e.ReadOnly() {
		return // another's to mend: its notes say whose
	}
	if len(e.Issues) > 0 {
		warnBlock(c, "issues", e.Issues, nil)
	}
	if e.Shadowed {
		other := e.Name
		if e.Repo != nil {
			other = e.Repo.Key + "-" + e.Name
		}
		warnBlock(c, "shadow", []string{"全局也有一个 " + e.Name + "。同名时全局的优先，" + repoName(e) + " 里的这份不会被用上。改个名字，两份就都能用。"}, func() {
			if e.Stored && accentButton(c, "改名为 "+other, "改名为 "+other).Clicked() {
				a.later(func() {
					if a.skillDo("已改名为 "+other+"，两份都能用了", func() (skills.Undo, error) { return skills.Rename(env, e, other, r.Tools) }) {
						a.skills.sel = strings.TrimSuffix(e.Key, e.Name) + other
					}
				})
			}
			if textKey(c, "看全局的那份", "看全局的那份", "").Clicked() {
				a.skills.sel = "g/" + e.Name
			}
		})
	}
	for _, tl := range r.Tools {
		cp := e.Copies[tl.ID]
		if cp == nil {
			continue
		}
		if cp.State == skills.Linked && cp.Tracked {
			warnBlock(c, "tracked:"+tl.ID, []string{repoName(e) + " 仓库里 " + tl.Name + " 的这个链接已经提交了。它指向这台电脑上的路径，队友和 CI 那边打不开。"}, func() {
				if accentButton(c, "换成真文件", "换成仓库里的真文件").Clicked() {
					a.later(func() { a.skillMakeReal(e) })
				}
			})
		}
		if cp.State == skills.Dangling {
			warnBlock(c, "gone:"+tl.ID, []string{tl.Name + " 里的链接指向 " + tildePath(cp.Target, env.Home) + "，那里没有了。"}, func() {
				if accentButton(c, "拿掉这个链接", "拿掉 "+tl.Name+" 里的链接").Clicked() {
					a.later(func() {
						a.skillDo("已拿掉 "+tl.Name+" 里的链接", func() (skills.Undo, error) { return skills.Forget(e, tl.ID) })
					})
				}
			})
		}
	}
	if e.Secret {
		say := "这个技能的文件里有一串像密钥的字。"
		if e.Stored {
			say += "它在 MyMind 里，会跟着导图同步到 GitHub：挪到环境变量里更稳妥。"
		} else {
			say += "收进 MyMind 之前先挪到环境变量里，不然会跟着导图同步到 GitHub。"
		}
		warnBlock(c, "secret", []string{say}, nil)
	}
}

// skillNotes says what a plugin's, another computer's or a tool's own
// skill is, and the next thing to do with it.
func (a *app) skillNotes(c *ui.Context, e *skills.Entry) {
	t := c.Theme()
	env := a.skills.env
	r := a.skills.report
	note := func(key, text string, act func()) {
		ui.Column(c.Key("note:"+key)).Gap(10).FillWidth().Margin(0, 0, 8).Children(func() {
			raise(ui.Box(c.Key("block")).Padding(10, 12).Radius(radiusCard).FillWidth(), t).Children(func() {
				ui.Text(c, text).FontSize(textSmall).TextColor(ink2(t)).LineHeight(1.5).Wrap()
			})
			if act != nil {
				ui.Row(c).Gap(6).Children(act)
			}
		})
	}
	switch {
	case e.Scope == skills.Plugin:
		note("plugin", "由插件 "+e.Plugin+" 装上和更新。要停用就停用整个插件；要改它，先复制成一份自己的。", nil)
	case e.Scope == skills.Bundled:
		tool := "工具"
		if tl, ok := r.Tool(e.By); ok {
			tool = tl.Name
		}
		why := "在 " + tool + " 放自带技能的地方"
		if e.Marked {
			why = "你标的"
		} else if !strings.HasPrefix(e.Key, "b/") {
			why = "它指向 " + tool + " 的安装目录"
		}
		note("bundled", "装 "+tool+" 时带的（"+why+"），由 "+tool+" 自己更新。改了会在它更新时被盖掉，换成链接也可能被它换回来，所以这里只显示、不处理，有什么问题也不提示。不想用它，在 "+tool+" 里关掉。", nil)
	case len(e.Copies) == 0 && len(e.Others) > 0:
		var where []string
		for m, ts := range e.Others {
			var names []string
			for _, id := range ts {
				if tl, ok := r.Tool(id); ok {
					names = append(names, tl.Name)
				} else {
					names = append(names, id)
				}
			}
			where = append(where, m+" 上链接在 "+strings.Join(names, "、"))
		}
		slices.Sort(where)
		note("others", "在别的电脑上收进了 MyMind，随导图同步过来了："+strings.Join(where, "；")+"。", func() {
			if e.Scope == skills.Project && e.Repo == nil {
				return
			}
			if accentButton(c, "在这台链接上", "在这台电脑的工具里链接上").Clicked() {
				a.later(func() {
					a.skillDo("已在这台链接上 "+e.Name, func() (skills.Undo, error) { return skills.InstallHere(env, e, r.Tools) })
				})
			}
		})
	case e.Stored && len(e.Copies) == 0:
		note("off", "停用了：MyMind 里留着，哪个工具里都没有链接。打开下面的开关就能再用。", nil)
	case !e.Stored:
		var in []string
		owner := ""
		for _, tl := range r.Tools {
			if cp := e.Copies[tl.ID]; cp != nil && cp.State == skills.Own {
				in = append(in, tl.Name)
				if owner == "" {
					owner = tl.ID
				}
			}
		}
		if owner == "" {
			return
		}
		note("own", "只在 "+strings.Join(in, "、")+" 里有，是它自己的一份。收进 MyMind：文件搬到 MyMind 的技能目录里，原处换成链接，用起来和现在一样；之后可以一键链接到别的工具，也会跟着导图同步到别的电脑。", func() {
			if e.Secret {
				ui.Text(c, "先把密钥挪走再收进来").FontSize(textCaption).TextColor(t.Danger)
				return
			}
			if accentButton(c, "收进 MyMind", "收进 MyMind").Clicked() {
				a.later(func() {
					a.skillDo("已收进 MyMind："+e.Name, func() (skills.Undo, error) { return skills.Adopt(env, e, owner) })
				})
			}
		})
	}
}

// skillCopies is the tools a skill is in: a switch each, for one MyMind
// has, which links it there or takes the link out.
func (a *app) skillCopies(c *ui.Context, e *skills.Entry) {
	t := c.Theme()
	env := a.skills.env
	r := a.skills.report
	ui.Column(c.Key("copies")).Gap(8).FillWidth().Margin(10, 0, 0).Children(func() {
		ui.Text(c, "在哪些工具里").FontSize(textSmall).FontWeight(700)
		raise(ui.Column(c.Key("rows")).Padding(2, 12).Radius(radiusCard).FillWidth(), t).Children(func() {
			for i, tl := range r.Tools {
				if i > 0 {
					setLine(c)
				}
				tl := tl
				cp := e.Copies[tl.ID]
				ui.Row(c.Key("copy:"+tl.ID)).MinHeight(50).Padding(6, 0).Gap(10).AlignItems(ui.Center).FillWidth().Children(func() {
					toolMark(c, tl, 20)
					ui.Column(c).Gap(2).Grow(1).Shrink(1).Basis(0).MinWidth(0).Children(func() {
						ui.Text(c, tl.Name).FontSize(textBody)
						say, col, mono := "没有", t.TextMuted, false
						switch {
						case cp == nil:
							if e.Stored && e.Scope == skills.Project && len(tl.Project) == 0 {
								say = "没有项目里的技能目录"
							}
						case cp.State == skills.Diverged:
							say, col = "有一份它自己的，和 MyMind 的不一样", t.Accent
						case cp.State == skills.Same:
							say, col = "有一份它自己的，和 MyMind 的一样", t.Accent
						case cp.State == skills.Dangling:
							say, col = "链接指向的文件夹没了", t.Danger
						case cp.State == skills.Linked:
							say, mono = tildePath(cp.Path, env.Home)+" → MyMind", true
						default:
							say, mono = tildePath(cp.Path, env.Home), true
						}
						txt := ui.Text(c, say).FontSize(11.5).TextColor(col).LineHeight(1.4).Wrap()
						if mono {
							txt.Font("monospace")
						}
					})
					if cp != nil && (cp.State == skills.Diverged || cp.State == skills.Same) {
						if textKey(c, "看差异", "看 "+tl.Name+" 的差异", "").Clicked() {
							a.later(func() { a.skillShowDiff(e, tl.ID) })
						}
					}
					if !e.Stored || e.ReadOnly() || (cp != nil && cp.State != skills.Linked) {
						return
					}
					if e.Scope == skills.Project && (e.Repo == nil || len(tl.Project) == 0) {
						return
					}
					on := cp != nil
					was := on
					ui.Switch(c.Key("sw:"+tl.ID), &on).Shrink(0).Label("链接到 " + tl.Name)
					if on != was {
						a.later(func() {
							if was {
								a.skillDo("已拿掉 "+tl.Name+" 里的链接，MyMind 里的还在", func() (skills.Undo, error) { return skills.Unlink(env, e, tl.ID) })
							} else {
								a.skillDo("已链接到 "+tl.Name+"，新开的会话生效", func() (skills.Undo, error) { return skills.Link(env, e, tl) })
							}
						})
					}
				})
			}
		})
	})
}

// skillScope is where a skill of MyMind is loaded, and its links in a
// repository.
func (a *app) skillScope(c *ui.Context, e *skills.Entry) {
	t := c.Theme()
	env := a.skills.env
	r := a.skills.report
	repos := env.Repos
	ui.Column(c.Key("scope")).Gap(8).FillWidth().Margin(14, 0, 0).Children(func() {
		ui.Text(c, "放在哪一层").FontSize(textSmall).FontWeight(700)
		raise(ui.Column(c.Key("rows")).Padding(2, 12).Radius(radiusCard).FillWidth(), t).Children(func() {
			sub := "这台电脑上每个会话都会加载"
			if e.Scope == skills.Project {
				sub = "只在 " + repoName(e) + " 的会话里加载"
			}
			setRow(c, map[bool]string{true: "全局", false: "只在项目里"}[e.Scope == skills.Global], sub, func() {
				ui.Box(c).Width(140).Children(func() {
					cur := 0
					if e.Scope == skills.Project {
						cur = 1
					}
					if to := segmented(c, "skill-scope", "放在", []string{"全局", "项目"}, cur); to != cur {
						switch {
						case to == 0:
							a.later(func() {
								if a.skillDo("已改成全局，每个会话都会加载", func() (skills.Undo, error) { return skills.SetScope(env, e, skills.Global, nil, r.Tools) }) {
									a.skills.sel = "g/" + e.Name
								}
							})
						case len(repos) == 0:
							a.tell("还没有关联代码仓库的项目：在导图的设置里填上项目代码目录")
						default:
							rp := repos[0]
							for _, x := range repos {
								if x.Root == a.skills.effRepo {
									rp = x
								}
							}
							a.later(func() { a.skillToRepo(e, rp) })
						}
					}
				})
			})
			if e.Scope != skills.Project {
				return
			}
			setLine(c)
			setRow(c, "项目", "", func() {
				dropButton(c, "skill-repo", repoName(e), "换一个项目", "挪到另一个项目").Menu(func(m *ui.Menu) {
					for _, rp := range repos {
						rp := rp
						if m.Item(rp.Name).Checked(e.Repo != nil && rp.Root == e.Repo.Root).Chosen() && (e.Repo == nil || rp.Root != e.Repo.Root) {
							a.later(func() { a.skillToRepo(e, rp) })
						}
					}
				})
			})
			setLine(c)
			setRow(c, "仓库里是链接", "提交不提交由你。链接指向这台电脑上的路径；要给队友用，换成真文件。", func() {
				linked := false
				for _, cp := range e.Copies {
					linked = linked || cp.State == skills.Linked
				}
				if linked && textKey(c, "换成真文件", "换成仓库里的真文件", "文件放进仓库，其他工具用仓库内的相对链接指向它").Clicked() {
					a.later(func() { a.skillMakeReal(e) })
				}
			})
		})
	})
}

func (a *app) skillToRepo(e *skills.Entry, rp skills.Repo) {
	env := a.skills.env
	if a.skillDo("已放进 "+rp.Name+" 的仓库，只在它的会话里加载", func() (skills.Undo, error) {
		return skills.SetScope(env, e, skills.Project, &rp, a.skills.report.Tools)
	}) {
		a.skills.sel = "p/" + rp.Key + "/" + e.Name
	}
}

func (a *app) skillMakeReal(e *skills.Entry) {
	env := a.skills.env
	a.skillDo("已换成 "+repoName(e)+" 仓库里的真文件，提交后队友也能用", func() (skills.Undo, error) {
		return skills.MakeReal(env, e, a.skills.report.Tools)
	})
}

// skillFile is a skill's SKILL.md, its secrets in dots, and its files.
func (a *app) skillFile(c *ui.Context, e *skills.Entry) {
	t := c.Theme()
	ui.Column(c.Key("file")).Gap(8).FillWidth().Margin(14, 0, 0).Children(func() {
		raw := e.Raw // read in the scan: nothing reads a file in a frame
		ui.Row(c).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
			ui.Text(c, "SKILL.md").FontSize(textSmall).FontWeight(700).Grow(1)
			if textKey(c, "复制", "复制 SKILL.md", "复制到剪贴板").Clicked() {
				a.plat.CopyText(raw)
				a.tell("已复制")
			}
		})
		text := library.Mask(raw)
		if r := []rune(text); len(r) > 3000 {
			text = string(r[:3000]) + "\n…"
		}
		ui.Box(c.Key("md")).Padding(10, 12).Radius(radiusCard).Background(blockFill(t)).FillWidth().Children(func() {
			ui.Text(c, strings.TrimRight(text, "\n")).Font("monospace").FontSize(11.5).LineHeight(1.55).TextColor(ink2(t)).Wrap().Selectable()
		})
		if len(e.Files) > 1 {
			ui.Text(c, "文件").FontSize(textSmall).FontWeight(700).Margin(6, 0, 0)
			ui.Column(c.Key("files")).Gap(3).FillWidth().Children(func() {
				for _, f := range e.Files {
					ui.Row(c.Key(f)).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, f).Font("monospace").FontSize(12).TextColor(ink2(t)).SingleLine().Shrink(1).MinWidth(0)
						if skills.IsScript(f) {
							pill(c, "会被执行", t.Text.Alpha(0.05), t.TextMuted)
						}
					})
				}
			})
		}
	})
}

// skillShowDiff reads a tool's copy beside MyMind's, to choose one.
func (a *app) skillShowDiff(e *skills.Entry, tool string) {
	cp := e.Copies[tool]
	if cp == nil || !e.Stored {
		return
	}
	ours, _ := os.ReadFile(filepath.Join(e.StorePath, "SKILL.md"))
	theirs, _ := os.ReadFile(filepath.Join(cp.Path, "SKILL.md"))
	lines := skills.Changed(skills.Diff(library.Mask(string(ours)), library.Mask(string(theirs))), 2)
	a.skills.diff = &skillDiff{e.Key, tool, lines}
	a.skills.diffOn = true
}

// skillDiffView is a tool's copy beside MyMind's, and the choice of one.
func (a *app) skillDiffView(c *ui.Context, e *skills.Entry, d *skillDiff) {
	r := a.skills.report
	env := a.skills.env
	tl, _ := r.Tool(d.tool)
	if !a.skills.diffOn {
		a.later(func() { a.skills.diff = nil })
		return
	}
	modal(c, "skill-diff", &a.skills.diffOn, func() {
		t := c.Theme()
		w, _ := c.Size()
		ui.Column(c).Gap(10).Width(min(640, w-80)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tl.Name+" 里的 "+e.Name+" 和 MyMind 的不一样").FontSize(textTitle).FontWeight(700).Grow(1).Wrap()
				if iconButton(c, iconClose, "关闭", "关闭（Esc）", 28, true).Clicked() {
					a.skills.diffOn = false
				}
			})
			ui.Text(c, "可能是 "+tl.Name+" 重写了这个技能，或者收进来之前它就有自己的一份。红的行只在 MyMind 的那份里，蓝的只在 "+tl.Name+" 的这份里。").
				FontSize(textCaption).TextColor(t.TextMuted).LineHeight(1.5).Wrap()
			ui.Scroll(c.Key("lines")).MaxHeight(360).FillWidth().Children(func() {
				ui.Column(c).Padding(8, 0).Radius(radiusRow).Background(blockFill(t)).FillWidth().Children(func() {
					if len(d.lines) == 0 {
						ui.Text(c, "SKILL.md 一样，别的文件有不同。").FontSize(12).TextColor(t.TextMuted).Padding(0, 12)
					}
					for i, l := range d.lines {
						bg, fg, mark := t.Text.Alpha(0), ink2(t), "  "
						switch l.Kind {
						case '-':
							bg, fg, mark = t.Danger.Alpha(0.1), t.Danger, "- "
						case '+':
							bg, fg, mark = t.Accent.Alpha(0.1), t.Accent, "+ "
						case '~':
							fg, mark = faint(t), "⋯"
						}
						ui.Box(c.Key(i)).Padding(0, 12).Background(bg).FillWidth().Children(func() {
							ui.Text(c, mark+l.Text).Font("monospace").FontSize(12).LineHeight(1.6).TextColor(fg).Wrap()
						})
					}
				})
			})
			ui.Row(c).Gap(8).GapY(8).Wrap().AlignItems(ui.Center).Children(func() {
				if accentButton(c, "用这份，换回链接", "用 "+tl.Name+" 的这份").Clicked() {
					a.skills.diffOn = false
					a.later(func() {
						a.skillDo("已用这份，换回了链接，其他工具也更新了", func() (skills.Undo, error) { return skills.Resolve(env, e, d.tool, true) })
					})
				}
				if textKey(c, "丢掉这份，换回链接", "丢掉 "+tl.Name+" 的这份", "放进 MyMind 的回收里").Clicked() {
					a.skills.diffOn = false
					a.later(func() {
						a.skillDo("已换回链接，那份复制放进了回收", func() (skills.Undo, error) { return skills.Resolve(env, e, d.tool, false) })
					})
				}
				if textKey(c, "先不管", "先不管", "").Clicked() {
					a.skills.diffOn = false
				}
			})
			ui.Text(c, "用这份：它搬进 MyMind 替换原来那份，其他工具通过链接一起更新。").FontSize(textCaption).TextColor(t.TextMuted).Wrap()
		})
	})
}
