# OpenCourt

面向法律场景的开源智能体框架（桌面端）。为法院、原告、被告及其代理人提供**中立、严谨**的信息整理与逻辑推演辅助：梳理案件事实、识别证据盲区、进行基本逻辑推演，避免在诉讼或审判过程中遗漏关键信息。

> ⚖️ AI 辅助工具，不替代裁判与专业判断。详见 [DISCLAIMER.md](DISCLAIMER.md)。

## 「不引导」原则

- **不提供倾向性结论**：绝不输出「建议起诉」「大概率胜诉」「应当调解」等表述（代码级门禁强制拦截）
- **不消解事实冲突**：冲突并列展示，交由用户自行判断
- **不补全缺失信息**：仅指出「无法判断」与「补证方向」，绝不以模型幻觉填补空白
- **不替代专业决策**：裁判权、诉讼策略制定权、调解决策权完全归属人类

## 形态与技术栈

| 项 | 说明 |
|---|---|
| 分发形态 | **桌面应用**（主）：下载安装包即用，三平台（macOS / Windows / Linux） |
| 框架 | [Wails v2](https://wails.io)：Go 内核 + Vue 3 / TypeScript 前端，单二进制、无 HTTP 服务、无端口暴露 |
| 存储 | SQLite（`modernc.org/sqlite`，纯 Go 无 CGO），数据全本地、不出域 |
| 模型 | 默认离线。本地 Ollama / vLLM 经 `plugins/model/` 显式启用后使用；框架不内置任何模型与法条数据 |
| 机构服务器模式 | 保留可选 Docker 部署，见 `deploy/` |

## 四层架构（依赖严格单向）

```
profiles/  adversary/        ← 立场层 / 对抗推演层（按角色加载）
        ↘        ↙
          neutral/           ← 中立逻辑层（汇聚/核验/要件匹配/盲区/程序守护）
            ↓
          core/              ← 内核层（五态模型/状态机/脱敏/门禁/审计/检索/引用校验）
            ↑
         plugins/            ← 数据源适配（法条源/类案源/模型），仅通过接口对接 core
```

`config/` 为主配置（`OPENCOURT_*` 环境变量可覆盖）；`tests/compliance/` 为合规测试。

## 核心机制（已实现，可运行）

- **事实五态模型**：已溯源确认 / 单源未补强 / 对方主张未验证 / 存在冲突 / 无依据（`core/domain`）
- **状态机三条硬规则**（`core/statemachine`）：证据驱动升级、假设标签贯穿、冲突不自动消解——状态升级由确定性代码仲裁，模型仅提交候选
- **幻觉门禁与引导性检测**（`core/gate`）：虚假事实引用拦截、倾向性表述拦截、已废止法条标红
- **强制脱敏**（`core/redact`）：姓名一致替换（本地还原表）、证件号/账号掩码、地址泛化、案号保留
- **审计日志**（`core/audit`）：JSONL 哈希链，篡改可检出
- **四个 Profile**（`profiles/*/profile.yaml`）：court / plaintiff / defendant / party

## 快速开始

```bash
# 前置：Go 1.21+、Node 18+（国内网络建议 GOPROXY=https://goproxy.cn,direct）
make setup     # 安装 Wails CLI
make dev       # 开发模式（热重载桌面窗口）
make test      # 全部测试（单测 + 合规测试）
make build     # 产出安装包：build/bin/OpenCourt.app（macOS）/.msi/.AppImage
```

首次运行：`cp config/settings.example.yaml config/settings.yaml`（可跳过，默认配置即安全值：离线、遥测关闭、脱敏 high）。

## VS Code 一键调试（按 F5，选择配置）

| 配置 | 一键行为 | 断点位置 |
|---|---|---|
| **1. 调试桌面应用 (Delve 直接启动)** | Delve 编译并启动**桌面应用原生窗口** | `core/ neutral/ profiles/ adversary/` 任意 Go 代码 |
| **2. 调试桌面应用 (wails dev 热重载 + 自动附加)** | 自动起 `wails dev`（前端热重载），任务等待桌面进程出现后自动附加 Delve（:2345），**无需手动选进程** | Go 代码；改 Go 触发重启后需再次 F5 重新附加 |
| **3. 调试前端 (Chrome attach :34115)** | 等待 DevServer 就绪 → 以 9222 端口启动 Chrome 打开 DevServer → 附加 | `frontend/src/**/*.vue / *.ts` |
| **4. 桌面端全栈调试 (Go + 前端)** | 组合 2+3：桌面应用 Go 断点与前端断点同时生效 | Go + 前端 |
| **5. 调试当前 Go 测试** | 调试当前包测试 | 当前 `*_test.go` 所在包 |

前置：安装推荐扩展（打开项目会提示，含 `golang.go`）；`make setup` 安装 Wails CLI；`dlv`（`go install github.com/go-delve/delve/cmd/dlv@latest`，Go 扩展也可自动安装）。Chrome 调试需本机装有 Google Chrome。

## 项目边界（不做的事）

不预测判决结果 · 不引导用户决策 · 不内置裁判文书/法条数据 · 不替代律师和法官判断 · 不恢复隐名信息

## 许可证与合规

- 代码：MIT（见 [LICENSE](LICENSE)）
- 免责声明：[DISCLAIMER.md](DISCLAIMER.md)
- 贡献指南：[CONTRIBUTING.md](CONTRIBUTING.md)
- 架构文档：[docs/architecture.md](docs/architecture.md)
