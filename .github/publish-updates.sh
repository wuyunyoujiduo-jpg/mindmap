#!/usr/bin/env bash
# Publishes the update files that mygo build wrote in the folders given
# (archives, deltas, manifests) to mindmap-models' release desktop-latest,
# which the apps read. The manifests go last, so that an app never reads
# one whose files are missing. Then what no manifest names any more is
# taken off the release (prune-updates.sh).
#
# Each version used to go to a release desktop-v<version> too, for the
# apps before 0.30.1, which read the newest of those: the ones there are
# do for them, their manifests naming desktop-latest's files, which
# prune-updates.sh keeps.
set -euo pipefail
repo="${UPDATES_REPO:?}"
release=desktop-latest
files=() manifests=() names=()
for dir in "$@"; do
  for f in "$dir"/*.tar.gz "$dir"/*.delta; do
    if [ -e "$f" ]; then
      files+=("$f")
      names+=("${f##*/}")
    fi
  done
  for f in "$dir"/update-*.json; do
    if [ -e "$f" ]; then
      manifests+=("$f")
      names+=("${f##*/}")
    fi
  done
done
if [ ${#manifests[@]} -eq 0 ]; then
  echo "No update files: is MYGO_UPDATER_PRIVATE_KEY set?" >&2
  exit 1
fi
if [ ${#files[@]} -gt 0 ]; then
  gh release upload "$release" "${files[@]}" --repo "$repo" --clobber
fi
gh release upload "$release" "${manifests[@]}" --repo "$repo" --clobber

# The update is out. Clearing what is left over is never a reason for a
# release to fail; and it is macOS's job's to do, which every release
# runs, for all the platforms: Windows' shell is not what it was tried in.
case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*) ;;
*)
  bash "$(dirname "$0")/prune-updates.sh" "${names[@]}" ||
    echo "::warning::desktop-latest 里的旧文件这次没有清理（原因见上面几行），不影响这次发布。"
  ;;
esac
