package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
)

// Sync with GitHub: the maps folder, maps and library alike, kept in step
// with a private repository of the user's, so that other computers have
// them too (internal/ghsync). Once set up it runs by itself: soon after a
// change here, and every few minutes for the changes of other computers.

const (
	syncAfterChange = 15 * time.Second // after the last change here
	syncEvery       = 5 * time.Minute  // for other computers' changes
	syncRetry       = 2 * time.Minute  // after a failure
)

func (a *app) syncConfig() ghsync.Config {
	return ghsync.Config{Repo: ghsync.Clean(a.cfg.SyncRepo), Branch: a.cfg.SyncBranch, Token: ghsync.Clean(a.cfg.SyncToken), API: a.cfg.GitHubAPI}
}

// syncReady tells whether a repository and a token are set.
func (a *app) syncReady() bool { return a.cfg.SyncRepo != "" && a.cfg.SyncToken != "" && a.dir != "" }

// syncChange notes a change here, for the next sync to take up.
func (a *app) syncChange() { a.syncChanged = a.now() }

// syncDue starts a sync when one is due; tick calls it.
func (a *app) syncDue() {
	if !a.syncReady() || a.cfg.SyncOff || a.syncBusy {
		return
	}
	now := a.now()
	wait := syncEvery
	if a.syncErr != "" {
		wait = syncRetry
	}
	switch {
	case a.syncTried.IsZero(), // just started
		a.syncChanged.After(a.syncTried) && now.Sub(a.syncChanged) >= syncAfterChange,
		now.Sub(a.syncTried) >= wait:
		a.syncNow()
	}
}

// syncNow syncs, in the background.
func (a *app) syncNow() {
	if !a.syncReady() || a.syncBusy {
		return
	}
	if a.dirty && !a.conflict {
		a.save() // what is being edited goes too
	}
	a.syncBusy, a.syncTried, a.syncMsg = true, a.now(), "正在同步…"
	dir, cfg, run, post := a.dir, a.syncConfig(), a.syncRun, a.post
	a.async(func() (any, error) {
		return run(context.Background(), dir, cfg, func(msg string) { post(func() { a.syncMsg = msg }) })
	}, func(v any, err error) {
		a.syncBusy, a.syncMsg = false, ""
		byHand := a.syncByHand
		a.syncByHand = false
		res, _ := v.(ghsync.Result)
		if err != nil {
			a.syncErr = err.Error() // stays said where the sync is, until it is mended
			return
		}
		a.syncErr, a.syncAt, a.syncLast = "", a.now(), res
		if byHand && len(res.Conflicts) == 0 {
			if res.Synced() {
				a.tell(fmt.Sprintf("已同步：下载 %d 个、上传 %d 个文件", res.Pulled, res.Pushed))
			} else {
				a.tell("已同步，已经是最新的")
			}
		}
		if len(res.Written) > 0 {
			// The maps and the library read what came, as they do
			// any change made outside the app.
			a.refreshFiles()
			a.checkLibrary()
		}
		if n := len(res.Conflicts); n > 0 {
			a.tell(fmt.Sprintf("同步时 %d 个文件在两台电脑上都改了：两份都保留了，另一份名字带「GitHub 冲突」", n))
		}
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
	ui.Text(c, "把导图和收集的资料同步到你的一个私有 GitHub 仓库：在这台电脑上的修改会自动上传，其他电脑的修改会自动下载。两台电脑改了同一张导图时，两份都会保留。超过 50MB 的文件只留在本机。").
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
			switchRow(c, &auto, "自动同步", "修改后十几秒内自动上传，每 5 分钟检查一次其他电脑的修改", func() {
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
