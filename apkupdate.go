package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/apkinstall"
)

// The updates of the Android app, half by itself (0.107.1, the user's): an
// app on Android cannot replace itself, as MyGo's updater does a
// desktop's, so it finds a newer version once a day (apkManifestURL, which
// a release publishes), says what is new, downloads the package as a card
// of the long work (jobs), checks it, and opens the system's installer on
// it (apkinstall, Kotlin), whose one key installs it over the app. The
// user lets the app install packages once (安装未知应用): sent to the
// system's page, the installer opens when the app is come back to.
// A package installs over the app only when signed by the same key
// (.github/android-upload.sealed).

// apkManifestURL is where a release says its Android package is.
const apkManifestURL = "https://github.com/wuyunyoujiduo-jpg/mindmap-models/releases/download/desktop-latest/android.json"

// apkEvery is how often a phone looks for a newer version by itself.
const apkEvery = 24 * time.Hour

// apkRelease is a version of the Android package, as the release says it.
type apkRelease struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

// apkGetter downloads url into w, telling how far it got of how much (0
// when not said).
type apkGetter func(ctx context.Context, url string, w io.Writer, progress func(done, size int64)) error

// apkState is where the update of the Android app stands.
type apkState struct {
	checking bool        // looking for a newer version, asked for
	offer    *apkRelease // a newer version, said in a window while open
	open     bool
	busy     bool // downloading it
	done     int64
	since    time.Time
	cancel   func()
	ready    string // the package downloaded and checked, to install
	allow    bool   // sent to the system's page to let the app install
	result   string // the download's card, done
	failed   bool
}

func httpDownload(ctx context.Context, url string, w io.Writer, progress func(done, size int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s 返回 %d", req.URL.Host, resp.StatusCode)
	}
	size := max(resp.ContentLength, 0)
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			progress(done, size)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// newerVersion tells whether version a is above b, number by number.
func newerVersion(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// apkDir is where packages are downloaded: the app's cache, which the
// system's installer is given (MyGo's FileProvider serves it).
func apkDir() string { return filepath.Join(os.TempDir(), "updates") }

// apkUpdates says whether the app updates itself so: an Android app's
// that can open the installer.
func (a *app) apkUpdates() bool { return a.installer != nil }

// apkDue looks for a newer version, by itself, once a day.
func (a *app) apkDue() {
	if !a.apkUpdates() || a.cfg.NoApkCheck || a.now().Sub(a.cfg.ApkChecked) < apkEvery {
		return
	}
	a.apkCheck(false)
}

// apkCheck looks for a newer version; asked for (manual), it says what it
// found, newer or not, and why it could not.
func (a *app) apkCheck(manual bool) {
	if a.apk.checking || a.apk.busy {
		return
	}
	a.apk.checking = manual
	get := a.apkGet
	a.async(func() (any, error) {
		var buf strings.Builder
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := get(ctx, apkManifestURL, &buf, func(int64, int64) {}); err != nil {
			return nil, err
		}
		var r apkRelease
		if err := json.Unmarshal([]byte(buf.String()), &r); err != nil || r.Version == "" || r.URL == "" {
			return nil, errors.New("更新信息读不懂")
		}
		return &r, nil
	}, func(v any, err error) {
		a.apk.checking = false
		if err != nil {
			if manual {
				a.tell("检查更新失败：" + err.Error())
			}
			return
		}
		a.cfg.ApkChecked = a.now()
		a.cfg.save()
		r := v.(*apkRelease)
		if !newerVersion(r.Version, version) {
			if manual {
				a.tell("已经是最新版本 " + version)
			}
			return
		}
		a.apk.offer, a.apk.open = r, true
	})
}

// apkDownload downloads the version offered, as a card of the long work,
// checks it and installs it.
func (a *app) apkDownload() {
	r := a.apk.offer
	if r == nil || a.apk.busy {
		return
	}
	a.apk.open = false
	if a.apk.ready != "" && strings.Contains(filepath.Base(a.apk.ready), r.Version) {
		a.apkInstall() // downloaded already
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.apk.busy, a.apk.done, a.apk.since, a.apk.cancel = true, 0, a.now(), cancel
	a.apk.result, a.apk.failed, a.apk.ready = "", false, ""
	get, post := a.apkGet, a.post
	a.async(func() (any, error) {
		defer cancel()
		dir := apkDir()
		os.RemoveAll(dir) // a package of before, or half of one
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		dst := filepath.Join(dir, "MyMind-"+r.Version+".apk")
		f, err := os.Create(dst)
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		err = get(ctx, r.URL, io.MultiWriter(f, h), func(done, _ int64) { post(func() { a.apk.done = done }) })
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		switch {
		case err != nil:
		case r.SHA256 != "" && hex.EncodeToString(h.Sum(nil)) != r.SHA256:
			err = errors.New("下载的安装包校验不符，请重试")
		}
		if err != nil {
			os.Remove(dst)
			return nil, err
		}
		return dst, nil
	}, func(v any, err error) {
		a.apk.busy, a.apk.cancel = false, nil
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return // put away by its ✕: nothing to say
			}
			a.apk.result, a.apk.failed = "新版本没有下载下来："+err.Error(), true
			return
		}
		a.apk.ready = v.(string)
		a.apkInstall()
	})
}

// apkInstall opens the system's installer on the package downloaded, or,
// while the app may not install packages, the window that sends the user
// to the system's page to allow it.
func (a *app) apkInstall() {
	if a.apk.ready == "" || a.installer == nil {
		return
	}
	if !a.installer.CanInstall() {
		a.apk.allow, a.apk.open = true, true
		return
	}
	a.apk.allow, a.apk.open = false, false
	if err := a.installer.Install(a.apk.ready); err != nil {
		a.apk.result, a.apk.failed = "没能打开系统的安装程序："+err.Error(), true
		return
	}
	a.apk.result = "新版本已下载"
}

// apkActive is the app come back to: from the system's page where it was
// let install packages, the installer opens; else, once a day, a look for
// a newer version.
func (a *app) apkActive() {
	if !a.apkUpdates() {
		return
	}
	if a.apk.allow && a.apk.ready != "" && a.installer.CanInstall() {
		a.apkInstall()
		return
	}
	a.apkDue()
}

// apkJob is the download's card among the long work's.
func (a *app) apkJob(now time.Time) (jobCard, bool) {
	r := a.apk.offer
	switch {
	case a.apk.busy && r != nil:
		line, frac := "正在连接…", float32(-1)
		if r.Size > 0 {
			frac = min(float32(a.apk.done)/float32(r.Size), 1)
			line = fmt.Sprintf("%.1f / %.1f MB · %s", float64(a.apk.done)/1e6, float64(r.Size)/1e6, timeLeft(now, a.apk.since, frac))
		}
		cancel := a.apk.cancel
		return jobCard{key: "apk", icon: "import", title: "下载新版本 " + r.Version, line: line, frac: frac, since: a.apk.since, cancel: cancel}, true
	case a.apk.result != "":
		job := jobCard{key: "apk", title: a.apk.result, done: true, failed: a.apk.failed, close: func() { a.apk.result = "" }}
		if a.apk.failed {
			job.line = "点这里再试一次"
			job.open = func() { a.apk.result = ""; a.apkDownload() }
		} else {
			job.line = "在系统的窗口里点「安装」，数据都在"
			job.open = func() { a.apk.result = ""; a.apkInstall() }
		}
		return job, true
	}
	return jobCard{}, false
}

// apkUpdateView is the window that says a newer version is out, and what
// is new in it; or that the app is to be let install it.
func (a *app) apkUpdateView(c *ui.Context) {
	r := a.apk.offer
	if !a.apk.open || r == nil {
		return
	}
	t := c.Theme()
	_, h := c.Size()
	modal(c, "apk-update", &a.apk.open, func() {
		ui.Column(c).Width(modalRoom(c, "apk-update", 420)).Gap(14).Children(func() {
			if a.apk.allow {
				ui.Text(c, "允许 MyMind 安装更新").FontSize(textTitle).FontWeight(700)
				ui.Text(c, "安卓要先允许 MyMind「安装未知应用」，只用允许这一次。在打开的设置里打开开关，再回到 MyMind，就会接着安装 "+r.Version+"。").
					FontSize(textBody).TextColor(ink2(t)).LineHeight(1.5).Wrap()
				ui.Row(c).Gap(10).Justify(ui.End).FillWidth().Children(func() {
					if button(c, "以后再说").Clicked() {
						a.apk.open = false
					}
					if accentButton(c, "去允许", "打开系统设置，允许 MyMind 安装应用").Clicked() {
						if err := a.installer.AllowInstall(); err != nil {
							a.tell("没能打开系统设置：" + err.Error())
						}
					}
				})
				return
			}
			ui.Text(c, "MyMind "+r.Version+" 可以更新了").FontSize(textTitle).FontWeight(700)
			ui.Text(c, "现在是 "+version).FontSize(textSmall).TextColor(t.TextMuted)
			if notes := strings.TrimSpace(strings.ReplaceAll(r.Notes, "**", "")); notes != "" { // CHANGELOG.md's, its bold dropped
				ui.Scroll(c.Key("apk-notes")).MaxHeight(max(h*0.45, 160)).FillWidth().Children(func() {
					ui.Text(c, notes).FontSize(textBody).LineHeight(1.55).Wrap()
				})
			}
			ui.Row(c).Gap(10).Justify(ui.End).FillWidth().Children(func() {
				if button(c, "以后再说").Clicked() {
					a.apk.open = false
				}
				label := "更新"
				if r.Size > 0 {
					label = fmt.Sprintf("更新（%.0f MB）", float64(r.Size)/1e6)
				}
				if accentButton(c, label, "下载新版本的安装包").Clicked() {
					a.later(a.apkDownload)
				}
			})
		})
	})
}

// apkSettings is the version's part of the settings on a phone: a look
// for a newer version now, and whether to look by itself.
func (a *app) apkSettings(c *ui.Context) {
	rowsField(c, "版本", "有新版本时下载安装包，在系统的窗口里点「安装」就更新好了，导图和收集都还在。", func() {
		setRow(c, appName+" "+version, "", func() {
			if busyClicked(busyKey(c, "检查更新…", "看看有没有新版本", a.apk.checking, !a.apk.busy, false), a.apk.checking, !a.apk.busy) {
				a.apkCheck(true)
			}
		})
		setLine(c)
		auto := !a.cfg.NoApkCheck
		switchRow(c, &auto, "自动检查更新", "每天看一次；有新版本时说更新了什么，点「更新」才下载", func() {
			a.cfg.NoApkCheck = !auto
			a.cfg.save()
		})
	})
}

// apkInstaller is the system's installer where there is one: on Android.
func apkInstaller() apkinstall.Installer {
	if !onPhone {
		return nil
	}
	in, err := apkinstall.NewInstaller()
	if err != nil {
		return nil
	}
	return in
}
