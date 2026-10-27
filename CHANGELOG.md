# 更新日志

本项目遵循 Semantic Versioning，版本语义见开发文档 10.4。

## Unreleased（阶段二开发中）

### 变更
- **语料许可可见性**：「法条数据」面板显示 manifest 内的机器可读许可声明（无声明时如实提示自行确认来源合法性）；`LawDataStatus` 新增 `license` 字段
- **代码许可证统一为 Apache-2.0**（OpenCourt 由 MIT 变更，与 OpenLaw 一致；新增明示专利授权条款）；语料数据另行许可：官方文本公有领域 + 本项目汇编产物 CC BY 4.0 + 欧盟部分保留 © European Union 署名（OpenLaw `DATA-LICENSE.md`，随每个语料包分发副本；`index.json` 附 license 字段）；新增第三方依赖许可清单（dmg 内附副本）；DISCLAIMER 第 5 条对齐「官方语料包单独发布」的新分发模式

### 新增
- **国家规章库清单浏览（人在环代理浏览）**：内置智能体经 BrowserOS neo 驱动用户真实浏览器浏览 `www.gov.cn/zhengce/xxgk/gjgzk/` 官方列表页并导出 OpenLaw 兼容清单——`neutral/govlistassist.go`（起点 URL 白名单、evaluate 只读 DOM 抽取、官方「下一页」两段式点击翻页、翻页间隔下限 2s、单会话 ≤200 页、URL 硬校验去重）；插件面新增 `Evaluate`（只读数据抽取，防注入包裹剥离）；`BrowseGovList(cat, maxPages)` Wails 绑定，产物写 `appData/incoming/govgz/list_agent_<ts>.json`，审计 `BROWSER_GOVLIST`；合规边界与「用户自己浏览+导出」等价（绝不程序化访问 sousuo/sousuoht 检索域与 Athena 鉴权），清单交 OpenLaw `openlaw-govcn -source gzk` 走详情页合规抓取+人工确认。真机联调 `TestLiveGovListBrowse`（双开关显式启用）
- **网上立案辅助操作（P2 两段式）**：`core/gate/action.go` 动作红线代码级强制（提交/支付/签名/发送类点击与密码/验证码/人脸类字段一律拦截，Execute 处不可绕过）；插件 `Snapshot/ProposeFill/ProposeClick/ExecuteAction`（候选必须来自 Propose 登记、gate 复核、一次性消费；断线重连后候选全部作废）+ `HandToHuman/AwaitHuman` 交人接管；`neutral/filingassist.go` 快照表单解析与填写计划（弱态事实标黄）；Wails 绑定与全链路审计（执行回执返回 UI 前强制脱敏）
- **前端「网络查阅与立案辅助」面板**：BrowserOS 启用状态徽章与启用指引；网络查阅结果展示（脱敏正文/快照哈希/引用坐标/弱态状态/脱敏计数）；两段式表单辅助 UI（候选清单含红线拦截项红底原因、弱态值黄标、逐条确认执行、交人接管）
- **网络资料查阅（BrowserOS 辅助，P0/P1）**：对接用户自装的 BrowserOS neo（外部可选组件，AGPL-3.0，本仓库不分发；经本机回环 MCP Streamable HTTP 驱动，OpenCourt 仍无监听端口）。`plugins/mcp_client.go` 最小 MCP 客户端（零新增依赖）；`plugins/browser_browseros.go` 只读插件（打开/读取/grep/关闭，默认禁用，连接失败给指引绝不降级抓取，剥离防注入包裹）；`neutral/browserassist.go` 查阅管道：抓取 → 强制脱敏 → 「单源未补强」事实入库（引用=`web:URL@时间#快照哈希`，联网源永不可进强态）；配置 `browseros.endpoint/enabled` + `OPENCOURT_BROWSEROS_*`；真机联调测试（`go test -tags live`）与合规测试（未脱敏不得进模型、未启用必须报错）。设计见 docs/browseros-integration.md
- **法条数据应用内分发**：首屏「法条数据」面板——在线拉取发布索引（GitHub Release `index.json`），勾选下载 core/evidence/update 数据包（下载即 sha256 校验）；亦支持手动选择 zip/目录；core 原子替换带回滚，evidence 只补缺不覆盖
- **程序自更新（macOS）**：启动静默检查 GitHub Release（semver 比较），首屏横幅提示新版本与更新说明；一键「下载并更新」——下载 dmg + 强制 `.sha256` 资产校验（缺失即拒绝安装），分离脚本完成「等待退出 → 挂载 → 替换 .app（失败回滚）→ 重启」；`app.update_api` 可配置镜像源
- **文档接入（core/ingest）**：文本型 PDF 逐页文本层解析（纯 Go）；OCR 适配接口——扫描件/图片强制经 OCR，引擎未配置时明确拒绝，绝不静默入库
- **阅卷 Agent（neutral/reader）**：模型只生成候选事实（描述+片段+页码），核验器逐条回原文定位、状态机判定初始态；定位失败的候选拒绝入库并记审计；时间/金额要素由正则确定性抽取（模型不参与）
- **时间线构建（neutral/timeline）**：事实按同页时间要素锚定排序；无锚点排末尾并显式标注，不虚构日期
- **App 服务链路**：PickDocumentFile / IngestDocument / RunReader / BuildTimeline / RunAggregate / RunElementMatch / RunBlindspots，前端新增「文档接入与阅卷」面板
- **存储**：documents / elements 表；未定位要素（Located=false）禁止入库

### 变更
- **模型对接统一为 OpenAI 兼容接口**（`/chat/completions` + `/embeddings`），不再假定 Ollama：`base_url` 可指向 OpenAI、云厂商兼容端点或本地兼容服务（LM Studio / llama.cpp / vLLM / Ollama `/v1`）；API Key 仅经环境变量注入；每个请求强制附加「禁止用于模型训练」标识头；默认禁用（默认离线安全）

## v0.1.0-preview（2026-10-23）

首个技术预览版（Tech Preview / Alpha），面向开发者展示「反幻觉、可溯源」安全内核。

### 新增

- **内核层（core/）**：事实五态模型与数据模型（开发文档 3.1、6.1-6.3）；状态机三硬规则的确定性仲裁（3.2）；幻觉门禁与引导性表述拦截（4.6、7.4）；强制脱敏与还原表（4.2、7.3）；哈希链审计日志（4.8）；引用定位校验（4.5）；关键词检索与法条废止分离（4.4）；SQLite 本地存储
- **中立逻辑层（neutral/）**：汇聚器（时间线+冲突并列标记）、核验器打回、要件匹配器、盲区生成器、程序期限守护（4.9-4.13）
- **立场层（profiles/）**：court / plaintiff / defendant / party 四套 Profile（5.6 配置表）与默认拒绝式加载器
- **对抗层（adversary/）**：辩论轨迹抖动监控（3.7）；对方模拟与漂移检测接口
- **插件接口（plugins/）**：法条源 / 类案源 / 模型适配器接口；本地 Ollama 适配骨架（默认禁用）
- **桌面端**：Wails v2 + Vue3/TS 单应用；事实看板（五态色标）、录入与状态升级演示、输出门禁自检；数据全本地、默认离线、无端口暴露
- **VS Code 一键调试**：桌面应用 Delve 直接启动 / wails dev 自动附加 / Chrome 前端附加 / 全栈组合
- **合规测试（tests/compliance/）**：虚假事实注入拦截、脱敏泄漏检查、冲突不自动消解、引导性表述拦截
- **文档**：README / DISCLAIMER / CONTRIBUTING / docs/architecture / deploy 说明

### 已知限制

- 文档接入仅支持纯文本/Markdown；PDF、扫描件、OCR、Word、压缩包未实现
- 模型接入未做真机评测；开发文档 11.1 的判决书评测集验收未执行
- macOS 安装包自签名（未公证）；Windows/Linux 安装包待 CI 补齐
- 服务器模式 HTTP API 未实现

### 下载

- macOS（Apple Silicon）：`OpenCourt-0.1.0-preview-darwin-arm64.dmg`
- 自签名说明：首次打开请右键 → 打开，或执行 `xattr -dr com.apple.quarantine /Applications/OpenCourt.app`
