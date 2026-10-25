#!/usr/bin/env bash
# OpenLaw 语料分层包（按数据层拆分下载，检索与证据解耦）：
#   dist/lawdata-core-<YYYYMMDD>.zip       核心包：db/ + manifest/（装上即检索；
#                                          未装证据层前详情如实显示 UNVERIFIED）
#   dist/lawdata-evidence-<YYYYMMDD>.zip   证据层包：docx/ md/ provenance/ pdf/
#                                          （OpenCourt 启动自动合并，溯源链补全）
#   *.sha256                               各包下载完整性校验值（发布页公示）
#
# 用法：
#   scripts/package-lawdata.sh [OpenLaw数据目录]     # 默认 ../OpenLaw/data
#   scripts/package-lawdata.sh <数据目录> -full      # 另产出单包全量（内网/离线机）
# 前置：
#   数据目录已生成 manifest 签名（openlaw-check -build-manifest），
#   否则用户端法条详情显示 UNVERIFIED（防篡改闸口，OpenLaw docs/deploy.md §5）。
#   manifest 随核心包分发：它清单覆盖全部层（含证据层），因此证据层合并后
#   SDK 读取链路即可逐文件验证哈希与签名——任何渠道到达都可验证。
set -euo pipefail
cd "$(dirname "$0")/.."

LAW_DATA="${1:-../OpenLaw/data}"
FULL=""
[ "${2:-}" = "-full" ] && FULL=1
DATE="$(date +%Y%m%d)"
mkdir -p dist

if [ ! -f "${LAW_DATA}/manifest/manifest.json" ]; then
  echo "警告: ${LAW_DATA} 未生成 manifest（用户端详情将显示 UNVERIFIED）。" >&2
  echo "      发布前应在数据侧执行 openlaw-check -build-manifest -key <独立存储>。" >&2
fi

SRC_ABS="$(cd "$LAW_DATA" && pwd)"
pack() { # pack <out.zip> <zip根名> <源目录> [zip -x 排除...]
  local out="$1" root="$2" src="$3"; shift 3
  local stage; stage="$(mktemp -d)"
  ln -s "$src" "$stage/$root"   # zip 默认跟随目录符号链接 → 根名可控
  ( cd "$stage" && zip -qr "$OLDPWD/$out" "$root" "$@" )
  rm -rf "$stage"
  shasum -a 256 "$out" > "$out.sha256"
  echo "完成: $out ($(du -h "$out" | cut -f1))"
}

CORE="dist/lawdata-core-${DATE}.zip"
EVID="dist/lawdata-evidence-${DATE}.zip"
rm -f "$CORE" "$CORE.sha256" "$EVID" "$EVID.sha256"

pack "$CORE" law-data "$SRC_ABS" \
  -x 'law-data/logs/*' -x 'law-data/incoming/*' \
  -x 'law-data/docx/*' -x 'law-data/md/*' -x 'law-data/pdf/*' -x 'law-data/provenance/*'
pack "$EVID" law-evidence "$SRC_ABS" \
  -x 'law-evidence/db/*' -x 'law-evidence/logs/*' -x 'law-evidence/incoming/*' -x 'law-evidence/manifest/*'

if [ -n "$FULL" ]; then
  FULLPKG="dist/lawdata-full-${DATE}.zip"
  rm -f "$FULLPKG" "$FULLPKG.sha256"
  pack "$FULLPKG" law-data "$SRC_ABS" -x 'law-data/logs/*' -x 'law-data/incoming/*'
fi

echo "校验:"; cat "$CORE.sha256" "$EVID.sha256"
echo "发布页需公示 sha256 与 manifest 签名（Ed25519，见 OpenLaw docs/deploy.md）。"
