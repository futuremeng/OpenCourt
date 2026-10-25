#!/usr/bin/env bash
# OpenCourt 桌面安装包（含法条数据）——普通用户形态：
#   下载 dmg → 打开 → 双击 OpenCourt → 首次启动自动安装法条数据 → 直接用
#
# 用法：
#   scripts/package-release.sh [OpenLaw数据目录]     # 默认 ../OpenLaw/data
# 前置：
#   - wails CLI（make setup）
#   - OpenLaw 数据目录已生成 manifest 签名（openlaw-check -build-manifest），
#     否则用户端法条详情会显示 UNVERIFIED（防篡改闸口，见 docs/deploy.md）
set -euo pipefail
cd "$(dirname "$0")/.."

LAW_DATA="${1:-../OpenLaw/data}"
VER="$(grep -o 'const version = "[^"]*"' main.go | cut -d'"' -f2)"
ARCH="$(uname -m)"
OUT="dist/OpenCourt-${VER}-darwin-${ARCH}.dmg"

# 数据完整性前置检查：manifest 未生成则警告（不阻断，但发布版应带签名）
if [ ! -f "${LAW_DATA}/manifest/manifest.json" ]; then
  echo "警告: ${LAW_DATA} 未生成 manifest（用户端详情将显示 UNVERIFIED）。" >&2
  echo "      发布前应在数据侧执行 openlaw-check -build-manifest -key <独立存储>。" >&2
fi

export PATH="$PATH:$(go env GOPATH)/bin"
wails build -clean

STAGE="dist/stage-${VER}"
rm -rf "$STAGE" "dist/OpenCourt-${VER}-darwin-${ARCH}"
mkdir -p "$STAGE"
cp -R build/bin/OpenCourt.app "$STAGE/"
echo "拷贝法条数据（约数分钟）…"
cp -R "$LAW_DATA" "$STAGE/law-data"
rm -rf "$STAGE/law-data/logs"          # 运行产物不随包分发
cp scripts/INSTALL.md "$STAGE/读我-安装说明.md"

mkdir -p dist
rm -f "$OUT"
hdiutil create -volname "OpenCourt ${VER}" -srcfolder "$STAGE" -ov -format UDZO "$OUT" >/dev/null
rm -rf "$STAGE"
echo "完成: $OUT"
echo "分发提示: 正式对外分发需 codesign + notarize（deploy/README.md）。"
