package versions

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

func TestStore(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	path := filepath.Join(t.TempDir(), "a.mindmap.json")
	f := model.NewFile("根")
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	if ok, err := s.Save(path, f, Open, "", now); !ok || err != nil {
		t.Fatal(ok, err)
	}
	// The same again is not kept.
	if ok, _ := s.Save(path, f, Auto, "", now.Add(time.Minute)); ok {
		t.Fatal("duplicate kept")
	}
	f.Sheets[0].AddChild(f.Sheets[0].Root.ID, "新")
	if ok, _ := s.Save(path, f, AI, "加了一个节点", now.Add(2*time.Minute)); !ok {
		t.Fatal("not kept")
	}
	vs := s.WithNotes(s.List(path))
	if len(vs) != 2 || vs[0].Reason != AI || vs[0].Note != "加了一个节点" || vs[1].Reason != Open {
		t.Fatalf("%+v", vs)
	}
	g, err := s.Load(vs[1])
	if err != nil || len(g.Sheets[0].Root.Children) != 0 {
		t.Fatal(err)
	}
	// Old versions thin out.
	for i := 0; i < 48; i++ {
		f.Sheets[0].Root.Title = time.Duration(i).String()
		s.Save(path, f, Auto, "", now.Add(-10*24*time.Hour+time.Duration(i)*10*time.Minute))
	}
	s.Prune(path, now)
	if n := len(s.List(path)); n > 2+3 { // the two recent, and a day's worth in one or two days
		t.Fatalf("kept %d", n)
	}
	// Renamed maps keep their versions.
	to := filepath.Join(filepath.Dir(path), "b.mindmap.json")
	s.Rename(path, to)
	if len(s.List(to)) == 0 || len(s.List(path)) != 0 {
		t.Fatal("rename")
	}
}
