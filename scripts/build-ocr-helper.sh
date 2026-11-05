#!/bin/sh
# 构建 macOS Vision OCR helper（O1）：产物 ocr_helper，随 .app 打入 Contents/Resources/。
# 需要 Command Line Tools（swiftc）；仅 macOS。
set -eu
cd "$(dirname "$0")/ocr_helper"
swiftc -O -o ocr_helper main.swift
echo "built: $(pwd)/ocr_helper"