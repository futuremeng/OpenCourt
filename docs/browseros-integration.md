# BrowserOS 对接调研与集成设计

> 状态：调研结论 + 设计草案（未实施）。对应问题：如何把 [BrowserOS](https://github.com/browseros-ai/BrowserOS) 作为 OpenCourt 的「网络资料查阅」与「人民法院在线服务平台（zxfw.court.gov.cn）在线立案辅助」的浏览器操作工具。

## 1. BrowserOS 是什么

BrowserOS（browseros-ai/BrowserOS，AGPL-3.0，Chromium fork）同一 monorepo 发布两种形态：

| 形态 | 定位 | 内置 agent | MCP 端点（本机回环） |
|---|---|---|---|
| **BrowserOS neo** | 「给 agent 用的第二浏览器」：保留用户登录态，agent 在独立标签页并行操作，新标签页 dashboard 实时观看 + 会话录像回放 | 无（由外部 agent 经 MCP 驱动） | `http://127.0.0.1:9200/mcp`（**Streamable HTTP**；端口随构建可能变化，须从浏览器内 MCP 页复制） |
| **BrowserOS（人类版）** | 「给人用的 AI 浏览器」：侧栏聊天 + 内置 agent loop，可 BYO LLM（OpenAI 兼容 / Ollama / LM Studio） | 有 | `chrome://browseros/mcp` 页显示，如 `http://127.0.0.1:9239/mcp`；53+ 浏览器工具 + 40+ OAuth 应用工具 |

关键模块（monorepo）：

```
packages/browseros/          # Chromium fork（补丁/构建/签名）
packages/browseros-agent/
  apps/claw-server/          # neo 后端：agent 连接的 MCP endpoint + dashboard API（Rust）
  apps/claw-app/             # neo 新标签页 dashboard（观看/回放/管理会话）
  apps/server/               # 人类版 Bun server：浏览器 MCP 工具 + 内置 agent loop
  packages/browser-mcp/      # 浏览器 MCP 工具面（工具定义在此）
  crates/browseros-mcp/      # Rust MCP server 实现
skills/browseros-neo/SKILL.md # 官方 agent 使用说明书（推荐随集成安装）
```

隐私模型（与 OpenCourt「数据不出域」高度契合）：登录态、会话记录、截图全部存本机 `~/.browserclaw/`；无云同步、无服务端历史；URL/页面内容/prompt/工具参数与结果/密码/API key **从不外发**；匿名产品事件可一键关闭。

## 2. 对接面（neo 工具清单与核心循环）

neo 经 MCP 暴露的核心工具（以工具描述为准）：

- `tabs`（new/list/…，agent 拥有自己的标签页组）、`navigate`
- `snapshot`：页面渲染为可访问性树，交互元素带 `[ref=eN]` 句柄
- `act`：按 ref 操作元素，支持 `fields[]` 批量填表，读回 diff 自验证
- `read`（页面转 Markdown）、`grep`（页内检索不回传全页）、`evaluate`
- `run`：沙箱脚本，一次调用组合 snapshot→act→verify / 批量抽取（30 秒硬上限，非 Node 非页面上下文）
- 截图 / 存 PDF / 下载 / `upload`（本地文件上传）
- `name_session`（会话命名分类）、`request_human_help` / `await_human_help`（**登录、短信码、验证码、审批等必须人来做的环节，agent 阻塞等待人类接管后交还控制权**）

官方最佳实践见 `skills/browseros-neo/SKILL.md`（`npx skills add browseros-ai/browseros --skill browseros-neo`）。

## 3. 三种对接方案对比

| 方案 | 形态 | 与 OpenCourt 桌面安全模型（无 HTTP 服务、无监听端口）的契合度 | 结论 |
|---|---|---|---|
| **A. OpenCourt 作 MCP client，连 BrowserOS neo** | OpenCourt 主动出站连接 `127.0.0.1:<port>/mcp`；浏览器只是被驱动的执行器 | ✅ 出站回环连接，不新增任何监听端口；插件形态天然落在 `plugins/` 层 | **推荐（产品化路线）** |
| B. OpenCourt 暴露为 MCP server，由编码 agent（Claude Code 等）同时编排 OpenCourt + browseros-neo | 开发/极客玩法 | ❌ 需要 OpenCourt 开监听端口，违背「无端口暴露」；但零改动即可用 | 仅开发期验证用 |
| C. BrowserOS 人类版作宿主 agent，把 OpenCourt 接入其 Connect Apps | 浏览器内置 agent 调 OpenCourt 工具 | ❌ 同 B；且 agent loop 在浏览器侧，绕开 OpenCourt 的状态机/门禁/脱敏 | 不采用 |

**选 A**。BrowserOS 是独立进程 + 标准 MCP 协议交互，不与 OpenCourt 发生代码链接。

## 4. 推荐架构（贴合四层单向依赖）

```
frontend ──(Wails IPC)──→ main.go
                             ↓ 组装
        neutral/ ──→ core/ ←── plugins/browser_browseros（新，仅接口对接）
                              │ MCP Streamable HTTP（出站，仅 127.0.0.1）
                              ↓
                     BrowserOS neo（本机进程，用户登录态在其 profile 内）
```

### 4.1 新插件接口（`plugins/plugins.go` 追加）

```go
// BrowserPlugin 浏览器辅助插件接口（默认禁用；仅经用户显式启用后连接本机 BrowserOS）。
type BrowserPlugin interface {
    Name() string
    Enabled() bool // 未配置 OPENCOURT_BROWSEROS_MCP_URL 时恒为 false，调用即报错（默认离线纪律）

    // —— 只读：网络资料查阅 ——
    Open(ctx context.Context, url string) (Page, error)          // 打开并取快照
    ReadPage(ctx context.Context, pageID string) (string, error) // Markdown 化正文
    SearchPage(ctx context.Context, pageID, query string) ([]string, error)

    // —— 辅助操作：一律「候选动作」两段式，执行需前端逐条确认 ——
    ProposeActions(ctx context.Context, pageID string, fields map[string]string) ([]ActionCandidate, error)
    ExecuteAction(ctx context.Context, actionID string) (ActionReceipt, error) // 仅确认后的候选可执行
    HandToHuman(ctx context.Context, pageID, reason string) error // 登录/验证码/提交 → request_human_help
}
```

实现要点（`plugins/browser_browseros.go`）：

- MCP client：Streamable HTTP 连 `OPENCOURT_BROWSEROS_MCP_URL`（Key 同纪律：仅环境变量，禁写配置文件）。Go 侧可用 `github.com/modelcontextprotocol/go-sdk` 或 `mark3labs/mcp-go`，或最小 JSON-RPC 自实现。
- 端口不硬编码：文档明确「端口随构建变化，从 neo 的 MCP 页复制」，故端点必须可配置，并在连接失败时给出「启动 BrowserOS neo → 新标签页 → MCP → 复制端点」的引导文案（SKILL.md 同款纪律：连接缺失不得静默降级到其他抓取方式）。
- 会话纪律：每次任务 `name_session`（2-3 词任务名 + 无 PII 摘要）；OpenCourt 用自己的标签页组，不动用户标签页。

### 4.2 数据流与安全（关键：浏览器内容进入 core 前的强制管道）

```
页面内容(read/grep/snapshot)
  → core/redact 强制脱敏（姓名/证件/账号/地址）后才允许进入模型上下文
  → 以「单源未补强」态入库：Citation = URL + 抓取时间 + 页面快照哈希（core/verify 可回查）
  → 冲突照常并列，不自动消解
每一次 Open/act/upload → core/audit 哈希链记录（工具名、参数摘要、目标 URL、回执）
```

写操作三段式：**Propose（模型/规则产候选）→ 用户逐条确认（前端 UI）→ Execute**。`ExecuteAction` 只接受已确认的 actionID——与状态机「模型仅提交候选、代码仲裁」的既有纪律同构。


## 5. 两个目标场景

### 5.1 网络资料查阅（低风险，先行）

- 用途：查法院管辖/立案须知、诉讼费标准、送达地址确认要求等**官方网页信息**；补充 OpenLaw 离线语料之外的时效性信息。
- 定位：**辅助源，不是数据源**。框架不内置任何法条/案例数据的原则不变——浏览器抓回的内容一律按「单源未补强」事实入库并标出处，绝不直接当法条引用（引用仍走 LawSourcePlugin 的版本/废止状态契约）。
- 为什么必须真浏览器：已实测 `zxfw.court.gov.cn` 为纯 JS 渲染 SPA，无头 HTTP 抓取只返回空壳页（`Please enable JavaScript`）；且需登录态。BrowserOS neo 的 `snapshot/read/grep` 恰好解决。

### 5.2 zxfw.court.gov.cn 在线立案辅助（受控，后置）

平台已知形态（以实际页面为准）：实名注册/登录（手机号验证码、人脸识别、电子签名）→ 选择法院与案件类型 → 填写当事人信息、诉讼请求、事实与理由 → 上传起诉材料（起诉状、身份证明、证据清单）→ 提交 → 法院审核（受理/补正/不予受理）。

辅助流程设计（agent 只做「读结构、备草稿、代填」，人做「认证、决定、提交」）：

1. **读表单**：`snapshot` 解析在线立案表单字段结构 → 映射到 OpenCourt 案件数据（当事人、诉讼请求、事实时间线、证据清单）。
2. **备草稿**：从 core 存储生成各字段候选值——仅「已溯源确认」事实可直接填入；「单源未补强/存在冲突」字段在 UI 上标黄，要求用户确认或改写（不把弱态事实静默写进官方表单）。
3. **代填**：`act` 的 `fields[]` 批量填入**用户已逐条确认**的值；每批填完读回 diff 展示给用户核对。
4. **人工环节强制交人**：登录、短信验证码、人脸识别、电子签名、以及**最终「提交」按钮**——一律 `HandToHuman`（neo 的 `request_human_help`），cockpit 中用户接管完成后交还。
5. **材料上传**：起诉状等由 OpenCourt 生成/导出到本地文件，经 `upload` 工具挂载，上传前用户确认文件。
6. **回执留痕**：提交成功页、案号/查询码经 `read` + 截图入库，写入审计链，形成当事人自己的办理记录。

### 5.3 合规红线（写入 gate 层，代码级强制）

- **绝不自动点击「提交/确认立案」**：ExecuteAction 白名单排除提交类控件；提交必须 HandToHuman。
- **实名与生物认证绝不代理**：登录、人脸、验证码环节 agent 必须挂起等人。
- **不存储、不读取平台凭据**：登录态只存在于 BrowserOS 本机 profile，OpenCourt 不导出、不复制 cookie。
- **不虚构、不补全**：表单缺什么就标「待补」，绝不用模型幻觉填官方文书（与「不补全缺失信息」原则一致）。
- **不引导**：对立案成功率、管辖判断不给倾向性结论；仅呈现平台要求与已确认事实。
- 材料真实性与提交责任归属当事人（DISCLAIMER 延伸：本辅助不改变这一点）。

## 6. 许可证边界

- BrowserOS 为 **AGPL-3.0**；OpenCourt 为 Apache-2.0。方案 A 中 BrowserOS 是**用户自行安装的独立进程**，OpenCourt 仅经标准 MCP 协议（本机回环 HTTP）调用，无代码链接、无静态/动态依赖 → AGPL 传染性不延及 OpenCourt 源码。
- 需在 `THIRD-PARTY-NOTICES.txt` 与 README 中把 BrowserOS 标注为**外部可选组件**（类似「用户自装浏览器」），并明确 OpenCourt 不分发 BrowserOS 二进制。
- 若未来改为 fork/内嵌 BrowserOS 代码，则整体落入 AGPL——明确不做。

## 7. 实施路线图

| 阶段 | 内容 | 依赖 |
|---|---|---|
| P0 | `BrowserPlugin` 接口 + `browser_browseros` 只读实现（Open/ReadPage/SearchPage）+ 端点配置与连接引导 + 审计 | 用户自装 BrowserOS neo |
| P1 | 抓取内容 → redact → 「单源未补强」入库（Citation 含 URL/时间/快照哈希）；合规测试：未脱敏内容不得进模型 | core/redact、core/verify |
| P2 | 两段式辅助操作（Propose→确认→Execute）+ 前端确认 UI + HandToHuman | Wails IPC 扩展 |
| P3 | zxfw 在线立案辅助流程（表单映射、草稿生成、代填、回执留痕）+ gate 红线（禁自动提交） | P0–P2 + 真实平台联调 |

## 8. 风险与未决事项

- **版本漂移**：neo 迭代快，工具名/参数以 MCP 工具描述为准，插件须运行时 `tools/list` 自适应，不硬编码 schema；端口不可假设固定。
- **平台变动与反自动化**：zxfw 表单结构随版本变化；频繁自动化操作可能触发验证码/风控——这正是「代填不代交、认证必交人」设计的另一理由。
- **官方口径确认**：在线立案的具体步骤/材料要求应以平台与法院现行指引为准，文档中的流程为通用描述。
- **评测**：「表单字段映射准确率」应纳入后续合规测试，映射不确定时降级为纯人工填写建议。

## 9. 参考

- 仓库：https://github.com/browseros-ai/BrowserOS （AGPL-3.0）
- 文档：https://docs.browseros.com （neo MCP：/neo/mcp/manual；人类版 MCP：/browseros/features/use-with-claude-code）
- 官方 skill：https://raw.githubusercontent.com/browseros-ai/BrowserOS/main/skills/browseros-neo/SKILL.md
- 目标平台：https://zxfw.court.gov.cn/ （JS 渲染 SPA，须真浏览器 + 登录态）
