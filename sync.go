package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/versions"
)

// Sync with GitHub: the maps folder, maps and library alike, kept in step
// with a private repository of the user's, so that other computers have
// them too (internal/ghsync). Once set up it runs by itself, every few
// minutes: what changed here since goes up together, and what changed on
// other computers comes down. Before 0.101.3 a change went up fifteen
// seconds after it: GitHub counts the calls of an account, and the user
// asked for one round every five minutes with all of it.

const (
	syncEvery = 5 * time.Minute // a round: the changes here, and other computers'
	syncRetry = 2 * time.Minute // after a failure
)

func (a *app) syncConfig() ghsync.Config {
	cfg := ghsync.Config{Repo: ghsync.Clean(a.cfg.SyncRepo), Branch: a.cfg.SyncBranch, Token: ghsync.Clean(a.cfg.SyncToken), API: a.cfg.GitHubAPI}
	// The AI agents' skills, kept in ~/.agents/skills, which the agents
	// read and so cannot be in the maps folder, go with the maps, at the
	// path they had when they were in it.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cfg.Mounts = map[string]string{".skills": skillsStore(home)}
	}
	cfg.State = syncStatePath(a.dir)
	cfg.Same = sameMap
	return cfg
}

// syncStatePath is where the sync keeps what it remembers of the maps
// folder dir: in the app's own folder, by a hash of the folder's path,
// as the maps' versions are. It was in the folder itself
// (.sync/github.json), and went wherever the folder went: a backup put
// back, or a folder a cloud keeps on two computers, brought one
// computer's memory of its last sync to another state of the files, and
// the next sync pulled over them. "" when the app has no folder of its
// own, and the state stays where it was.
func syncStatePath(dir string) string {
	p := configPath()
	if p == "" || dir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "sync", folderKey(dir)+".json")
}

// folderKey names a folder by a hash of its path.
func folderKey(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	h := sha1.Sum([]byte(filepath.Clean(dir)))
	return hex.EncodeToString(h[:8])
}

// syncAdoptState moves the state a sync before 0.105.2 kept in the maps
// folder to where it is kept now, the once: the first time this version
// runs, with a sync set up, for the folder it has then. So no one syncs
// anew from nothing; and a state found in a folder later is not this
// computer's memory of it, and is not taken.
func (a *app) syncAdoptState() {
	if a.cfg.SyncStateOut || a.dir == "" {
		return
	}
	state := syncStatePath(a.dir)
	if state == "" {
		return
	}
	if a.syncReady() {
		ghsync.Adopt(a.dir, state)
	}
	a.cfg.SyncStateOut = true
	a.cfg.save()
}

// sameMap tells whether two files of two names are one map, renamed: by
// the IDs of its first canvas and central topic (ghsync.Config.Same).
func sameMap(from, to string, old, renamed []byte) bool {
	if !strings.HasSuffix(from, model.Ext) || !strings.HasSuffix(to, model.Ext) {
		return false
	}
	a, err := model.Parse(old)
	if err != nil {
		return false
	}
	b, err := model.Parse(renamed)
	if err != nil {
		return false
	}
	return a.Sheets[0].ID == b.Sheets[0].ID && a.Sheets[0].Root.ID == b.Sheets[0].Root.ID
}

// followSynced follows the maps the sync merged into another name (a map
// renamed on one computer and changed on the other): what this computer
// keeps by their paths, the open map among it, and the pictures either
// side added under the old name meanwhile, which go to the new one's
// folder. The files themselves the sync has moved.
func (a *app) followSynced(dir string, renamed map[string]string) {
	for from, to := range renamed {
		from, to := filepath.Join(dir, filepath.FromSlash(from)), filepath.Join(dir, filepath.FromSlash(to))
		a.versions.Join(from, to)
		if entries, err := os.ReadDir(model.AssetsDir(from)); err == nil {
			_ = os.MkdirAll(model.AssetsDir(to), 0o755)
			for _, e := range entries {
				dst := filepath.Join(model.AssetsDir(to), e.Name())
				if _, err := os.Stat(dst); err != nil {
					_ = os.Rename(filepath.Join(model.AssetsDir(from), e.Name()), dst)
				}
			}
			_ = os.Remove(model.AssetsDir(from)) // when nothing is left in it
		}
		was := a.modTime
		a.followed(func(p string) string {
			if p == from {
				return to
			}
			return p
		})
		if a.path == to {
			// The open map is of the file as it was before the merge: read
			// again, or merged with what was changed here since, as a file
			// changed outside is (tick).
			a.modTime = was
		}
	}
}

// openMapGone takes the open map whose file the sync removed, with no
// new name to follow (followSynced): deleted on the other computer. Not
// changed here, it is closed, as a map deleted here is, and a version of
// it is kept; changed here, it is saved under its name, and the next sync
// sends it back (deleted there, changed here: kept).
func (a *app) openMapGone(dir string, written []string) {
	if a.path == "" || a.file == nil {
		return
	}
	rel, err := filepath.Rel(dir, a.path)
	if err != nil || !slices.Contains(written, filepath.ToSlash(rel)) {
		return
	}
	if _, err := os.Lstat(a.path); !errors.Is(err, fs.ErrNotExist) {
		return
	}
	name := mapName(a.path)
	if a.dirty || a.conflict || a.editing != "" {
		a.commitEditIfAny()
		a.conflict = false
		a.save() // the file gone, nothing to merge with: written whole
		a.tell(fmt.Sprintf("「%s」在另一台电脑上删了，这边改过，已留下", name))
		return
	}
	a.keepVersion(versions.BeforeExternal) // to find it by, as it was here
	a.path, a.file, a.doc = "", nil, nil
	a.refreshFiles()
	if len(a.files) > 0 {
		_ = a.open(a.files[0].Path)
	}
	a.tell(fmt.Sprintf("「%s」在另一台电脑上删了", name))
}

// mergeMapVersions merges a map changed on two computers, topic by topic
// (model.Merge), for the sync: two files of one map were two to compare
// by eye, which no one can. What both changed is this computer's, and
// its clashes are asked after (mergeAsk), as git asks.
func mergeMapVersions(p string, base, local, remote []byte) ([]byte, []model.Clash, bool) {
	if !strings.HasSuffix(p, model.Ext) {
		return nil, nil, false
	}
	var b *model.File
	if base != nil {
		var err error
		if b, err = model.Parse(base); err != nil {
			return nil, nil, false
		}
	}
	l, err := model.Parse(local)
	if err != nil {
		return nil, nil, false
	}
	r, err := model.Parse(remote)
	if err != nil {
		return nil, nil, false
	}
	m, clashes := model.Merge(b, l, r)
	data, err := m.Marshal()
	if err != nil {
		return nil, nil, false
	}
	if _, err := model.Parse(data); err != nil {
		return nil, nil, false
	}
	return data, clashes, true
}

// mergeConflictCopies merges into their maps the copies syncs before
// 0.100 kept beside a map changed on two computers, "AI (GitHub 冲突 …)":
// with no version both came from, what is in either is kept, and what
// differs is asked (mergeAsk). The map's version before is kept, the copy
// goes to the trash, and the toast takes it back.
func (a *app) mergeConflictCopies() {
	type undo struct {
		path, copy  string
		was, copied []byte
	}
	var done []undo
	for _, fi := range a.files {
		orig, ok := ghsync.CopyOf(fi.Path)
		if !ok || a.conflict && orig == a.path {
			continue
		}
		if _, err := os.Stat(orig); err != nil {
			continue // the map is gone: the copy is a map of its own
		}
		if orig == a.path && a.dirty {
			a.save()
		}
		was, err1 := os.ReadFile(orig)
		copied, err2 := os.ReadFile(fi.Path)
		if err1 != nil || err2 != nil {
			continue
		}
		// When each was: this computer's last change, and the conflict's,
		// which the copy's name says.
		var when ghsync.MergeInfo
		if info, err := os.Stat(orig); err == nil {
			when.LocalAt = info.ModTime()
		}
		when.RemoteAt = copyTime(fi.Path)
		merged, clashes, ok := mergeMapVersions(orig, nil, was, copied)
		if !ok {
			continue
		}
		if f, err := model.Parse(was); err == nil {
			_, _ = a.versions.Save(orig, f, versions.BeforeMerge, "", a.now())
		}
		if err := writeFile(orig, merged); err != nil {
			continue
		}
		if err := a.plat.Trash(fi.Path); err != nil {
			_ = os.Remove(fi.Path)
		}
		a.removeMapMeta(fi.Path)
		done = append(done, undo{orig, fi.Path, was, copied})
		a.askMerge(orig, clashes, when)
	}
	if len(done) == 0 {
		return
	}
	a.refreshFiles()
	opened := slices.ContainsFunc(done, func(u undo) bool { return u.path == a.path })
	if opened {
		a.reload()
	}
	msg := fmt.Sprintf("「%s」在两台电脑上改过的两份，已合成一张", mapName(done[0].path))
	if len(done) > 1 {
		msg = fmt.Sprintf("%d 张导图在两台电脑上改过的两份，已各合成一张", len(done))
	}
	a.tellUndo(msg, func() {
		for _, u := range done {
			_ = writeFile(u.path, u.was)
			_ = writeFile(u.copy, u.copied)
		}
		a.refreshFiles()
		if opened {
			a.reload()
		}
		a.merges = slices.DeleteFunc(a.merges, func(m *mergeAsk) bool {
			return slices.ContainsFunc(done, func(u undo) bool { return u.path == m.path })
		})
		a.saveMerges()
	})
}

// copyTime is when a sync kept a conflict copy, as its name says.
func copyTime(p string) time.Time {
	base := filepath.Base(p)
	i := strings.LastIndex(base, "(GitHub 冲突 ")
	if i < 0 {
		return time.Time{}
	}
	rest := base[i+len("(GitHub 冲突 "):]
	if len(rest) < len("2006-01-02 1504") {
		return time.Time{}
	}
	at, err := time.ParseInLocation("2006-01-02 1504", rest[:len("2006-01-02 1504")], time.Local)
	if err != nil {
		return time.Time{}
	}
	return at
}

// writeFile writes data to path through a file renamed over it.
func writeFile(path string, data []byte) error {
	tmp := path + ".merge.tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// syncReady tells whether a repository and a token are set.
func (a *app) syncReady() bool { return a.cfg.SyncRepo != "" && a.cfg.SyncToken != "" && a.dir != "" }

// syncDue starts a sync when one is due; tick calls it.
func (a *app) syncDue() {
	a.syncAdoptState()
	if a.mergesOf != a.dir { // what waited to be asked when the app closed
		a.mergesOf = a.dir
		a.loadMerges()
	}
	if !a.syncReady() || a.cfg.SyncOff || a.syncBusy {
		return
	}
	now := a.now()
	if now.Before(a.syncHold) {
		return
	}
	wait := syncEvery
	if a.syncErr != "" {
		wait = syncRetry
	}
	if a.syncTried.IsZero() || now.Sub(a.syncTried) >= wait { // just started, or a round is due
		a.syncNow()
	}
}

// syncNow syncs, in the background.
func (a *app) syncNow() {
	if !a.syncReady() || a.syncBusy {
		return
	}
	a.syncAdoptState()
	// What is being edited goes too; but not a save that would merge a
	// file changed outside under a title being typed (tick's rule).
	if a.dirty && !a.conflict && (a.editing == "" || modTime(a.path).Equal(a.modTime)) {
		a.save()
	}
	a.keepTyped() // and what was typed in a clip's details
	a.syncBusy, a.syncTried, a.syncMsg = true, a.now(), "正在同步…"
	dir, cfg, run, post := a.dir, a.syncConfig(), a.syncRun, a.post
	kept, now := a.versions, a.now()
	var mu sync.Mutex
	type found struct {
		clashes []model.Clash
		when    ghsync.MergeInfo
	}
	clashes := map[string]found{}
	cfg.Merge = func(p string, base, local, remote []byte, when ghsync.MergeInfo) ([]byte, bool) {
		data, cl, ok := mergeMapVersions(p, base, local, remote)
		if !ok {
			return nil, false
		}
		full := filepath.Join(dir, filepath.FromSlash(p))
		// The map as it was here is kept, to go back to: a map not open,
		// merged, had no version of before, and a topic the other computer
		// deleted, left alone here, was gone with nothing to find it by.
		if f, err := model.Parse(local); err == nil {
			_, _ = kept.Save(full, f, versions.BeforeMerge, "", now)
		}
		if len(cl) > 0 {
			mu.Lock()
			clashes[full] = found{cl, when}
			mu.Unlock()
		}
		return data, true
	}
	a.async(func() (any, error) {
		return run(context.Background(), dir, cfg, func(msg string) { post(func() { a.syncMsg = msg }) })
	}, func(v any, err error) {
		a.syncBusy, a.syncMsg = false, ""
		byHand := a.syncByHand
		a.syncByHand = false
		res, _ := v.(ghsync.Result)
		if err != nil {
			a.syncErr = err.Error() // stays said where the sync is, until it is mended
			// GitHub's calls used up: not tried again till they are
			// counted anew, each try a call refused.
			a.syncHold = ghsync.RetryAfter(err)
		} else {
			a.syncErr, a.syncAt, a.syncLast, a.syncHold = "", a.now(), res, time.Time{}
			if byHand && len(res.Conflicts) == 0 {
				if res.Synced() {
					a.tell(fmt.Sprintf("已同步：下载 %d 个、上传 %d 个文件", res.Pulled, res.Pushed))
				} else {
					a.tell("已同步，已经是最新的")
				}
			}
		}
		// What follows is of what the sync did, whether or not it ended
		// well: a sync that failed after files came left them unread by
		// the app, the maps' list and the library as they were before.
		a.followSynced(dir, res.Renamed)
		a.openMapGone(dir, res.Written)
		if len(res.Written) > 0 {
			// The maps and the library read what came, as they do
			// any change made outside the app.
			a.refreshFiles()
			a.checkLibrary()
			// A password set or changed on the other computer, and clips
			// it kept before it had one.
			a.vault.Reload()
			if a.vaultSweep() > 0 {
				a.noteLibraryWrite()
			}
		}
		mu.Lock()
		for _, p := range res.Merged {
			full := filepath.Join(dir, filepath.FromSlash(p))
			a.askMerge(full, clashes[full].clashes, clashes[full].when)
		}
		mu.Unlock()
		switch n := len(res.Merged); {
		case n == 1:
			a.tell(fmt.Sprintf("「%s」在两台电脑上都改了，已按主题合并", mapName(res.Merged[0])))
		case n > 1:
			a.tell(fmt.Sprintf("%d 张导图在两台电脑上都改了，已按主题合并", n))
		}
		if n := len(res.Conflicts); n > 0 {
			a.tell(fmt.Sprintf("同步时 %d 个文件在两台电脑上都改了：两份都保留了，另一份名字带「GitHub 冲突」", n))
		}
		a.mergeConflictCopies()
	})
}

// syncLabel says how the sync stands, in a few words.
func (a *app) syncLabel() string {
	switch {
	case a.syncBusy:
		return "同步中…"
	case a.syncErr != "":
		return "同步失败"
	case !a.syncAt.IsZero():
		return "已同步 " + a.syncAt.Format("15:04")
	}
	return "未同步"
}

// openSyncSettings opens the settings at the sync's tab.
func (a *app) openSyncSettings() {
	a.openSettings()
	a.settingsTab = 3
	a.settingsHome = false
}

// syncFoot is the sidebar's line on the sync, which opens its settings.
func (a *app) syncFoot(c *ui.Context) {
	if !a.syncReady() {
		return
	}
	t := c.Theme()
	col := t.TextMuted
	if a.syncErr != "" {
		col = t.Danger
	}
	tip := a.syncErr
	if tip == "" {
		tip = "导图和收集的资料同步到 GitHub：" + a.cfg.SyncRepo
	}
	b := ui.Row(c.Key("sync-foot")).Padding(5, 8).Gap(5).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Label("同步状态").Tooltip(tip)
	if b.Hovered() {
		b.Background(t.Text.Alpha(0.06))
	}
	if b.Clicked() {
		a.later(a.openSyncSettings)
	}
	b.Children(func() {
		if a.syncBusy {
			ui.Spinner(c).Size(12, 12)
		} else {
			glyph(c, iconCloud, 14, col)
		}
		ui.Text(c, a.syncLabel()).FontSize(textCaption).TextColor(col).NoWrap().PassThrough()
	})
}

// syncSettings sets up the sync: the repository, the token, and whether
// it runs by itself.
func (a *app) syncSettings(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "把导图和收集的资料同步到你的一个私有 GitHub 仓库：在这台电脑上的修改会自动上传，其他电脑的修改会自动下载。两台电脑改了同一张导图时，会按主题自动合并。超过 50MB 的文件只留在本机。").
		FontSize(12).TextColor(t.TextMuted).Wrap()
	saveDrafts := func() {
		repo := strings.Trim(ghsync.Clean(a.syncDrafts[0]), "/")
		repo = strings.TrimSuffix(strings.TrimPrefix(repo, "https://github.com/"), ".git")
		token := ghsync.Clean(a.syncDrafts[1])
		if repo == a.cfg.SyncRepo && token == a.cfg.SyncToken {
			return
		}
		a.cfg.SyncRepo, a.cfg.SyncToken = repo, token
		a.syncDrafts[0] = repo
		a.cfg.save()
		a.syncErr, a.syncCheck, a.syncTried = "", "", time.Time{}
	}
	rowsField(c, "GitHub", "令牌用 Fine-grained 的：只选这个仓库，Contents 权限设为 Read and write。", func() {
		setRowSays(c, "仓库", "先在 GitHub 新建一个空的私有仓库", syncRepoWrong(a.syncDrafts[0]), func() {
			if ui.Link(c, "新建仓库", "").FontSize(textSmall).Clicked() {
				a.plat.OpenURL("https://github.com/new")
			}
			in := ui.TextInput(c.Key("sync-repo"), &a.syncDrafts[0]).Placeholder("用户名/仓库名").Label("同步仓库").Width(240)
			if in.Submitted() || in.PressedOutside() {
				saveDrafts()
			}
		})
		setLine(c)
		setRowSays(c, "授权令牌", "只存在这台电脑上", syncTokenWrong(a.syncDrafts[1]), func() {
			if ui.Link(c, "创建令牌", "").FontSize(textSmall).Clicked() {
				a.plat.OpenURL("https://github.com/settings/personal-access-tokens/new")
			}
			in := ui.TextInput(c.Key("sync-token"), &a.syncDrafts[1]).Password().Placeholder("github_pat_…").Label("同步令牌").Width(240)
			if in.Submitted() || in.PressedOutside() {
				saveDrafts()
			}
		})
	})
	rowsField(c, "同步", "", func() {
		ui.Column(c).Padding(12, 0, 10).Gap(8).FillWidth().Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				// Each shows its wait on itself, its words and width as they
				// were; what comes of it stays said under them.
				can := a.syncDrafts[0] != "" && a.syncDrafts[1] != ""
				if busyClicked(busyKey(c, "检查连接", "看看仓库和令牌对不对", a.syncChecking, can, false), a.syncChecking, can) {
					saveDrafts()
					a.syncChecking, a.syncCheck = true, ""
					cfg, check := a.syncConfig(), a.syncCheckRepo
					a.async(func() (any, error) { return check(context.Background(), cfg) }, func(v any, err error) {
						a.syncChecking = false
						r, _ := v.(*ghsync.Repo)
						switch {
						case err != nil:
							a.syncCheck = "✕ " + err.Error()
						case !r.Private:
							a.syncCheck = "⚠ 已连接，但这是公开仓库：任何人都能看到你的导图和资料，建议在 GitHub 里改成私有"
						default:
							a.syncCheck = "✓ 已连接私有仓库 " + r.FullName
						}
					})
				}
				if busyClicked(busyKey(c, "立即同步", "现在就和 GitHub 上的对一遍", a.syncBusy, a.syncReady(), true), a.syncBusy, a.syncReady()) {
					saveDrafts()
					a.syncByHand = true // its result is said, as it was asked for
					a.syncNow()
				}
			})
			if a.syncCheck != "" {
				col := t.TextMuted
				switch {
				case strings.HasPrefix(a.syncCheck, "✓"):
					col = t.Success
				case strings.HasPrefix(a.syncCheck, "⚠"):
					col = t.Warning
				case strings.HasPrefix(a.syncCheck, "✕"):
					col = t.Danger
				}
				ui.Text(c, a.syncCheck).FontSize(12).TextColor(col).Wrap()
			}
			auto := !a.cfg.SyncOff
			switchRow(c, &auto, "自动同步", "每 5 分钟一次：这段时间里的修改一起上传，其他电脑的修改一起下载。想马上同步就点「立即同步」", func() {
				a.cfg.SyncOff = !auto
				a.cfg.save()
			})
			// How the last sync went.
			ui.Column(c).Gap(4).Children(func() {
				switch {
				case a.syncBusy:
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c)
						ui.Text(c, a.syncMsg).FontSize(12).TextColor(t.TextMuted)
					})
				case a.syncErr != "":
					ui.Text(c, "同步失败："+a.syncErr).FontSize(12).TextColor(t.Danger).Wrap()
				case !a.syncAt.IsZero():
					r := a.syncLast
					msg := "上次同步 " + a.syncAt.Format("15:04")
					if r.Synced() {
						msg += fmt.Sprintf("：下载 %d 个、上传 %d 个、删除 %d 个文件", r.Pulled, r.Pushed, r.Deleted)
					} else {
						msg += "：已是最新"
					}
					ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted)
				}
				if n := len(a.syncLast.Skipped); n > 0 {
					ui.Text(c, fmt.Sprintf("%d 个文件超过 50MB，没有同步：%s", n, strings.Join(a.syncLast.Skipped, "、"))).
						FontSize(12).TextColor(t.Warning).Wrap()
				}
				if n := len(a.syncLast.Unwritten); n > 0 {
					ui.Text(c, fmt.Sprintf("%d 个文件在这台电脑上写不了（文件名这个系统不接受，或文件夹不让写），先留在 GitHub 上，其余的照常同步：%s", n, strings.Join(a.syncLast.Unwritten, "、"))).
						FontSize(12).TextColor(t.Warning).Wrap()
				}
			})
		})
	})
}

// syncRepoWrong says what is wrong with a repository's name as it is
// written, or nothing: it is told at the field, not after a failed sync.
func syncRepoWrong(repo string) string {
	repo = strings.TrimSpace(repo)
	switch {
	case repo == "":
		return ""
	case strings.Contains(repo, "github.com"):
		return "只写「用户名/仓库名」，不用整个网址"
	case !syncRepoForm.MatchString(repo):
		return "格式是「用户名/仓库名」，中间一个斜杠"
	}
	return ""
}

var syncRepoForm = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)

// syncTokenWrong says what is wrong with a token as it is written.
func syncTokenWrong(token string) string {
	token = strings.TrimSpace(token)
	switch {
	case token == "":
		return ""
	case strings.ContainsAny(token, " \t\n"):
		return "令牌里不该有空格，可能多复制了内容"
	case !strings.HasPrefix(token, "github_pat_") && !strings.HasPrefix(token, "ghp_"):
		return "GitHub 的令牌以 github_pat_ 或 ghp_ 开头"
	}
	return ""
}
