package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
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

// TestVideoSeenByTheAI: a video is organized from a few of its pictures,
// taken across the window's frames; its title, a random file name, gives
// way to the AI's, and what the AI says of what it cannot know is not
// kept. Where no picture can be taken, the AI is told so, and asked once.
func TestVideoSeenByTheAI(t *testing.T) {
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText(`{"title": "彩条测试画面", "desc": "一段彩色竖条和计时器的测试视频。", "summary": "这是一个无法判断用途的视频，需要打开确认。", "category": "资源", "tags": ["视频"], "keywords": ["彩条"], "synonyms": ["测试图"], "analysis": "- 现状：无法判断", "text": "彩色竖条，左上角有计时器，从 0 秒走到 2 秒。"}`)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	a.cfg.LibraryManual = true // organized when asked, below
	clip, err := a.addClipFile("testdata/clip.mp4", "拖入")
	if err != nil {
		t.Fatal(err)
	}
	if clip.Title != "clip" {
		t.Fatalf("title %q", clip.Title)
	}
	a.showLibrary()
	a.organizeClip(clip.ID)
	for i := 0; i < 400 && clip.Organized == nil; i++ {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	if clip.Organized == nil || a.grab != nil {
		t.Fatalf("not organized: %q", a.libStatus[clip.ID])
	}
	shots := a.vidShots[clip.ID]
	req, _ := json.Marshal(fake.requests[0].Messages)
	images := strings.Count(string(req), `"type":"image"`)
	t.Logf("pictures taken: %d, sent: %d", len(shots.pics), images)
	if len(shots.pics) > 0 {
		if images != len(shots.pics) || !strings.Contains(string(req), "按时间顺序截取的画面") {
			t.Fatalf("%d pictures sent of %d: %s", images, len(shots.pics), req)
		}
		if len(shots.pics) != grabShots {
			t.Fatalf("%d pictures, not %d", len(shots.pics), grabShots)
		}
		if dir := os.Getenv("MINDMAP_SNAPSHOTS"); dir != "" {
			for i, p := range shots.pics {
				_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("video-shot-%d.jpg", i)), p, 0o644)
			}
		}
	} else if !strings.Contains(string(req), "画面没能读到") {
		t.Fatalf("not told the pictures were not seen: %s", req)
	}
	if clip.Title != "彩条测试画面" || clip.Desc != "一段彩色竖条和计时器的测试视频。" || !strings.Contains(clip.Text, "计时器") {
		t.Fatalf("title %q desc %q text %q", clip.Title, clip.Desc, clip.Text)
	}
	if clip.Summary != "" || clip.Analysis != "" {
		t.Fatalf("what is not known was kept: %q %q", clip.Summary, clip.Analysis)
	}
}

// TestShortClipsStayShort: of a line, the AI keeps a title, tags and the
// words it is found by, and the details fold what it wrote at length.
func TestShortClipsStayShort(t *testing.T) {
	c := &library.Clip{Kind: library.KindLink, URL: "https://example.com", Text: "一句话的网页。"}
	applyOrganized(c, &clipOrganized{Title: "一页", Desc: "这是一个网页，无法判断它的用途。", Summary: "它讲了一句话。", Analysis: "- 现状\n- 建议", Keywords: []string{"网页"}}, testNow)
	if c.Title != "一页" || c.Summary != "" || c.Analysis != "" || c.Desc != "" {
		t.Fatalf("%+v", c)
	}
}
