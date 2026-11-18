// Package resumecover keeps an Android app's last picture over its window
// while it is away, and as it comes back until it has drawn again: Android
// makes a SurfaceView's surface anew as the app returns, and MyGo draws
// into it a frame or more later, the window black or bare meanwhile (the
// user's, 0.108.10: "应该有个最后的快照图吧"). It is a native module of
// MyGo, its Kotlin in kotlin/; elsewhere NewCover returns
// errors.ErrUnsupported.
package resumecover

// Cover lays the window's last picture over it.
//
//mygo:native
//mygo:mainthread
type Cover interface {
	// Watch has the picture taken each time the app is put away, as the
	// activity pauses with its surface still there, and laid over the
	// window; it goes by itself a while after the app comes back, if Lift
	// is not called first.
	Watch() error
	// Lift takes the picture away, the window having drawn again.
	Lift()
}
