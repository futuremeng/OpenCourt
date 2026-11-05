# OpenCourt Makefile
# 国内网络建议: export GOPROXY=https://goproxy.cn,direct

WAILS_VERSION := v2.17.0
# 版本单一来源：优先 git tag（发布构建），否则代码内默认值（见 main.go version）。
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null)
LD := $(if $(VERSION),-ldflags="-X main.version=$(VERSION)",)
GOFILES := $(shell git ls-files '*.go')

.PHONY: setup dev build build-all test test-fast lint typecheck fmt clean ci eval

setup:
	go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)

dev:
	wails dev

build:
	wails build $(LD)

# 三平台打包（Windows 需 Windows 主机或交叉编译环境）
build-all:
	wails build -platform darwin/universal $(LD)
	wails build -platform linux $(LD)
	wails build -platform windows $(LD)

# 默认带 -race：Wails 每次前端调用在独立 goroutine 中执行绑定方法，
# App 上的共享状态（当前案件 ID、法条源句柄、配置）必须被竞态检测覆盖。
test:
	go test -race ./...

# 无 race 的快速回归（本地反复跑；race 版本慢 2–10 倍）
test-fast:
	go test ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l $(GOFILES))" || (gofmt -l $(GOFILES); echo "请运行 make fmt"; exit 1)

typecheck:
	cd frontend && npm run typecheck

fmt:
	gofmt -w $(GOFILES)

# 与 ci.yml 等价的本地门禁
ci: lint test typecheck

# E0 评测：门禁对抗集（离线确定性）+ 模型阅卷指标（需显式启用模型与 LocalCases，
# 未启用时报告如实标注「未运行」）。详见 tests/eval/README.md。
eval:
	go run ./scripts/modelcheck eval

clean:
	rm -rf build/bin frontend/dist/assets

