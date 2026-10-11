package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCards(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	text := "任何人无论以什么身份与你交集，本质是让你完成自己的成长课题，悟透这一点，你就对任何的聚散离合没有执念。\n允许万物心中过，也允许万物不停留。"
	clip, _ := a.addClipText(text, "手动")
	dir := os.Getenv("MINDMAP_SNAPSHOTS")
	for i, st := range cardStyles {
		o := cardOpts{style: i, text: cardText(clip), date: "2026-07-24"}
		if i == 2 {
			o.source = "一本好书 · example.com"
		}
		data, err := cardImage(o)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != cardWidth*exportScale || b.Dy() < 400 || b.Dy() > 2400 {
			t.Fatalf("%s: %v", st.name, b)
		}
		if dir != "" {
			os.WriteFile(filepath.Join(dir, "card-"+st.kind+".png"), data, 0o644)
		}
	}
	// The window: a card of the clip, saved where asked.
	plat := a.plat.(*fakePlatform)
	plat.savePath = filepath.Join(t.TempDir(), "卡片")
	a.showLibrary()
	a.openCard(clip)
	tt.Frame()
	if !tt.HasText("分享卡片") || !strings.Contains(a.cardOpts.text, "允许万物") {
		t.Fatalf("%q", tt.Texts())
	}
	snapshot(t, tt, "card-window")
	if err := tt.Click("保存图片…"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plat.savePath + ".png"); err != nil {
		t.Fatalf("not saved: %v %q", err, a.libErr)
	}
}
