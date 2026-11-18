// Package apkinstall hands an Android package the app downloaded to the
// system's installer, which asks the user to install it over the app: an
// app on Android cannot replace itself (MyMind's apkupdate.go). It is a
// native module of MyGo, its Kotlin in kotlin/; elsewhere NewInstaller
// returns errors.ErrUnsupported.
package apkinstall

// Installer opens the system's installer.
//
//mygo:native
//mygo:mainthread
type Installer interface {
	// CanInstall tells whether the user has let the app install packages
	// (Android's 安装未知应用, per app).
	CanInstall() bool
	// AllowInstall opens the system's page where the user lets the app
	// install packages.
	AllowInstall() error
	// Install opens the system's installer on the package at path, a file
	// of the app's cache.
	Install(path string) error
}
