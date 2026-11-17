#!/usr/bin/env bash
# Checked before a release is built (build.yml's test job, for a run by
# hand with "release" and for a tag pushed): a release goes out from main,
# under a tag not yet there, of a version above the one the apps are
# served. A branch left behind once shipped a version from an old base
# over a newer one; this is what refuses it. What cannot be checked is
# refused too: a manifest that does not come is no leave to publish.
#
# It reads GITHUB_REF, GITHUB_REF_NAME and GITHUB_REPOSITORY, and asks
# GitHub with GH_TOKEN. tools/check-release.sh tries it.
set -euo pipefail

fail() {
  echo "::error::$*"
  exit 1
}
semver='^[0-9]+\.[0-9]+\.[0-9]+$'

version="$(jq -er '.version | strings' mygo.json)" || fail "mygo.json 里读不到 version。"
[[ "$version" =~ $semver ]] || fail "mygo.json 的 version「$version」不是 x.y.z 的样子。"
tag="v$version"

case "${GITHUB_REF:-}" in
refs/tags/*)
  # A tag pushed: it is the version's.
  [ "${GITHUB_REF_NAME:-}" = "$tag" ] ||
    fail "标签 ${GITHUB_REF_NAME:-?} 和 mygo.json 的版本 $version 对不上。"
  ;;
refs/heads/main)
  # Run by hand: the release makes the tag, which is not to be there.
  # matching-refs answers with the tags that begin so, none being an
  # empty list and not an error.
  refs="$(gh api "repos/${GITHUB_REPOSITORY:?}/git/matching-refs/tags/$tag" --jq '.[].ref')" ||
    fail "问不到标签 $tag 是否已经存在，没有发布。过一会儿重新运行。"
  if printf '%s\n' "$refs" | tr -d '\r' | grep -Fxq "refs/tags/$tag"; then
    fail "标签 $tag 已经存在：$version 发布过了。先把 mygo.json 的 version 改成新的版本号。"
  fi
  ;;
*)
  fail "只能从 main 发布，这次运行的是 ${GITHUB_REF:-?}。在 Actions 页面的「Use workflow from」里选 main。"
  ;;
esac

# The version the apps are served: the manifest of Apple silicon, which
# every release publishes.
base="$(jq -er '.updates.url | strings' mygo.json)" || fail "mygo.json 里读不到 updates.url。"
manifest="${base%/}/update-darwin-arm64.json"
live="$(curl -fsSL --retry 3 --retry-delay 2 --max-time 60 "$manifest" | jq -er '.version | strings')" ||
  fail "读不到线上的更新清单（$manifest），没法确认 $version 比线上的新，没有发布。过一会儿重新运行。"
[[ "$live" =~ $semver ]] || fail "线上的更新清单里的版本「$live」不是 x.y.z 的样子，没有发布。"
newest="$(printf '%s\n%s\n' "$live" "$version" | sort -V | tail -n 1)"
if [ "$version" = "$live" ] || [ "$newest" != "$version" ]; then
  fail "mygo.json 的版本 $version 不比线上的 $live 新：是不是从旧的代码发布，或者忘了改版本号？没有发布。"
fi
echo "可以发布 $tag：线上是 $live。"
