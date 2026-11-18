package main

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/resumecover"
)

// A phone's app put away shows, as it comes back, the picture it last
// had until it has drawn again (internal/resumecover): Android makes the
// window's surface anew, and it stood black a moment, then white once the
// window had a color (0.108.9); the user asked for the last picture, as
// the system's apps show (0.108.10).

// coverLift is how many frames, and how long, the app draws after coming
// back before its picture is taken away: the first may come before the
// surface is there.
const (
	coverLiftFrames = 3
	coverLiftAfter  = 120 * time.Millisecond
)

// resumeCover is the cover on Android, watching; nil elsewhere.
func resumeCover() resumecover.Cover {
	cv, err := resumecover.NewCover()
	if err != nil || cv.Watch() != nil {
		return nil
	}
	return cv
}

// coverBack starts counting the frames drawn since the app came back.
func (a *app) coverBack() {
	if a.cover != nil {
		a.coverFrames, a.coverSince = coverLiftFrames, time.Time{}
	}
}

// coverTick, each frame, takes the picture away once the app has drawn
// for a while since it came back.
func (a *app) coverTick(c *ui.Context) {
	if a.cover == nil || a.coverFrames <= 0 {
		return
	}
	if a.coverSince.IsZero() {
		a.coverSince = c.Now()
	}
	a.coverFrames--
	if a.coverFrames > 0 || c.Now().Sub(a.coverSince) < coverLiftAfter {
		a.coverFrames = max(a.coverFrames, 1)
		c.After(16 * time.Millisecond)
		return
	}
	a.coverFrames = 0
	a.cover.Lift()
}
