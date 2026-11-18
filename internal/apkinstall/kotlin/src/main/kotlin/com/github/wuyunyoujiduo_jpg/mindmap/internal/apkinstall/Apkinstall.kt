package com.github.wuyunyoujiduo_jpg.mindmap.internal.apkinstall

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.core.content.FileProvider
import dev.mygo.Native
import java.io.File

// The system's installer, opened on a package the app downloaded into its
// cache, which MyGo's FileProvider (<package>.mygo.files) serves to it.
// The permission REQUEST_INSTALL_PACKAGES is the app's (mygo.json), and
// whether the user allowed it is asked of the package manager: Android 8
// and later, MyGo's least.

fun makeInstaller(): Installer = SystemInstaller()

private class SystemInstaller : Installer {
    override fun canInstall(): Boolean = Native.context.packageManager.canRequestPackageInstalls()

    override fun allowInstall() {
        val ctx = Native.context
        start(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:" + ctx.packageName)))
    }

    override fun install(path: String) {
        val ctx = Native.context
        val uri = FileProvider.getUriForFile(ctx, ctx.packageName + ".mygo.files", File(path))
        start(
            Intent(Intent.ACTION_VIEW)
                .setDataAndType(uri, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION),
        )
    }

    // From the activity showing the app, else as a task of its own.
    private fun start(i: Intent) {
        val from: Context = Native.activity ?: Native.context.also { i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK) }
        from.startActivity(i)
    }
}
