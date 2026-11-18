package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// fakeInstaller is the system's installer of a test.
type fakeInstaller struct {
	can       bool
	allowed   int
	installed []string
	fail      error
}

func (f *fakeInstaller) CanInstall() bool    { return f.can }
func (f *fakeInstaller) AllowInstall() error { f.allowed++; return nil }
func (f *fakeInstaller) Install(path string) error {
	if f.fail != nil {
		return f.fail
	}
	f.installed = append(f.installed, path)
	return nil
}

// apkServer serves a release's manifest and its package, counting what
// was asked.
func apkServer(t *testing.T, ver string, pkg []byte, sum string) (apkGetter, *int) {
	if sum == "" {
		h := sha256.Sum256(pkg)
		sum = hex.EncodeToString(h[:])
	}
	man, _ := json.Marshal(apkRelease{Version: ver, Notes: "- **新东西**：一行", URL: "https://example.com/mymind-" + ver + ".apk", SHA256: sum, Size: int64(len(pkg))})
	asked := new(int)
	return func(ctx context.Context, url string, w io.Writer, progress func(done, size int64)) error {
		*asked++
		switch {
		case url == apkManifestURL:
			_, err := w.Write(man)
			return err
		case strings.HasSuffix(url, ".apk"):
			w.Write(pkg)
			progress(int64(len(pkg)), int64(len(pkg)))
			return nil
		}
		return errors.New("404")
	}, asked
}

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.107.1", "0.107.0", true},
		{"0.108.0", "0.107.9", true},
		{"0.107.10", "0.107.9", true},
		{"1.0.0", "0.999.999", true},
		{"0.107.0", "0.107.0", false},
		{"0.106.9", "0.107.0", false},
		{"0.107", "0.107.0", false},
	} {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

// TestApkUpdate goes the way of an update on a phone: a newer version
// found and said, downloaded and checked, the user sent once to let the
// app install, and the installer opened when the app is come back to.
func TestApkUpdate(t *testing.T) {
	a, _ := newTestApp(t)
	t.Setenv("TMPDIR", t.TempDir())
	in := &fakeInstaller{}
	pkg := []byte("the package of 99.0.0")
	a.installer = in
	a.apkGet, _ = apkServer(t, "99.0.0", pkg, "")
	tt := ui.NewTester(a.view, 390, 844)

	a.apkCheck(true)
	tt.Frame()
	if !a.apk.open || a.apk.offer == nil || !tt.HasText("MyMind 99.0.0 可以更新了") || !tt.HasText("- 新东西：一行") {
		t.Fatalf("no offer: %+v %q", a.apk, tt.Texts())
	}
	snapshot(t, tt, "apk-offer")
	if a.cfg.ApkChecked.IsZero() {
		t.Fatal("when it looked not kept")
	}
	if err := tt.Click("更新（0 MB）"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// Downloaded and checked; the app may not install yet: sent to allow it.
	if a.apk.ready == "" || !a.apk.allow || !a.apk.open {
		t.Fatalf("%+v", a.apk)
	}
	if got, _ := os.ReadFile(a.apk.ready); string(got) != string(pkg) {
		t.Fatalf("downloaded %q", got)
	}
	tt.Frame()
	snapshot(t, tt, "apk-allow")
	if err := tt.Click("去允许"); err != nil {
		t.Fatal(err)
	}
	if in.allowed != 1 || len(in.installed) != 0 {
		t.Fatalf("%+v", in)
	}
	// Back in the app, allowed: the installer opens on the package.
	a.apkActive()
	if len(in.installed) != 0 {
		t.Fatal("installed while not allowed")
	}
	in.can = true
	a.apkActive()
	if len(in.installed) != 1 || in.installed[0] != a.apk.ready || a.apk.open {
		t.Fatalf("%+v %+v", in, a.apk)
	}
	if job, ok := a.apkJob(time.Now()); !ok || job.title != "新版本已下载" || !strings.Contains(job.line, "点「安装」") || job.open == nil {
		t.Fatalf("card %+v", job)
	}
	tt.Frame()
	snapshot(t, tt, "apk-card")
	a.openSettings()
	tt.Frame()
	clickInSettings := tt.HasText("检查更新…") && tt.HasText("自动检查更新")
	if !clickInSettings {
		t.Fatalf("no 检查更新 in the settings: %q", tt.Texts())
	}
}

// TestApkUpdateChecked: a package whose SHA-256 is not the release's is
// not installed, and its card says so and offers it again.
func TestApkUpdateChecked(t *testing.T) {
	a, _ := newTestApp(t)
	t.Setenv("TMPDIR", t.TempDir())
	in := &fakeInstaller{can: true}
	a.installer = in
	a.apkGet, _ = apkServer(t, "99.0.0", []byte("pkg"), strings.Repeat("0", 64))
	a.apkCheck(false)
	a.apkDownload()
	if len(in.installed) != 0 || a.apk.ready != "" || !a.apk.failed || !strings.Contains(a.apk.result, "校验不符") {
		t.Fatalf("%+v %+v", in, a.apk)
	}
	if job, ok := a.apkJob(time.Now()); !ok || !job.failed || job.open == nil {
		t.Fatalf("%+v", job)
	}
}

// TestApkUpdateDue: the look by itself is once a day, never where the
// app has no installer, and not when turned off; a version not newer is
// not offered.
func TestApkUpdateDue(t *testing.T) {
	a, _ := newTestApp(t)
	get, asked := apkServer(t, version, []byte("pkg"), "")
	a.apkGet = get
	a.apkDue() // a desktop's: no installer
	if *asked != 0 {
		t.Fatal("looked on a desktop")
	}
	a.installer = &fakeInstaller{}
	a.apkDue()
	if *asked != 1 || a.apk.open {
		t.Fatalf("asked %d, open %v", *asked, a.apk.open)
	}
	a.apkDue()
	a.apkActive()
	if *asked != 1 {
		t.Fatalf("looked again the same day: %d", *asked)
	}
	a.cfg.ApkChecked = a.now().Add(-25 * time.Hour)
	a.cfg.NoApkCheck = true
	a.apkDue()
	if *asked != 1 {
		t.Fatal("looked though turned off")
	}
	a.cfg.NoApkCheck = false
	a.apkActive()
	if *asked != 2 {
		t.Fatalf("not looked a day on: %d", *asked)
	}
}
