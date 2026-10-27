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

- **立案最终提交是绝对红线，严禁自动执行**：无论用户是否在界面上二次确认，`gate.CheckBrowserAction` 一律拦截提交类动作（词表覆盖 zxfw 实测词汇：提交/递交/重新提起/提起申请/确认立案/预览和提交/pay/submit/delete 等）。执行链三重硬约束使红线不可绕过：① 被拦候选**不进入** pending 登记表（持有 actionID 也无法执行）；② `ExecuteAction` 执行时**复查** gate；③ 候选一次性消费。表单提交必须经当事人在平台页面内亲自审核并完成（HandToHuman）。
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

## 10. P0/P1 已实施（2026-10-25）+ 本机实测记录

已落地代码：

- `plugins/mcp_client.go`：最小 MCP Streamable HTTP 客户端（initialize/tools/list/tools/call；SSE 与 JSON 双形态；`Mcp-Session-Id` 会话）——零新增依赖
- `plugins/browser_browseros.go`：BrowserOS neo 只读插件（`Open/ReadPage/SearchPage/ClosePage`；默认禁用；失败给指引不降级；剥离 `UNTRUSTED_PAGE_CONTENT` 防注入包裹并提取 origin）
- `plugins/plugins.go`：`BrowserPlugin` 接口
- `neutral/browserassist.go`：查阅管道（抓取 → 强制脱敏 → 「单源未补强」事实，Citation=`web:URL@抓取时间#快照哈希`）
- `main.go`：`ResearchWebPage(url, names)` 绑定 + `BROWSER_RESEARCH*`/`REDACT` 审计
- `config`：`browseros.endpoint/enabled`（默认禁用）+ `OPENCOURT_BROWSEROS_ENDPOINT/ENABLED`
- 测试：`plugins/mcp_client_test.go`、`plugins/browser_browseros_test.go`、`neutral/browserassist_test.go`、`tests/compliance`（联网内容未脱敏不得进模型；未启用必须报错）

实测细节（BrowserOS neo 0.0.80，本机）：

- 端点：**9200 被本机 Elasticsearch 占用，neo 实际监听 `127.0.0.1:9211/mcp`**——印证「端口不可硬编码，必须配置注入」
- 握手：`initialize`（SSE 响应 + `mcp-session-id` 头）→ `notifications/initialized`（202）→ `tools/list`（25 个工具）→ `tools/call`
- 关键 schema：`tabs{action:"new",url}` → 文本 `opened page <N>`；`read{page,format:"markdown"}`；`grep{page,pattern,over:"content"}`；`name_session{name,category,summary}`；`request_human_help{reason,kind,...}`（P2 用）
- 页面内容一律包裹 `[UNTRUSTED_PAGE_CONTENT nonce=… origin=URL] … [END_…]`——防提示注入，包裹外还有 "Tip:" 会话提示需丢弃
- 真机联调：`OPENCOURT_BROWSEROS_ENDPOINT=http://127.0.0.1:<port>/mcp go test -tags live ./plugins -run Live -v` → PASS（打开/读取/grep/关闭 example.com）



### P2 辅助操作（2026-10-25 已实施）

- `core/gate/action.go`：动作红线（代码级强制，插件 Execute 处不可绕过）——
  点击类拦截 提交/确认立案/支付/签名/发送/删除；填写类拦截 密码/验证码/短信/签名/人脸
- 插件两段式：`Snapshot`（interactive 模式，[ref=eN] 句柄）→ `ProposeFill/ProposeClick`
  （产候选 + gate 预筛，Allowed=false 原样上报 UI）→ 用户逐条确认 → `ExecuteAction`
  （三重硬约束：必须来自 Propose 登记 / gate 放行复核 / 一次性消费；连接重置后候选全部作废）
- `HandToHuman/AwaitHuman`：登录/验证码/实名/最终提交交人接管（request_human_help/await_human_help）
- `neutral/filingassist.go`：快照表单解析（ExtractFormFields）+ 值映射（BuildFillPlan，
  弱态事实带 FactState 供 UI 标黄）+ `PlanPage` 一键生成候选计划
- `main.go`：`BrowserOpenPage/BrowserPlanFill/BrowserExecuteAction/BrowserHandToHuman/BrowserAwaitHuman`
  绑定；审计事件 `BROWSER_OPEN/PLAN/PLAN_FAIL/EXECUTE/EXECUTE_FAIL/ACTION_BLOCKED/HUMAN/HUMAN_WAIT`；
  执行回执 diff 属页面内容，返回 UI 前强制脱敏
- 纪律：真机联调测试只测只读面（快照），绝不在真实站点执行写操作

### 前端面板（2026-10-25 已实施）

- 顶栏新增 `BrowserOS 已启用/禁用` 徽章（AppInfo.browserEnabled）；未启用时面板内给出完整启用指引（不回退其他抓取）
- 「网络查阅」：URL + 当事人姓名 → 脱敏后正文、快照哈希、脱敏计数、引用坐标（web:URL@时间#哈希）、弱态事实状态
- 「立案表单辅助」：打开页面 → 字段=值（`?` 尾缀标弱态）→ 生成候选计划 → 候选清单（红线拦截项红底展示规则与原因；弱态标黄）→ 逐条「确认执行」→ 回执（已脱敏）；交人接管（登录/验证码/实名/最终提交）与等待接管结果

### 国家规章库清单浏览（2026-10-26 已实施；真机校准同日）

把 BrowserOS 对接用于 OpenLaw 语料管线的**清单层**（详见 OpenLaw `docs/sources-govcn.md`）：

- `neutral/govlistassist.go`：`RunGovListBrowse` 人在环代理浏览——打开官方列表页
  （起点 URL 白名单，唯一常量）→ `evaluate` 只读 DOM 抽取条目（无 fetch/XHR/导航）→
  官方「下一页」分页按钮 DOM click 翻页（等价人工点击，页面自身 JS 取数）→
  URL 硬校验（https + `.gov.cn` 域名族 + `.htm/.html/.shtml`）+ 去重 → 产出与 OpenLaw
  `openlaw-govcn -source gzk` 完全兼容的 `list.json`
- 真机核实形态（neo 0.0.80）：默认列表即**混合清单**（部门规章+地方政府规章，共约
  1000 页）；分类按发布机关**逐条推断**（puborg 含「人民政府」→ 地方政府规章，与
  OpenLaw `gvgzResolveLevel` 同规则）；地方政府规章详情页直链地方政府网站（同
  `.gov.cn` 域名族）——清单如实记录，OpenLaw 侧主机策略决定抓取范围
- SPA 就绪重试：翻页/首屏异步渲染滞后时只重读 DOM（零额外网络请求），最多 4 次
- 硬护栏：翻页间隔下限 2s（默认 2.5s）、单会话 ≤200 页、未知分类过滤值拒绝、
  插件未启用直接报错不降级；evaluate 仅两段固定脚本（抽取/翻页），非自由代码
- `main.go`：`BrowseGovList(cat, maxPages, startPage)` 绑定（cat 为可选过滤：空 = 混合全收；
  startPage 跨会话续接）；产物写 `appData/incoming/govgz/list_agent_<ts>.json`；
  审计 `BROWSER_GOVLIST(_FAIL)`（页数/起始页/条数/停止原因/清单 sha256）
- **跨会话续接（全库约 1000 页 > 单会话 200 页硬上限）**：起始页经官方「跳转」控件
  （`input.jumpText` + 「确定」按钮）到达，并以分页控件选中页码核验——等价人工输入
  页码；核验不过直接报错，绝不在错误页上采集。CLI 批量入口 `cmd/govlistcollect`
  （与应用面板同一代码路径与护栏）+ `scripts/govcollect.sh`（1/201/401/601/801 五会话
  串行，会话间冷却 30s）
- 合规边界：这与「用户自己浏览并点导出」等价——只走主站 robots 允许的官方 UI；
  绝不程序化访问 sousuo/sousuoht 检索域，绝不触碰 Athena RSA 鉴权；
  清单之后由 OpenLaw 侧详情页合规抓取 + 人工确认闸口完成入库
- 测试：`neutral/govlistassist_test.go`（多页翻页/去重/分类过滤/护栏/翻页间隔
  ≥2s 断言）；`plugins/browseros_live_test.go` 增加 `TestLiveGovListBrowse`
  （双开关显式启用：`OPENCOURT_BROWSEROS_ENDPOINT` + `OPENCOURT_GOVLIST_LIVE=1`；
  真机已通过：2 页 13 条混合清单，cat 逐条推断正确）

