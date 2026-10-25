# OpenCourt Makefile
# 国内网络建议: export GOPROXY=https://goproxy.cn,direct

WAILS_VERSION := v2.17.0

.PHONY: setup dev build test lint fmt clean

setup:
	go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)

dev:
	wails dev

build:
	wails build

# 三平台打包（Windows 需 Windows 主机或交叉编译环境）
build-all:
	wails build -platform darwin/universal
	wails build -platform linux
	wails build -platform windows

test:
	go test ./... -v

lint:
	go vet ./...
	gofmt -l .
	@test -z "$$(gofmt -l .)" || (echo "请运行 make fmt"; exit 1)

fmt:
	gofmt -w .

clean:
	rm -rf build/bin frontend/dist/assets
