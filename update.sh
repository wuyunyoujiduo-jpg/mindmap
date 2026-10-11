#!/bin/bash
# Installs or updates MyMind on macOS from the repository's releases.
#
#   ./update.sh            # the latest release
#   ./update.sh 0.6.0      # a version of your choice
#   ./update.sh -f         # reinstall even when up to date
#
# It needs the GitHub CLI, signed in, as the repository is private:
#   brew install gh && gh auth login
#
# Files downloaded by the command line are not quarantined, so macOS opens
# the app without asking to allow it in System Settings.
set -euo pipefail

REPO="wuyunyoujiduo-jpg/mindmap"
APP_NAME="MyMind"
APP_DIR="${APP_DIR:-/Applications}"
APP="$APP_DIR/$APP_NAME.app"

force=0
version=""
for arg in "$@"; do
	case "$arg" in
	-f | --force) force=1 ;;
	-h | --help)
		sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*) version="${arg#v}" ;;
	esac
done

say() { printf '\033[1m%s\033[0m\n' "$*"; }
fail() {
	printf '\033[31m%s\033[0m\n' "$*" >&2
	exit 1
}

[ "$(uname)" = "Darwin" ] || fail "只支持 macOS。"
command -v gh >/dev/null || fail "需要 GitHub 命令行工具：brew install gh && gh auth login"
gh auth status >/dev/null 2>&1 || fail "GitHub 命令行工具还没有登录：gh auth login"

# The release to install.
if [ -n "$version" ]; then
	tag="v$version"
	gh release view "$tag" -R "$REPO" >/dev/null 2>&1 || fail "找不到版本 $tag。"
else
	tag="$(gh release view -R "$REPO" --json tagName -q .tagName)" || fail "无法读取最新版本。"
fi
want="${tag#v}"

# The version installed, if any.
have=""
if [ -f "$APP/Contents/Info.plist" ]; then
	have="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist" 2>/dev/null || true)"
fi
if [ "$have" = "$want" ] && [ "$force" = 0 ]; then
	say "$APP_NAME $have 已经是最新版本。"
	exit 0
fi
if [ -n "$have" ]; then
	say "更新 $APP_NAME：$have → $want"
else
	say "安装 $APP_NAME $want"
fi

work="$(mktemp -d)"
mnt="$work/mnt"
cleanup() {
	hdiutil detach -quiet "$mnt" 2>/dev/null || true
	rm -rf "$work"
}
trap cleanup EXIT

say "下载…"
# The disk image of this Mac's chip, or the one for both of releases
# before 0.25.1.
arch=arm64
[ "$(uname -m)" = "x86_64" ] && arch=amd64
gh release download "$tag" -R "$REPO" -p "*.$arch.dmg" -D "$work" 2>/dev/null ||
	gh release download "$tag" -R "$REPO" -p '*.dmg' -D "$work" || fail "下载失败。"
dmg="$(ls "$work"/*.dmg | head -n 1)"

# Quit the app if it runs, so that it saves; it saves as it quits.
if pgrep -xq "$APP_NAME"; then
	say "退出正在运行的 $APP_NAME…"
	osascript -e "quit app \"$APP_NAME\"" >/dev/null 2>&1 || true
	for _ in $(seq 1 50); do
		pgrep -xq "$APP_NAME" || break
		sleep 0.2
	done
	pgrep -xq "$APP_NAME" && fail "$APP_NAME 没有退出，请手动退出后再试。"
fi

say "安装到 $APP_DIR…"
mkdir -p "$mnt"
hdiutil attach -quiet -nobrowse -readonly -mountpoint "$mnt" "$dmg" || fail "无法打开安装包。"
src="$mnt/$APP_NAME.app"
[ -d "$src" ] || fail "安装包里没有 $APP_NAME.app。"
sudo=""
[ -w "$APP_DIR" ] || sudo="sudo"
$sudo rm -rf "$APP"
$sudo ditto "$src" "$APP"
$sudo xattr -cr "$APP" 2>/dev/null || true

# The app was called MindMap before 0.25.2: the old one goes.
old="$APP_DIR/MindMap.app"
if [ -d "$old" ] && [ "$old" != "$APP" ]; then
	osascript -e 'quit app "MindMap"' >/dev/null 2>&1 || true
	$sudo rm -rf "$old"
fi

say "完成：$APP_NAME $want"
open "$APP"
