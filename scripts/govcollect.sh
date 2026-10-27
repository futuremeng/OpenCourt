#!/bin/bash
# 缺口补采·第二轮（高空页阈值穿越官方清单的大段稀疏空区）
set -u
EP="http://127.0.0.1:9010/mcp"
BIN=/Users/futuremeng/github/futuremeng/OpenCourt/dist/govlistcollect
LOG=/tmp/govcollect_fill2.log
: > "$LOG"
run() {
  local start=$1 pages=$2 es=$3
  echo "=== 会话 start=$start pages=$pages emptystop=$es $(date +%T) ===" >> "$LOG"
  "$BIN" -endpoint "$EP" -start "$start" -pages "$pages" -emptystop "$es" >> "$LOG" 2>&1
  local rc=$?
  echo "=== 会话 start=$start 退出码 $rc $(date +%T) ===" >> "$LOG"
  sleep 30
}
run 382 19 25
run 472 129 40
echo "FILL2-DONE" >> "$LOG"
