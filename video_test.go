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
	"github.com/egoist/mygo/ui/video"
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
	// At rest a card opens no player: its poster, taken once.
	if a.vids["card:"+clip.ID] != nil {
		t.Fatal("a player on a card not pointed at")
	}
	for i := 0; i < 200 && len(a.grabs) > 0; i++ {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	tt.Frame()
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
	if clip.Organized == nil || len(a.grabs) > 0 {
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
	} else if p, err := video.Open("testdata/clip.mp4"); err == nil {
		p.Close()
		t.Fatal("the system plays video, and no picture was taken of it")
	} else if !strings.Contains(string(req), "画面没能读到") {
		t.Fatalf("not told the pictures were not seen: %s", req)
	} else if _, kept := a.vidShots[clip.ID]; kept {
		t.Fatal("pictures not taken were kept: they would not be tried again")
	}
	// Its poster is kept with the cache, and its card shows it.
	if len(shots.pics) > 0 {
		if _, err := os.Stat(posterPath(clip)); err != nil || a.videoLen(clip) <= 0 {
			t.Fatalf("no poster: %v, length %v", err, a.videoLen(clip))
		}
	}
	if clip.Title != "彩条测试画面" || clip.Desc != "一段彩色竖条和计时器的测试视频。" || !strings.Contains(clip.Text, "计时器") {
		t.Fatalf("title %q desc %q text %q", clip.Title, clip.Desc, clip.Text)
	}
	if clip.Summary != "" || clip.Analysis != "" {
		t.Fatalf("what is not known was kept: %q %q", clip.Summary, clip.Analysis)
	}
}

// TestShortClipsStayShort: of a line, the AI keeps a title, tags and the
// words it is found by; what it says of what it does not know goes, a
// sentence at a time, and only of its own answer: content that speaks of
// what cannot be decided stays, and so does a description the user wrote.
func TestShortClipsStayShort(t *testing.T) {
	c := &library.Clip{Kind: library.KindLink, URL: "https://example.com", Text: "一句话的网页。"}
	applyOrganized(c, &clipOrganized{Title: "一页", Desc: "这是一个网页，无法判断它的用途。", Summary: "它讲了一句话。", Analysis: "- 现状\n- 建议", Keywords: []string{"网页"}}, testNow)
	if c.Title != "一页" || c.Summary != "" || c.Analysis != "" || c.Desc != "这是一个网页。" {
		t.Fatalf("%+v", c)
	}
	for _, s := range []string{"讲为什么量子态无法确定的科普文章", "合同条款，需确认付款日期", "介绍没有信息差时如何定价", "模型无法从少量样本泛化",
		// what the second review found dropped
		"作者认为目前难以判断其长期影响。", "医生无法确定这是否是过敏。", "数据库损坏后无法从文件恢复。", "研究者不能确定其来源。", "难以看出这段代码的问题", "用户难以判断内容真假",
		// and the third
		"该视频介绍了用户不能登录时的排查方法。", "这个视频讲的是为什么程序无法启动。", "该图片展示了手机无法连网时的设置页面", "此文件夹无法删除的解决办法",
		"系统无法确定该文件的编码时会回退到 UTF-8。", "这条笔记记录了服务器不能启动的排查过程，最后是端口冲突", "无法判断它是病毒还是细菌"} {
		if fillerSaid(s) {
			t.Errorf("content taken for filler: %q", s)
		}
	}
	for _, s := range []string{"目前无法从文件名判断它拍的是什么、来自哪里、有什么用。", "需要打开播放并补充一句说明", "内容还需打开确认", "无法判断这个视频的内容",
		"这条资料暂时无法归类", "无法判断它的用途。", "无法判断它是什么。", "无法确定视频内容。", "从文件名无法判断用途。", "仅凭文件名无法得知内容。",
		"文件名没有提供任何信息。", "在确认内容前，无法判断它是素材、录屏还是教程", "这个视频看不出来源。"} {
		if !fillerSaid(s) {
			t.Errorf("filler not seen: %q", s)
		}
	}
	for in, want := range map[string]string{
		"一段讲缓存的文章。目前无法从文件名判断它的用途。适合后端。":    "一段讲缓存的文章。适合后端。",
		"这篇文章讲缓存，分为三部分，但无法判断这个文件的内容":       "这篇文章讲缓存，分为三部分。",
		"- 先更新库再删缓存\n- 需要打开查看原文\n- 过期时间兜底": "- 先更新库再删缓存\n- 过期时间兜底",
		"讲了缓存；这个视频看不出来源。":                  "讲了缓存。",
		"这个视频无法判断，但讲了 TTL。":                "讲了 TTL。",
		"1. 先更新库\n2. 需要打开查看\n3. 过期兜底":      "1. 先更新库\n3. 过期兜底",
		"- 需要打开确认\n  续行：删缓存\n- 兜底":         "- 兜底",
	} {
		if got := dropFiller(in); got != want {
			t.Errorf("dropFiller(%q) = %q, want %q", in, got, want)
		}
	}
	// Organized again, a description the user wrote stays when the answer
	// has none, filler or not.
	mine := &library.Clip{Kind: library.KindText, Text: strings.Repeat("长文", 300), Desc: "我写的：需要打开查看的那份合同"}
	applyOrganized(mine, &clipOrganized{Title: "合同", Desc: "无法判断它的用途。"}, testNow)
	if mine.Desc != "我写的：需要打开查看的那份合同" {
		t.Fatalf("the user's description: %q", mine.Desc)
	}
}

// TestVideoPosterGrabGivesWay: organizing a video whose poster is being
// taken, as when 重新整理 is pressed on a wall of videos new to posters,
// gives the AI all its pictures, not the poster's one; and the job is
// counted once however often it is asked for (the review of 0.98.3).
func TestVideoPosterGrabGivesWay(t *testing.T) {
	if p, err := video.Open("testdata/clip.mp4"); err != nil {
		t.Skip("no video here:", err)
	} else {
		p.Close()
	}
	a, tt := newTestApp(t)
	fake := &fakeClaude{replies: []string{claudeText(`{"title": "彩条", "category": "资源", "tags": [], "keywords": [], "synonyms": [], "text": "彩条"}`)}}
	a.sendAI = fake.send
	a.cfg.AIKey = "sk-ant-test"
	a.cfg.LibraryManual = true
	clip, err := a.addClipFile("testdata/clip.mp4", "拖入")
	if err != nil {
		t.Fatal(err)
	}
	a.showLibrary()
	// The wall asks for its poster; frames till that grab has its times.
	for i := 0; i < 200 && (len(a.grabs) == 0 || a.grabs[0].at == nil); i++ {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	if len(a.grabs) == 0 || a.grabs[0].forAI || len(a.grabs[0].at) != 1 {
		t.Fatalf("no poster's grab under way: %d", len(a.grabs))
	}
	a.organizeClip(clip.ID)
	a.organizeClip(clip.ID) // asked again meanwhile: one job
	if a.orgTotal != 1 {
		t.Fatalf("the job counted %d times", a.orgTotal)
	}
	for i := 0; i < 400 && clip.Organized == nil; i++ {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	if clip.Organized == nil {
		t.Fatal("not organized")
	}
	req, _ := json.Marshal(fake.requests[0].Messages)
	if n := strings.Count(string(req), `"type":"image"`); n != grabShots {
		t.Fatalf("the AI was sent %d pictures, not %d", n, grabShots)
	}
}
