package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The stamp stays while the folders of skills do, a scan's own writing
// among them, and changes with a skill added, changed where it is,
// linked elsewhere or gone, and with a tool found.
func TestStamp(t *testing.T) {
	w := newWorld(t)
	cc := filepath.Join(w.home, ".claude", "skills")
	dir := skill(t, cc, "deploy", "ships")
	scan := func() string {
		r := Scan(w.env)
		Repair(w.env, r)
		_ = Remember(w.env, r)
		return Stamp(w.env)
	}
	was := scan()
	if now := scan(); now != was || Stamp(w.env) != was {
		t.Fatal("changed with nothing changed")
	}
	moved := func(what string) {
		t.Helper()
		now := Stamp(w.env)
		if now == was {
			t.Fatalf("not changed by %s", what)
		}
		if was = scan(); Stamp(w.env) != was {
			t.Fatalf("not settled after %s", what)
		}
	}
	skill(t, cc, "review", "reads")
	moved("a skill added")
	md := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(md, []byte("---\nname: deploy\ndescription: ships it\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(md, time.Now(), time.Now().Add(time.Hour))
	moved("a skill changed")
	if _, err := Adopt(w.env, Scan(w.env).Entry("g/deploy"), "claude"); err != nil {
		t.Fatal(err)
	}
	moved("a skill taken in")
	if err := os.RemoveAll(filepath.Join(cc, "review")); err != nil {
		t.Fatal(err)
	}
	moved("a skill gone")
	skill(t, filepath.Join(w.home, ".newagent", "skills"), "a", "does a")
	moved("a tool found")
}

// What a page shows of a skill is made as it is read: its secrets in
// dots, and of a long SKILL.md its head.
func TestShown(t *testing.T) {
	w := newWorld(t)
	const key = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	dir := skill(t, filepath.Join(w.home, ".claude", "skills"), "call", "uses "+key)
	long := "---\nname: call\ndescription: uses " + key + "\n---\n" + key + "\n" + strings.Repeat("字", 2*ShownLen)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}
	e := Scan(w.env).Entry("g/call")
	if strings.Contains(e.About+e.Shown, key) || !strings.Contains(e.About, "uses ") || !e.Secret {
		t.Fatalf("not masked: %q %q", e.About, e.Shown[:200])
	}
	if n := len([]rune(e.Shown)); n != ShownLen+2 || !strings.HasSuffix(e.Shown, "\n…") {
		t.Fatalf("its head: %d characters", n)
	}
	if !strings.Contains(e.Raw, key) || !strings.Contains(e.Description, key) {
		t.Fatal("as written no more")
	}
}
