# 贡献指南

## 开发环境搭建

```bash
git clone https://github.com/futuremeng/OpenCourt.git
cd OpenCourt
make setup     # 安装 Wails CLI（go install github.com/wailsapp/wails/v2/cmd/wails@latest）
cp config/settings.example.yaml config/settings.yaml
make test      # 运行全部测试
make dev       # 启动桌面开发模式
```

国内网络建议：`export GOPROXY=https://goproxy.cn,direct`。

## 代码规范

- **语言**：Go 1.21+（gofmt 格式化，golangci-lint 静态检查）；前端 TypeScript + Vue 3
- **命名**：Go 惯例（导出 PascalCase，非导出 camelCase）；常量 UPPER_SNAKE_CASE
- **类型**：公开函数必须有完整签名与 Docstring（中文，说明 Args/Returns/Raises 语义）
- **依赖方向（强制）**：`profiles/` 与 `adversary/` → `neutral/` → `core/`；`plugins/` 仅通过接口对接 `core/`。**禁止 `core/` 依赖 `neutral/` 或 `profiles/`**，确保内核中立
- **PR 审查标准**：逻辑正确、测试覆盖、无硬编码密钥、**无引导性逻辑**（不得新增任何倾向性输出路径）、文档同步更新

## 开发流程

- 分支策略：`main` 稳定分支，`develop` 开发分支；功能分支 `feature/xxx`，修复分支 `fix/xxx`
- 提交规范：Conventional Commits，如 `feat(core): add pdf parser`
- PR 流程：关联 Issue → 描述变更 → 通过 CI → 至少 1 名 Maintainer 审核

## 测试规范

- 单元测试：各 Go 包内 `*_test.go`，核心包（statemachine/gate/redact/audit）覆盖率目标 > 80%
- 合规测试：`tests/compliance/`，验证脱敏、溯源、门禁等安全约束——**涉及输出路径的变更必须补充合规测试**
- 运行：`make test`（等价 `go test ./... -v`）

## 文档规范

- 代码变更必须同步更新 `docs/` 下对应文档；文档使用 Markdown，中文撰写，术语统一

## 问题反馈与讨论

- Issue：使用模板，提供复现步骤、环境信息、日志片段
- 安全漏洞：请私下报告 security@opencourt.dev，勿公开

## 行为准则

尊重多元，禁止歧视、骚扰、人身攻击；聚焦技术，避免政治、宗教等无关争论；遵守法律，不得利用本项目进行非法活动。违反者将被警告或移出社区。
