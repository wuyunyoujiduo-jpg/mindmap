package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// A video kept in 收集 is a file, as it was before: what tells it is its
// name's ending (isVideo). It shows as one where it is looked at: on its
// card its first picture, which plays without sound under the pointer, as
// a grid of videos does; in its details a player with its sound, its keys
// and its time; larger over the window (videoBig). The user dropped one in
// and saw a card with a random name and nothing of it, the AI having had
// nothing but that name.
//
// The players are the system's (MyGo's ui/video: AVFoundation on macOS),
// opened as they are first shown and closed once they are not, a frame
// later (sweepVideos): a wall scrolled past keeps none open.

// videoExts are the endings of the files that are videos.
var videoExts = []string{".mp4", ".mov", ".m4v", ".webm", ".mkv", ".avi", ".3gp"}

// isVideo tells whether a clip is a video: a file of a video's ending.
func isVideo(c *library.Clip) bool {
	return c.Kind == library.KindFile && slices.Contains(videoExts, strings.ToLower(filepath.Ext(cmpOr(c.File, c.Name))))
}

// clipKindLabel is the word of a clip's kind, a video's 视频.
func clipKindLabel(c *library.Clip) string {
	if isVideo(c) {
		return "视频"
	}
	return library.KindLabel(c.Kind)
}

// vidPlayer is a player of the app's: the system's, or why there is none.
type vidPlayer struct {
	p      *ui.VideoPlayer
	err    error
	used   int  // the frame it was last shown in
	posed  bool // its first picture shows, held: what a card shows at rest
	played bool // played by the user: no more held at its first picture
}

// broken tells what keeps it from showing, or nothing.
func (v *vidPlayer) broken() error {
	if v.err != nil {
		return v.err
	}
	return v.p.Err()
}

// videoOf is the player of key, opened on the file at path when there is
// none: without sound, which the details give it when it is played.
func (a *app) videoOf(key, path string) *vidPlayer {
	if a.vids == nil {
		a.vids = map[string]*vidPlayer{}
	}
	v := a.vids[key]
	if v == nil {
		v = &vidPlayer{}
		if path == "" {
			v.err = fmt.Errorf("文件不见了")
		} else if p, err := video.Open(path); err != nil {
			v.err = err
		} else {
			v.p = p.SetMuted(true)
		}
		a.vids[key] = v
	}
	v.used = a.vidFrame
	return v
}

// sweepVideos closes the players not shown in the frame before: called as
// a frame begins.
func (a *app) sweepVideos() {
	for k, v := range a.vids {
		if v.used < a.vidFrame {
			if v.p != nil {
				v.p.Close()
			}
			delete(a.vids, k)
		}
	}
	a.vidFrame++
}

// closeVideos closes the players of a clip now, not a frame on: its file
// is about to be moved away, which a system that holds an open file in
// its place (Windows) refuses.
func (a *app) closeVideos(id string) {
	for _, k := range []string{"card:" + id, "play:" + id} {
		if v := a.vids[k]; v != nil {
			if v.p != nil {
				v.p.Close()
			}
			delete(a.vids, k)
		}
	}
}

// holdFirst keeps a player at its first picture: once it has one, paused
// and back at its start. It plays till then, without sound, for a player
// paused before its first picture shows none.
func holdFirst(v *vidPlayer) {
	switch {
	case !v.p.Ready():
	case !v.posed:
		v.p.Pause()
		v.p.Seek(0)
		v.posed = true
	case v.p.Playing():
		v.p.Pause()
		v.p.Seek(0)
	}
}

// videoTime says a length as a clock does: 1:05, 1:02:05.
func videoTime(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// videoCard is a video's face on its card: its poster, a picture taken
// of it once and kept (read away from the window's work, as every
// picture of a card), a round key of play over it and its length at its
// corner; under the pointer, a player of it, without sound. A player on
// each card of the wall was heavy, the review found: the wall builds all
// its cards, scrolled past or not.
func (a *app) videoCard(c *ui.Context, clip *library.Clip, hovered bool) {
	poster := posterPath(clip)
	a.lookPoster(clip)
	var pic *ui.Bitmap
	if a.vidPosterHas[clip.ID] {
		pic, _ = a.cardPicture(poster)
	}
	ui.Box(c.Key("video-face")).Height(110).FillWidth().Radius(8).Clip().Background(ui.Hex("#1d1d1f")).PassThrough().Children(func() {
		if pic != nil {
			ui.Image(c, pic).Fill().Fit(ui.Cover).PassThrough()
		}
		playing := false
		if hovered {
			if v := a.videoOf("card:"+clip.ID, a.lib.FilePath(clip)); v.broken() == nil {
				ui.Video(c, v.p).Absolute().Left(0).Top(0).Right(0).Bottom(0).Fit(ui.Cover).PassThrough()
				if v.p.Ready() && !v.p.Playing() {
					v.p.SetLoop(true).Play()
				}
				playing = v.p.Ready()
			}
		}
		if !playing {
			ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().PassThrough().Children(func() {
				ui.Box(c).Size(34, 34).Radius(17).Background(ui.Hex("#000000").Alpha(0.45)).Center().PassThrough().Children(func() {
					glyph(c, iconPlay, 13, ui.Hex("#ffffff"))
				})
			})
		}
		if d := a.videoLen(clip); d > 0 {
			ui.Box(c).Absolute().Right(6).Bottom(6).Padding(1, 6).Radius(5).Background(ui.Hex("#000000").Alpha(0.55)).PassThrough().Children(func() {
				ui.Text(c, videoTime(d)).FontSize(textCaption).TextColor(ui.Hex("#ffffff")).NoWrap()
			})
		}
	})
}

// videoDetail is a video in its details: a player, a click on its picture
// playing or pausing it, and under it its keys: play, the time and where
// it is, which a drag moves, its sound, and larger.
func (a *app) videoDetail(c *ui.Context, clip *library.Clip) {
	if a.videoBig == clip.ID {
		// Shown larger over the window: one picture of it at a time.
		t := c.Theme()
		ui.Box(c.Key("video-away")).Height(56).FillWidth().Radius(10).Background(blockFill(t)).Center().Children(func() {
			ui.Text(c, "正在放大播放").FontSize(textSmall).TextColor(t.TextMuted)
		})
		return
	}
	a.videoPlayer(c, clip, 600, 340, false) // narrowed to the column
}

// videoPlayer draws the player of a clip's video in a room w by h at
// most: its picture as large as its shape allows, and its keys under it.
func (a *app) videoPlayer(c *ui.Context, clip *library.Clip, w, h float32, big bool) {
	t := c.Theme()
	v := a.videoOf("play:"+clip.ID, a.lib.FilePath(clip))
	if err := v.broken(); err != nil {
		ui.Column(c.Key("video-broken")).Gap(4).FillWidth().Padding(14).Radius(10).Background(blockFill(t)).Children(func() {
			ui.Text(c, "这个视频在这里放不了").FontSize(textSmall).FontWeight(700)
			ui.Text(c, "可能是系统不支持它的编码。可以点下面的文件名，用默认应用打开。").FontSize(textCaption).TextColor(t.TextMuted).Wrap()
		})
		return
	}
	if !v.played {
		holdFirst(v)
	}
	// As wide as the room, or as its height allows its shape: in the
	// details, as wide as the column, whatever its width.
	vw, vh := v.p.Size()
	shape := float32(16) / 9
	if vw > 0 && vh > 0 {
		shape = float32(vw) / float32(vh)
	}
	pw := min(w, h*shape)
	toggle := func() {
		if v.p.Playing() && v.played {
			v.p.Pause()
			return
		}
		v.played = true
		v.p.SetMuted(a.videoMuted).SetLoop(false)
		v.p.Play()
	}
	ui.Column(c.Key("video-player")).Gap(8).AlignItems(ui.Center).FillWidth().Children(func() {
		pic := ui.Box(c.Key("video-picture")).Width(pw).MaxWidthPercent(100).AspectRatio(shape).Radius(10).Clip().Background(ui.Hex("#000000")).Cursor(ui.CursorPointer).
			Label("视频画面").Tooltip("点一下播放或暂停")
		if pic.Clicked() {
			toggle()
		}
		pic.Children(func() {
			ui.Video(c, v.p).Fill().Fit(ui.Contain).Label("视频 " + clip.Label()).PassThrough()
			if !v.played || !v.p.Playing() {
				ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().PassThrough().Children(func() {
					ui.Box(c).Size(48, 48).Radius(24).Background(ui.Hex("#000000").Alpha(0.45)).Center().PassThrough().Children(func() {
						glyph(c, iconPlay, 18, ui.Hex("#ffffff"))
					})
				})
			}
			if v.p.Buffering() || !v.p.Ready() {
				ui.Spinner(c).Size(18, 18).Absolute().Right(10).Top(10)
			}
		})
		ui.Row(c.Key("video-keys")).Gap(8).AlignItems(ui.Center).Width(pw).MaxWidthPercent(100).Children(func() {
			playing := v.played && v.p.Playing()
			name, tip := "播放", "播放"
			if playing {
				name, tip = "暂停", "暂停"
			}
			key := ui.Box(c.Key("video-play")).Size(28, 28).Radius(radiusSmall).Center().Cursor(ui.CursorPointer).Label(name).Tooltip(tip)
			if key.Hovered() {
				key.Background(t.Text.Alpha(0.06))
			}
			if key.Clicked() {
				toggle()
			}
			key.Children(func() {
				if playing {
					ui.Row(c).Gap(3).PassThrough().Children(func() {
						ui.Box(c).Size(3, 11).Radius(1).Background(t.Text)
						ui.Box(c).Size(3, 11).Radius(1).Background(t.Text)
					})
				} else {
					glyph(c, iconPlay, 11, t.Text)
				}
			})
			pos, all := v.p.Position(), v.p.Duration()
			ui.Text(c, videoTime(pos)).FontSize(textCaption).TextColor(t.TextMuted).NoWrap().Shrink(0)
			at := pos.Seconds()
			if ui.Slider(c.Key("video-seek"), &at, 0, max(all.Seconds(), 0.001)).Label("播放位置").Grow(1).Shrink(1).MinWidth(40).Changed() {
				v.played = true // a drag is the user's: the picture is no more held
				v.p.Seek(time.Duration(at * float64(time.Second)))
			}
			ui.Text(c, videoTime(all)).FontSize(textCaption).TextColor(t.TextMuted).NoWrap().Shrink(0)
			word, tip := "静音", "关掉声音"
			if a.videoMuted {
				word, tip = "开声音", "打开声音"
			}
			if textKey(c, word, "声音", tip).Clicked() {
				a.videoMuted = !a.videoMuted
				if v.played {
					v.p.SetMuted(a.videoMuted)
				}
			}
			if !big {
				if textKey(c, "放大", "放大播放", "放大播放").Clicked() {
					a.videoBig = clip.ID
				}
			}
		})
	})
}

// videoBigView is a video larger, over the window: Escape or a click
// beside it closes it, and it plays on in the details.
func (a *app) videoBigView(c *ui.Context) {
	clip := a.lib.Find(a.videoBig)
	if clip == nil || !isVideo(clip) {
		a.videoBig = ""
		return
	}
	w, h := c.Size()
	shade := ui.Box(c.Key("video-big")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(ui.RGBA(0, 0, 0, 0.82)).Center().
		Focusable().FocusRing(false).AutoFocus().Label("放大播放")
	if shade.Clicked() || shade.Shortcut(0, ui.KeyEscape) || shade.OverlayShortcut(0, ui.KeyEscape) {
		a.videoBig = ""
	}
	shade.Children(func() {
		ui.Column(c.Key("video-big-room")).Padding(16).Radius(14).Background(c.Theme().Background).Children(func() {
			a.videoPlayer(c, clip, min(w-120, 1280), min(h-160, 760), true)
		})
	})
}

// The pictures of a video are taken away from the clip's job, in a queue
// of their own (a.grabs), a step at each of the window's frames: a player
// of its own shown in a window that is not on the screen (a Tester, which
// draws on the CPU what the system decoded), opened, its first picture
// waited for, then each time gone to and what shows kept once it changed.
// Of a video to organize, a few for the AI (grabShots), the first of
// them its poster; of a video only shown, its poster. Its time is counted
// while the window shows, for a window not seen draws no frame; and what
// could not be seen is not kept, so that it is tried again. The review of
// 0.98.0 found the clip queue held for the length of a grab, a grab timed
// out on a window put behind another, and that failure kept.

// vidShots are the pictures taken of a video for the AI, kept till the
// app is closed, and its length.
type vidShots struct {
	pics [][]byte // JPEGs, in time's order
	dur  time.Duration
}

// say tells the AI what it is sent of the video.
func (s vidShots) say() string {
	if len(s.pics) == 0 {
		return ""
	}
	return fmt.Sprintf("时长 %s，附 %d 张按时间顺序截取的画面", videoTime(s.dur), len(s.pics))
}

// frameGrab is the taking of a video's pictures, a step a frame.
type frameGrab struct {
	id     string
	forAI  bool // the clip's organizing waits for it, and goes on once done
	p      *ui.VideoPlayer
	tt     *ui.Tester
	at     []time.Duration
	i      int
	steps  int
	spent  time.Duration // the time it was stepped, the window showing
	lastAt time.Time
	last   uint64 // what the picture before showed, to tell a new one
	shots  vidShots
}

// grabShots is how many pictures the AI sees of a video.
const grabShots = 4

// queueGrab asks for a video's pictures: for the AI, or its poster alone.
// One for the AI goes ahead of the posters (a wall of new videos queues
// one each), and a poster's grab asked for by the AI takes its pictures
// again: its one picture was given the AI for four (the review of 0.98.3).
func (a *app) queueGrab(id string, forAI bool) {
	for k, g := range a.grabs {
		if g.id != id {
			continue
		}
		if forAI && !g.forAI {
			// Its times again, for the AI, and its time from now; not yet
			// stepped, it goes where one for the AI goes (the review of
			// 0.99.1: it waited behind the posters).
			g.forAI = true
			g.at, g.i, g.steps, g.shots.pics, g.spent = nil, 0, 0, nil, 0
			if g.p == nil {
				a.grabs = slices.Delete(a.grabs, k, k+1)
				a.grabs = slices.Insert(a.grabs, a.aiGrabAt(), g)
			}
		}
		return
	}
	g := &frameGrab{id: id, forAI: forAI}
	at := len(a.grabs)
	if forAI {
		at = a.aiGrabAt()
	}
	a.grabs = slices.Insert(a.grabs, at, g)
}

// aiGrabAt is where a grab for the AI goes in the queue: after those for
// the AI, and after the one being stepped.
func (a *app) aiGrabAt() int {
	at := 0
	for at < len(a.grabs) && (a.grabs[at].forAI || a.grabs[at].p != nil) {
		at++
	}
	return at
}

// grabbingForAIOf tells whether a clip's organizing waits on its pictures.
func (a *app) grabbingForAIOf(id string) bool {
	for _, g := range a.grabs {
		if g.id == id && g.forAI {
			return true
		}
	}
	return false
}

// grabbingForAI tells whether a clip's organizing waits on its pictures.
func (a *app) grabbingForAI() bool {
	for _, g := range a.grabs {
		if g.forAI {
			return true
		}
	}
	return false
}

// stepGrab takes a step of the pictures being taken: at each of the
// window's frames while it shows.
func (a *app) stepGrab(c *ui.Context) {
	if len(a.grabs) == 0 || c.WindowHidden() {
		return
	}
	g := a.grabs[0]
	done := func() {
		if g.p != nil {
			g.p.Close()
		}
		a.grabs = a.grabs[1:]
		cl := a.lib.Find(g.id)
		if cl != nil && len(g.shots.pics) > 0 {
			a.savePoster(cl, g.shots.pics[0], g.shots.dur)
		}
		if !g.forAI {
			return
		}
		if a.vidShots == nil {
			a.vidShots = map[string]vidShots{}
		}
		a.vidShots[g.id] = g.shots // used once when empty: tried again after
		// Its organizing, which waited, goes on first.
		a.libQueue = append([]clipJob{{g.id, true}}, a.libQueue...)
		a.nextClipJob()
	}
	now := time.Now()
	if !g.lastAt.IsZero() {
		g.spent += min(now.Sub(g.lastAt), 100*time.Millisecond) // a gap is time not shown
	}
	g.lastAt = now
	if g.p == nil {
		cl := a.lib.Find(g.id)
		if cl == nil {
			delete(a.libStatus, g.id)
			done()
			return
		}
		p, err := video.Open(a.lib.FilePath(cl))
		if err != nil {
			done()
			return
		}
		g.p = p.SetMuted(true)
		g.tt = ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Fill().Fit(ui.Contain) }, 512, 288)
	}
	if g.spent > 20*time.Second || g.p.Err() != nil {
		done()
		return
	}
	g.tt.Frame()
	if !g.p.Ready() {
		return
	}
	if g.at == nil {
		// The times: the middles of the parts, so that neither the black
		// of its start nor its end is taken; the poster alone, at a tenth.
		g.shots.dur = g.p.Duration()
		if vw, vh := g.p.Size(); vw > 0 && vh > 0 {
			k := min(1, 512/float64(max(vw, vh)))
			g.tt.SetSize(max(int(float64(vw)*k), 16), max(int(float64(vh)*k), 16))
		}
		if g.forAI {
			for k := range grabShots {
				g.at = append(g.at, time.Duration(float64(g.shots.dur)*(float64(k)+0.5)/grabShots))
			}
		} else {
			g.at = []time.Duration{g.shots.dur / 10}
		}
		g.p.Pause()
		g.tt.Frame()
		g.last = pictureSum(g.tt.Image()) // what shows before the seek
		g.p.Seek(g.at[0])
		return
	}
	g.steps++
	if g.steps < 4 {
		return // the seek's picture has not come yet
	}
	img := g.tt.Image()
	sum := pictureSum(img)
	if sum == g.last && g.steps < 60 {
		return // the picture before still shows
	}
	g.last = sum
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 72}); err == nil {
		g.shots.pics = append(g.shots.pics, buf.Bytes())
	}
	g.i++
	if g.i == len(g.at) {
		done()
		return
	}
	g.steps = 0
	g.p.Seek(g.at[g.i]) // g.last is what shows till its picture comes
}

// pictureSum tells pictures apart: a sum of a few of their pixels.
func pictureSum(img *image.RGBA) uint64 {
	var s uint64
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += max(b.Dy()/16, 1) {
		for x := b.Min.X; x < b.Max.X; x += max(b.Dx()/16, 1) {
			p := img.RGBAAt(x, y)
			s = s*31 + uint64(p.R)<<16 + uint64(p.G)<<8 + uint64(p.B)
		}
	}
	return s
}

// posterPath is where a video's poster is kept: with the app's cache, not
// the library, which is synced. Its length is in a file beside it.
func posterPath(c *library.Clip) string { return posterPathOf(c.ID) }

func posterPathOf(id string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "MyMind", "posters", id+".jpg")
}

// clipGone clears what the app keeps of a clip beside the library, once
// the clip is deleted, here or elsewhere: a video's poster and its
// length, in the app's cache, which stayed there for good. Of a clip
// taken back the poster is taken again.
func (a *app) clipGone(id string) {
	delete(a.vidPosterAsked, id)
	delete(a.vidPosterHas, id)
	delete(a.vidLens, id)
	delete(a.vidShots, id)
	p := posterPathOf(id)
	if p == "" {
		return
	}
	a.forgetPicture(p)
	a.async(func() (any, error) {
		_ = os.Remove(p)
		_ = os.Remove(strings.TrimSuffix(p, ".jpg") + ".len")
		return nil, nil
	}, func(any, error) {})
}

// savePoster keeps a video's poster and its length.
func (a *app) savePoster(c *library.Clip, pic []byte, dur time.Duration) {
	a.posterMaps()
	a.vidLens[c.ID] = dur
	p := posterPath(c)
	if p == "" || os.MkdirAll(filepath.Dir(p), 0o755) != nil {
		return
	}
	if os.WriteFile(p, pic, 0o644) == nil {
		_ = os.WriteFile(strings.TrimSuffix(p, ".jpg")+".len", []byte(strconv.FormatInt(int64(dur/time.Millisecond), 10)), 0o644)
		a.forgetPicture(p) // read as missing before: read again
		a.vidPosterHas[c.ID] = true
	}
}

// videoLen is a video's length, as kept with its poster: 0 when unknown
// (lookPoster reads it).
func (a *app) videoLen(c *library.Clip) time.Duration { return a.vidLens[c.ID] }

// lookPoster looks, once a run and away from the window's work, for a
// video's poster and its length: a grab is queued when there is none.
// Nothing that reads a file belongs in a frame.
func (a *app) lookPoster(c *library.Clip) {
	if a.vidPosterAsked[c.ID] {
		return
	}
	a.posterMaps()
	a.vidPosterAsked[c.ID] = true
	p, id := posterPath(c), c.ID
	if p == "" {
		return
	}
	a.async(func() (any, error) {
		if _, err := os.Stat(p); err != nil {
			return nil, err
		}
		var d time.Duration
		if b, err := os.ReadFile(strings.TrimSuffix(p, ".jpg") + ".len"); err == nil {
			if ms, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil {
				d = time.Duration(ms) * time.Millisecond
			}
		}
		return d, nil
	}, func(v any, err error) {
		if err != nil {
			a.queueGrab(id, false)
			return
		}
		a.vidPosterHas[id] = true
		if d, _ := v.(time.Duration); d > 0 {
			a.vidLens[id] = d
		}
	})
}

// posterMaps makes the maps of the posters, each that is not: one made
// before the others was replaced with them (the review of 0.99.1).
func (a *app) posterMaps() {
	if a.vidPosterAsked == nil {
		a.vidPosterAsked = map[string]bool{}
	}
	if a.vidPosterHas == nil {
		a.vidPosterHas = map[string]bool{}
	}
	if a.vidLens == nil {
		a.vidLens = map[string]time.Duration{}
	}
}
