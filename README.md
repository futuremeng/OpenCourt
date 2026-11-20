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
| 模型 | 默认离线。统一对接 **OpenAI 兼容接口**（`/chat/completions` + `/embeddings`），不假定服务商——可指向任何云 API 或本地兼容服务（LM Studio / llama.cpp / vLLM / Ollama `/v1` 等）；经配置显式启用后才会发起请求；框架不内置任何模型与法条数据 |
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
- **文档接入**（`core/ingest`）：文本 / 文本型 PDF（逐页文本层）；扫描件与图片强制经 OCR——引擎未配置时明确拒绝，绝不静默入库
- **阅卷 Agent**（`neutral/reader`）：模型只产候选事实，核验器逐条回原文定位仲裁；定位失败即拒绝入库并记审计；时间/金额要素由正则确定性抽取
- **时间线 / 要件匹配 / 盲区**（`neutral`）：按时间要素锚定排序（不虚构日期）；弱态事实不得覆盖要件；盲区只输出「无法判断 + 补证方向」
- **OpenAI 兼容模型适配**（`plugins/model_openai.go`）：默认禁用、Key 仅经环境变量注入、请求强制附加「禁止用于模型训练」标识头
- **网络资料查阅（BrowserOS 辅助）**（`plugins/browser_browseros.go` + `neutral/browserassist.go`）：默认禁用；经本机回环 MCP 驱动用户自装的 BrowserOS neo；抓回内容强制脱敏后才进模型，一律以「单源未补强」事实入库（引用含 URL/时间/快照哈希），联网源永不进强态；详见 `docs/browseros-integration.md`
- **网上立案辅助（两段式）**（`core/gate/action.go` + `neutral/filingassist.go`）：表单填写一律「候选 → 用户逐条确认 → 执行」；提交/支付/签名与密码/验证码/人脸环节红线代码级强制不代做，登录与最终提交交人接管；执行回执返回 UI 前强制脱敏

## 快速开始

```bash
# 前置：Go 1.27+（与 go.mod 一致）、Node 20+（国内网络建议 GOPROXY=https://goproxy.cn,direct）
make setup      # 安装 Wails CLI
make dev        # 开发模式（热重载桌面窗口）
make test       # 全部测试（-race：Wails 每次前端调用在独立 goroutine 中执行）
make test-fast  # 无 race 的快速回归
make lint       # go vet + gofmt 检查
make typecheck  # 前端类型检查（vue-tsc --noEmit）
make ci         # 与 .github/workflows/ci.yml 等价的本地门禁
make build      # 产出安装包：build/bin/OpenCourt.app（macOS）/.msi/.AppImage
```

首次运行：`cp config/settings.example.yaml config/settings.yaml`（可跳过，默认配置即安全值：离线、遥测关闭、脱敏 high）。

版本号单一来源：发布构建由 `make build` 从 `git describe --tags` 注入
（`-ldflags "-X main.version=$(git describe --tags --abbrev=0)"`）；源码内
`main.go` 的 `version` 常量只是无 tag 时（开发版）的兜底值，不要手工改。

密钥注入：`.env` 只在**进程当前工作目录**读取（`config.LoadDotEnv(".env")`，
已存在的环境变量优先）。因此它只对 `make dev` / 命令行启动生效——从 Finder
双击启动的 .app 工作目录是 `/`，不会读到项目下的 `.env`；分发形态请改用系统
密钥链或环境注入，切勿把密钥写进 `config/settings.yaml`。

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

## 版本边界（v0.3.0）

⚠️ **本版本仍是面向开发者的技术预览，不是面向法律从业者的产品**：

- **已可用**：反幻觉安全内核（事实五态、状态机仲裁、门禁、脱敏、哈希链审计）、
  四套 Profile、合规测试、桌面应用外壳；**案件实体与案件级隔离**（多案件切换、
  跨案件引用编译期 + 门禁双拦截）；**法条检索与文书引用核验**（OpenLaw 进程内
  SDK，效力状态透传 `effect_status`，OL 错误码原样上浮）；**类案检索**（LocalCases
  回环手动通道，LC-6 发现文件就绪前 UI 已标注；要旨永不进强态 + 固定标注）；
  **按案由要件匹配**（要件名封闭候选集，集外一律置空）；文档接入（文本 / 文本型
  PDF）与阅卷 Agent、时间线与盲区；**扫描件/图片 OCR**（`ingest.ocr.engine`：
  macOS Vision / Tesseract 本地引擎，默认 none 明确拒绝；OCR 文本入库封顶弱态、
  低置信页标黄需人工校对，绝不由模型自动改字）；**Word/docx 解析**（纯 Go；
  修订痕迹与批注作独立标注区块上浮、不作正文引用来源；表格线性化标注；
  内嵌图片走 OCR 管道）；**审校与报告导出**（五项确定性审校检查，模型零参与；
  Markdown 报告默认脱敏导出、原样导出需二次确认并留审计；报告全文过输出门禁）；
  法条数据应用内分发与自更新（macOS）；
  BrowserOS 网络查阅与网上立案辅助（两段式，默认禁用）；OpenAI 兼容模型接口
  （默认禁用）；**庭审阶段状态机与对抗推演**（四阶段确定性硬守卫、对方模拟输入按
  Visibility 硬过滤、立场漂移检测、被拦截轮次照常留痕）；**本案卷宗混合检索**
  （页级向量 + RRF 融合，降级四态显式标注，模型未启用为纯关键词；只搜本案卷宗，
  法条归 OpenLaw、类案归 LocalCases）
- **尚不可用**：模型真机评测（需显式启用模型与 LocalCases 后 `make eval`）、
  跨部署交换包、法院场景包（v0.4 P1，见
  [docs/plan-v0.4.md](docs/plan-v0.4.md)）
- **验收进度**：门禁对抗集 runner 已就位（`make eval`，21 条样本，误拦率/漏拦率
  均 0.00%，报告见 `tests/eval/`）；开发文档 11.1 的模型阅卷指标（100 份真实
  判决书、幻觉率 < 1%、溯源准确率 > 98%）**尚未运行**——需显式启用模型与
  LocalCases 后 `make eval`（流程见 `tests/eval/README.md`），结果如实进本清单
- **请勿用于真实案件**。macOS 构建为自签名，首次打开需右键 → 打开（或
  `xattr -dr com.apple.quarantine /Applications/OpenCourt.app`）

## 许可证与合规

- 代码：Apache-2.0（见 [LICENSE](LICENSE)，与 OpenLaw 一致）
- 语料数据：另行许可（法条原文为公有领域；本项目汇编产物 CC BY 4.0；EU 数据保留 © European Union 署名），见 OpenLaw [DATA-LICENSE.md](https://github.com/futuremeng/OpenLaw/blob/main/DATA-LICENSE.md)，每个语料包内附副本
- 第三方依赖许可清单：[THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt)（dmg 内附副本）
- 免责声明：[DISCLAIMER.md](DISCLAIMER.md)
- 贡献指南：[CONTRIBUTING.md](CONTRIBUTING.md)
- 架构文档：[docs/architecture.md](docs/architecture.md)
- 下一阶段规划（庭审模拟与案件推演方向，盘点 + 里程碑 M1–M8）：
  [docs/roadmap-next.md](docs/roadmap-next.md)
- **v0.4 实施方案（提案：评测前置 + 真实卷宗接入 + 报告导出 + 法院场景包）**：
  [docs/plan-v0.4.md](docs/plan-v0.4.md)
- **v0.2 实施方案（已定案：案件实体 + 法条链路 + 类案接入）**：
  [docs/plan-v0.2.md](docs/plan-v0.2.md)
- 跨项目对接契约：[docs/openlaw-integration.md](docs/openlaw-integration.md) ·
  [docs/localcases-integration.md](docs/localcases-integration.md) ·
  调用方式与语料所有权设计：[docs/integration-model.md](docs/integration-model.md)
