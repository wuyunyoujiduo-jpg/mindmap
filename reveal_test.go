package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The key that shows a file where it is kept names the place as the
// system does: 访达 is the Mac's, and the app ships for Windows too.
func TestRevealWord(t *testing.T) {
	for goos, want := range map[string]string{"darwin": "在访达中显示", "windows": "在资源管理器中显示", "linux": "在文件夹中显示"} {
		if got := revealOn(goos); got != want {
			t.Errorf("%s: %q", goos, got)
		}
	}
	if revealWord() != revealOn(runtime.GOOS) {
		t.Fatal("not this system's")
	}
	// 技能 says it so: where the store is, and of a skill chosen.
	a, tt := newTestApp(t)
	writeSkill(t, filepath.Join(os.Getenv("HOME"), ".claude", "skills"), "deploy", "发布到预发环境")
	tt.Frame()
	if err := tt.Click("技能"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if err := tt.Click("deploy"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	n := 0
	for _, s := range tt.Texts() {
		if s == revealWord() {
			n++
		}
		if runtime.GOOS != "darwin" && strings.Contains(s, "访达") {
			t.Fatalf("访达 where there is none: %q", s)
		}
	}
	if n != 2 {
		t.Fatalf("%d keys say %q: %q", n, revealWord(), tt.Texts())
	}
	_ = a
}
