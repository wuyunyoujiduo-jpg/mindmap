package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// What changes the folders: each gives a way back, which undoes it while
// nothing else changed the same folders.

// Undo takes back what was done.
type Undo func() error

// trashDir is where what is put away waits, in Env.Keep (in the store
// when there is none, hidden there), not synced: a way back.
const trashDir = ".trash"

// linkIn puts at a link to target, a skill of the scope: from inside the
// repository for a project's, so that the repository's other clones
// follow it too; else whole, a folder of the home's being linked to.
func linkIn(scope Scope, target, at string) error {
	if scope == Project {
		return makeRelLink(target, at)
	}
	return makeLink(target, at)
}

// Where is where a skill of the store goes in a tool's folders: e's
// scope's first folder.
func Where(env Env, t Tool, e *Entry) string {
	return where(env, t, e.Scope, e.Repo, e.Name)
}

func where(env Env, t Tool, scope Scope, repo *Repo, name string) string {
	switch scope {
	case Global:
		return filepath.Join(env.Home, t.Global[0], name)
	case Project:
		if repo == nil || len(t.Project) == 0 {
			return ""
		}
		return filepath.Join(repo.Root, t.Project[0], name)
	}
	return ""
}

// storePath is where MyMind keeps a skill.
func storePath(env Env, scope Scope, repo *Repo, name string) string {
	if scope == Project && repo != nil {
		return filepath.Join(ProjectStore(repo.Root), name)
	}
	return filepath.Join(env.Store, name)
}

// disabledPath is where a skill turned off waits.
func disabledPath(env Env, e *Entry) string {
	keep := env.Keep
	if keep == "" {
		keep = filepath.Join(env.Store, trashDir)
	}
	if e.Scope == Project && e.Repo != nil {
		return filepath.Join(keep, disabledDir, projectsDir, e.Repo.Key, e.Name)
	}
	return filepath.Join(keep, disabledDir, e.Name)
}

// Adopt takes a skill into MyMind: its folder, of tool's copy, moves to
// the store and a link takes its place; the other tools' copies alike
// are put away for links too. Those not alike stay, to be looked at.
func Adopt(env Env, e *Entry, tool string) (Undo, error) {
	c := e.Copies[tool]
	if e.Stored || c == nil || c.State != Own || e.Scope == Plugin {
		return nil, errors.New("这个技能不能收进来")
	}
	dst := storePath(env, e.Scope, e.Repo, e.Name)
	if exists(dst) {
		return nil, fmt.Errorf("MyMind 里已经有一个 %s", e.Name)
	}
	sum := treeSum(c.Path)
	if err := moveDir(c.Path, dst); err != nil {
		return nil, err
	}
	// A tool that reads MyMind's folder itself is given no link: it would
	// read the skill twice.
	native := env.ReadsID(tool, e.Scope)
	if !native {
		if err := linkIn(e.Scope, dst, c.Path); err != nil {
			_ = moveDir(dst, c.Path)
			return nil, err
		}
	}
	type moved struct{ from, to string }
	var alike []moved
	for id, o := range e.Copies {
		if o == c || o.Target != "" || treeSum(o.Path) != sum {
			continue
		}
		if to, err := putAway(env, o.Path); err == nil {
			if env.ReadsID(id, e.Scope) || linkIn(e.Scope, dst, o.Path) == nil {
				alike = append(alike, moved{o.Path, to})
			} else {
				_ = moveDir(to, o.Path)
			}
		}
	}
	return func() error {
		for _, m := range alike {
			_ = os.Remove(m.from)
			_ = moveDir(m.to, m.from)
		}
		if !native {
			if err := os.Remove(c.Path); err != nil {
				return err
			}
		}
		return moveDir(dst, c.Path)
	}, nil
}

// GiveBack undoes a taking in, as Adopt's way back does once the app
// has forgotten it: each of this computer's links to MyMind's copy is
// replaced by a folder of its own, a copy of it, and MyMind's copy is
// put away. A skill no tool links here is not given back: it would be
// gone.
func GiveBack(env Env, e *Entry) (Undo, error) {
	if !e.Stored || e.ReadOnly() {
		return nil, errors.New("这个技能不在 MyMind 里")
	}
	var linked []*Copy
	for _, c := range e.Copies {
		if c.State == Linked {
			linked = append(linked, c)
		}
	}
	// A tool that read MyMind's folder itself gets a copy in a folder of
	// its own, where there is one (Codex's ~/.codex/skills).
	var own []string
	for _, t := range env.Table {
		if c := e.Copies[t.ID]; c == nil || c.State != Native {
			continue
		}
		dirs := t.Global
		root := env.Home
		if e.Scope == Project && e.Repo != nil {
			dirs, root = t.Project, e.Repo.Root
		}
		for _, d := range dirs {
			if p := filepath.Join(root, d, e.Name); filepath.Clean(filepath.Join(root, d)) != filepath.Clean(filepath.Dir(e.StorePath)) && !exists(p) {
				own = append(own, p)
				break
			}
		}
	}
	if len(linked) == 0 && len(own) == 0 {
		return nil, errors.New("这台电脑上没有工具链接着它，放回去就没了；不要了就删除")
	}
	var undos []Undo
	for _, p := range own {
		if err := copyDir(e.StorePath, p); err != nil {
			_ = os.RemoveAll(p)
			_ = all(undos)()
			return nil, err
		}
		undos = append(undos, func() error { return os.RemoveAll(p) })
	}
	for _, c := range linked {
		if err := os.Remove(c.Path); err != nil {
			_ = all(undos)()
			return nil, err
		}
		if err := copyDir(e.StorePath, c.Path); err != nil {
			_ = os.RemoveAll(c.Path)
			_ = linkIn(e.Scope, e.StorePath, c.Path)
			_ = all(undos)()
			return nil, err
		}
		undos = append(undos, func() error {
			if err := os.RemoveAll(c.Path); err != nil {
				return err
			}
			return linkIn(e.Scope, e.StorePath, c.Path)
		})
	}
	to, err := putAway(env, e.StorePath)
	if err != nil {
		_ = all(undos)()
		return nil, err
	}
	undos = append(undos, func() error { return moveDir(to, e.StorePath) })
	return all(undos), nil
}

// LinkedAt is when the first of this computer's links to MyMind's copy
// was made: when the skill was taken in here, or first linked. Zero for
// none.
func (e *Entry) LinkedAt() time.Time {
	var at time.Time
	for _, c := range e.Copies {
		if c.State != Linked {
			continue
		}
		if fi, err := os.Lstat(c.Path); err == nil && (at.IsZero() || fi.ModTime().Before(at)) {
			at = fi.ModTime()
		}
	}
	return at
}

// LastTaken is the skills taken in last, together: those first linked
// within a few minutes of the newest. A batch of 全部收进, found by its
// links once its way back is forgotten.
func (r *Report) LastTaken() (taken []*Entry, at time.Time) {
	for _, e := range r.Entries {
		if t := e.LinkedAt(); e.Stored && !e.ReadOnly() && t.After(at) {
			at = t
		}
	}
	if at.IsZero() {
		return nil, at
	}
	for _, e := range r.Entries {
		if t := e.LinkedAt(); e.Stored && !e.ReadOnly() && !t.IsZero() && at.Sub(t) <= 5*time.Minute {
			taken = append(taken, e)
		}
	}
	return taken, at
}

// Link puts a link to MyMind's copy into tool's folder.
func Link(env Env, e *Entry, t Tool) (Undo, error) {
	if !e.Stored {
		return nil, errors.New("先收进 MyMind")
	}
	if env.Reads(t, e.Scope) {
		return nil, fmt.Errorf("%s 直接读这个目录，不用链接", t.Name)
	}
	var on Undo
	if e.Disabled { // turned on again first
		u, err := Enable(env, e)
		if err != nil {
			return nil, err
		}
		on = u
	}
	at := Where(env, t, e)
	if at == "" {
		return nil, errors.New("这个工具没有项目里的技能目录")
	}
	if _, err := os.Lstat(at); err == nil {
		if tg, ok := linkTarget(at); ok && !exists(tg) {
			_ = os.Remove(at) // a link to nothing: in the way, and of no use
		} else {
			return nil, fmt.Errorf("%s 里已经有一个 %s", t.Name, e.Name)
		}
	}
	if err := linkIn(e.Scope, e.StorePath, at); err != nil {
		if on != nil {
			_ = on()
		}
		return nil, err
	}
	return func() error {
		if err := os.Remove(at); err != nil {
			return err
		}
		if on != nil {
			return on()
		}
		return nil
	}, nil
}

// Enable turns a skill turned off on again: back where the tools read it.
func Enable(env Env, e *Entry) (Undo, error) {
	if !e.Disabled {
		return func() error { return nil }, nil
	}
	dst := storePath(env, e.Scope, e.Repo, e.Name)
	if exists(dst) {
		return nil, fmt.Errorf("已经有一个 %s 在用", e.Name)
	}
	from := e.StorePath
	if err := moveDir(from, dst); err != nil {
		return nil, err
	}
	e.StorePath, e.Disabled = dst, false
	return func() error {
		e.StorePath, e.Disabled = from, true
		return moveDir(dst, from)
	}, nil
}

// Unlink takes MyMind's link out of tool's folder: only a link, never a
// folder of the tool's own.
func Unlink(env Env, e *Entry, tool string) (Undo, error) {
	c := e.Copies[tool]
	if c == nil || c.Target == "" {
		return nil, errors.New("那里不是链接")
	}
	target := c.Target
	if err := os.Remove(c.Path); err != nil {
		return nil, err
	}
	return func() error { return linkIn(e.Scope, target, c.Path) }, nil
}

// Disable turns a skill off: every link of this computer out, and
// MyMind's copy moved where no tool reads it (a tool that reads the
// store itself would go on using it), to be turned on again.
func Disable(env Env, e *Entry) (Undo, error) {
	if e.Disabled {
		return func() error { return nil }, nil
	}
	undo, err := unlinkAll(env, e)
	if err != nil {
		return nil, err
	}
	if !e.Stored {
		return undo, nil
	}
	to := disabledPath(env, e)
	if exists(to) {
		if _, err := putAway(env, to); err != nil {
			_ = undo()
			return nil, err
		}
	}
	from := e.StorePath
	if err := moveDir(from, to); err != nil {
		_ = undo()
		return nil, err
	}
	return func() error {
		if err := moveDir(to, from); err != nil {
			return err
		}
		return undo()
	}, nil
}

// unlinkAll takes every link of this computer out, MyMind's copy kept.
func unlinkAll(env Env, e *Entry) (Undo, error) {
	var undos []Undo
	for tool, c := range e.Copies {
		if c.Target == "" {
			continue
		}
		u, err := Unlink(env, e, tool)
		if err != nil {
			_ = all(undos)()
			return nil, err
		}
		undos = append(undos, u)
	}
	return all(undos), nil
}

// Delete puts the skill away: the links out, the folders (MyMind's, or a
// tool's own) into the store's trash.
func Delete(env Env, e *Entry) (Undo, error) {
	if e.Scope == Plugin {
		return nil, errors.New("插件的技能要停用整个插件")
	}
	undo, err := unlinkAll(env, e)
	if err != nil {
		return nil, err
	}
	undos := []Undo{undo}
	put := func(p string) error {
		to, err := putAway(env, p)
		if err != nil {
			return err
		}
		undos = append(undos, func() error { return moveDir(to, p) })
		return nil
	}
	if e.Stored {
		if err := put(e.StorePath); err != nil {
			_ = all(undos)()
			return nil, err
		}
	}
	for _, c := range e.Copies {
		if c.Target == "" && c.State != Native { // a tool's own folder; MyMind's went above
			if err := put(c.Path); err != nil {
				_ = all(undos)()
				return nil, err
			}
		}
	}
	return all(undos), nil
}

// SetScope moves a skill of MyMind between every session's and a
// project's: its folder in the store, and its links with it.
func SetScope(env Env, e *Entry, scope Scope, repo *Repo, tools []Tool) (Undo, error) {
	if !e.Stored || scope == Plugin || (scope == Project && repo == nil) {
		return nil, errors.New("这个技能不能挪")
	}
	if scope == e.Scope && (scope == Global || e.Repo != nil && e.Repo.Key == repo.Key) {
		return func() error { return nil }, nil
	}
	dst := storePath(env, scope, repo, e.Name)
	if exists(dst) {
		return nil, fmt.Errorf("那里已经有一个 %s", e.Name)
	}
	var linked []Tool
	for _, t := range tools {
		if c := e.Copies[t.ID]; c != nil && c.State == Linked {
			linked = append(linked, t)
		}
	}
	undo, err := unlinkAll(env, e)
	if err != nil {
		return nil, err
	}
	if err := moveDir(e.StorePath, dst); err != nil {
		_ = undo()
		return nil, err
	}
	var made []string
	for _, t := range linked {
		at := where(env, t, scope, repo, e.Name)
		if at == "" || exists(at) || env.Reads(t, scope) {
			continue
		}
		if linkIn(scope, dst, at) == nil {
			made = append(made, at)
		}
	}
	from := e.StorePath
	return func() error {
		for _, at := range made {
			_ = os.Remove(at)
		}
		if err := moveDir(dst, from); err != nil {
			return err
		}
		return undo()
	}, nil
}

// Rename gives a skill of MyMind another name: its folder, its SKILL.md's
// name and its links.
func Rename(env Env, e *Entry, name string, tools []Tool) (Undo, error) {
	if !e.Stored || !skillName.MatchString(name) {
		return nil, errors.New("名字只能用小写字母、数字和连字符")
	}
	dst := filepath.Join(filepath.Dir(e.StorePath), name)
	if exists(dst) {
		return nil, fmt.Errorf("已经有一个 %s", name)
	}
	var linked []Tool
	for _, t := range tools {
		if c := e.Copies[t.ID]; c != nil && c.State == Linked {
			linked = append(linked, t)
		}
	}
	undo, err := unlinkAll(env, e)
	if err != nil {
		return nil, err
	}
	if err := moveDir(e.StorePath, dst); err != nil {
		_ = undo()
		return nil, err
	}
	md := filepath.Join(dst, "SKILL.md")
	old, _ := os.ReadFile(md)
	if len(old) > 0 {
		_ = os.WriteFile(md, []byte(setName(string(old), name)), 0o644)
	}
	var made []string
	for _, t := range linked {
		at := where(env, t, e.Scope, e.Repo, name)
		if at != "" && !exists(at) && !env.Reads(t, e.Scope) && linkIn(e.Scope, dst, at) == nil {
			made = append(made, at)
		}
	}
	from := e.StorePath
	return func() error {
		for _, at := range made {
			_ = os.Remove(at)
		}
		if len(old) > 0 {
			_ = os.WriteFile(md, old, 0o644)
		}
		if err := moveDir(dst, from); err != nil {
			return err
		}
		return undo()
	}, nil
}

var nameLine = regexp.MustCompile(`(?m)^name:.*$`)

// setName is a SKILL.md with its name changed.
func setName(text, name string) string {
	meta, _, ok := Frontmatter(text)
	if !ok {
		return text
	}
	if _, has := meta["name"]; has {
		return nameLine.ReplaceAllLiteralString(text, "name: "+name)
	}
	return strings.Replace(text, "---", "---\nname: "+name, 1)
}

// MakeReal has a project's skill be files in its repository, for the
// repository's other clones: MyMind's copy goes into the first tool's
// folder there, in the link's place, and the other tools' links there
// point to it, from inside the repository. MyMind keeps it no more.
func MakeReal(env Env, e *Entry, tools []Tool) (Undo, error) {
	if !e.Stored || e.Scope != Project || e.Repo == nil {
		return nil, errors.New("只有项目里的技能可以换成真文件")
	}
	if strings.HasPrefix(e.StorePath, e.Repo.Root+string(filepath.Separator)) {
		return nil, errors.New("它已经是仓库里的真文件")
	}
	var first *Copy
	var rest []*Copy
	for _, t := range tools {
		if c := e.Copies[t.ID]; c != nil && c.State == Linked {
			if first == nil {
				first = c
			} else {
				rest = append(rest, c)
			}
		}
	}
	if first == nil {
		return nil, errors.New("仓库里还没有它的链接")
	}
	if err := os.Remove(first.Path); err != nil {
		return nil, err
	}
	if err := copyDir(e.StorePath, first.Path); err != nil {
		_ = os.RemoveAll(first.Path)
		_ = makeLink(e.StorePath, first.Path)
		return nil, err
	}
	for _, c := range rest {
		rel, err := filepath.Rel(filepath.Dir(c.Path), first.Path)
		if err != nil {
			continue
		}
		if os.Remove(c.Path) == nil {
			_ = os.Symlink(rel, c.Path)
		}
	}
	to, err := putAway(env, e.StorePath)
	if err != nil {
		return nil, err
	}
	store := e.StorePath
	return func() error {
		if err := moveDir(to, store); err != nil {
			return err
		}
		for _, c := range append([]*Copy{first}, rest...) {
			_ = os.RemoveAll(c.Path)
			_ = makeLink(store, c.Path)
		}
		return nil
	}, nil
}

// Resolve settles a tool's folder of its own for a skill MyMind has:
// theirs takes it as MyMind's copy (the one before put away); otherwise
// it is put away. Either way a link takes its place.
func Resolve(env Env, e *Entry, tool string, theirs bool) (Undo, error) {
	c := e.Copies[tool]
	if c == nil || !e.Stored || (c.State != Diverged && c.State != Same) {
		return nil, errors.New("这里没有要处理的")
	}
	if theirs {
		was, err := putAway(env, e.StorePath)
		if err != nil {
			return nil, err
		}
		if err := moveDir(c.Path, e.StorePath); err != nil {
			_ = moveDir(was, e.StorePath)
			return nil, err
		}
		if err := linkIn(e.Scope, e.StorePath, c.Path); err != nil {
			_ = moveDir(e.StorePath, c.Path)
			_ = moveDir(was, e.StorePath)
			return nil, err
		}
		return func() error {
			_ = os.Remove(c.Path)
			if err := moveDir(e.StorePath, c.Path); err != nil {
				return err
			}
			return moveDir(was, e.StorePath)
		}, nil
	}
	to, err := putAway(env, c.Path)
	if err != nil {
		return nil, err
	}
	if err := linkIn(e.Scope, e.StorePath, c.Path); err != nil {
		_ = moveDir(to, c.Path)
		return nil, err
	}
	return func() error {
		_ = os.Remove(c.Path)
		return moveDir(to, c.Path)
	}, nil
}

// Forget takes a link to nothing out.
func Forget(e *Entry, tool string) (Undo, error) {
	c := e.Copies[tool]
	if c == nil || c.State != Dangling {
		return nil, errors.New("这个链接是好的")
	}
	if err := os.Remove(c.Path); err != nil {
		return nil, err
	}
	return func() error { return os.Symlink(c.Target, c.Path) }, nil
}

// CopyOwn makes a plugin's skill one of MyMind's, every session's, linked
// to the tool that has the plugin.
func CopyOwn(env Env, e *Entry, t Tool) (Undo, error) {
	c := e.Copies[t.ID]
	if e.Scope != Plugin || c == nil {
		return nil, errors.New("只有插件的技能能复制")
	}
	dst := storePath(env, Global, nil, e.Name)
	at := where(env, t, Global, nil, e.Name)
	if exists(dst) || exists(at) {
		return nil, fmt.Errorf("已经有一个 %s", e.Name)
	}
	if err := copyDir(c.Path, dst); err != nil {
		_ = os.RemoveAll(dst)
		return nil, err
	}
	if !env.Reads(t, Global) {
		if err := makeLink(dst, at); err != nil {
			_ = os.RemoveAll(dst)
			return nil, err
		}
	}
	return func() error {
		_ = os.Remove(at)
		return os.RemoveAll(dst)
	}, nil
}

// Repair mends this computer's links to MyMind's copies: those that
// point where the copies were and are not now (the maps folder moved, or
// the store went from it to ~/.agents/skills) point to where they are;
// and those of a tool that reads the store itself are taken out, which
// would have it read a skill twice. It gives how many it mended.
func Repair(env Env, r *Report) int {
	n := 0
	oldStore := func(t, name string) bool {
		s := filepath.ToSlash(t)
		return filepath.Base(t) == name && (strings.Contains(s, "/.skills/") || strings.Contains(s, "/.mymind-skills/"))
	}
	for _, e := range r.Entries {
		if !e.Stored || e.Disabled {
			continue
		}
		for id, c := range e.Copies {
			if c.State != Dangling || !oldStore(c.Target, e.Name) || os.Remove(c.Path) != nil {
				continue
			}
			n++
			if env.ReadsID(id, e.Scope) {
				continue
			}
			if linkIn(e.Scope, e.StorePath, c.Path) == nil {
				c.State, c.Target = Linked, e.StorePath
			}
		}
		for _, t := range r.Tools {
			if !env.Reads(t, e.Scope) {
				continue
			}
			dirs, root := t.Global, env.Home
			if e.Scope == Project {
				if e.Repo == nil {
					continue
				}
				dirs, root = t.Project, e.Repo.Root
			}
			for _, d := range dirs {
				p := filepath.Join(root, d, e.Name)
				if filepath.Clean(p) == filepath.Clean(e.StorePath) {
					continue
				}
				if tg, ok := linkTarget(p); ok && (samePath(tg, e.StorePath) || oldStore(tg, e.Name)) && os.Remove(p) == nil {
					n++
				}
			}
		}
	}
	return n
}

// Migrate moves the store of before, old (the maps folder's .skills),
// into env's: every session's skills into ~/.agents/skills, a project's
// into its repository's .agents/skills, what computers said and what was
// put away beside them. Where a skill of the name is there already, the
// newer stays and the other is put away. The links are mended by the
// next scan (Repair). It gives how many skills it moved.
func Migrate(env Env, old string) (int, error) {
	if old == "" || !isDir(old) || filepath.Clean(old) == filepath.Clean(env.Store) {
		return 0, nil
	}
	n := 0
	var first error
	place := func(from, to string) {
		if exists(to) {
			if treeSum(from) == treeSum(to) || modOf(to).After(modOf(from)) {
				if _, err := putAway(env, from); err != nil && first == nil {
					first = err
				}
				return
			}
			if _, err := putAway(env, to); err != nil {
				if first == nil {
					first = err
				}
				return
			}
		}
		if err := moveDir(from, to); err != nil {
			if first == nil {
				first = err
			}
			return
		}
		n++
	}
	for _, d := range skillDirs(old) {
		if _, ok := linkTarget(d); ok {
			continue
		}
		place(d, filepath.Join(env.Store, filepath.Base(d)))
	}
	for _, rp := range env.Repos {
		for _, d := range skillDirs(filepath.Join(old, projectsDir, rp.Key)) {
			place(d, filepath.Join(ProjectStore(rp.Root), filepath.Base(d)))
		}
	}
	files, _ := filepath.Glob(filepath.Join(old, machinesDir, "*.json"))
	for _, f := range files {
		to := filepath.Join(env.Store, machinesDir, filepath.Base(f))
		if !exists(to) {
			_ = moveDir(f, to)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(old, trashDir)); err == nil {
		for _, e := range entries {
			_, _ = putAway(env, filepath.Join(old, trashDir, e.Name()))
		}
	}
	removeEmpty(old)
	return n, first
}

// modOf is when a skill's SKILL.md was last changed.
func modOf(dir string) time.Time {
	fi, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// removeEmpty removes dir and the folders in it that hold nothing.
func removeEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			removeEmpty(filepath.Join(dir, e.Name()))
		} else if e.Name() == ".DS_Store" {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	_ = os.Remove(dir) // only when empty
}

// Remember writes what this computer linked into the store, for the
// others; only when it changed, so that the sync has nothing new
// otherwise.
func Remember(env Env, r *Report) error {
	m := machine{Name: env.Machine, Links: map[string][]string{}}
	for _, e := range r.Entries {
		if !e.Stored {
			continue
		}
		var tools []string
		for _, t := range r.Tools {
			if c := e.Copies[t.ID]; c != nil && (c.State == Linked || c.State == Native) {
				tools = append(tools, t.ID)
			}
		}
		if len(tools) > 0 {
			m.Links[e.Key] = tools
		}
	}
	if len(m.Links) == 0 && !isDir(env.Store) {
		return nil
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(env.Store, machinesDir, safeName(env.Machine)+".json")
	if old, err := os.ReadFile(p); err == nil && string(old) == string(data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// InstallHere links a skill another computer has to the same tools here,
// those of them that are here; to the first tool found, if none is.
func InstallHere(env Env, e *Entry, here []Tool) (Undo, error) {
	want := map[string]bool{}
	for _, tools := range e.Others {
		for _, t := range tools {
			want[t] = true
		}
	}
	var undos []Undo
	var linkable []Tool // a tool that reads the store itself has it already
	for _, t := range here {
		if !env.Reads(t, e.Scope) {
			linkable = append(linkable, t)
		}
	}
	here = linkable
	for _, t := range here {
		if !want[t.ID] {
			continue
		}
		u, err := Link(env, e, t)
		if err != nil {
			_ = all(undos)()
			return nil, err
		}
		undos = append(undos, u)
	}
	if len(undos) == 0 && len(here) > 0 {
		u, err := Link(env, e, here[0])
		if err != nil {
			return nil, err
		}
		undos = append(undos, u)
	}
	return all(undos), nil
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9._\-]+`)

func safeName(s string) string {
	s = strings.Trim(unsafe.ReplaceAllString(s, "-"), "-.")
	if s == "" {
		return "computer"
	}
	return s
}

// RepoKey is a repository's folder in the store, by its name, apart from
// those taken.
func RepoKey(name string, taken map[string]bool) string {
	base := strings.ToLower(safeName(name))
	key := base
	for i := 2; taken[key]; i++ {
		key = fmt.Sprintf("%s-%d", base, i)
	}
	return key
}

// putAway moves p into the store's trash, and gives where.
func putAway(env Env, p string) (string, error) {
	dir := filepath.Join(env.Store, trashDir)
	if env.Keep != "" {
		dir = filepath.Join(env.Keep, trashDir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	to := filepath.Join(dir, fmt.Sprintf("%s-%d", filepath.Base(p), time.Now().UnixNano()))
	return to, moveDir(p, to)
}

// moveDir moves a folder; across disks, by copying it.
func moveDir(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := os.Rename(from, to); err == nil {
		return nil
	} else if _, err2 := os.Lstat(to); err2 == nil {
		return err
	}
	if err := copyDir(from, to); err != nil {
		_ = os.RemoveAll(to)
		return err
	}
	return os.RemoveAll(from)
}

// copyDir copies a folder, its files' modes kept.
func copyDir(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(dst, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(t, dst)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// all is the undos, the last first.
func all(undos []Undo) Undo {
	undos = slices.Clone(undos)
	return func() error {
		var first error
		for i := len(undos) - 1; i >= 0; i-- {
			if undos[i] == nil {
				continue
			}
			if err := undos[i](); err != nil && first == nil {
				first = err
			}
		}
		return first
	}
}
