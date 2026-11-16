package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestVideoClip: a video dropped in 收集 is a video there: its own kind in
// the left column, its first picture on its card, a player in its details
// and larger over the window, closed by Escape before the details are.
// Where the system plays no video (a Linux without GStreamer), each says
// so, and the file still opens.
func TestVideoClip(t *testing.T) {
	a, tt := newTestApp(t)
	clip, err := a.addClipFile("testdata/clip.mp4", "拖入")
	if err != nil {
		t.Fatal(err)
	}
	if !isVideo(clip) || clipKindLabel(clip) != "视频" || ofKind(clip, kindDocs) || !ofKind(clip, kindVideos) {
		t.Fatalf("not a video: %+v", clip)
	}
	a.showLibrary()
	tt.SetSize(1280, 800)
	// The system's player opens in the background: frames till it shows.
	settle := func() {
		for i := 0; i < 40; i++ {
			tt.Frame()
			if v := a.vids["card:"+clip.ID]; v == nil || v.broken() != nil || v.posed {
				if v2 := a.vids["play:"+clip.ID]; v2 == nil || v2.broken() != nil || v2.posed {
					return
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	settle()
	if err := tt.Click("视频"); err != nil {
		t.Fatalf("no kind of videos: %v", err)
	}
	settle()
	v := a.vids["card:"+clip.ID]
	if v == nil {
		t.Fatal("no player on the card")
	}
	t.Logf("the system's player: %v", v.broken())
	snapshot(t, tt, "video-card")

	a.libSel = clip.ID
	settle()
	if v := a.vids["play:"+clip.ID]; v == nil {
		t.Fatal("no player in the details")
	} else if v.broken() == nil && v.p.Playing() && !v.played {
		t.Fatal("it plays before it is asked to")
	}
	snapshot(t, tt, "video-detail")
	if a.vids["play:"+clip.ID].broken() == nil {
		if err := tt.Click("放大"); err != nil {
			t.Fatal(err)
		}
		settle()
		snapshot(t, tt, "video-big")
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if a.videoBig != "" || a.libSel != clip.ID {
			t.Fatalf("Escape: big %q, details of %q", a.videoBig, a.libSel)
		}
	}
	// Once not shown, its players are closed.
	a.libSel, a.libKind = "", kindMemos
	tt.Frame()
	tt.Frame()
	if len(a.vids) != 0 {
		t.Fatalf("players left open: %d", len(a.vids))
	}
}
