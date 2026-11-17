#!/usr/bin/env bash
# Takes off mindmap-models' release desktop-latest the archives and deltas
# that no manifest names any more. Every release adds an archive and up
# to three deltas there, and a release of GitHub's holds a thousand files:
# left to grow, it would one day take no more, and no update could go out.
#
# What stays:
#   - every manifest (update-*.json), and what any of them names: its
#     archive, its deltas and the archives of the versions before, which
#     the next build makes its deltas from (MyGo's docs/updates.md). The
#     manifests of every platform are read, as a release publishes Apple
#     silicon's alone but now and then;
#   - what the manifests of the newest desktop-v<version> release name:
#     the apps before 0.30.1 read those, and they name files here;
#   - the names given as arguments, which is what was just uploaded;
#   - whatever is neither an archive nor a delta (agent-tools.json).
#
# It deletes only when all of it read as expected and stops at the first
# thing that did not, deleting nothing more: publish-updates.sh makes a
# warning of that and never a failed release. DRY_RUN=1 lists and deletes
# nothing; PRUNE_MAX is how many go in one run, the oldest first, the
# rest in the runs after: fifty, a second apart, in a release, which
# counts macOS's minutes; the workflow Prune updates clears more at once.
# Written for macOS's bash 3.2 and BSD tools too. tools/check-release.sh
# tries it.
set -euo pipefail
repo="${UPDATES_REPO:?}"
release=desktop-latest
legacy=desktop-v
max="${PRUNE_MAX:-50}"
pause="${PRUNE_PAUSE:-1}"
dry="${DRY_RUN:-}"
tab=$'\t'

stop() {
  echo "清理 $release 停下了：$*。" >&2
  exit 1
}
case "$max" in
'' | *[!0-9]*) stop "PRUNE_MAX「$max」不是数字" ;;
esac

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir "$work/latest" "$work/legacy"

# download FOLDER < "id<tab>name" lines: the files of those ids, exactly,
# and not what a URL may still serve of a file just replaced.
download() {
  local aid name
  while IFS="$tab" read -r aid name; do
    gh api -H "Accept: application/octet-stream" "repos/$repo/releases/assets/$aid" </dev/null >"$1/$name"
  done
}

# The release's files, an id and a name a line.
id="$(gh api "repos/$repo/releases/tags/$release" --jq .id | tr -d '\r')"
case "$id" in
'' | *[!0-9]*) stop "读不到这个 release" ;;
esac
gh api --paginate "repos/$repo/releases/$id/assets?per_page=100" --jq '.[] | "\(.id)\t\(.name)"' |
  tr -d '\r' >"$work/assets"
awk -F"$tab" 'NF != 2 || $1 !~ /^[0-9]+$/ || $2 == "" || $2 ~ /\// { bad = 1 } END { exit bad }' "$work/assets" ||
  stop "文件列表读出来不对"
cut -f2 "$work/assets" >"$work/names"
for n in ${1+"$@"}; do
  grep -Fxq -- "$n" "$work/names" || stop "刚传上去的 $n 不在文件列表里，列表不全"
done

# Its manifests.
grep -E "$tab"'update-[^/]*\.json$' "$work/assets" >"$work/manifests" || stop "上面没有更新清单"
download "$work/latest" <"$work/manifests"

# The manifests of the release the apps before 0.30.1 read: the first
# tagged desktop-v, neither a draft nor a prerelease, as GitHub lists
# them (MyGo's internal/update/github.go). None is none to keep for.
gh api --paginate "repos/$repo/releases?per_page=100" >"$work/releases.json"
jq -es --arg r "$release" 'flatten | any(.[]; .tag_name == $r)' "$work/releases.json" >/dev/null ||
  stop "release 的列表读出来不对"
jq -rs --arg p "$legacy" '
  [flatten[] | select((.draft or .prerelease) | not) | select(.tag_name | startswith($p))][0] // empty
  | .assets[] | select(.name | test("^update-[^/]*\\.json$")) | "\(.id)\t\(.name)"' "$work/releases.json" |
  tr -d '\r' >"$work/legacy.list"
download "$work/legacy" <"$work/legacy.list"

# named MANIFEST: the files it names, or a failure for what is no
# manifest or names a file in a way not understood.
named() {
  jq -er '
    if (.version | type) == "string" and (.url | type) == "string"
      and (.deltas // [] | type) == "array" and (.previous // [] | type) == "array"
    then .url, (.deltas // [])[].url, (.previous // [])[].url
    else error("not a manifest") end
    | if type == "string" and test("^https://[^?#%]+/[^/?#%]+$") then sub("^.*/"; "")
      else error("a URL not understood") end' "$1" | tr -d '\r'
}
: >"$work/keep"
for f in "$work"/latest/* "$work"/legacy/*; do
  if [ -e "$f" ]; then
    named "$f" >>"$work/keep" || stop "清单 ${f##*/} 读不懂"
  fi
done
# A manifest of the release whose own archive is not on it: not as
# expected.
for f in "$work"/latest/*; do
  n="$(jq -r '.url | sub("^.*/"; "")' "$f" | tr -d '\r')"
  grep -Fxq -- "$n" "$work/names" || stop "清单 ${f##*/} 指的 $n 不在上面"
done
for n in ${1+"$@"}; do
  printf '%s\n' "$n" >>"$work/keep"
done

# What goes: the archives and deltas that are not kept.
grep -E '\.(tar\.gz|delta)$' "$work/assets" >"$work/archives" || true
awk -F"$tab" 'NR == FNR { keep[$0] = 1; next } !($2 in keep)' "$work/keep" "$work/archives" |
  sort -n >"$work/gone"
all="$(wc -l <"$work/assets" | tr -d ' ')"
total="$(wc -l <"$work/archives" | tr -d ' ')"
gone="$(wc -l <"$work/gone" | tr -d ' ')"
echo "清理 $release：共 $all 个文件，其中更新包和增量包 $total 个，$gone 个已经没有清单用到。"
if [ "$gone" -eq 0 ]; then
  exit 0
fi
[ "$gone" -lt "$total" ] || stop "算出来要删掉所有的包"
if [ "$gone" -gt "$max" ]; then
  echo "这次最多删 $max 个，其余的下次再删。"
fi

count=0
awk -v max="$max" 'NR <= max' "$work/gone" >"$work/batch"
while IFS="$tab" read -r aid name; do
  if [ -n "$dry" ]; then
    echo "会删除 $name"
    continue
  fi
  gh api -X DELETE "repos/$repo/releases/assets/$aid" </dev/null >/dev/null ||
    stop "删除 $name 失败（这次已删 $count 个）"
  echo "已删除 $name"
  count=$((count + 1))
  sleep "$pause"
done <"$work/batch"
