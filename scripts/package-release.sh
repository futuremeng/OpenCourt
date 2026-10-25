#!/usr/bin/env bash
# OpenCourt 程序包（轻量 dmg，约 15MB）——语料独立分发（scripts/package-lawdata.sh）。
# 分开理由：程序与语料更新节奏不同（程序修 bug 频繁发版，语料大而慢）；
# 用户程序升级不必重下 3.3G，语料可断点续传、多应用复用。
#
# 用法：scripts/package-release.sh
set -euo pipefail
cd "$(dirname "$0")/.."

VER="$(grep -o 'const version = "[^"]*"' main.go | cut -d'"' -f2)"
OUT="dist/OpenCourt-${VER}-darwin-universal.dmg"

export PATH="$PATH:$(go env GOPATH)/bin"
wails build -clean -universal

STAGE="dist/stage-${VER}"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp -R build/bin/OpenCourt.app "$STAGE/"
cp scripts/INSTALL.md "$STAGE/读我-安装说明.md"

mkdir -p dist
rm -f "$OUT"
hdiutil create -volname "OpenCourt ${VER}" -srcfolder "$STAGE" -ov -format UDZO "$OUT" >/dev/null
rm -rf "$STAGE"
# 自更新依赖同名 .sha256 资产（缺失则客户端拒绝无校验安装）
shasum -a 256 "$OUT" > "$OUT.sha256"
echo "完成: $OUT (+.sha256)"
echo "语料包: scripts/package-lawdata.sh（独立下载分发）"
echo "分发提示: 正式对外分发需 codesign + notarize（deploy/README.md）。"
