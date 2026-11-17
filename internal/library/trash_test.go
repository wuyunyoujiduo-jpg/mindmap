package library

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
)

// A clip deleted has its file moved into the library's trash, which the
// sync leaves alone, and is put back from there: into the library as it
// is by then, read again or not.
func TestTrashMovesTheFile(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir)
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	src := filepath.Join(t.TempDir(), "录屏.mp4")
	data := bytes.Repeat([]byte("video"), 1000)
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := l.AddFile(src, "拖入", now)
	if err != nil {
		t.Fatal(err)
	}
	file := l.FilePath(c)
	taken, err := l.Trash(c.ID, now)
	if err != nil || l.Find(c.ID) != nil || taken.ID() != c.ID {
		t.Fatalf("trash: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("the file is still among the files")
	}
	waiting, _ := os.ReadDir(filepath.Join(dir, ".trash"))
	if len(waiting) != 1 {
		t.Fatalf("%d files in the trash", len(waiting))
	}
	rel := ".library/.trash/" + waiting[0].Name()
	if ghsync.Included(rel) {
		t.Fatalf("the trash would be synced: %s", rel)
	}
	// What was put there since a time stays; the library read again, as
	// after a sync, takes the clip back.
	l.EmptyTrash(now.Add(-time.Minute))
	again, _ := Open(dir)
	if again.Find(c.ID) != nil {
		t.Fatal("the clip is still read")
	}
	if err := again.PutBack(taken); err != nil || again.Find(c.ID) == nil {
		t.Fatalf("put back: %v", err)
	}
	if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("the file is not back: %v", err)
	}
	if third, _ := Open(dir); third.Find(c.ID) == nil {
		t.Fatal("the clip's own file is not back")
	}
	if left, _ := os.ReadDir(filepath.Join(dir, ".trash")); len(left) != 0 {
		t.Fatalf("%d left in the trash", len(left))
	}

	// Emptied, the trash holds nothing, and what was in it cannot be
	// taken back: which is said.
	taken, err = again.Trash(c.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	again.EmptyTrash(now.Add(time.Minute))
	if _, err := os.Stat(filepath.Join(dir, ".trash")); !os.IsNotExist(err) {
		t.Fatal("the trash is not emptied")
	}
	if err := again.PutBack(taken); err == nil || again.Find(c.ID) != nil {
		t.Fatalf("put back from an empty trash: %v", err)
	}
}

// A file is kept by its copy, told from those kept by what it holds, with
// nothing left behind when it is there already.
func TestAddFileCopies(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir)
	now := time.Now()
	from := t.TempDir()
	doc := filepath.Join(from, "说明.md")
	if err := os.WriteFile(doc, []byte("# 接口说明\n登录要带上令牌"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := l.AddFile(doc, "拖入", now)
	if err != nil || c.Kind != KindFile || c.Title != "说明" || !strings.Contains(c.Text, "登录要带上令牌") || c.Hash != hashOf([]byte("# 接口说明\n登录要带上令牌")) {
		t.Fatalf("kept: %v %+v", err, c)
	}
	twin := filepath.Join(from, "另一个名字.md")
	if err := os.WriteFile(twin, []byte("# 接口说明\n登录要带上令牌"), 0o644); err != nil {
		t.Fatal(err)
	}
	var dup *DuplicateError
	if _, err := l.AddFile(twin, "拖入", now); !errors.As(err, &dup) || dup.Clip != c {
		t.Fatalf("kept twice: %v", err)
	}
	files, _ := os.ReadDir(l.FilesDir())
	if len(files) != 1 || files[0].Name() != c.File {
		t.Fatalf("files left: %v", files)
	}
	if _, err := l.AddFile(filepath.Join(from, "没有.png"), "拖入", now); err == nil {
		t.Fatal("a file that is not there was kept")
	}
}

// A clip's file written before 0.88 or 0.54 has keys nothing reads any
// more (kept, folder): it is read as any, and written without them.
func TestOldKeysAreDropped(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "clips"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "clips", "abc123.json")
	old := `{"id":"abc123","kind":"text","text":"旧的一条","kept":true,"folder":"项目资料","created":"2025-01-02T03:04:05Z","updated":"2025-01-02T03:04:05Z"}`
	if err := os.WriteFile(p, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := Open(dir)
	if err != nil || len(l.Clips) != 1 || len(l.Broken) != 0 || l.Clips[0].Text != "旧的一条" {
		t.Fatalf("read: %v, %d clips, broken %v", err, len(l.Clips), l.Broken)
	}
	if err := l.Save(l.Clips[0], time.Now()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if strings.Contains(string(data), "kept") || strings.Contains(string(data), "folder") || !strings.Contains(string(data), "旧的一条") {
		t.Fatalf("written: %s", data)
	}
}

// A note with a key in it is found kept already once the keys are
// sealed, while they can be opened; one that differs by its key alone is
// another note; and locked, it is kept again rather than lost.
func TestDuplicateSealed(t *testing.T) {
	now := time.Now()
	l, _ := Open(t.TempDir())
	if _, err := l.Vault.Setup("correct horse", now); err != nil {
		t.Fatal(err)
	}
	const note = "OpenAI 的 key\nOPENAI_API_KEY=sk-proj-9fK2mB7tLx0QaZr4Hn8Qx7b"
	first := &Clip{Kind: KindText, Text: note}
	if err := l.Add(first, now); err != nil || !first.Sealed() {
		t.Fatalf("kept: %v", err)
	}
	var dup *DuplicateError
	if err := l.Add(&Clip{Kind: KindText, Text: note}, now); !errors.As(err, &dup) || dup.Clip != first {
		t.Fatalf("the same note kept twice: %v", err)
	}
	// The key changed for a new one: another note.
	other := &Clip{Kind: KindText, Text: strings.Replace(note, "9fK2", "0000", 1)}
	if err := l.Add(other, now); err != nil {
		t.Fatalf("a note with another key: %v", err)
	}
	if err := l.Add(&Clip{Kind: KindText, Text: other.Text + "  "}, now); !errors.As(err, &dup) {
		t.Fatalf("read again, the second is not found: %v", err)
	}
	// With no secret in it, as ever.
	if err := l.Add(&Clip{Kind: KindText, Text: "买牛奶"}, now); err != nil {
		t.Fatal(err)
	}
	if err := l.Add(&Clip{Kind: KindText, Text: "买牛奶 "}, now); !errors.As(err, &dup) {
		t.Fatalf("a plain note kept twice: %v", err)
	}
	// Locked, what is sealed cannot be compared: kept, not refused.
	l.Vault.Lock()
	if err := l.Add(&Clip{Kind: KindText, Text: note}, now); err != nil {
		t.Fatalf("locked: %v", err)
	}
}

// A text is as long as it reads: a key sealed in it is its dots.
func TestReadLen(t *testing.T) {
	l, _ := Open(t.TempDir())
	if _, err := l.Vault.Setup("correct horse", time.Now()); err != nil {
		t.Fatal(err)
	}
	c := &Clip{Kind: KindText, Text: "key：sk-abcdefghijklmnopqrstuvwxyz0123456789abcd"}
	if err := l.Add(c, time.Now()); err != nil || !c.Sealed() {
		t.Fatal(err)
	}
	if n := ReadLen(c.Text); n != len([]rune("key："+masked)) {
		t.Errorf("a sealed key reads %d long: %q", n, c.Text)
	}
	if ReadLen("  买牛奶 \n") != 3 {
		t.Error("a plain text")
	}
}
