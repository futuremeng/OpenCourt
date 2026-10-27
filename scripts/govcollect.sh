#!/bin/bash
# 国家规章库清单分批采集·续采（人在环代理浏览；每会话 200 页硬上限，官方跳转控件驱动）
# 已有分片覆盖 1-99、201-268；本脚本补齐其余区段。
set -u
EP="http://127.0.0.1:9010/mcp"
BIN=/Users/futuremeng/github/futuremeng/OpenCourt/dist/govlistcollect
LOG=/tmp/govcollect.log
: > "$LOG"
run() {
  local start=$1 pages=$2
  echo "=== 会话 start=$start pages=$pages $(date +%T) ===" >> "$LOG"
  "$BIN" -endpoint "$EP" -start "$start" -pages "$pages" >> "$LOG" 2>&1
  local rc=$?
  echo "=== 会话 start=$start 退出码 $rc $(date +%T) ===" >> "$LOG"
  if [ $rc -ne 0 ]; then
    echo "COLLECT-ABORT" >> "$LOG"
    exit $rc
  fi
  sleep 30
}
run 100 101
run 269 132
run 401 200
run 601 200
run 801 200
echo "COLLECT-DONE" >> "$LOG"
