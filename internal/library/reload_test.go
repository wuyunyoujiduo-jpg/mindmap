package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// sameIndex tells whether two indexes are of the same words.
func sameIndex(t *testing.T, got, want *Index) {
	t.Helper()
	if !slices.Equal(got.clips, want.clips) || !reflect.DeepEqual(got.terms, want.terms) || !slices.Equal(got.lens, want.lens) ||
		!slices.Equal(got.lower, want.lower) || !reflect.DeepEqual(got.df, want.df) || got.avg != want.avg {
		t.Fatalf("the index brought up to date is not the index made anew: %d clips and %d terms, for %d and %d", len(got.clips), len(got.df), len(want.clips), len(want.df))
	}
}

// The index is brought up to date from the one before as clips are kept,
// changed, deleted and put back, and is then the index made anew; with
// nothing asked for, the clips are listed with no index made.
func TestIndexKeptUpToDate(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	l, _ := Open(t.TempDir())
	add := func(i int) *Clip {
		c := &Clip{Kind: KindText, Title: fmt.Sprintf("笔记 %d", i), Text: fmt.Sprintf("第 %d 条讲缓存和索引 cache index %d", i, i%3), Tags: []string{fmt.Sprintf("t%d", i%2)}}
		if err := l.Add(c, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
		return c
	}
	var clips []*Clip
	for i := range 6 {
		clips = append(clips, add(i))
	}
	if hits := l.Search("", nil); len(hits) != 6 || l.index != nil {
		t.Fatalf("nothing asked for: %d clips, an index made: %v", len(hits), l.index != nil)
	}
	if hits := l.Search("  ，。", func(c *Clip) bool { return c == clips[2] }); len(hits) != 1 || l.index != nil {
		t.Fatalf("signs alone: %d clips, an index made: %v", len(hits), l.index != nil)
	}
	if hits := l.Search("缓存", nil); len(hits) != 6 {
		t.Fatalf("found %d", len(hits))
	}
	first := l.index
	check := func(when string) {
		t.Helper()
		sameIndex(t, l.idx(), NewIndex(l.Clips))
		if l.index == nil {
			t.Fatalf("%s: the index was dropped", when)
		}
	}
	// A clip's words changed and kept: found by the new ones alone.
	clips[1].Text = "改成了讲数据库的 database"
	if err := l.Save(clips[1], now); err != nil {
		t.Fatal(err)
	}
	if hits := l.Search("数据库", nil); len(hits) != 1 || hits[0].Clip != clips[1] {
		t.Fatalf("the words typed are not found: %v", hits)
	}
	if hits := l.Search("database", nil); len(hits) != 1 {
		t.Fatalf("the words typed are not found: %v", hits)
	}
	check("a clip changed")
	if got := l.Search("cache", nil); slices.ContainsFunc(got, func(h Hit) bool { return h.Clip == clips[1] }) {
		t.Fatal("found by the words it had")
	}
	// One kept, one deleted, one taken and put back.
	clips = append(clips, add(6))
	check("a clip kept")
	if err := l.Delete(clips[0].ID); err != nil {
		t.Fatal(err)
	}
	check("a clip deleted")
	taken, err := l.Trash(clips[3].ID, now)
	if err != nil {
		t.Fatal(err)
	}
	check("a clip taken")
	if err := l.PutBack(taken); err != nil {
		t.Fatal(err)
	}
	check("a clip put back")
	// Its words were cut up once: the clips that did not change are in
	// the index as they were.
	for _, c := range []*Clip{clips[2], clips[4], clips[5]} {
		if reflect.ValueOf(l.index.terms[l.index.at[c]]).Pointer() != reflect.ValueOf(first.terms[first.at[c]]).Pointer() {
			t.Fatalf("the words of %q were cut up again", c.Title)
		}
	}
	// Sealed, the tokens are no words of it.
	if _, err := l.Vault.Setup("correct horse", now); err != nil {
		t.Fatal(err)
	}
	clips[2].Note = "token=zyxwvutsrq"
	if err := l.Save(clips[2], now); err != nil {
		t.Fatal(err)
	}
	check("a secret sealed")
	if hits := l.Search("zyxwvutsrq", nil); len(hits) != 0 {
		t.Fatal("found by a secret sealed")
	}
}

// Reload reads the files that changed in the folder, and those alone: a
// clip an agent kept, one changed by a sync, one deleted, one that will
// not read; a clip in two files; a file that cannot be read for now.
func TestReload(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	l, _ := Open(dir)
	var clips []*Clip
	for i := range 5 {
		c := &Clip{Kind: KindText, Text: fmt.Sprintf("第 %d 条", i)}
		if err := l.Add(c, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
		clips = append(clips, c)
	}
	path := func(name string) string { return filepath.Join(dir, "clips", name) }
	later := now
	write := func(name string, c *Clip) { // as another program writes it
		t.Helper()
		data, _ := json.MarshalIndent(c, "", "  ")
		if err := os.WriteFile(path(name), data, 0o644); err != nil {
			t.Fatal(err)
		}
		later = later.Add(time.Hour)
		_ = os.Chtimes(path(name), later, later)
	}
	reload := func() (fresh, changed []*Clip, gone []string) {
		t.Helper()
		fresh, changed, gone, err := l.Reload()
		if err != nil {
			t.Fatal(err)
		}
		// Whatever was read again, the library is as one opened anew.
		anew, _ := Open(dir)
		if len(anew.Clips) != len(l.Clips) || !slices.Equal(anew.Broken, l.Broken) {
			t.Fatalf("%d clips and broken %v, for %d and %v opened anew", len(l.Clips), l.Broken, len(anew.Clips), anew.Broken)
		}
		for i, c := range anew.Clips {
			got := *l.Clips[i]
			got.worked = nil
			if !reflect.DeepEqual(&got, c) {
				t.Fatalf("clip %d is %+v, opened anew %+v", i, got, *c)
			}
		}
		for _, c := range l.Clips {
			if l.Find(c.ID) != c {
				t.Fatalf("%q is not found by its ID", c.Text)
			}
		}
		if l.index != nil {
			sameIndex(t, l.idx(), NewIndex(l.Clips))
		}
		return fresh, changed, gone
	}
	l.Search("第", nil) // an index, to be kept up to date
	v := l.Version
	if fresh, changed, gone := reload(); len(fresh)+len(changed)+len(gone) != 0 || l.Version != v {
		t.Fatal("nothing changed, and something was read again")
	}
	// What the library wrote itself is not read again.
	clips[0].Note = "自己写的"
	if err := l.Save(clips[0], now); err != nil {
		t.Fatal(err)
	}
	if fresh, changed, gone := reload(); len(fresh)+len(changed)+len(gone) != 0 {
		t.Fatal("its own writing was taken for a change from outside")
	}

	// One kept outside, one changed, one deleted.
	agent := &Clip{ID: NewID(), Kind: KindText, Text: "agent 记的 password: hunter2hunter2", Created: now.Add(time.Hour), Updated: now}
	write(agent.ID+".json", agent)
	edited := *clips[1]
	edited.worked = nil
	edited.Text = "同步下来的新内容"
	write(clips[1].ID+".json", &edited)
	if err := os.Remove(path(clips[2].ID + ".json")); err != nil {
		t.Fatal(err)
	}
	held := clips[1]
	held.Label() // worked out of the words it had
	fresh, changed, gone := reload()
	if len(fresh) != 1 || fresh[0].ID != agent.ID || len(changed) != 1 || changed[0] != held || !slices.Equal(gone, []string{clips[2].ID}) {
		t.Fatalf("fresh %v, changed %v, gone %v", fresh, changed, gone)
	}
	if held.Text != "同步下来的新内容" || held.Label() != "同步下来的新内容" || l.Find(held.ID) != held {
		t.Fatalf("the clip held is not the clip as it is now: %q, %q", held.Text, held.Label())
	}
	if l.Clips[0].ID != agent.ID || !l.Clips[0].Secret() || l.Find(clips[2].ID) != nil {
		t.Fatal("the new one is not first, or the one deleted is there")
	}
	if l.Clips[4] != clips[0] || l.Clips[3] != held || l.Clips[1] != clips[4] {
		t.Fatal("the clips that did not change are not the clips they were")
	}
	if hits := l.Search("同步下来", nil); len(hits) != 1 || hits[0].Clip != held {
		t.Fatal("not found by its new words")
	}

	// One that will not read, then mended; a file that is no clip's.
	if err := os.WriteFile(path(clips[3].ID+".json"), []byte("{ 坏的"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(path("笔记.txt"), []byte("不是资料"), 0o644)
	if _, _, gone := reload(); !slices.Equal(gone, []string{clips[3].ID}) || !slices.Equal(l.Broken, []string{clips[3].ID + ".json"}) {
		t.Fatalf("gone %v, broken %v", gone, l.Broken)
	}
	write(clips[3].ID+".json", clips[3])
	if fresh, _, _ := reload(); len(fresh) != 1 || fresh[0].ID != clips[3].ID || len(l.Broken) != 0 {
		t.Fatalf("mended: fresh %v, broken %v", fresh, l.Broken)
	}

	// Its file renamed: the same clip, in place.
	c4 := l.Find(clips[4].ID)
	if err := os.Rename(path(c4.ID+".json"), path("renamed.json")); err != nil {
		t.Fatal(err)
	}
	if fresh, changed, gone := reload(); len(fresh)+len(gone) != 0 || len(changed) != 1 || changed[0] != c4 {
		t.Fatalf("renamed: fresh %v, changed %v, gone %v", fresh, changed, gone)
	}

	// A clip in two files, as a cloud's copy is: the one updated later,
	// the other said with the broken ones; and back to one.
	copyOf := *c4
	copyOf.worked = nil
	copyOf.Text, copyOf.Updated = "另一台电脑改的", now.Add(48*time.Hour)
	write(c4.ID+" 2.json", &copyOf)
	reload()
	if got := l.Find(c4.ID); got == nil || got.Text != "另一台电脑改的" || len(l.Broken) != 1 || !l.twice {
		t.Fatalf("a clip in two files: %+v, broken %v", got, l.Broken)
	}
	if err := os.Remove(path("renamed.json")); err != nil {
		t.Fatal(err)
	}
	reload()
	if got := l.Find(c4.ID); got == nil || got.Text != "另一台电脑改的" || len(l.Broken) != 0 || l.twice {
		t.Fatalf("one file again: %+v, broken %v", got, l.Broken)
	}

	// A file that cannot be read for now is not a clip gone.
	if os.Getuid() != 0 {
		keep := l.Find(clips[0].ID)
		p := path(keep.ID + ".json")
		write(keep.ID+".json", keep)
		_ = os.Chmod(p, 0)
		if _, _, gone, err := l.Reload(); err != nil || len(gone) != 0 || l.Find(keep.ID) != keep {
			t.Fatalf("a file locked was taken for a clip gone: %v, %v", gone, err)
		}
		_ = os.Chmod(p, 0o644)
		reload()
	}

	// The folder gone, as one not made yet: no clips.
	if err := os.RemoveAll(filepath.Join(dir, "clips")); err != nil {
		t.Fatal(err)
	}
	if _, _, gone := reload(); len(gone) == 0 || len(l.Clips) != 0 {
		t.Fatalf("the folder gone: %d clips left", len(l.Clips))
	}
}

// The vectors are read away from who holds the library when it says so:
// till they are given it has none and asks for none to be made, and what
// was kept is not made again; they are written by a work of their own.
func TestVectorsLater(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	l, _ := Open(dir)
	for i := range 3 {
		if err := l.Add(&Clip{Kind: KindText, Text: fmt.Sprintf("第 %d 条", i)}, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	ids := l.ToEmbed("m")
	there, texts := l.EmbedTexts(append([]string{"gone"}, ids...))
	if len(ids) != 3 || !slices.Equal(there, ids) || len(texts) != 3 {
		t.Fatalf("to embed %v, there %v", ids, there)
	}
	vecs := make([][]float32, len(ids))
	for i := range vecs {
		vecs[i] = []float32{float32(i), 1}
	}
	l.KeepVectors("m", ids[:2], texts[:2], vecs[:2])
	if _, err := os.Stat(filepath.Join(dir, "vectors.json")); err == nil {
		t.Fatal("kept in memory, and written")
	}
	// Written away from who holds the library, which goes on meanwhile:
	// what is kept after the work was taken is not of it.
	write := l.VectorsWriter()
	wrote := make(chan error)
	go func() { wrote <- write() }()
	l.KeepVectors("m", ids[2:], texts[2:], vecs[2:])
	for _, c := range l.Clips {
		_ = l.Vector(c, "m")
	}
	other := filepath.Join(t.TempDir(), "一个文件.txt")
	_ = os.WriteFile(other, []byte("文件的内容"), 0o644)
	copied := make(chan *Copied)
	go func() { // and a file copied aside, as one dropped on the window is
		c, err := l.CopyFile(other, "拖入")
		if err != nil {
			t.Error(err)
		}
		copied <- c
	}()
	_ = l.Search("第", nil)
	if err := <-wrote; err != nil {
		t.Fatal(err)
	}
	file, err := l.AddCopied(<-copied, now)
	if err != nil || file.Text != "文件的内容" {
		t.Fatalf("a file copied aside: %v", err)
	}
	if err := l.Delete(file.ID); err != nil { // the three notes again
		t.Fatal(err)
	}

	l2, _ := Open(dir)
	l2.VectorsLater = true
	if l2.HasVectors("m") || len(l2.ToEmbed("m")) != 0 || l2.Vector(l2.Clips[0], "m") != nil {
		t.Fatal("not read yet, and vectors are had or asked for")
	}
	read := ReadVectors(dir, "m")
	l2.UseVectors(read)
	if got := l2.ToEmbed("m"); !l2.HasVectors("m") || !slices.Equal(got, ids[2:]) {
		t.Fatalf("read: to embed %v, for %v", got, ids[2:])
	}
	if v := l2.Vector(l2.Find(ids[1]), "m"); len(v) != 2 || v[0] != 1 {
		t.Fatalf("the vector read: %v", v)
	}
	// Of another model, none: all are to be made.
	l2.UseVectors(ReadVectors(dir, "other"))
	if got := l2.ToEmbed("other"); len(got) != 3 {
		t.Fatalf("another model: to embed %v", got)
	}
	// The text made a vector of is cut where it is, as it was.
	long := &Clip{Kind: KindText, Text: strings.Repeat("长", 4000)}
	if got := []rune(EmbedText(long)); len(got) != 1500 {
		t.Fatalf("the text of a vector: %d characters", len(got))
	}
}
