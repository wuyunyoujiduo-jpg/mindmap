#!/usr/bin/env bash
# Publishes the update files that mygo build wrote in the folders given
# (archives, deltas, manifests) to mindmap-models: to desktop-latest, which
# the app reads, and to desktop-v<version>, for the versions before
# 0.30.1, which find the newest such release. The manifests go last, so
# that an app never reads one whose files are missing.
set -euo pipefail
repo="${UPDATES_REPO:?}"
tag="desktop-v$(jq -r .version mygo.json)"
files=() manifests=()
for dir in "$@"; do
  for f in "$dir"/*.tar.gz "$dir"/*.delta; do
    [ -e "$f" ] && files+=("$f")
  done
  for f in "$dir"/update-*.json; do
    [ -e "$f" ] && manifests+=("$f")
  done
done
if [ ${#manifests[@]} -eq 0 ]; then
  echo "No update files: is MYGO_UPDATER_PRIVATE_KEY set?" >&2
  exit 1
fi
for release in desktop-latest "$tag"; do
  if [ ${#files[@]} -gt 0 ]; then
    gh release upload "$release" "${files[@]}" --repo "$repo" --clobber
  fi
  gh release upload "$release" "${manifests[@]}" --repo "$repo" --clobber
done
