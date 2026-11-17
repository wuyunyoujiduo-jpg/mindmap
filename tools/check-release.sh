#!/usr/bin/env bash
# Tries the scripts a release runs (.github/preflight.sh, publish-updates.sh
# and prune-updates.sh) against a GitHub that is a folder: a gh and a curl
# made here, in a temporary folder, stand first on PATH. Nothing real is
# asked or changed. It tells what the scripts decide, not that GitHub
# answers as the stand-ins do: that only a release shows.
#
#   tools/check-release.sh
set -euo pipefail
here="$(cd "$(dirname "$0")/../.github" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export FAKE="$tmp/hub" UPDATES_REPO=me/models PRUNE_PAUSE=0 GITHUB_REPOSITORY=me/app LC_ALL=C
mkdir "$tmp/bin" "$tmp/repo"
PATH="$tmp/bin:$PATH"
base=https://github.com/me/models/releases/download/desktop-latest
failed=0

cat >"$tmp/bin/gh" <<'EOF'
#!/usr/bin/env bash
# gh, of a folder: assets.tsv is desktop-latest's files ("id<tab>name"),
# files/<id> what each holds, releases.json the releases as listed, tags
# the other repository's refs. A call that matches the pattern in fail is
# refused.
set -euo pipefail
echo "$*" >>"$FAKE/calls"
if [ -s "$FAKE/fail" ] && printf '%s' "$*" | grep -Eq -- "$(cat "$FAKE/fail")"; then
  echo "gh: refused" >&2
  exit 1
fi
drop() { # drop NAME: off the release
  awk -F'\t' -v n="$1" '$2 != n' "$FAKE/assets.tsv" >"$FAKE/assets.new"
  mv "$FAKE/assets.new" "$FAKE/assets.tsv"
}
if [ "$1 $2" = "release upload" ]; then
  shift 3
  for f in "$@"; do
    case "$f" in --clobber | --repo | "$UPDATES_REPO") continue ;; esac
    id=$(($(cat "$FAKE/next") + 1))
    echo "$id" >"$FAKE/next"
    drop "${f##*/}"
    printf '%s\t%s\n' "$id" "${f##*/}" >>"$FAKE/assets.tsv"
    cp "$f" "$FAKE/files/$id"
  done
  exit 0
fi
[ "$1" = api ] || exit 2
shift
filter=. method=GET path=
while [ $# -gt 0 ]; do
  case "$1" in
  --paginate) ;;
  --jq) filter="$2" && shift ;;
  -H) shift ;;
  -X) method="$2" && shift ;;
  *) path="$1" ;;
  esac
  shift
done
case "$method $path" in
"GET repos/$UPDATES_REPO/releases/tags/desktop-latest") echo '{"id": 77}' | jq -r "$filter" ;;
"GET repos/$UPDATES_REPO/releases/77/assets?per_page=100")
  # In two pages, each filtered alone, as gh does.
  jq -Rn '[inputs | split("\t") | {id: (.[0] | tonumber), name: .[1]}]' "$FAKE/assets.tsv" >"$FAKE/all.json"
  jq '.[:5]' "$FAKE/all.json" | jq -r "$filter"
  jq '.[5:]' "$FAKE/all.json" | jq -r "$filter"
  ;;
"GET repos/$UPDATES_REPO/releases?per_page=100") cat "$FAKE/releases.json" ;;
"GET repos/$UPDATES_REPO/releases/assets/"*) cat "$FAKE/files/${path##*/}" ;;
"DELETE repos/$UPDATES_REPO/releases/assets/"*)
  awk -F'\t' -v id="${path##*/}" '$1 != id' "$FAKE/assets.tsv" >"$FAKE/assets.new"
  mv "$FAKE/assets.new" "$FAKE/assets.tsv"
  ;;
"GET repos/$GITHUB_REPOSITORY/git/matching-refs/tags/"*)
  jq -Rn --arg p "refs/tags/${path##*/tags/}" '[inputs | select(startswith($p)) | {ref: .}]' "$FAKE/tags" |
    jq -r "$filter"
  ;;
*)
  echo "gh: a call the stand-in does not know: $method $path" >&2
  exit 2
  ;;
esac
EOF
cat >"$tmp/bin/curl" <<'EOF'
#!/usr/bin/env bash
# curl, of a file: the manifest the apps are served, or none.
[ -e "$FAKE/live.json" ] && cat "$FAKE/live.json"
EOF
chmod +x "$tmp/bin/gh" "$tmp/bin/curl"

# manifest VERSION TARGET "FROM…" "BEFORE…": as mygo build writes one.
manifest() {
  jq -n --arg v "$1" --arg t "$2" --arg d "$3" --arg p "$4" --arg b "$base" '
    {version: $v, url: "\($b)/mymind-\($v)-\($t).tar.gz", size: 1, signature: "s",
     deltas: [$d | split(" ")[] | {from: ., url: "\($b)/mymind-\(.)-to-\($v)-\($t).delta", size: 1, signature: "s"}],
     previous: [$p | split(" ")[] | {version: ., url: "\($b)/mymind-\(.)-\($t).tar.gz", size: 1, signature: "s"}]}'
}
# asset NAME [FILE]: a file on desktop-latest. Its id is said in $id.
asset() {
  id=$(($(cat "$FAKE/next") + 1))
  echo "$id" >"$FAKE/next"
  printf '%s\t%s\n' "$id" "$1" >>"$FAKE/assets.tsv"
  if [ -n "${2:-}" ]; then cp "$2" "$FAKE/files/$id"; else echo x >"$FAKE/files/$id"; fi
}
# hub: desktop-latest as it stands after 0.105.1, which Apple silicon
# alone had; Intel and Windows at 0.33.1; and the releases the apps
# before 0.30.1 look through, a draft first.
hub() {
  rm -rf "$FAKE"
  mkdir -p "$FAKE/files"
  echo 100 >"$FAKE/next"
  : >"$FAKE/assets.tsv"
  : >"$FAKE/fail"
  : >"$FAKE/calls"
  local v
  for v in 0.102.0 0.103.0 0.104.0 0.105.0 0.105.1; do
    asset "mymind-$v-darwin-arm64.tar.gz"
  done
  for v in 0.100.0 0.101.0 0.102.0; do asset "mymind-$v-to-0.105.0-darwin-arm64.delta"; done
  for v in 0.103.0 0.104.0 0.105.0; do asset "mymind-$v-to-0.105.1-darwin-arm64.delta"; done
  for v in 0.31.6 0.31.7 0.32.0 0.33.1; do asset "mymind-$v-darwin-amd64.tar.gz"; done
  for v in 0.31.6 0.31.7 0.32.0; do asset "mymind-$v-to-0.33.1-darwin-amd64.delta"; done
  asset mymind-0.33.1-windows-amd64.tar.gz
  asset agent-tools.json
  manifest 0.105.1 darwin-arm64 "0.105.0 0.104.0 0.103.0" "0.105.0 0.104.0" >"$tmp/arm64.json"
  manifest 0.33.1 darwin-amd64 "0.32.0 0.31.7 0.31.6" "0.32.0 0.31.7" >"$tmp/amd64.json"
  manifest 0.33.1 windows-amd64 "" "" >"$tmp/windows.json"
  asset update-darwin-arm64.json "$tmp/arm64.json"
  asset update-darwin-amd64.json "$tmp/amd64.json"
  asset update-windows-amd64.json "$tmp/windows.json"
  cp "$tmp/arm64.json" "$FAKE/files/901"
  manifest 0.106.0 darwin-arm64 "" "" >"$FAKE/files/902"
  jq -n '[{tag_name: "desktop-v0.106.0", draft: true, prerelease: false, assets: [{id: 902, name: "update-darwin-arm64.json"}]},
          {tag_name: "desktop-latest", draft: false, prerelease: false, assets: []},
          {tag_name: "desktop-v0.105.1", draft: false, prerelease: false,
           assets: [{id: 900, name: "mymind-0.105.1-darwin-arm64.tar.gz"}, {id: 901, name: "update-darwin-arm64.json"}]}],
         [{tag_name: "desktop-v0.105.0", draft: false, prerelease: false, assets: [{id: 903, name: "update-darwin-arm64.json"}]}]' \
    >"$FAKE/releases.json"
}
names() { cut -f2 "$FAKE/assets.tsv" | sort; }
# The archives and deltas of the hub that no manifest names.
stale="mymind-0.100.0-to-0.105.0-darwin-arm64.delta
mymind-0.101.0-to-0.105.0-darwin-arm64.delta
mymind-0.102.0-darwin-arm64.tar.gz
mymind-0.102.0-to-0.105.0-darwin-arm64.delta
mymind-0.103.0-darwin-arm64.tar.gz
mymind-0.31.6-darwin-amd64.tar.gz"

# check WHAT STATUS WANTED-STATUS [GONE]: the script ended as wanted,
# and took off the release the names of GONE, a line each, and no other.
check() {
  local what="$1" got="$2" want="$3" gone="${4:-}" was
  was="$(comm -23 "$tmp/before" <(names))"
  if [ "$got" != "$want" ] || [ "$was" != "$gone" ]; then
    failed=1
    echo "FAILED  $what: ended $got (wanted $want); took off: ${was:-nothing} (wanted: ${gone:-nothing})"
    sed 's/^/        /' "$tmp/out"
  else
    echo "ok      $what"
  fi
}
# also WHAT COMMAND…: one thing more that is to hold.
also() {
  local what="$1"
  shift
  if "$@"; then
    echo "ok      $what"
  else
    failed=1
    echo "FAILED  $what"
  fi
}
# prune WHAT WANTED-STATUS GONE [ARGUMENT…]: runs prune-updates.sh on the
# hub as it is.
prune() {
  local what="$1" want="$2" gone="$3" got=0
  shift 3
  names >"$tmp/before"
  bash "$here/prune-updates.sh" "$@" >"$tmp/out" 2>&1 || got=$?
  check "$what" "$got" "$want" "$gone"
}

echo "prune-updates.sh"
hub && prune "takes off what no manifest names, whatever the platform" 0 "$stale"
hub && prune "a second run finds nothing" 0 "$stale" && prune "  (the second)" 0 ""
hub && DRY_RUN=1 prune "a dry run deletes nothing" 0 ""
also "  and says what it would delete" grep -q "会删除 mymind-0.102.0-darwin-arm64.tar.gz" "$tmp/out"
hub && PRUNE_MAX=2 prune "at most PRUNE_MAX, the oldest first" 0 "mymind-0.102.0-darwin-arm64.tar.gz
mymind-0.103.0-darwin-arm64.tar.gz"
# Three versions on: the apps before 0.30.1 still read desktop-v0.105.1's
# manifest, whose files stay though desktop-latest's names them no more.
hub
for v in 0.106.0 0.107.0 0.108.0; do asset "mymind-$v-darwin-arm64.tar.gz"; done
manifest 0.108.0 darwin-arm64 "" "0.107.0 0.106.0" >"$tmp/new.json"
mkdir -p "$tmp/up" && cp "$tmp/new.json" "$tmp/up/update-darwin-arm64.json"
gh release upload desktop-latest "$tmp/up/update-darwin-arm64.json" --repo "$UPDATES_REPO" --clobber
prune "keeps what the newest desktop-v release's manifest names" 0 "$stale"
jq -n '[{tag_name: "desktop-latest", draft: false, prerelease: false, assets: []}]' >"$FAKE/releases.json"
prune "  (which goes once there is no such release)" 0 "mymind-0.103.0-to-0.105.1-darwin-arm64.delta
mymind-0.104.0-darwin-arm64.tar.gz
mymind-0.104.0-to-0.105.1-darwin-arm64.delta
mymind-0.105.0-darwin-arm64.tar.gz
mymind-0.105.0-to-0.105.1-darwin-arm64.delta
mymind-0.105.1-darwin-arm64.tar.gz"
hub && jq -n '[{tag_name: "desktop-latest", draft: false, prerelease: false, assets: []}]' >"$FAKE/releases.json"
prune "with no desktop-v release there is none to keep for" 0 "$stale"
hub && echo '{"version": "0.33.1"' >"$FAKE/files/$(grep update-windows "$FAKE/assets.tsv" | cut -f1)"
prune "a manifest that does not read: nothing goes" 1 ""
hub && echo '{"version": "0.33.1", "url": "mymind.tar.gz"}' >"$FAKE/files/$(grep update-windows "$FAKE/assets.tsv" | cut -f1)"
prune "a manifest whose URL is not understood: nothing goes" 1 ""
hub && manifest 0.34.0 windows-amd64 "" "" >"$FAKE/files/$(grep update-windows "$FAKE/assets.tsv" | cut -f1)"
prune "a manifest whose archive is not there: nothing goes" 1 ""
hub && echo '[]' >"$FAKE/files/901"
prune "a desktop-v manifest that does not read: nothing goes" 1 ""
hub && echo 'assets\?per_page' >"$FAKE/fail"
prune "the files cannot be listed: nothing goes" 1 ""
hub && echo 'releases\?per_page' >"$FAKE/fail"
prune "the releases cannot be listed: nothing goes" 1 ""
hub && echo '[]' >"$FAKE/releases.json"
prune "the releases listed are not all: nothing goes" 1 ""
hub && echo 'octet-stream' >"$FAKE/fail"
prune "a manifest cannot be downloaded: nothing goes" 1 ""
hub && prune "what was uploaded is not listed: nothing goes" 1 "" mymind-0.105.2-darwin-arm64.tar.gz
hub && prune "what was uploaded is kept, named or not" 0 "$(echo "$stale" | grep -v 0.31.6)" mymind-0.31.6-darwin-amd64.tar.gz
hub && echo 'DELETE' >"$FAKE/fail"
prune "a deletion refused stops it" 1 ""
hub && PRUNE_MAX=x prune "PRUNE_MAX that is no number" 1 ""

echo "publish-updates.sh"
build() { # a build of 0.105.2 for Apple silicon, as mygo build leaves it
  rm -rf "$tmp/repo/build" && mkdir -p "$tmp/repo/build/darwin-arm64"
  local d="$tmp/repo/build/darwin-arm64"
  echo x >"$d/mymind-0.105.2-darwin-arm64.tar.gz"
  echo x >"$d/mymind-0.105.1-to-0.105.2-darwin-arm64.delta"
  manifest 0.105.2 darwin-arm64 "0.105.1" "0.105.1 0.105.0" >"$d/update-darwin-arm64.json"
}
publish() { # publish WHAT WANTED-STATUS GONE
  local got=0
  names >"$tmp/before"
  (cd "$tmp/repo" && bash "$here/publish-updates.sh" build/darwin-*) >"$tmp/out" 2>&1 || got=$?
  check "$1" "$got" "$2" "$3"
}
# After 0.105.2: its files are there, and 0.104.0's archive and the deltas
# to 0.105.1 are named no more, but for desktop-v0.105.1's, which stay.
hub && build && publish "uploads, the manifests last, and prunes" 0 "$stale"
uploads() { grep '^release upload' "$FAKE/calls"; }
manifests_last() {
  [ "$(uploads | wc -l)" -eq 2 ] && uploads | tail -n 1 | grep -q 'update-darwin-arm64.json' &&
    ! uploads | head -n 1 | grep -q 'update-'
}
to_latest_alone() { ! uploads | grep -qv '^release upload desktop-latest '; }
has_new() { names | grep -q 'mymind-0.105.2-darwin-arm64.tar.gz'; }
also "  the manifests in a second upload, after the files" manifests_last
also "  to desktop-latest and to no other release" to_latest_alone
also "  the new archive is there" has_new
hub && build && echo 'assets\?per_page' >"$FAKE/fail"
publish "a prune that fails is a warning, not a failed release" 0 ""
also "  and the warning is said" grep -q '^::warning::' "$tmp/out"
hub && build && echo 'release upload' >"$FAKE/fail"
publish "an upload refused fails, and nothing is pruned" 1 ""
hub && rm -rf "$tmp/repo/build" && mkdir -p "$tmp/repo/build/darwin-arm64"
publish "no update files fails" 1 ""
printf '#!/bin/sh\necho MINGW64_NT-10.0\n' >"$tmp/bin/uname" && chmod +x "$tmp/bin/uname"
hub && build && publish "on Windows it uploads and leaves the pruning" 0 ""
rm "$tmp/bin/uname"

echo "preflight.sh"
# flight WHAT WANTED-STATUS VERSION REF LIVE [TAG…]: preflight.sh for
# mygo.json's VERSION run on REF, the apps served LIVE ("" for a manifest
# that does not come), the repository having the tags given; $refuse is
# what GitHub refuses.
flight() {
  local what="$1" want="$2" version="$3" ref="$4" live="$5" got=0
  shift 5
  rm -rf "$FAKE" && mkdir -p "$FAKE"
  printf '%s' "${refuse:-}" >"$FAKE/fail"
  printf 'refs/tags/%s\n' "$@" >"$FAKE/tags"
  [ -z "$live" ] || printf '%s' "$live" >"$FAKE/live.json"
  jq -n --arg v "$version" --arg u "$base" '{version: $v, updates: {url: $u}}' >"$tmp/repo/mygo.json"
  (cd "$tmp/repo" && GITHUB_REF="$ref" GITHUB_REF_NAME="${ref##*/}" bash "$here/preflight.sh") >"$tmp/out" 2>&1 || got=$?
  if [ "$got" != "$want" ]; then
    failed=1
    echo "FAILED  $what: ended $got (wanted $want)"
    sed 's/^/        /' "$tmp/out"
  else
    echo "ok      $what"
  fi
}
main=refs/heads/main
flight "from main, a new version, no such tag" 0 0.105.2 $main '{"version": "0.105.1"}' v0.105.1 v0.105.0
flight "0.105.10 is above 0.105.9" 0 0.105.10 $main '{"version": "0.105.9"}' v0.105.9
flight "0.106.0 is above 0.105.9" 0 0.106.0 $main '{"version": "0.105.9"}' v0.105.9
flight "a tag that only begins as the version's is another" 0 0.105.1 $main '{"version": "0.105.0"}' v0.105.10 v0.105.0
flight "from a branch" 1 0.105.2 refs/heads/old '{"version": "0.105.1"}' v0.105.1
flight "the tag is there" 1 0.105.2 $main '{"version": "0.105.1"}' v0.105.1 v0.105.2
flight "the version the apps are served" 1 0.105.1 $main '{"version": "0.105.1"}' v0.105.0
flight "a version below it" 1 0.99.0 $main '{"version": "0.105.1"}' v0.98.0
flight "0.105.9 is below 0.105.10" 1 0.105.9 $main '{"version": "0.105.10"}' v0.105.8
flight "the manifest does not come" 1 0.105.2 $main "" v0.105.1
flight "the manifest is no manifest" 1 0.105.2 $main "Not Found" v0.105.1
flight "the manifest has no version" 1 0.105.2 $main '{"url": "x"}' v0.105.1
flight "a version that is no x.y.z" 1 0.105 $main '{"version": "0.105.1"}' v0.105.1
flight "a tag pushed, the version's" 0 0.105.2 refs/tags/v0.105.2 '{"version": "0.105.1"}' v0.105.2
flight "a tag pushed, another's" 1 0.105.2 refs/tags/v0.105.3 '{"version": "0.105.1"}' v0.105.3
flight "a tag pushed of the version served" 1 0.105.1 refs/tags/v0.105.1 '{"version": "0.105.1"}' v0.105.1
refuse=matching-refs flight "the tags cannot be asked" 1 0.105.2 $main '{"version": "0.105.1"}' v0.105.1

# The Mac's bash (3.2) takes the first byte of a character not ASCII for
# a letter of a name: "$release：" was read as an unset variable, and the
# release's pruning stopped there (0.105.2). A name before such a
# character is written in braces.
if grep -nP '\$[A-Za-z_][A-Za-z0-9_]*[^\x00-\x7F]' "$here"/*.sh "$here"/workflows/*.yml "$here"/../tools/*.sh "$here"/../update.sh; then
  echo "FAIL    a shell name before a character not ASCII: write it \${name}"
  failed=1
else
  echo "ok      names before characters not ASCII are in braces"
fi
if [ "$failed" != 0 ]; then
  echo "Some checks failed."
  exit 1
fi
echo "All as wanted."
