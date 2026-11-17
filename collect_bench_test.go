package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
)

// The frames of 收集 timed on what the reading of the whole measured
// (docs/审查清单.md, 26 to 31): run with
//
//	go test -run xxx -bench Collect -benchtime 20x .
//
// Each is a frame of the page as it stands, nothing changed between two:
// what the page works out of the clips is to be worked out once.

// benchApp is an app for a benchmark, as newTestApp's.
func benchApp(b *testing.B) (*app, *ui.Tester) {
	b.Helper()
	b.Setenv("HOME", b.TempDir())
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())
	b.Setenv("XDG_CACHE_HOME", b.TempDir())
	clock = func() time.Time { return testNow }
	a := newApp(&fakePlatform{}, config{Dir: b.TempDir(), PanelOpen: true, LibraryManual: true})
	a.noLocalModel, a.noMotion = true, true
	return a, ui.NewTester(a.view, 1280, 800)
}

// wordy is a text of about n characters that names a key and a token in
// every line, as notes on a program do, with no secret in it: the rules
// are tried on all of it.
func wordy(i, n int) string {
	var b strings.Builder
	for line := 0; b.Len() < n*3; line++ { // three bytes a character at most
		fmt.Fprintf(&b, "第 %d 条的第 %d 行：缓存的 key 怎么取，token 过期了怎么换，接口的返回值先看状态码。\n", i, line)
	}
	return string([]rune(b.String())[:n])
}

// addNotes keeps n notes of about size characters.
func addNotes(tb testing.TB, a *app, n, size int) {
	tb.Helper()
	for i := range n {
		c := &library.Clip{Kind: library.KindText, Text: wordy(i, size)}
		if err := a.lib.Add(c, testNow.Add(time.Duration(i)*time.Second)); err != nil {
			tb.Fatal(err)
		}
	}
}

// addLinks keeps n links not sorted out yet, each with a page of about
// size characters.
func addLinks(tb testing.TB, a *app, n, size int) {
	tb.Helper()
	for i := range n {
		c := &library.Clip{Kind: library.KindLink, URL: fmt.Sprintf("https://example.com/post/%d", i), Title: fmt.Sprintf("文章 %d", i), Text: wordy(i, size)}
		if err := a.lib.Add(c, testNow.Add(time.Duration(i)*time.Second)); err != nil {
			tb.Fatal(err)
		}
	}
}

// showWall shows 收集's wall, settled.
func showWall(a *app, tt *ui.Tester) {
	a.showLibrary()
	a.libHome = false
	tt.Frame()
	tt.Frame()
}

// BenchmarkCollectWallNotes is a frame of a wall of 120 notes of 1,500
// characters (26).
func BenchmarkCollectWallNotes(b *testing.B) {
	a, tt := benchApp(b)
	addNotes(b, a, 120, 1500)
	showWall(a, tt)
	b.ResetTimer()
	for range b.N {
		tt.Frame()
	}
}

// BenchmarkCollectWallLinks is a frame of a wall of 120 links not sorted
// out, of 20,000 characters each (26).
func BenchmarkCollectWallLinks(b *testing.B) {
	a, tt := benchApp(b)
	addLinks(b, a, 120, 20000)
	showWall(a, tt)
	b.ResetTimer()
	for range b.N {
		tt.Frame()
	}
}

// BenchmarkCollectTyping is the frame after a letter typed in the details
// of a note, among 800 notes (27).
func BenchmarkCollectTyping(b *testing.B) {
	a, tt := benchApp(b)
	addNotes(b, a, 800, 1500)
	showWall(a, tt)
	a.libSel = a.lib.Clips[0].ID
	tt.Frame()
	if err := tt.Click("资料内容"); err != nil {
		b.Fatal(err)
	}
	tt.Frame()
	b.ResetTimer()
	for range b.N {
		tt.Type("字")
		tt.Frame()
	}
}

// BenchmarkCollectTypingKept is the same with each letter kept, as every
// letter was, and as the last one is when the typing pauses: the clip's
// file written, and what is worked out of the clips brought up to date.
func BenchmarkCollectTypingKept(b *testing.B) {
	a, tt := benchApp(b)
	addNotes(b, a, 800, 1500)
	showWall(a, tt)
	a.libSel = a.lib.Clips[0].ID
	tt.Frame()
	if err := tt.Click("资料内容"); err != nil {
		b.Fatal(err)
	}
	tt.Frame()
	b.ResetTimer()
	for range b.N {
		tt.Type("字")
		a.keepTyped()
		tt.Frame()
	}
}

// BenchmarkCollectBacklinks is a frame of a wall of 120 cards beside 150
// maps (28).
func BenchmarkCollectBacklinks(b *testing.B) {
	a, tt := benchApp(b)
	addBenchMaps(b, a, 150)
	for i := range 120 {
		c := &library.Clip{Kind: library.KindText, Text: fmt.Sprintf("第 %d 条，一句话的灵感。", i)}
		if err := a.lib.Add(c, testNow.Add(time.Duration(i)*time.Second)); err != nil {
			b.Fatal(err)
		}
	}
	showWall(a, tt)
	b.ResetTimer()
	for range b.N {
		tt.Frame()
	}
}

// addBenchMaps writes n maps of a few topics into the app's folder.
func addBenchMaps(tb testing.TB, a *app, n int) {
	tb.Helper()
	for i := range n {
		f := model.NewFile(fmt.Sprintf("项目 %d", i))
		for j := range 8 {
			f.Sheets[0].Root.Children = append(f.Sheets[0].Root.Children, model.NewNode(fmt.Sprintf("主题 %d-%d", i, j)))
		}
		if err := f.Save(filepath.Join(a.dir, fmt.Sprintf("项目 %d%s", i, model.Ext))); err != nil {
			tb.Fatal(err)
		}
	}
	a.refreshFiles()
}

// BenchmarkCollectLinkOffers is what [[ offers, worked out for a part
// typed, among 800 clips and 150 maps (31).
func BenchmarkCollectLinkOffers(b *testing.B) {
	a, _ := benchApp(b)
	addBenchMaps(b, a, 150)
	addNotes(b, a, 800, 1500)
	a.linkOffers("主题", 4)
	b.ResetTimer()
	for range b.N {
		a.linkOffers("主题", 4)
	}
}

// BenchmarkCollectLinkOffersTyped is the same for a part typed anew each
// time, as at each letter after a [[: found once, with every map read
// already.
func BenchmarkCollectLinkOffersTyped(b *testing.B) {
	a, _ := benchApp(b)
	addBenchMaps(b, a, 150)
	addNotes(b, a, 800, 1500)
	a.linkOffers("主题", 4)
	b.ResetTimer()
	for i := range b.N {
		a.linkOffers(fmt.Sprintf("主题 %d", i%100), 4)
	}
}

// BenchmarkCollectOutsideChangeSealed is BenchmarkCollectOutsideChange
// with a second password set: what came is looked at for what is to be
// sealed.
func BenchmarkCollectOutsideChangeSealed(b *testing.B) {
	vault.Quick()
	a, _ := benchApp(b)
	addNotes(b, a, 800, 1500)
	if _, err := a.vault.Setup("correct horse", a.now()); err != nil {
		b.Fatal(err)
	}
	a.noteLibraryWrite()
	path := filepath.Join(a.lib.Dir, "clips", a.lib.Clips[0].ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := range b.N {
		b.StopTimer()
		if err := os.WriteFile(path, data, 0o644); err != nil {
			b.Fatal(err)
		}
		at := time.Now().Add(time.Duration(i+1) * time.Hour)
		_ = os.Chtimes(path, at, at)
		_ = os.Chtimes(filepath.Dir(path), at, at)
		b.StartTimer()
		a.checkLibrary()
	}
}

// BenchmarkCollectOutsideChange is the library taken again after one clip
// was written outside the app, among 800 (30).
func BenchmarkCollectOutsideChange(b *testing.B) {
	a, _ := benchApp(b)
	addNotes(b, a, 800, 1500)
	a.noteLibraryWrite()
	path := filepath.Join(a.lib.Dir, "clips", a.lib.Clips[0].ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := range b.N {
		b.StopTimer()
		// Written as an agent writes it, and its folder's time moved on.
		if err := os.WriteFile(path, data, 0o644); err != nil {
			b.Fatal(err)
		}
		at := time.Now().Add(time.Duration(i+1) * time.Hour)
		_ = os.Chtimes(path, at, at)
		_ = os.Chtimes(filepath.Dir(path), at, at)
		b.StartTimer()
		a.checkLibrary()
	}
}
