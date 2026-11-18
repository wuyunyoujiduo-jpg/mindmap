package com.github.wuyunyoujiduo_jpg.mindmap.internal.resumecover

import android.app.Activity
import android.app.Application
import android.graphics.Bitmap
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.PixelCopy
import android.view.SurfaceView
import android.view.View
import android.view.ViewGroup
import android.widget.FrameLayout
import android.widget.ImageView
import dev.mygo.*

// The app's last picture over its window while it is away: taken from
// MyGo's SurfaceView by PixelCopy as the activity pauses, its surface still
// there, and laid over the decor view, where it covers the surface made
// anew on return until Go has drawn into it (Lift), or a moment after
// (liftAfter), whichever comes first. The picture takes no touch: a finger
// goes to what is under it.

fun makeCover(): Cover = PictureCover()

private const val liftAfter = 1500L // ms after the app is back, at most

private class PictureCover : Cover {
    private val main = Handler(Looper.getMainLooper())
    private var shown: ImageView? = null
    private var watching = false

    override fun watch() {
        if (watching) return
        watching = true
        val app = Native.context.applicationContext as Application
        app.registerActivityLifecycleCallbacks(object : Application.ActivityLifecycleCallbacks {
            override fun onActivityPaused(a: Activity) = take(a)
            override fun onActivityResumed(a: Activity) {
                main.postDelayed({ lift() }, liftAfter)
            }
            override fun onActivityCreated(a: Activity, b: Bundle?) {}
            override fun onActivityStarted(a: Activity) {}
            override fun onActivityStopped(a: Activity) {}
            override fun onActivitySaveInstanceState(a: Activity, b: Bundle) {}
            override fun onActivityDestroyed(a: Activity) {
                if (shown?.context === a) shown = null
            }
        })
    }

    override fun lift() {
        val v = shown ?: return
        shown = null
        v.animate().alpha(0f).setDuration(120).withEndAction { (v.parent as? ViewGroup)?.removeView(v) }.start()
    }

    private fun take(a: Activity) {
        val root = a.window?.decorView as? ViewGroup ?: return
        val surface = largestSurface(root) ?: return
        val w = surface.width
        val h = surface.height
        if (w <= 0 || h <= 0 || !surface.holder.surface.isValid) return
        val pic = runCatching { Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888) }.getOrNull() ?: return
        runCatching {
            PixelCopy.request(surface, pic, { result ->
                if (result == PixelCopy.SUCCESS) show(a, root, surface, pic) else pic.recycle()
            }, main)
        }.onFailure { pic.recycle() }
    }

    private fun show(a: Activity, root: ViewGroup, surface: SurfaceView, pic: Bitmap) {
        if (a.isFinishing || a.isDestroyed) return
        shown?.let { (it.parent as? ViewGroup)?.removeView(it) }
        val at = IntArray(2)
        surface.getLocationInWindow(at)
        val v = ImageView(a)
        v.setImageBitmap(pic)
        v.scaleType = ImageView.ScaleType.FIT_XY
        v.isClickable = false
        v.isFocusable = false
        v.importantForAccessibility = View.IMPORTANT_FOR_ACCESSIBILITY_NO
        val lp = FrameLayout.LayoutParams(surface.width, surface.height)
        lp.leftMargin = at[0]
        lp.topMargin = at[1]
        root.addView(v, lp)
        shown = v
    }

    // MyGo's surface: the largest SurfaceView, a video's player being one
    // too.
    private fun largestSurface(v: View): SurfaceView? {
        if (v is SurfaceView) return v
        val g = v as? ViewGroup ?: return null
        var best: SurfaceView? = null
        for (i in 0 until g.childCount) {
            val s = largestSurface(g.getChildAt(i)) ?: continue
            if (best == null || s.width * s.height > best.width * best.height) best = s
        }
        return best
    }
}
