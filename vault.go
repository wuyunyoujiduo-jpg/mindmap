package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
)

// The secrets of 收集 sealed (internal/vault): the user asked for a
// password manager after 1Password's, and chose this of it (0.101): what
// looks like a key or a password is kept and synced sealed by a second
// password, and stays where it was, in 收集, found under 含密钥. Not a
// fifth place, and no keychain of one system's: the other computer is to
// read it too.
//
// Nothing is asked as a thing is kept, the public key sealing it; the
// password is asked to see a secret, copy it or change it, once for a
// while (vaultLockAfter). It is changed with the password now or the
// recovery code, and the code is new with it, as the user asked. What
// rules cannot tell from a word is sealed whole from its menu (整条加密,
// Clip.Locked). Turned off, all is opened and kept as written again.
//
// The clips in memory are as their files: tokens where secrets were. The
// details open them for the clip shown (fieldText), and hold what is
// being written as it was typed (drafts), for a secret is sealed, and so
// changed, under the hand that writes it.

// What the vault's window asks.
const (
	askUnlock = iota // the password, to open
	askSetup         // a new password, twice
	askChange        // the password now or the code, and a new one twice
	askCode          // the recovery code, shown once
	askOff           // whether to turn it off
)

type vaultAsk struct {
	open   bool
	mode   int
	old    string // the password now, or the recovery code
	byCode bool   // old is the recovery code
	pass   string
	again  string
	wrong  string
	busy   bool
	code   string // the new recovery code
	saved  bool   // it was written down, the user says
	fresh  bool   // the code is of a vault just set up
	sealed int    // how many clips were sealed as it was
	then   func() // what was asked for, done once unlocked
	back   bool   // the settings were open, and show again after
}

// A draft is a part of a clip as it was typed, while what is kept of it
// is sealed.
type draft struct{ plain, sealed string }

// vaultLocks are the times after which the secrets are locked again.
var vaultLocks = []struct {
	name string
	min  int
}{{"5 分钟", 0}, {"15 分钟", 15}, {"1 小时", 60}, {"不锁", -1}}

func (a *app) vaultLockAfter() time.Duration {
	switch m := a.cfg.VaultLock; {
	case m < 0:
		return 0
	case m == 0:
		return 5 * time.Minute
	default:
		return time.Duration(m) * time.Minute
	}
}

// clipboardClear is how long a secret copied stays on the clipboard.
const clipboardClear = time.Minute

// vaultSpare is where the keys' files of a maps folder are kept a second
// time on this computer.
func vaultSpare(dir string) string {
	p := configPath()
	if p == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(filepath.Dir(p), "vault", hex.EncodeToString(sum[:6]))
}

// keepVault gives a library read again the vault the app has, which is
// unlocked or not as it was, and seals what came unsealed.
func (a *app) keepVault(l *library.Library) {
	if l.Vault == nil {
		l.Vault = vault.Open(filepath.Join(l.Dir, "vault"))
	}
	if a.vault != nil && a.vault.Dir == l.Vault.Dir {
		a.vault.Reload()
		l.Vault = a.vault
	} else {
		a.lockVault()
		a.vault = l.Vault
		if spare := vaultSpare(a.dir); spare != "" {
			a.vault.Spare(spare)
		}
	}
	a.vaultSweep()
}

// vaultSweep seals what the clips hold unsealed while the vault is on,
// clips of a computer that has no password yet or of an agent, and opens
// what is left sealed once it is off and unlocked. It returns how many
// clips it changed.
func (a *app) vaultSweep() int {
	if a.lib == nil || a.vault == nil {
		return 0
	}
	var n int
	var err error
	switch {
	case a.vault.On():
		n, err = a.lib.SealAll()
	case a.vault.Known() && a.vault.Unlocked():
		n, _, err = a.lib.UnsealAll()
	}
	if err != nil {
		a.libErr = err.Error()
	}
	return n
}

// lockVault locks the secrets again and hides those shown.
func (a *app) lockVault() {
	a.vault.Lock()
	a.vaultUntil = time.Time{}
	a.hideSecrets()
}

func (a *app) hideSecrets() { a.secretShown, a.drafts = "", nil }

// vaultUsed puts off the time the secrets lock by themselves.
func (a *app) vaultUsed() {
	a.vaultUntil = time.Time{}
	if d := a.vaultLockAfter(); d > 0 {
		a.vaultUntil = a.now().Add(d)
	}
}

// withVault does then with the secrets unlocked, asking for the password
// first when they are not.
func (a *app) withVault(then func()) {
	if a.vault.Unlocked() {
		a.vaultUsed()
		then()
		return
	}
	a.askVault(askUnlock, then)
}

// askVault opens the vault's window.
func (a *app) askVault(mode int, then func()) {
	back := a.settingsOpen || a.vaultAsk.open && a.vaultAsk.back
	a.vaultAsk = vaultAsk{open: true, mode: mode, then: then, back: back}
	a.settingsOpen = false // one window at a time
	if mode == askSetup && a.syncReady() {
		// The other computer may have set one: heard of before a second is
		// made here, which would seal by a key of its own.
		a.syncNow()
	}
}

// closeVaultAsk closes the window, forgetting what was written in it.
func (a *app) closeVaultAsk() {
	back := a.vaultAsk.back
	a.vaultAsk = vaultAsk{}
	if back {
		a.settingsOpen = true
	}
}

// vaultTick locks the secrets when their time is up, clears the
// clipboard of one copied, and hides those shown of a clip left.
func (a *app) vaultTick(c *ui.Context, now time.Time) {
	if a.vault == nil {
		return
	}
	if a.vault.Unlocked() && !a.vaultUntil.IsZero() {
		if wait := a.vaultUntil.Sub(now); wait <= 0 {
			a.lockVault()
		} else {
			c.After(wait)
		}
	}
	if a.copied != "" {
		if wait := a.copiedUntil.Sub(now); wait <= 0 {
			if a.plat.ClipboardText() == a.copied {
				a.plat.CopyText("")
			}
			a.copied = ""
		} else {
			c.After(wait)
		}
	}
	if a.secretShown != "" && a.secretShown != a.libSel {
		a.hideSecrets()
	}
	if a.marking.id != a.libSel {
		a.marking = mark{}
	}
	for key := range a.drafts {
		if !strings.HasPrefix(key, a.libSel+"\x00") || a.libSel == "" {
			a.hideSecrets()
			break
		}
	}
}

// typing tells whether a part of the clip shows as it was typed.
func (a *app) typing(clip *library.Clip) bool {
	for key := range a.drafts {
		if strings.HasPrefix(key, clip.ID+"\x00") {
			return true
		}
	}
	return false
}

// fieldText is a part of a clip as its details show it, and whether it
// may be written in: as it is being typed; as it is, with no secret in
// it; opened, when the clip's secrets were asked for; else in dots, to
// be read alone.
func (a *app) fieldText(clip *library.Clip, field, stored string) (text string, write bool) {
	if d, ok := a.drafts[clip.ID+"\x00"+field]; ok && d.sealed == stored {
		return d.plain, true
	}
	if !library.HasSecret(stored) {
		return stored, true
	}
	if a.secretShown == clip.ID {
		if p, ok := a.vault.Reveal(stored); ok {
			return p, true
		}
	}
	return library.Mask(stored), false
}

// fieldTyped notes what was typed in a part of a clip, once kept: a
// secret in it is sealed by then, and the field goes on showing what was
// typed.
func (a *app) fieldTyped(clip *library.Clip, field, typed, stored string) {
	key := clip.ID + "\x00" + field
	if typed == stored && !library.HasSecret(typed) {
		delete(a.drafts, key)
		return
	}
	if a.drafts == nil {
		a.drafts = map[string]draft{}
	}
	a.drafts[key] = draft{typed, stored}
	if a.vault.Unlocked() {
		a.vaultUsed()
	}
}

// showSecrets shows the secrets of a clip in its details, the password
// asked first when they are sealed.
func (a *app) showSecrets(clip *library.Clip) {
	show := func() {
		probe := *clip
		if !a.lib.Unseal(&probe) {
			a.tell("这条是用另一个二次密码加密的，用那个密码解锁")
			return
		}
		a.secretShown = clip.ID
	}
	if clip.Sealed() {
		a.withVault(show)
	} else {
		show()
	}
}

// theSecret is the one secret of a clip, when it has one alone: what its
// key to copy copies.
func theSecret(clip *library.Clip) (string, bool) {
	all := library.Secrets(clip.Title + "\n" + clip.Text + "\n" + clip.Note)
	if len(all) != 1 {
		return "", false
	}
	return all[0], true
}

// copySecret copies the one secret of a clip, without showing it.
func (a *app) copySecret(clip *library.Clip) {
	s, ok := theSecret(clip)
	if !ok {
		return
	}
	a.copyOpened(s, vault.Has(s))
}

// copyClip copies a clip's words, what is sealed in them opened.
func (a *app) copyClip(clip *library.Clip) {
	a.copyOpened(clipCopyText(clip), clip.Sealed())
}

func (a *app) copyOpened(text string, sealed bool) {
	if !sealed {
		text, _ = library.Marks(text) // the marks are the file's
		a.plat.CopyText(text)
		a.tell("已复制")
		return
	}
	a.withVault(func() {
		p, ok := a.vault.Reveal(text)
		if !ok {
			a.tell("这条是用另一个二次密码加密的，用那个密码解锁")
			return
		}
		p, _ = library.Marks(p) // the marks are the file's
		a.plat.CopyText(p)
		if a.cfg.VaultKeepClipboard {
			a.tell("已复制")
			return
		}
		a.copied, a.copiedUntil = p, a.now().Add(clipboardClear)
		a.tell("已复制，60 秒后从剪贴板清掉")
	})
}

// sealedOut is a clip's words as they go elsewhere, to a map's topic or a
// task: what is sealed in them in dots. The secret stays in the clip.
// (Not all that the rules take for a secret: the words go there for
// good, and the rules take some plain words for one, docs/审查清单.md.)
func sealedOut(s string) string {
	if vault.Has(s) || library.Marked(s) {
		return library.Mask(s)
	}
	return s
}

// lockWhole seals the whole of a clip's text and note, or back.
func (a *app) lockWhole(clip *library.Clip, on bool) {
	if on {
		clip.Locked = true
		a.saveClip(clip)
		a.hideSecrets()
		a.tellUndo("已整条加密：内容和备注都加密了，标题留着好找", func() { a.lockWhole(clip, false) })
		return
	}
	a.withVault(func() {
		if !a.lib.Unseal(clip) {
			a.tell("这条是用另一个二次密码加密的，用那个密码解锁")
			return
		}
		clip.Locked = false
		a.saveClip(clip) // what looks like a secret in it is sealed again
		a.tell("已取消整条加密，只加密其中的密钥")
	})
}

// sealedCount is how many clips hold a secret sealed.
func (a *app) sealedCount() int {
	n := 0
	for _, c := range a.lib.Clips {
		if c.Sealed() {
			n++
		}
	}
	return n
}

// vaultOff turns the vault off: what is sealed is opened and kept as
// written.
func (a *app) vaultOff() {
	if err := a.vault.TurnOff(a.now()); err != nil {
		a.tell("没能关闭：" + err.Error())
		return
	}
	n, left, err := a.lib.UnsealAll()
	a.noteLibraryWrite()
	a.hideSecrets()
	switch {
	case err != nil:
		a.tell("已关闭加密，有的没能解开：" + err.Error())
	case left > 0:
		a.tell(fmt.Sprintf("已关闭加密，解开了 %d 条；还有 %d 条是另一个密码加密的", n, left))
	default:
		a.tell(fmt.Sprintf("已关闭加密，%d 条已解开，照原样保存", n))
	}
}

// vaultSubmit does what the vault's window asks for with what is written
// in it. The password's work takes a moment, and is done off the window.
func (a *app) vaultSubmit() {
	k := &a.vaultAsk
	if k.busy {
		return
	}
	v, now := a.vault, a.now()
	fresh := func() bool { // a new password, as it is to be
		switch {
		case len([]rune(k.pass)) < vault.MinPassword:
			k.wrong = "密码至少 8 位"
		case k.pass != k.again:
			k.wrong = "两次输入的不一样"
		default:
			return true
		}
		return false
	}
	run := func(work func() (string, error), done func(code string)) {
		k.busy, k.wrong = true, ""
		a.async(func() (any, error) { return work() }, func(res any, err error) {
			k.busy = false
			if err != nil {
				k.wrong = err.Error()
				return
			}
			code, _ := res.(string)
			done(code)
		})
	}
	switch k.mode {
	case askUnlock:
		pass := k.old
		run(func() (string, error) { return "", v.Unlock(pass) }, func(string) {
			then := k.then
			a.closeVaultAsk()
			a.vaultUsed()
			a.vaultSweep()
			if then != nil {
				then()
			}
		})
	case askSetup:
		if !fresh() {
			return
		}
		pass := k.pass
		run(func() (string, error) { return v.Setup(pass, now) }, func(code string) {
			a.vaultUsed()
			k.sealed = a.vaultSweep()
			a.noteLibraryWrite()
			k.mode, k.code, k.fresh = askCode, code, true
			k.old, k.pass, k.again = "", "", ""
		})
	case askChange:
		if !fresh() {
			return
		}
		old, pass, byCode := k.old, k.pass, k.byCode
		run(func() (string, error) {
			// The password now is asked though the secrets are unlocked:
			// the window left open is not to let another change it.
			var err error
			if byCode {
				err = v.UnlockCode(old)
			} else {
				err = v.Unlock(old)
			}
			if err != nil {
				return "", err
			}
			return v.Change(pass, now)
		}, func(code string) {
			a.vaultUsed()
			k.mode, k.code, k.fresh = askCode, code, false
			k.old, k.pass, k.again = "", "", ""
		})
	case askCode:
		if !k.saved {
			return
		}
		then, wasFresh, n := k.then, k.fresh, k.sealed
		a.closeVaultAsk()
		switch {
		case wasFresh && n > 0:
			a.tell(fmt.Sprintf("已开启：%d 条里的密钥已加密", n))
		case wasFresh:
			a.tell("已开启：以后收进来的密钥会加密保存")
		default:
			a.tell("二次密码已修改，旧的密码和恢复码都作废了")
		}
		if then != nil {
			then()
		}
	case askOff:
		a.closeVaultAsk()
		a.vaultOff()
	}
}

// vaultView is the vault's window: the password asked, set, changed, the
// recovery code shown once.
func (a *app) vaultView(c *ui.Context) {
	k := &a.vaultAsk
	if !k.open {
		return
	}
	t := c.Theme()
	modal(c, "vault", &k.open, func() {
		w, _ := c.Size()
		ui.Column(c).Gap(12).Width(min(400, w-80)).Children(func() {
			title := func(s string) { ui.Text(c, s).FontSize(17).FontWeight(700) }
			says := func(s string) {
				ui.Text(c, s).FontSize(textSmall).TextColor(ink2(t)).LineHeight(1.5).Wrap()
			}
			pass := func(key string, v *string, hint string, focus bool) ui.Element {
				in := ui.TextInput(c.Key(key), v).Password().Placeholder(hint).Label(hint).FillWidth()
				if focus {
					in.AutoFocus()
				}
				if in.Submitted() {
					a.later(a.vaultSubmit)
				}
				return in
			}
			wrong := func() {
				if k.wrong != "" {
					ui.Text(c.Key("wrong"), k.wrong).FontSize(textSmall).TextColor(t.Danger).Wrap()
				}
			}
			foot := func(act string, enabled bool, left func()) {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(4, 0, 0).Children(func() {
					if left != nil {
						left()
					}
					ui.Spacer(c)
					if softButton(c, "取消", "").Clicked() && !k.busy {
						a.later(a.closeVaultAsk)
					}
					if busyKey(c, act, "", k.busy, enabled, true).Clicked() && enabled {
						a.later(a.vaultSubmit)
					}
				})
			}
			switch k.mode {
			case askUnlock:
				title("输入二次密码")
				pass("vault-old", &k.old, "二次密码", true)
				wrong()
				foot("解锁", k.old != "", func() {
					if ghostButton(c, "忘了密码？用恢复码", "用恢复码设一个新密码").Margin(0, 0, 0, -10).Clicked() {
						then := k.then
						a.later(func() {
							a.askVault(askChange, then)
							a.vaultAsk.byCode = true
						})
					}
				})
			case askSetup:
				title("设置二次密码")
				says("用来加密收集里的密钥。忘了只能用恢复码找回。")
				pass("vault-pass", &k.pass, "二次密码，至少 8 位", true)
				pass("vault-again", &k.again, "再输一遍", false)
				wrong()
				if a.cfg.SyncRepo != "" && a.secretCount() > 0 {
					ui.Row(c.Key("history")).Padding(8, 10).Radius(8).Background(t.Warning.Alpha(0.12)).Children(func() {
						ui.Text(c, "以前同步到 GitHub 的历史记录里还留着原文。重要的密钥建议换成新的。").FontSize(textSmall).LineHeight(1.5).Wrap().Grow(1).Basis(0)
					})
				}
				foot("下一步", k.pass != "" && k.again != "", nil)
			case askChange:
				title("修改二次密码")
				if k.byCode {
					in := ui.TextInput(c.Key("vault-code"), &k.old).Placeholder("恢复码").Label("恢复码").Font("monospace").FillWidth().AutoFocus()
					if in.Submitted() {
						a.later(a.vaultSubmit)
					}
				} else {
					pass("vault-old", &k.old, "现在的密码", true)
				}
				swap := "忘了？改用恢复码"
				if k.byCode {
					swap = "改用现在的密码"
				}
				if ghostButton(c, swap, "").Margin(-6, 0, 0, -10).AlignSelf(ui.Start).Clicked() {
					k.byCode, k.old, k.wrong = !k.byCode, "", ""
				}
				pass("vault-pass", &k.pass, "新密码，至少 8 位", false)
				pass("vault-again", &k.again, "再输一遍", false)
				wrong()
				says("改了以后恢复码也换新的，旧的密码和恢复码都作废。")
				foot("下一步", k.old != "" && k.pass != "" && k.again != "", nil)
			case askCode:
				title("记下恢复码")
				says("忘了密码时用它重设。它只显示这一次：抄在纸上，或存到别的地方，不要存进收集。")
				raise(ui.Column(c.Key("code")).Padding(14).Radius(radiusCard).FillWidth().AlignItems(ui.Center), t).Children(func() {
					ui.Text(c, k.code).Font("monospace").FontSize(17).FontWeight(700).LetterSpacing(0.5).Wrap().Selectable()
				})
				ui.Checkbox(c, &k.saved, "我已经存好了")
				ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(4, 0, 0).Children(func() {
					if softButton(c, "复制", "复制恢复码").Clicked() {
						a.plat.CopyText(k.code)
						a.copied, a.copiedUntil = k.code, a.now().Add(clipboardClear)
						a.tell("已复制，60 秒后从剪贴板清掉")
					}
					ui.Spacer(c)
					if inkButton(c, "完成", k.saved).Clicked() {
						a.later(a.vaultSubmit)
					}
				})
			case askOff:
				title("关闭加密？")
				says(fmt.Sprintf("已加密的 %d 条会解开，照原样保存和同步。以后收进来的密钥也不再加密。", a.sealedCount()))
				foot("关闭加密", true, nil)
			}
		})
	})
	if !k.open {
		// Put away by a click beside it or Escape. The code is shown once:
		// it stays till it is said to be written down; and a password
		// being set is not left half set, its code never shown.
		if k.busy || k.mode == askCode && !k.saved {
			k.open = true
			return
		}
		a.closeVaultAsk()
	}
}

// vaultSettings is the settings' page of the vault.
func (a *app) vaultSettings(c *ui.Context) {
	v := a.vault
	if !v.On() {
		note := "只加密密钥本身，其余文字照常搜索、让 AI 整理。存的时候不问密码。规则认不出的密码，在详情里选中它点「把选中的设为密码」。"
		n := 0
		for _, cl := range a.lib.Clips {
			if cl.Unsealed() {
				n++
			}
		}
		if n > 0 {
			note = fmt.Sprintf("现在有 %d 条含密钥的记录是原样保存的。", n) + note
		}
		rowsField(c, "给密钥加密", note, func() {
			setRow(c, "二次密码", "收集里看起来像密钥或密码的内容，用它加密后再保存和同步。看和复制时才要输。", func() {
				if inkButton(c, "设置密码…", true).Clicked() {
					a.later(func() { a.askVault(askSetup, nil) })
				}
			})
			// Sealed before it was turned off, on a computer that had not heard.
			if n := a.sealedCount(); n > 0 {
				setLine(c)
				setRow(c, fmt.Sprintf("还有 %d 条是之前加密的", n), "用当时的二次密码解开，照原样保存", func() {
					if button(c, "解开…").Clicked() {
						a.later(func() { a.withVault(func() { a.tell("已解开") }) })
					}
				})
			}
		})
		return
	}
	state := "锁着"
	if v.Unlocked() {
		state = "已解锁"
		if !a.vaultUntil.IsZero() {
			state += "，" + a.vaultUntil.Format("15:04") + " 锁上"
		}
	}
	rowsField(c, "给密钥加密", "只加密密钥本身，其余文字照常搜索、让 AI 整理。规则认不出的，在详情里选中它点「把选中的设为密码」，或在那一条的菜单里选「整条加密」。图片、文件本身和链接地址不加密。", func() {
		setRow(c, "已开启", fmt.Sprintf("%d 条里的密钥已加密 · %s", a.sealedCount(), state), func() {
			if v.Unlocked() {
				if button(c, "现在锁上").Clicked() {
					a.lockVault()
				}
			} else if button(c, "解锁…").Clicked() {
				a.later(func() { a.withVault(func() {}) })
			}
		})
		setLine(c)
		setRow(c, "修改密码", "用现在的密码或恢复码。改了以后恢复码也换新的，旧的作废", func() {
			if button(c, "修改…").Clicked() {
				a.later(func() { a.askVault(askChange, nil) })
			}
		})
		setLine(c)
		setRow(c, "自动锁上", "解锁后过这么久就锁上，退出应用时也会", func() {
			at := 0
			names := make([]string, len(vaultLocks))
			for i, l := range vaultLocks {
				names[i] = l.name
				if l.min == a.cfg.VaultLock {
					at = i
				}
			}
			ui.Row(c).Width(250).Children(func() {
				if i := segmented(c, "vault-lock", "自动锁上", names, at); i != at {
					a.cfg.VaultLock = vaultLocks[i].min
					a.cfg.save()
					if v.Unlocked() {
						a.vaultUsed()
					}
				}
			})
		})
		setLine(c)
		clear := !a.cfg.VaultKeepClipboard
		switchRow(c, &clear, "复制后清空剪贴板", "复制的密钥 60 秒后从剪贴板清掉", func() {
			a.cfg.VaultKeepClipboard = !clear
			a.cfg.save()
		})
		setLine(c)
		setRow(c, "关闭加密", "把加密的内容解开，照原样保存。要先解锁", func() {
			if button(c, "关闭…").Clicked() {
				a.later(func() { a.withVault(func() { a.askVault(askOff, nil) }) })
			}
		})
	})
}

// secretKeys are the keys under a clip's content that holds a secret: to
// show it, or put it in dots again, and to copy it unseen when it is the
// only one. With no password set, the way to set one is said here, where
// the secret is looked at.
func (a *app) secretKeys(c *ui.Context, clip *library.Clip) {
	if !clip.Secret() {
		return
	}
	t := c.Theme()
	ui.Row(c.Key("secret-keys")).Gap(2).AlignItems(ui.Center).Wrap().Margin(0, 0, 0, -8).Children(func() {
		if a.secretShown == clip.ID || a.typing(clip) {
			if ghostButton(c, "遮住", "把密钥遮起来").Clicked() {
				a.later(a.hideSecrets)
			}
		} else {
			tip := "这条里有看起来像密钥或密码的内容，已遮住；点一下显示并可以编辑"
			if clip.Sealed() {
				tip = "密钥已加密，输入二次密码后显示并可以编辑"
			}
			if ghostButton(c, "显示密钥", tip).Clicked() {
				a.later(func() { a.showSecrets(clip) })
			}
		}
		if _, ok := theSecret(clip); ok {
			if ghostButton(c, "复制密钥", "不显示，直接复制").Clicked() {
				a.later(func() { a.copySecret(clip) })
			}
		}
	})
	if !a.vault.On() && clip.Unsealed() {
		ui.Row(c.Key("secret-plain")).Gap(2).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, "这条里的密钥是原样保存和同步的。").FontSize(textCaption).TextColor(t.TextMuted)
			if textLink(c, "设置二次密码加密", "设置二次密码").Clicked() {
				a.later(a.openVaultSettings)
			}
		})
	}
}

// A mark is the part of a clip's field chosen in it, which a key makes a
// secret, or (un) no secret any more.
type mark struct {
	id, field string
	from, to  int // in runes of the text shown
	un        bool
}

// secretField is a field of a clip's details as it is written in: the
// text without the marks of its secrets (library.Marks), those parts and
// what the rules find shown in the accent, bold. The user found the marks
// round a password strange to read: they are the file's alone.
type secretField struct {
	shown string
	parts [][2]int // marked by hand
}

func (a *app) secretField(full string) secretField {
	shown, parts := library.Marks(full)
	return secretField{shown, parts}
}

// style shows the secrets of the field in their look.
func (f secretField) style(in ui.Element, t *ui.Theme) {
	for _, p := range append(library.Found(f.shown), f.parts...) {
		in.TextRanges(ui.TextRange{Start: p[0], End: p[1], Color: t.Accent, Weight: 700})
	}
}

// typed is the field's text to keep once next was typed in it: the parts
// marked follow the edit. caret is where it left the caret, which tells
// where it was made when the letters by it are alike.
func (f secretField) typed(next string, caret int) string {
	if len(f.parts) == 0 {
		return next
	}
	return library.WithMarks(next, moveParts(f.parts, []rune(f.shown), []rune(next), caret))
}

// moveParts are the parts of a text after an edit of it, from was to
// now. What is typed in a part is of it; what is typed at its ends is
// not, nor what replaces it from outside; a part wholly replaced from
// its first letter to its last is the new letters (a password changed).
func moveParts(parts [][2]int, was, now []rune, caret int) [][2]int {
	// The edit replaced was[p:q] by now[p:p+n].
	suffix := func(k int) bool { // the last k letters are alike
		return k <= len(was) && k <= len(now) && slices.Equal(was[len(was)-k:], now[len(now)-k:])
	}
	s := -1
	if k := len(now) - caret; caret >= 0 && k >= 0 && suffix(k) {
		s = k // the edit ends at the caret
	} else {
		for s = 0; s < len(was) && s < len(now) && was[len(was)-1-s] == now[len(now)-1-s]; s++ {
		}
	}
	p := 0
	for p < len(was)-s && p < len(now)-s && was[p] == now[p] {
		p++
	}
	q, n := len(was)-s, len(now)-s-p
	delta := n - (q - p)
	var out [][2]int
	for _, part := range parts {
		from, to := part[0], part[1]
		switch {
		case from < p || from == p && q > p: // begins before the edit, or with it
		case from < q || from == p: // in what went, or just after what came
			from = p + n
		default:
			from += delta
		}
		switch {
		case to <= p: // ends before the edit: what is typed at its end is not of it
		case to < q:
			to = p
		default:
			to += delta
		}
		if from < to {
			out = append(out, [2]int{from, to})
		}
	}
	return out
}

// markKey gives a field of a clip's details the way to say a part of it
// is a secret, where no rule tells one (the user had a line of an
// address, a name and a password, and asked how to hide the password):
// the part chosen, by a key that shows under the field while one is, and
// by the field's menu; and the way back, with the caret in a part marked.
// The part is put between marks in what is kept (library.MarkOpen), which
// the app alone writes and no field shows, and sealed when a second
// password is set; the field shows it in dots at once, which is what was
// asked for.
func (a *app) markKey(c *ui.Context, clip *library.Clip, field string, in ui.Element, f secretField, set func(full string)) {
	from, to := in.TextSelection()
	from, to = min(from, to), max(from, to)
	chosen := mark{id: clip.ID, field: field, from: from, to: to}
	for _, p := range f.parts { // in a part marked, or over one: the way back
		if from < p[1] && to > p[0] || from == to && from >= p[0] && from <= p[1] {
			chosen.un = true
		}
	}
	// The part chosen stays the one to mark when the field is left for
	// the key: a click on the key takes the field's focus first.
	if in.Focused() {
		a.marking = mark{}
		if from != to || chosen.un {
			a.marking = chosen
		}
	}
	do := func(m mark) {
		a.later(func() { a.markSecret(f, m, set) })
	}
	in.ContextMenu(func(m *ui.Menu) {
		m.EditItems()
		m.Separator()
		if chosen.un {
			if m.Item("不再当作密码").Chosen() {
				do(chosen)
			}
		} else if m.Item("设为密码").Disabled(from == to).Chosen() {
			do(chosen)
		}
	})
	if m := a.marking; m.id == clip.ID && m.field == field {
		ui.Row(c.Key("mark-"+field)).Margin(0, 0, 0, -8).Children(func() {
			if m.un {
				if ghostButton(c, "不再当作密码", "这一段照原样显示和保存").Clicked() {
					do(m)
				}
			} else if ghostButton(c, "把选中的设为密码", "选中的这一段会遮住；设了二次密码的话会加密保存").Clicked() {
				do(m)
			}
		})
	}
}

// markSecret makes the part m of a field's text a secret, or no secret
// any more.
func (a *app) markSecret(f secretField, m mark, set func(full string)) {
	a.marking = mark{}
	r := []rune(f.shown)
	from, to := max(0, min(m.from, len(r))), max(0, min(m.to, len(r)))
	if m.un {
		parts := slices.DeleteFunc(slices.Clone(f.parts), func(p [2]int) bool {
			return from < p[1] && to > p[0] || from == to && from >= p[0] && from <= p[1]
		})
		set(library.WithMarks(f.shown, parts))
		a.tell("这一段不再当作密码")
		return
	}
	for from < to && unicode.IsSpace(r[from]) { // a word chosen with the space by it
		from++
	}
	for to > from && unicode.IsSpace(r[to-1]) {
		to--
	}
	if from >= to {
		return
	}
	if !library.CanMark(string(r[from:to])) {
		a.tell("选中的这一段里有 ⟦ ⟧ 这样的符号，不能设为密码")
		return
	}
	set(library.WithMarks(f.shown, append(slices.Clone(f.parts), [2]int{from, to})))
	a.hideSecrets() // in dots at once
	if a.vault.On() {
		a.tell("已设为密码，加密保存了")
	} else {
		a.tellAct("已设为密码，遮住了；设置二次密码后会加密", "去设置", a.openVaultSettings)
	}
}

// openVaultSettings opens the settings on the vault's page.
func (a *app) openVaultSettings() {
	a.openSettings()
	a.settingsTab = settingsTabOf("加密")
}

// vaultState says, on 含密钥's page, whether the secrets are unlocked,
// with the key to lock them now.
func (a *app) vaultState(c *ui.Context) {
	if a.libCat != secretCat || !a.vault.On() {
		return
	}
	t := c.Theme()
	if !a.vault.Unlocked() {
		ui.Text(c, "锁着").FontSize(textCaption).TextColor(t.TextMuted).Margin(0, 0, 0, 4)
		return
	}
	state := "已解锁"
	if !a.vaultUntil.IsZero() {
		state += "，" + a.vaultUntil.Format("15:04") + " 锁上"
	}
	ui.Text(c, state).FontSize(textCaption).TextColor(t.TextMuted).Margin(0, 0, 0, 4)
	if textLink(c, "现在锁上", "现在锁上").Clicked() {
		a.later(a.lockVault)
	}
}

// settingsTabs are the settings' pages, in their order.
var settingsTabs = []string{"通用", "资料整理", "备份", "同步", "加密", "连接"}

func settingsTabOf(name string) int {
	for i, n := range settingsTabs {
		if n == name {
			return i
		}
	}
	return 0
}
