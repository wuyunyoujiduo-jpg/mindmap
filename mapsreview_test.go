package main

import (
	"os"
	"strings"
	"testing"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// What a second reading of the maps' mending found.

// inMap tells whether a map's file has a topic of an ID.
func inMap(t *testing.T, path, id string) bool {
	t.Helper()
	f, err := model.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return f.Node(id) != nil
}

// Topics cut in one map and pasted into another are copies there, with
// IDs of their own: the first map's steps outlive its being left, and ⌘Z
// in it brought the topics back by the IDs the other map had by then.
func TestCutPastedIntoAnotherMapIsACopy(t *testing.T) {
	a, tt := newTestApp(t)
	first := a.path
	other, _, kid := newMapFile(t, a, "归档", "旧需求")
	n := find(t, a, "前端")
	id := n.ID
	a.selectNode(id)
	a.copyNodes(true)
	if err := a.open(other); err != nil {
		t.Fatal(err)
	}
	a.selectNode(kid.ID)
	if !a.paste() {
		t.Fatal("not pasted")
	}
	tt.Frame()
	pasted, _ := a.selectedNode()
	if pasted == nil || pasted.Title != "前端" {
		t.Fatal("the topic cut is not pasted")
	}
	if pasted.ID == id {
		t.Fatal("pasted into another map by the ID it had in the first")
	}
	if err := a.open(first); err != nil {
		t.Fatal(err)
	}
	a.undo() // the cut
	a.flush()
	if a.file.Node(id) == nil {
		t.Fatal("the cut is not undone")
	}
	if inMap(t, other, id) {
		t.Fatal("the same ID is in both maps")
	}

	// In its own map, still by its own ID.
	a.selectNode(id)
	a.copyNodes(true)
	a.selectNode(find(t, a, "功能").ID)
	if !a.paste() || a.file.Node(id) == nil {
		t.Fatal("cut and pasted in its map, it has another ID")
	}
}

// A move to another map is followed by its own step, not by whether the
// branch is here: undone, then the branch deleted here, an undo of a
// later change put it into the other map again; and moved on to a third
// map, a redo had it in two.
func TestMoveFollowsItsOwnStep(t *testing.T) {
	a, tt := newTestApp(t)
	b, _, bKid := newMapFile(t, a, "归档", "旧需求")
	c, _, cKid := newMapFile(t, a, "第三张", "别处")
	n := find(t, a, "前端")
	id := n.ID
	here := func() bool { return a.file.Node(id) != nil }

	// (a) moved, undone, deleted here, another change, that change undone.
	a.selectNode(id)
	a.moveToMap(b, "", bKid.ID)
	a.undo()
	if !here() || inMap(t, b, id) {
		t.Fatal("the move is not undone on both sides")
	}
	a.selectNode(id)
	a.deleteSelection()
	other := find(t, a, "后端")
	a.mutate("", func() { other.Title = "后端服务" })
	a.undo()
	tt.Frame()
	if inMap(t, b, id) {
		t.Fatal("an undo of another change put the branch deleted here into the other map")
	}
	a.undo() // the deletion
	if !here() || inMap(t, b, id) {
		t.Fatalf("the deletion undone: here %v, there %v", here(), inMap(t, b, id))
	}
	a.redo() // deleted again: not moved
	if here() || inMap(t, b, id) {
		t.Fatalf("the deletion made again: here %v, there %v", here(), inMap(t, b, id))
	}
	a.undo()

	// (b) moved to one map, undone, moved to another, undone, made again.
	a.selectNode(id)
	a.moveToMap(b, "", bKid.ID)
	a.undo()
	a.selectNode(id)
	a.moveToMap(c, "", cKid.ID)
	a.undo()
	a.redo()
	tt.Frame()
	if here() || inMap(t, b, id) || !inMap(t, c, id) {
		t.Fatalf("here %v, in the first map %v, in the second %v", here(), inMap(t, b, id), inMap(t, c, id))
	}
}

// Either map renamed, as one is by itself when its central topic is
// retitled, the move is followed still.
func TestMoveFollowedAcrossARename(t *testing.T) {
	a, tt := newTestApp(t)
	b, _, bKid := newMapFile(t, a, "归档", "旧需求")
	n := find(t, a, "前端")
	id := n.ID
	a.selectNode(id)
	a.moveToMap(b, "", bKid.ID)
	back := a.noticeUndo
	a.rename(b, "旧档案")
	tt.Frame()
	var renamed string
	for _, f := range a.files {
		if f.Name == "旧档案" {
			renamed = f.Path
		}
	}
	if renamed == "" || !inMap(t, renamed, id) {
		t.Fatal("the other map is not renamed with the branch in it")
	}
	a.undo()
	if a.file.Node(id) == nil || inMap(t, renamed, id) {
		t.Fatal("the other map renamed, an undo left the branch in both maps")
	}
	a.redo()
	if a.file.Node(id) != nil || !inMap(t, renamed, id) {
		t.Fatal("made again, the branch is not in the map renamed")
	}
	// This map renamed, and the toast's 撤销.
	was := a.path
	a.rename(a.path, "换了名字的导图")
	if a.path == was {
		t.Fatal("the map is not renamed")
	}
	back()
	a.flush()
	if a.file.Node(id) == nil || inMap(t, renamed, id) {
		t.Fatalf("this map renamed, 撤销: here %v, there %v", a.file.Node(id) != nil, inMap(t, renamed, id))
	}
}

// A branch moved to another map and changed there, then the move undone:
// it comes back as the other map had it. As it was moved, the topic added
// under it there was in neither map.
func TestMoveUndoneBringsWhatWasChangedThere(t *testing.T) {
	a, tt := newTestApp(t)
	first := a.path
	b, _, bKid := newMapFile(t, a, "归档", "旧需求")
	n := find(t, a, "前端")
	id := n.ID
	a.selectNode(id)
	a.moveToMap(b, "", bKid.ID)
	if err := a.open(b); err != nil {
		t.Fatal(err)
	}
	moved := a.file.Node(id)
	var added *model.Node
	a.mutate("", func() {
		moved.Title = "前端（改过）"
		added, _ = a.doc.AddChild(id, "在那边加的")
	})
	if err := a.open(first); err != nil {
		t.Fatal(err)
	}
	a.undo()
	tt.Frame()
	got := a.file.Node(id)
	if got == nil || got.Title != "前端（改过）" || a.file.Node(added.ID) == nil {
		t.Fatal("the branch is back as it was moved, not as the other map had it")
	}
	if inMap(t, b, id) || inMap(t, b, added.ID) {
		t.Fatal("the branch stayed in the other map")
	}
	if !strings.Contains(a.notice, "改过") {
		t.Fatalf("not said: %q", a.notice)
	}
	// Made again, it goes as it is now.
	a.redo()
	if a.file.Node(id) != nil || !inMap(t, b, added.ID) {
		t.Fatal("made again, the topic added there is lost")
	}
	a.undo()

	// The other map deleted: a redo cannot put the branch there, and
	// leaves it here, saying so.
	if a.file.Node(id) == nil {
		t.Fatal("not here")
	}
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	a.refreshFiles()
	a.redo()
	tt.Frame()
	if a.file.Node(id) == nil {
		t.Fatal("the other map gone, a redo took the branch out of this one: it is in neither")
	}
	if !strings.Contains(a.notice, "留在这张导图里") {
		t.Fatalf("not said: %q", a.notice)
	}
}
