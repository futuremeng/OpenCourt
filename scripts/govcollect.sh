#!/bin/bash
# 国家规章库清单分批采集（人在环代理浏览；每会话 200 页硬上限，官方跳转控件续接）
set -u
EP="http://127.0.0.1:9010/mcp"
BIN=/Users/futuremeng/github/futuremeng/OpenCourt/dist/govlistcollect
LOG=/tmp/govcollect.log
: > "$LOG"
for START in 1 201 401 601 801; do
  echo "=== 会话 start=$START $(date +%T) ===" >> "$LOG"
  "$BIN" -endpoint "$EP" -start "$START" -pages 200 >> "$LOG" 2>&1
  RC=$?
  echo "=== 会话 start=$START 退出码 $RC $(date +%T) ===" >> "$LOG"
  if [ $RC -ne 0 ]; then
    echo "COLLECT-ABORT" >> "$LOG"
    exit $RC
  fi
  # 会话间冷却 30s（对站点的额外礼貌间隔）
  sleep 30
done
echo "COLLECT-DONE" >> "$LOG"
