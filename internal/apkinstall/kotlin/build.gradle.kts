// The Kotlin of the native module apkinstall: an Android library, which mygo
// build adds to the app's Gradle build. The build sets its SDK levels, Java
// and Kotlin, and its dependency on MyGo's host library (dev.mygo), which
// brings coroutines: add only what the implementation uses.
plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.github.wuyunyoujiduo_jpg.mindmap.internal.apkinstall"
}

dependencies {
    implementation("androidx.core:core-ktx:1.16.0") // FileProvider, as MyGo's host has it
}
