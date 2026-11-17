package main

import (
	"fmt"
	"log"
	"runtime/debug"
)

// Work done off the window's thread (app.async) runs in a goroutine of
// its own, where a panic ends the whole app: a page that would not parse
// did, with whatever was not saved. guarded runs such work and hands a
// panic back as an error, which is said where the work's errors are and
// in a toast, and logged with where it came from: it is a fault still,
// to be found, not one to lose what the user has open for.

// workPanic is a panic of work done off the window's thread.
type workPanic struct{ what any }

func (p *workPanic) Error() string { return fmt.Sprintf("出错了：%v", p.what) }

// guarded runs work, a panic in it returned as a *workPanic.
func guarded(work func() (any, error)) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("后台任务出错：%v\n%s", r, debug.Stack())
			v, err = nil, &workPanic{r}
		}
	}()
	return work()
}

// panicked says a panic of work done off the window's thread, before the
// work's own end is told of it: what that says after, if anything, is
// said in its place.
func (a *app) panicked(err error) {
	if p, ok := err.(*workPanic); ok {
		a.tell(p.Error())
	}
}
