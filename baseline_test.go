package main

import (
	"os/exec"
	"testing"
)

// The canvas panel records the commit of the project's code the map
// matches.
func TestRecordBaseline(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "初始化项目"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	a, tt := newTestApp(t)
	a.file.Project = repo
	a.panelTab = tabCanvas
	tt.Frame()
	for i := 0; i < 30; i++ {
		tt.Scroll(1100, 500, 0, 300)
		tt.Frame()
	}
	if err := tt.Click("记录当前版本"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.file.Baseline == nil || a.file.Baseline.Subject != "初始化项目" {
		t.Fatalf("%+v", a.file.Baseline)
	}
	tt.Frame()
	for i := 0; i < 30; i++ {
		tt.Scroll(1100, 500, 0, 300)
		tt.Frame()
	}
	snapshot(t, tt, "baseline")
}
