package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// A skill renamed has the name of its head changed, and no line of the
// text under it, though it begins the same.
func TestRenameKeepsTheBody(t *testing.T) {
	w := newWorld(t)
	dir := skill(t, filepath.Join(w.home, ".claude", "skills"), "deploy", "ships")
	body := "\n# deploy\n\nAnother file's head, as an example:\n\n```yaml\nname: staging\nreplicas: 2\n```\n\nname: the first word of a line of the text\n"
	md := "---\ndescription: |\n  ships\n  name: not a key, a line of the description\nname: deploy\n---\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Scan(w.env)
	if _, err := Adopt(w.env, r.Entry("g/deploy"), "claude"); err != nil {
		t.Fatal(err)
	}
	r = Scan(w.env)
	un, err := Rename(w.env, r.Entry("g/deploy"), "ship", Detect(w.home, w.env.Table))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(w.env.Store, "ship", "SKILL.md"))
	want := "---\ndescription: |\n  ships\n  name: not a key, a line of the description\nname: ship\n---\n" + body
	if string(got) != want {
		t.Fatalf("renamed\n%s", got)
	}
	if e := Scan(w.env).Entry("g/ship"); e == nil || e.Title != "ship" {
		t.Fatalf("its name %+v", e)
	}
	if err := un(); err != nil {
		t.Fatal(err)
	}
	if back, _ := os.ReadFile(filepath.Join(w.env.Store, "deploy", "SKILL.md")); string(back) != md {
		t.Fatalf("taken back\n%s", back)
	}
	// Windows' line ends stay, and a head with no name gets one.
	if got := setName("---\r\nname: a\r\ndescription: d\r\n---\r\nname: b\r\n", "c"); got != "---\r\nname: c\r\ndescription: d\r\n---\r\nname: b\r\n" {
		t.Fatalf("line ends %q", got)
	}
	if got := setName("---\ndescription: d\n---\nname: b\n", "c"); got != "---\nname: c\ndescription: d\n---\nname: b\n" {
		t.Fatalf("no name %q", got)
	}
}
