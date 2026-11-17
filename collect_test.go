package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
)

// A note with a password in it, made a task with no second password set,
// has dots in the task's name as on its card, not the password.
func TestSecretStaysOutOfATask(t *testing.T) {
	a, _ := newTestApp(t)
	cl, err := a.addMemo("数据库密码：Hunter2Hunter2，别外传", "手动")
	if err != nil {
		t.Fatal(err)
	}
	a.clipToTask(cl)
	f, err := model.Load(a.inboxPath())
	if err != nil {
		t.Fatal(err)
	}
	kids := f.Sheets[0].Root.Children
	if len(kids) == 0 {
		t.Fatal("no task")
	}
	if title := kids[len(kids)-1].Title; strings.Contains(title, "Hunter2") || !strings.Contains(title, "••••") || !strings.Contains(title, "别外传") {
		t.Fatalf("the task's name: %q", title)
	}
	// And a sentence the rules took for a secret is a task as written.
	plain, err := a.addMemo("密码是什么时候改的我忘了", "手动")
	if err != nil {
		t.Fatal(err)
	}
	a.clipToTask(plain)
	f, _ = model.Load(a.inboxPath())
	kids = f.Sheets[0].Root.Children
	if title := kids[len(kids)-1].Title; title != "密码是什么时候改的我忘了" {
		t.Fatalf("a plain sentence: %q", title)
	}
}

// photo is a JPEG of w by h, red on its left half and blue on its right,
// with the EXIF orientation o.
func photo(t *testing.T, w, h, o int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{220, 30, 30, 255}
			if x >= w/2 {
				c = color.RGBA{30, 30, 220, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	// APP1 after the start of the image: "Exif", a TIFF header and a
	// directory of the one tag.
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08" + "\x00\x01" + "\x01\x12\x00\x03\x00\x00\x00\x01\x00\x00\x00\x00" + "\x00\x00\x00\x00")
	binary.BigEndian.PutUint16(tiff[18:], uint16(o))
	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(app1[2:], uint16(len(seg)+2))
	data := buf.Bytes()
	out := append([]byte{}, data[:2]...)
	out = append(out, app1...)
	out = append(out, seg...)
	return append(out, data[2:]...)
}

// A photo shows upright, as its camera's orientation says: taken with the
// phone held up it showed on its side, on the wall, in the details and on
// a topic.
func TestPhotosShowUpright(t *testing.T) {
	red := func(c color.Color) bool { r, _, b, _ := c.RGBA(); return r > 0x8000 && b < 0x8000 }
	blue := func(c color.Color) bool { r, _, b, _ := c.RGBA(); return b > 0x8000 && r < 0x8000 }
	for _, o := range []int{1, 3, 6, 8} {
		data := photo(t, 400, 200, o)
		if got := exifOrientation(data); got != o {
			t.Fatalf("orientation %d read as %d", o, got)
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for kind, name := range map[int]string{pictureWhole: "whole", pictureSmall: "small"} {
			up := upright(img, o, kind)
			w, h := up.Bounds().Dx(), up.Bounds().Dy()
			at := func(fx, fy float64) color.Color {
				return up.At(up.Bounds().Min.X+int(fx*float64(w)), up.Bounds().Min.Y+int(fy*float64(h)))
			}
			var ok bool
			switch o {
			case 1: // as kept: red at the left
				ok = w == 400 && h == 200 && red(at(0.1, 0.5)) && blue(at(0.9, 0.5))
			case 3: // upside down: red at the right
				ok = w == 400 && h == 200 && blue(at(0.1, 0.5)) && red(at(0.9, 0.5))
			case 6: // a quarter clockwise: what was left is at the top
				ok = w == 200 && h == 400 && red(at(0.5, 0.1)) && blue(at(0.5, 0.9))
			case 8: // a quarter the other way: what was left is at the foot
				ok = w == 200 && h == 400 && blue(at(0.5, 0.1)) && red(at(0.5, 0.9))
			}
			if !ok {
				t.Errorf("orientation %d, %s: %d×%d", o, name, w, h)
			}
		}
	}
	if exifOrientation(testPNG(t, 4, 4)) != 1 || exifOrientation(nil) != 1 || exifOrientation([]byte{0xff, 0xd8, 0xff, 0xe1, 0, 4}) != 1 {
		t.Error("an orientation read where there is none")
	}
	// Through the app's own reading, away from the window's: the details'
	// picture and its card's.
	a, _ := newTestApp(t)
	path := filepath.Join(t.TempDir(), "IMG_0001.jpg")
	if err := os.WriteFile(path, photo(t, 1600, 800, 6), 0o644); err != nil {
		t.Fatal(err)
	}
	cl, err := a.addClipFile(path, "拖入")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := a.filePicture(a.lib.FilePath(cl), false)
	if b == nil {
		t.Fatal("not read")
	}
	if w, h := b.Size(); w != 800 || h != 1600 {
		t.Fatalf("the photo whole is %d×%d", w, h)
	}
	card, _ := a.cardPicture(a.lib.FilePath(cl))
	if w, h := card.Size(); w != thumbWide || h != thumbTall {
		t.Fatalf("its card's picture is %d×%d", w, h)
	}
}

// A card's picture is cut to the card's shape, and a small one of a long
// page is small: 1170 by 20000, the "small" one was 640 by 10940, 28 MB.
func TestLongPictureIsSmall(t *testing.T) {
	long := image.NewGray(image.Rect(0, 0, 1170, 20000))
	card := upright(long, 1, pictureCard).Bounds()
	if card.Dx() != thumbWide || card.Dy() != thumbTall {
		t.Fatalf("a card's: %v", card)
	}
	small := upright(long, 1, pictureSmall).Bounds()
	if small.Dy() > smallMost || small.Dx()*20000 < small.Dy()*1100 || small.Dx()*20000 > small.Dy()*1240 {
		t.Fatalf("a small one: %v", small)
	}
	// One of an ordinary shape is as it was: no smaller than covers a card.
	if r := upright(image.NewGray(image.Rect(0, 0, 2000, 1200)), 1, pictureSmall).Bounds(); r.Dx() != 640 || r.Dy() != 384 {
		t.Fatalf("an ordinary one: %v", r)
	}
	if img := image.NewGray(image.Rect(0, 0, 300, 100)); upright(img, 1, pictureSmall) != image.Image(img) || upright(img, 1, pictureWhole) != image.Image(img) {
		t.Error("a small picture was made again")
	}
}

// The whole pictures looked at are kept while they take no more than
// their room, those shown last: each one opened was kept for good.
func TestWholePicturesHaveARoom(t *testing.T) {
	a, _ := newTestApp(t)
	was := wholeRoom
	wholeRoom = 2 << 20
	defer func() { wholeRoom = was }()
	dir := t.TempDir()
	var paths []string
	for i := range 8 {
		p := filepath.Join(dir, string(rune('a'+i))+".png")
		if err := os.WriteFile(p, testPNG(t, 500+i, 500), 0o644); err != nil { // a megabyte each, read
			t.Fatal(err)
		}
		paths = append(paths, p)
		if b, _ := a.filePicture(p, false); b == nil {
			t.Fatal("not read")
		}
		a.filePicture(paths[0], false) // the first is looked at all along
	}
	whole := 0
	for k, b := range a.bitmaps {
		if b != nil && wholeKey(k) && strings.HasPrefix(k, dir) {
			whole++
		}
	}
	if whole != wholeLeast {
		t.Fatalf("%d whole pictures kept", whole)
	}
	if a.bitmaps[paths[0]] == nil || a.bitmaps[paths[7]] == nil || a.bitmaps[paths[1]] != nil {
		t.Fatal("not those shown last were kept")
	}
	// One that went is read again when shown.
	if b, _ := a.filePicture(paths[1], false); b == nil {
		t.Fatal("not read again")
	}
}

// A picture whose file is not there yet, as one the sync has not brought
// down, is looked for again after a while: it was missing till the app
// was left.
func TestMissingPictureIsReadLater(t *testing.T) {
	a, _ := newTestApp(t)
	path := filepath.Join(t.TempDir(), "later.png")
	if b, loading := a.filePicture(path, true); b != nil || loading {
		t.Fatalf("a file not there: %v, loading %v", b, loading)
	}
	if err := os.WriteFile(path, testPNG(t, 40, 30), 0o644); err != nil {
		t.Fatal(err)
	}
	reads := 0
	run := a.async
	a.async = func(work func() (any, error), done func(any, error)) { reads++; run(work, done) }
	if b, _ := a.filePicture(path, true); b != nil || reads != 0 {
		t.Fatal("read again at once: every frame would")
	}
	a.now = func() time.Time { return testNow.Add(pictureRetry + time.Second) }
	if b, loading := a.filePicture(path, true); b == nil || loading || reads != 1 {
		t.Fatalf("not read once it is there: %v, %d reads", b, reads)
	}
	// One that is no picture is not read again and again.
	bad := filepath.Join(t.TempDir(), "bad.png")
	if err := os.WriteFile(bad, []byte("not a picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.filePicture(bad, true)
	a.now = func() time.Time { return testNow.Add(time.Hour) }
	if b, loading := a.filePicture(bad, true); b != nil || loading || reads != 2 {
		t.Fatalf("a broken picture: %v, %d reads", b, reads)
	}
}

// Whole sealing taken off a clip that another key sealed in part leaves
// the clip as it was: what did open stayed in memory as written, not
// kept, and was written so by the next thing that saved it.
func TestUnlockWholeLeavesNothingOpen(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.vault.Setup("correct horse", a.now()); err != nil {
		t.Fatal(err)
	}
	other := vault.Open(t.TempDir())
	if _, err := other.Setup("another one", a.now()); err != nil {
		t.Fatal(err)
	}
	foreign, ok := other.Seal("别处的密码")
	if !ok {
		t.Fatal("not sealed")
	}
	cl := &library.Clip{Kind: library.KindText, Title: "路由器", Text: "admin / plumtree", Note: foreign, Locked: true}
	if err := a.lib.Add(cl, a.now()); err != nil {
		t.Fatal(err)
	}
	if !vault.Has(cl.Text) || !a.vault.Unlocked() {
		t.Fatalf("not sealed whole: %q", cl.Text)
	}
	a.lockWhole(cl, false)
	if !cl.Locked || !vault.Has(cl.Text) || strings.Contains(cl.Text, "plumtree") {
		t.Fatalf("half opened: locked %v, %q", cl.Locked, cl.Text)
	}
	if !strings.Contains(a.notice, "另一个二次密码") {
		t.Fatalf("not said: %q", a.notice)
	}
}

// What is done to a clip once a password was asked, or taken back from a
// toast, is done to the clip the library has by then: read again
// meanwhile, as a sync does, the clip held before is another.
func TestClipFoundAgainAfterReload(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.vault.Setup("correct horse", a.now()); err != nil {
		t.Fatal(err)
	}
	cl, err := a.addMemo("路由器 admin / plumtree", "手动")
	if err != nil {
		t.Fatal(err)
	}
	id := cl.ID
	a.openLibrary() // as the sync that setting a password starts
	a.lockWhole(cl, true)
	now := a.lib.Find(id)
	if now == cl || !now.Locked || !vault.Has(now.Text) {
		t.Fatalf("sealed whole on a clip the library has no more: locked %v, %q", now.Locked, now.Text)
	}

	// Deleted, the library read again, and taken back.
	path := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(path, testPNG(t, 40, 30), 0o644); err != nil {
		t.Fatal(err)
	}
	pic, err := a.addClipFile(path, "拖入")
	if err != nil {
		t.Fatal(err)
	}
	file := a.lib.FilePath(pic)
	a.deleteClipUndoably(pic.ID)
	if _, err := os.Stat(file); !os.IsNotExist(err) || a.lib.Find(pic.ID) != nil {
		t.Fatal("not deleted")
	}
	a.openLibrary()
	a.noticeUndo()
	if a.lib.Find(pic.ID) == nil {
		t.Fatalf("taken back into a library that is shown no more: %q", a.notice)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("its file is not back")
	}
}

// A panic of work done off the window's thread is an error of that work,
// said, and not the end of the app.
func TestWorkPanicIsAnError(t *testing.T) {
	a, _ := newTestApp(t)
	v, err := guarded(func() (any, error) {
		var m map[string]int
		m["x"] = 1
		return "done", nil
	})
	var p *workPanic
	if v != nil || !errors.As(err, &p) || !strings.HasPrefix(err.Error(), "出错了：") {
		t.Fatalf("a panic: %v, %v", v, err)
	}
	a.panicked(err)
	if !strings.HasPrefix(a.notice, "出错了：") {
		t.Fatalf("not said: %q", a.notice)
	}
	a.notice = ""
	if v, err := guarded(func() (any, error) { return 3, os.ErrNotExist }); v != 3 || err != os.ErrNotExist {
		t.Fatalf("work that did not panic: %v, %v", v, err)
	}
	if a.panicked(os.ErrNotExist); a.notice != "" {
		t.Fatal("an error of the work's own was said as a panic")
	}
}

// A short note with a key in it is a short note still once the key is
// sealed: as long as it reads, not as its token is.
func TestSealedNoteIsStillBrief(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.vault.Setup("correct horse", a.now()); err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("接口说明。", 30) + "key：sk-abcdefghijklmnopqrstuvwxyz0123456789abcd"
	cl, err := a.addMemo(text, "手动")
	if err != nil {
		t.Fatal(err)
	}
	if !cl.Sealed() || len([]rune(cl.Text)) <= briefLen {
		t.Fatalf("not sealed, or its token is short: %d", len([]rune(cl.Text)))
	}
	if !briefClip(cl) || clipContentLen(cl, nil) > briefLen {
		t.Fatalf("a short note is long once sealed: %d", clipContentLen(cl, nil))
	}
}

// A # that is of a web address or a color is no tag of the note.
func TestMemoTagsNotOfAddresses(t *testing.T) {
	got := memoTags("看 https://example.com/guide#install-2 和 https://ttc.net/#/login，主色 #E8A13A 底色 #fff 写 C#，#设计 #decade #2024")
	if strings.Join(got, " ") != "设计 decade 2024" {
		t.Fatalf("tags %q", got)
	}
	if got := memoTags("#读书 a#b (#想法)"); strings.Join(got, " ") != "读书 想法" {
		t.Fatalf("tags %q", got)
	}
}

// Of files dropped, one that cannot be kept does not stop the rest, and
// how many could not is said; where the user is, not on 收集's page alone.
func TestFilesDroppedAreAllTried(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs a file that is there and cannot be read")
	}
	const unreadable = "/proc/self/mem"
	if _, err := os.Stat(unreadable); err != nil {
		t.Skip(err)
	}
	a, _ := newTestApp(t)
	dir := t.TempDir()
	var files []string
	for i, name := range []string{"一.png", "二.png"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, testPNG(t, 30+i, 20), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	a.goToday()
	a.dropClips([]string{files[0], unreadable, files[1]})
	if n := len(a.lib.Clips); n != 2 {
		t.Fatalf("%d kept of two that could be", n)
	}
	if !strings.Contains(a.notice, "存下了 2 个，另有 1 个没有存下") || !strings.Contains(a.notice, "mem") || a.libErr != "" {
		t.Fatalf("said %q, on 收集's page %q", a.notice, a.libErr)
	}
	left, _ := os.ReadDir(a.lib.FilesDir())
	if len(left) != 2 {
		t.Fatalf("%d files in the library", len(left))
	}
	// On 收集's page, in its line, which stays.
	a.openList(kindAll, "")
	a.notice = ""
	a.dropClips([]string{unreadable})
	if !strings.Contains(a.libErr, "没有存下 mem") || a.notice != "" {
		t.Fatalf("on 收集's page: %q, toast %q", a.libErr, a.notice)
	}
}

// A clip's category is as it is typed, a space in it too; its tags are
// taken off by a ✕ of their own, with no other key of the pointer.
func TestCategoryAndTagsInDetails(t *testing.T) {
	a, tt := newTestApp(t)
	cl, err := a.addMemo("一条笔记 #留着 #去掉", "手动")
	if err != nil {
		t.Fatal(err)
	}
	a.openList(kindAll, "")
	a.libSel = cl.ID
	tt.Frame()
	if err := tt.Click("资料分类"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, s := range []string{"M", "L", " ", "o", "p", "s", " "} {
		tt.Type(s)
		tt.Frame()
	}
	if cl.Category != "ML ops" {
		t.Fatalf("category %q", cl.Category)
	}
	if got := a.lib.Categories(); len(got) != 1 || got[0].Name != "ML ops" {
		t.Fatalf("categories %+v", got)
	}
	if _, ok := tt.Find("去掉标签 去掉"); !ok {
		t.Fatalf("no ✕ on a tag: %q", tt.Texts())
	}
	snapshot(t, tt, "clip-tags")
	if err := tt.Click("去掉标签 去掉"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if strings.Join(cl.Tags, " ") != "留着" || a.libTag != "" {
		t.Fatalf("tags %q, narrowed to %q", cl.Tags, a.libTag)
	}
	// The tag itself narrows 收集 to it, as it did.
	if err := tt.Click("标签 留着"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.libTag != "留着" {
		t.Fatalf("narrowed to %q", a.libTag)
	}
}

// A picture shown large has a key that copies it: its menu and ⌘C did,
// which not every device has.
func TestLargePictureCopiesByAKey(t *testing.T) {
	a, tt := newTestApp(t)
	fp := a.plat.(*fakePlatform)
	fp.clipImage = testPNG(t, 200, 100)
	n := find(t, a, "前端")
	a.selectNode(n.ID)
	if !a.pasteImage() || n.Image == nil {
		t.Fatal("no picture")
	}
	a.lightbox = n.ID
	tt.Frame()
	tt.Frame()
	for _, s := range tt.Texts() {
		if strings.Contains(s, "右键") || strings.Contains(s, "Esc") || strings.Contains(s, "⌘") {
			t.Fatalf("words of a device: %q", s)
		}
	}
	snapshot(t, tt, "lightbox-copy")
	if err := tt.Click("复制图片"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(fp.copiedImage) == 0 {
		t.Fatal("nothing copied")
	}
}

// A clip deleted leaves nothing of it in the app's cache: a video's
// poster and its length stayed there.
func TestPosterGoesWithItsClip(t *testing.T) {
	a, _ := newTestApp(t)
	cl, err := a.addClipFile("testdata/clip.mp4", "拖入")
	if err != nil {
		t.Fatal(err)
	}
	a.savePoster(cl, testPNG(t, 8, 8), 3*time.Second)
	p := posterPath(cl)
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	a.deleteClipUndoably(cl.ID)
	for _, f := range []string{p, strings.TrimSuffix(p, ".jpg") + ".len"} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Fatalf("left in the cache: %s", f)
		}
	}
	if a.vidPosterHas[cl.ID] || a.videoLen(cl) != 0 {
		t.Fatal("still taken to have a poster")
	}
}
