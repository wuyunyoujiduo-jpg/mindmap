#!/bin/sh
# Tells whether MyGo has a newer version than the one go.mod uses, and
# with -u upgrades to it.
#
#   tools/check-mygo.sh       # compare
#   tools/check-mygo.sh -u    # upgrade, then build and test
set -eu
cd "$(dirname "$0")/.."
have=$(go list -m -f '{{.Version}}' github.com/egoist/mygo)
latest=$(GOFLAGS=-mod=mod go list -m -f '{{.Version}}' github.com/egoist/mygo@latest)
if [ "$have" = "$latest" ]; then
	echo "MyGo $have 已是最新"
	exit 0
fi
echo "MyGo 有新版本：$have → $latest"
if [ "${1:-}" = "-u" ]; then
	go get "github.com/egoist/mygo@$latest"
	go mod tidy
	go build ./... && go vet ./... && go test ./...
	echo "已升级到 MyGo $latest；看看 https://github.com/egoist/mygo/releases 有哪些新功能"
fi
