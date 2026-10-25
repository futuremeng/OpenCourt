#!/usr/bin/env bash
# OpenLaw 语料独立包（与程序包分开下载）：
#   dist/lawdata-<YYYYMMDD>.zip         解压得 law-data/（OpenCourt 自动发现安装）
#   dist/lawdata-<YYYYMMDD>.zip.sha256  下载完整性校验值（发布页公示）
#
# 用法：
#   scripts/package-lawdata.sh [OpenLaw数据目录]     # 默认 ../OpenLaw/data
# 前置：
#   OpenLaw 数据目录已生成 manifest 签名（openlaw-check -build-manifest），
#   否则用户端法条详情显示 UNVERIFIED（防篡改闸口，OpenLaw docs/deploy.md §5）。
#   manifest/ 随包分发：OpenCourt 读取链路（SDK trace）会持续校验签名与哈希，
#   语料经任何渠道（网盘/镜像/断点续传）到达用户机器都可验证。
set -euo pipefail
cd "$(dirname "$0")/.."

LAW_DATA="${1:-../OpenLaw/data}"
DATE="$(date +%Y%m%d)"
mkdir -p dist
OUT="dist/lawdata-${DATE}.zip"

if [ ! -f "${LAW_DATA}/manifest/manifest.json" ]; then
  echo "警告: ${LAW_DATA} 未生成 manifest（用户端详情将显示 UNVERIFIED）。" >&2
  echo "      发布前应在数据侧执行 openlaw-check -build-manifest -key <独立存储>。" >&2
fi

rm -f "$OUT" "$OUT.sha256"
# zip 根目录为 law-data/（用户双击解压即得该文件夹）；logs/incoming 运行产物不随包
SRC_ABS="$(cd "$LAW_DATA" && pwd)"
( cd "$(dirname "$SRC_ABS")" && zip -qr "$OLDPWD/$OUT" "$(basename "$SRC_ABS")" \
    -x "$(basename "$SRC_ABS")/logs/*" -x "$(basename "$SRC_ABS")/incoming/*" )

shasum -a 256 "$OUT" > "$OUT.sha256"
echo "完成: $OUT ($(du -h "$OUT" | cut -f1))"
echo "校验: $(cat "$OUT.sha256")"
echo "发布页需公示 sha256 与 manifest 签名（Ed25519，见 OpenLaw docs/deploy.md）。"
