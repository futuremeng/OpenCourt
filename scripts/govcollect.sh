#!/bin/bash
# 全量清单采集（http 过滤 bug 修复后重采）：5 会话 × 200 页覆盖 1–1000，会话间冷却 30s
set -u
EP="http://127.0.0.1:9010/mcp"
BIN=/Users/futuremeng/github/futuremeng/OpenCourt/dist/govlistcollect
LOG=/tmp/govcollect_full2.log
: > "$LOG"
run() {
  local start=$1 pages=$2 es=$3
  echo "=== 会话 start=$start pages=$pages emptystop=$es $(date +%T) ===" >> "$LOG"
  "$BIN" -endpoint "$EP" -start "$start" -pages "$pages" -emptystop "$es" >> "$LOG" 2>&1
  local rc=$?
  echo "=== 会话 start=$start 退出码 $rc $(date +%T) ===" >> "$LOG"
  sleep 30
}
run 1 200 15
run 201 200 15
run 401 200 15
run 601 200 15
run 801 200 15
echo "FULL2-DONE" >> "$LOG"

