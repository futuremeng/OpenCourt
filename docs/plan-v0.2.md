# v0.2 实施方案（已定案）

本文是 [roadmap-next.md](roadmap-next.md) 的**执行裁定**：范围、设计决策、数据模型、
接口签名、文件清单、验收标准均已定死，按本文直接开工，不再讨论备选。

## 1. 范围

**做（v0.2）**

| 里程碑 | 内容 | 工期 |
|---|---|---|
| M1 | 案件实体与案件级隔离 | 3–4 天 |
| M2 | 法条检索链路 + 文书引用校验（OpenLaw） | 4–5 天 |
| M3 | 类案检索接入（LocalCases） | 4–5 天 |
| M4-接口 | `MatchElements` 支持从 OpenLaw `AnalyzeElements` 注入要件；无数据时显式降级 | 2 天 |

**不做（明确推到 v0.3 或永不）**

- M5 向量混合检索、M6 对抗推演与庭审流程引擎、M7 审校报告、M8 评测 runner。
- OCR 引擎接入、docx 解析、交换包、插件加载器。
- 图数据库、外部向量库、Drools、LangGraph、微服务拆分（见 roadmap-next §6）。

**为什么是这个范围**：M1–M3 全部是「把已有但断开的能力接起来」——OpenLaw SDK 已接入
但没有检索入口，`CaseSourcePlugin` 有接口没有实现，事实没有案件边界。这三项零新外部
依赖、零新数据依赖，且是 M6 庭审模拟的前提（无案件边界则推演无意义，无要件/类案则
争点归纳无骨架）。

## 2. 设计决策（定案）

### D0 项目边界总则：语料所有权决定能力归属

**定案规则**：谁拥有一份语料，谁就拥有该语料的**索引、检索与语料层校验**能力；
OpenCourt 只拥有**案件**，因此只负责调用、取舍、案件层检验与合规输出。
单向读，永不写回。

| 语料 / 能力 | 归属 | OpenCourt 允许做的 | OpenCourt 禁止做的 |
|---|---|---|---|
| 法规语料（PDF/md/db）、条文索引、版本链 | **OpenLaw** | 调 `Search`/`GetArticle`/`ResolveVersion` | 自建法条索引、自建法条检索、缓存条文正文供再召回 |
| 「这条引用是否真实/当时生效版本/引文是否一致/溯源是否断」 | **OpenLaw**（只有它有 PDF+哈希+manifest） | 调 `CheckCitation`/`DetectBreak`，把码值原样上浮 | 自行判定法条效力、自行判废止/修改关系 |
| 构成要件库（element 表）与人工确认 | **OpenLaw** | 按案由取要件、机械比对 | 用模型生成或补全要件 |
| 裁判文书语料与其索引、精确过滤、facet 计数 | **LocalCases** | 调 `/api/search`/`/api/case/{id}`/`/api/health` | 自建类案索引、抓取裁判文书、自行统计裁判倾向 |
| 案件、卷宗、事实五态、引用留痕、门禁、脱敏、审计、推演 | **OpenCourt** | 全部 | — |

**两种「检验」必须分开，不可混为一谈**：

1. **语料层校验（归数据项目）**：引用是否真实存在、当时生效版本为何、引文与原文是否
   一致、溯源链是否断、要件是否已人工确认。OpenLaw 有原始 PDF/哈希/manifest/版本链，
   只有它能判；OpenCourt 判不了也不得判。
2. **案件层检验（归 OpenCourt）**：这条引用是否挂到正确的事实与案件、该事实处于哪个
   五态、能否覆盖要件、能否经 `gate` 输出、是否已脱敏、是否写入审计。数据项目不知道
   案件上下文，做不了这件事。

一句话：**OpenLaw 与 LocalCases 回答「这个引用真不真」，OpenCourt 回答「这个引用在本案
里能不能用、怎么呈现」。**

**引用留痕不是语料副本**：`law_refs.quote` / `case_refs.summary` 存的是「我引用了什么」
的证据快照（含 `retrieved_at`），**不得用于再检索召回**，也不得被当作本地语料库；
需要更多内容时回源重新查询。

**脱敏唯一强制关口在 OpenCourt**（`core/redact`）：OpenLaw 的 `Desensitize` 与
LocalCases 侧的任何处理都只是各自语料的可选预处理，**不能替代** OpenCourt 的强制脱敏；
外部返回内容进模型或进 UI 前必须再过一次 `core/redact`。

**当前代码的四处越界（v0.2 内修掉）**

| # | 违例 | 证据 | 修法 |
|---|---|---|---|
| V1 | OpenCourt 自行推导法条效力状态 | `plugins/law_openlaw.go: openlawStatus()` 用 `expire_date` 是否为空判 `AMENDED/EFFECTIVE` | 效力状态是**语料层判断**，应下沉 OpenLaw：SDK 返回显式 `effect_status`（含 pending 语义），OpenCourt 只透传展示，不再自行映射 |
| V2 | `core/retrieve` 自称拥有法条/类案检索 | `retrieve.go` 定义 `LawEntry`/`CaseEntry`/`Result{Kind: LAW/CASE/DOC}` | 删除 `LawEntry`/`CaseEntry` 与 `Kind=LAW/CASE`；`core/retrieve` 定位收窄为**本案卷宗内检索**（OpenCourt 自己的语料），法条/类案一律走插件到数据项目 |
| V3 | 要件名由模型自由生成 | `neutral/reader.go` prompt：`"related_element":"可选要件名"` | 要件名必须来自 OpenLaw 要件库枚举（按案由注入候选集，模型只能从中选或留空）；无要件库时该字段一律置空，不匹配不入库 |
| V4 | 程序期限规则无出处、硬编码在 OpenCourt | `neutral/procedure_guard.go: DeadlineRule{Name, Days}` 由调用方手填 | 规则数据归 OpenLaw（期限规则作为带法条引用的语料），OpenCourt 只保留引擎 `CheckDeadlines` 与本案事件日期；v0.3 接 `DeadlineRule` 来源 |


### D1 案件隔离：单库逻辑隔离，不物理分库

- `opencourt.db` 内新增 `cases` 表；`facts / conflicts / assumptions / documents /
  elements` 全部加 `case_id TEXT NOT NULL DEFAULT 'CASE-DEFAULT'`。
- 存量数据迁移进 `CASE-DEFAULT`，**不丢历史**；迁移幂等（先查 `PRAGMA table_info`）。
- `profile.yaml` 的 `data_dir` **在 v0.2 明确不启用**：`profiles/loader.go` 加注释与
  加载告警，避免误认为已隔离。roadmap-next §1.2-1 的「形同虚设」据此改为「明确未启用」。

### D2 法条与类案不进 `facts`，走引用表

五态只描述**案件事实**。法条不是事实（不可能「证据驱动升级」），类案要旨不是本案事实
（不可被质证）。因此：

- 新增 `law_refs`（法条引用）与 `case_refs`（类案引用）两张表，均挂 `case_id`。
- `domain.Citation.LawVersion / LawRepealed` 由 `law_refs` 填充——roadmap-next §1.2-6
  的「休眠能力」自此激活。
- `gate.CheckGrounding` 增加分支：ID 前缀 `FACT-` 走事实索引；`LAW-` / `CASE-` 走
  引用表校验（不存在或 `UNVERIFIED`/`pending` 即违规）。

### D3 LocalCases 对接：外部可选组件，与 BrowserOS 同纪律

- stdlib `net/http`（零新增依赖），本机回环，`x-localcases-token` 头。
- 配置 `localcases.base_url / token_env / enabled`，**默认禁用**；token 只经环境变量
  注入（与 `model.api_key_env` 同风格），严禁落盘。
- 不自动拉起 LocalCases 进程；调用前探测 `/api/health`，失败返回指引常量
  `LocalCasesGuidance`（同 `BrowserOSGuidance`），**绝不降级到其他抓取方式**。
- 本项目仍无监听端口：全部为出站连接。

### D4 文书法条引用抽取：正则确定性，不用模型

「《中华人民共和国XX法》第X条」由正则确定性抽取（模型不参与），逐条交 OpenLaw
`CheckCitation(lawName, articleNo, quote, atDate)` 核验。这是 M7 审校的确定性内核，
必须可复现、可审计。

### D5 仓储作用域：`Store.WithCase(caseID)`

不往每个方法加 `caseID` 参数（改动面大且易漏）。引入 `*CaseScope` 视图：

```go
scope := a.store.WithCase(a.currentCaseID)
scope.ListFacts()   // 自动限定案件
```

跨案件访问在 SQL 层不可能发生（WHERE 强制），不靠 UI 隐藏。

### D6 分布统计是唯一合法的概率类输出

LocalCases `group_by` facet 返回的命中数/年份/法院分布**可以**呈现，但：

- 必须附固定标注：「以上为本地语料检索结果的分布，不是裁判概率或胜诉可能性」；
- `core/gate` 词表同步新增概率化表述（胜算、可能性多大、几成、倾向判决等），
  合规测试先行断言其被拦截。

### D7 语料所有权：只读打开 + 版本协商 fail-closed

三项目各自可独立分发、通常一并安装，因此必须防止「装了就多出一份语料副本」与
「消费方改动生产者数据」：

- **共享语料位置优先**（发现链见 [integration-model.md](integration-model.md) §3），
  命中即只读使用，**不复制、不安装、不迁移**；私有副本仅作为「只装了 OpenCourt」时的首次铺设。
- OpenLaw 语料一律经 **`sdk.OpenReadOnly`** 打开（`query_only` + 不跑迁移）；
  在 OL-7 落地前，M2 **不得**上线任何写路径调用。
- 打开前做 **schema/producer 版本协商**（OL-8）：超范围直接报错 + 出路指引，
  不迁移、不部分读、不猜。
- **OpenCourt 绝不启动 LocalCases / OpenLaw 进程**，连不上只给指引。

## 3. 数据模型

### 3.1 新增/变更 DDL（`core/storage`）

```sql
CREATE TABLE IF NOT EXISTS cases (
    case_id     TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    case_type   TEXT,                  -- 民事/行政…（案由自由文本，不自动判定）
    stage       TEXT NOT NULL,         -- domain.CaseStage 枚举码
    visibility  TEXT NOT NULL DEFAULT 'BOTH',
    created_at  TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS law_refs (
    ref_id         TEXT PRIMARY KEY,   -- LAW-<hex>
    case_id        TEXT NOT NULL,
    article_id     TEXT NOT NULL,      -- OpenLaw 稳定标识
    law_name       TEXT NOT NULL,
    article_no     TEXT,
    quote          TEXT,               -- 文书中的引文（核验用）
    effective_date TEXT,
    status         TEXT NOT NULL,      -- EFFECTIVE / AMENDED / UNVERIFIED
    confirm_status TEXT NOT NULL,      -- confirmed / pending（OpenLaw 原样透传）
    verify_result  TEXT,               -- OK 或 OpenLaw 错误码（OL-1001..OL-1006，见下）
    at_date        TEXT,               -- 行为发生时点（版本回溯基准）
    created_at     TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS case_refs (
    ref_id         TEXT PRIMARY KEY,   -- CASE-<hex>
    case_id        TEXT NOT NULL,
    source_case_id TEXT NOT NULL,      -- LocalCases 内部 id（字符串化）
    source_url     TEXT,
    case_name      TEXT, court TEXT, cause TEXT,
    judgment_date  TEXT, legal_basis TEXT,
    summary        TEXT,               -- 裁判要旨（已脱敏）
    retrieved_at   TIMESTAMP NOT NULL,
    facets_json    TEXT                -- 检索分布（标注为非概率）
);
-- 既有五表幂等迁移：ALTER TABLE <t> ADD COLUMN case_id TEXT NOT NULL DEFAULT 'CASE-DEFAULT'
```

### 3.2 `core/domain` 新增类型

```go
type Case struct {
    CaseID    string    `json:"case_id"`
    Title     string    `json:"title"`
    CaseType  string    `json:"case_type,omitempty"`
    Stage     CaseStage `json:"stage"`
    CreatedAt time.Time `json:"created_at"`
}

// LawRef 法条引用（非事实）。VerifyResult 为空 = 尚未核验。
type LawRef struct { ... }   // 字段同 law_refs 表

// CaseRef 类案引用（非事实）。永不进五态。
type CaseRef struct { ... }  // 字段同 case_refs 表
```

`Citation` 结构不变（`LawVersion` / `LawRepealed` 由 `law_refs` 回填）。

### 3.3 `core/storage` 新增 API

```go
func (s *Store) CreateCase(c domain.Case) error
func (s *Store) ListCases() ([]domain.Case, error)
func (s *Store) GetCase(caseID string) (domain.Case, error)

// WithCase 返回限定案件的仓储视图。所有读写自动带 WHERE case_id = ?。
func (s *Store) WithCase(caseID string) *CaseScope

type CaseScope struct{ ... }
// 既有方法（迁移为方法集）：
//   CreateFact / ListFacts / GetFact / UpdateFactState
//   ListConflicts / SaveDocument / ListDocuments / SaveElements / ListElements
// 新增：
//   AddLawRef(r domain.LawRef) error    / ListLawRefs() ([]domain.LawRef, error)
//   UpdateLawRefVerify(refID, result string) error
//   AddCaseRef(r domain.CaseRef) error  / ListCaseRefs() ([]domain.CaseRef, error)
```

`GetFact(id)` 在 scope 内查不到即返回 not-found——跨案件引用天然失败，
`gate.CheckGrounding` 因此自动拦截跨案件引用。

## 4. 接口签名

### 4.1 M2 法条链路：`neutral/lawassist.go`（新）

```go
// SearchLaw 关键词检索法条（OpenLaw sdk，禁直连 DB）。
func SearchLaw(ctx context.Context, src plugins.LawSourcePlugin, query string, topK int) ([]plugins.LawHit, error)

// ExtractLawCitations 正则确定性抽取文书中的法条引用（模型不参与）。
func ExtractLawCitations(text string) []LawCitationDraft

// VerifyCitations 逐条交 OpenLaw CheckCitation 核验，产出 law_refs 记录。
// VerifyResult 直接采用 OpenLaw 错误码，不做二次翻译（避免语义漂移）：
//   ok=核验通过；OL-1001 未找到法规；OL-1002 指定日期无生效版本；
//   OL-1003 条款号不存在；OL-1004 引文与原文差异大；OL-1005 版本已失效；
//   OL-1006 溯源断链（数据照常入库，UI 必须标「无法溯源」，绝不静默通过）。
// confirm_status != confirmed → ConfirmStatus=pending，UI 标「待人工确认」。
func VerifyCitations(ctx context.Context, v LawVerifier, atDate string, drafts []LawCitationDraft) ([]domain.LawRef, error)

// LawVerifier 便于测试的窄接口（OpenLawSource 实现）。
type LawVerifier interface {
    CheckCitation(ctx context.Context, lawName, articleNo, quote, atDate string) (plugins.CitationVerdict, error)
    FindRelation(ctx context.Context, articleID string) ([]plugins.Relation, error)
}
```

`plugins/law_openlaw.go` 相应扩展（sdk 已有能力，仅补透传与状态映射）：
`CheckCitation` / `FindRelation` / `ResolveVersion` 三个方法 + `CitationVerdict` /
`Relation` 两个 DTO。`confirm_status=pending` 必须原样上浮，UI 显示「待人工确认」。

**M2 前置（阻塞项）**：`plugins/law_openlaw.go` 的初始化必须改为
`sdk.OpenReadOnly` + 版本协商（依赖 **OL-7 / OL-8**，见
`OpenLaw/docs/tasks-opencourt-bridge.md`）。两项未落地前，M2 只能以
「只读、且语料 schema 已确认不高于当前 SDK」的方式演示，**不得**在用户机器上
用读写入口打开共享语料——现状 `store.Open()` 会跑迁移并把 `meta.schema_version`
回写成内嵌 SDK 的版本号，属于对生产者语料的破坏（隐患 H1/H2）。

### 4.2 M3 类案接入：`plugins/case_localcases.go`（新）

```go
type LocalCasesSource struct { baseURL, token string; enabled bool; client *http.Client }

func NewLocalCasesSource(baseURL, token string, enabled bool) *LocalCasesSource
func (s *LocalCasesSource) Name() string    // "localcases"
func (s *LocalCasesSource) Enabled() bool
// 实现 plugins.CaseSourcePlugin：
func (s *LocalCasesSource) SearchCases(ctx context.Context, query string, topK int) ([]plugins.CaseHit, error)
// 扩展能力（接口 CaseSourcePlugin 增补可选方法，或另立 FilteredCaseSource 接口）：
func (s *LocalCasesSource) SearchFiltered(ctx context.Context, q CaseQuery) (CaseSearchResult, error)
func (s *LocalCasesSource) Health(ctx context.Context) (bool, string)  // GET /api/health

const LocalCasesGuidance = "类案源未启用：请启动 LocalCases（serve，回环地址）并在配置中显式启用 localcases"
```

`CaseQuery` 字段（**完整契约与真实参数名见
[localcases-integration.md](localcases-integration.md) §4/§11**）：`Search / Court /
Cause / Law / Procedure / YearFrom / YearTo / DateFrom / DateTo / GroupBy / FacetSize /
Offset / IDAfter / Sort`。

⚠️ 两个易错点：法条过滤参数是 **`law`**（响应字段才叫 `legal_basis`）；接口**没有
`limit`**，每页固定 20 条，分页走 `offset` 或 `id_gt` 游标。

**M3 前置（阻塞项）**：token 发现。LocalCases 桌面版每次运行随机生成 token 且不落盘
（`desktop/src/main.rs:56`），config 里也没有 token 字段——**一并安装场景 OpenCourt
拿不到 token**（隐患 H3）。依赖 **LC-6 发现文件 / LC-7 health 版本**；未就绪时
`base_url` + `token_env` 仅供 CLI 用户手动接，UI 必须标注这是临时通道。
OpenCourt **不启动** LocalCases 进程。

### 4.3 M3 类案管道：`neutral/caseassist.go`（新）

```go
// RunCaseSearch 检索 → 要旨脱敏 → 产 CaseRef（不进 facts，永不进五态）。
func RunCaseSearch(ctx context.Context, src plugins.CaseSourcePlugin, names []string, q plugins.CaseQuery) (CaseSearchReport, error)
```

`CaseSearchReport` 含 `Refs []domain.CaseRef`、`Facets map[string][]FacetBucket`、
`Disclaimer string`（固定标注文案，见 D6）。未启用 → 直接报错，不降级。

### 4.4 M4 要件注入：`neutral/neutral.go` 扩展

```go
// ElementSource 要件来源（OpenLaw AnalyzeElements 适配器；nil = 无要件库）。
type ElementSource interface {
    ElementsByCaseType(ctx context.Context, caseType string) ([]ElementSpec, error)
}
// MatchElements 保持原签名（前端手输要件仍可用）；新增按案由版本：
func MatchElementsFrom(facts []domain.Fact, src ElementSource, caseType string) (MatchResult, error)
```

`src == nil` 或案由无要件 → 返回明确错误「无要件库，请手输要件或导入要件」，
**绝不回退到模型自由生成要件**。


## 5. 应用服务面（Wails 绑定）与配置

### 5.1 新增绑定（`main.go`）

```go
// M1 案件
CreateCase(title, caseType, stage string) (domain.Case, error)
ListCases() ([]domain.Case, error)
SwitchCase(caseID string) error          // 切当前案件；审计 CASE_SWITCH
CurrentCase() (domain.Case, error)

// M2 法条
SearchLaw(query string, topK int) ([]plugins.LawHit, error)
VerifyDocLawCitations(docID string, atDate string) ([]domain.LawRef, error)  // 抽取+核验+入库 law_refs
ListLawRefs() ([]domain.LawRef, error)
GetLawRelations(refID string) ([]plugins.Relation, error)

// M3 类案
SearchSimilarCases(search, court, cause string, yearFrom, yearTo, topK int) (neutral.CaseSearchReport, error)
ListCaseRefs() ([]domain.CaseRef, error)

// M4 要件
RunElementMatchByCaseType(caseType string) (neutral.MatchResult, error)
```

既有绑定（`ListFacts` / `CreateFact` / `UpgradeFact` / `RunReader` / `BuildTimeline` /
`RunAggregate` / `RunElementMatch` / `RunBlindspots` 等）内部一律改用
`a.store.WithCase(a.currentCaseID)`，签名不变，前端无感。

### 5.2 配置新增（`config/config.go` + `settings.example.yaml`）

```yaml
localcases:
  # 类案源（外部可选组件，默认禁用）：指向用户自装 LocalCases 的回环 HTTP 服务。
  # 留空或 enabled=false = 类案检索不可用（默认离线安全）。
  base_url: ""            # 例 http://127.0.0.1:5010
  token_env: ""           # 例 LOCALCASES_TOKEN；token 严禁写入本文件
  enabled: false
```

环境变量覆盖：`OPENCOURT_LOCALCASES_BASE_URL` / `OPENCOURT_LOCALCASES_TOKEN_ENV` /
`OPENCOURT_LOCALCASES_ENABLED`（`applyEnv` 补三条）。
`case.current` 持久化：`<app data dir>/state.json`（当前案件 id，重启恢复）。

### 5.3 前端（`frontend/src/`）

- 新增 `components/CaseBar.vue`：案件切换器（顶栏，显示当前案件 + 新建 + 切换）。
- 新增 `components/LawSearch.vue`：法条检索 + 引用核验结果表（`EFFECTIVE/AMENDED/
  UNVERIFIED` 徽章、`pending` 橙标「待人工确认」、断链红标「无法溯源」）。
- 新增 `components/SimilarCases.vue`：类案检索（过滤条件 + 结果卡 + 分布统计
  + 固定标注文案「不是裁判概率」）；未启用时显示启用指引而非空列表。
- `App.vue` 拆分上述面板（顺带解决 695 行单文件问题，仅拆不改行为）。

## 6. 文件清单

**新增**

```
core/domain/case.go                     Case / LawRef / CaseRef 类型
core/storage/cases.go                   cases 表 + WithCase/CaseScope + refs 仓储
core/storage/migrate_caseid.go          幂等 ALTER 迁移（PRAGMA table_info 探测）
neutral/lawassist.go                    抽取 + 核验 + 检索管道
neutral/lawassist_test.go
neutral/caseassist.go                   类案管道（脱敏 + CaseRef + 分布标注）
neutral/caseassist_test.go
plugins/case_localcases.go              LocalCases HTTP 客户端
plugins/case_localcases_test.go         httptest 假服务（含 token 校验、404、未启用）
plugins/law_openlaw_verdict.go          CheckCitation / FindRelation / ResolveVersion 透传
frontend/src/components/CaseBar.vue
frontend/src/components/LawSearch.vue
frontend/src/components/SimilarCases.vue
docs/plan-v0.2.md                       本文
```

**修改**

```
core/domain/models.go            注释：Citation 法条字段由 law_refs 回填
core/storage/storage.go          五表加 case_id；仓储迁入 CaseScope
core/gate/gate.go                CheckGrounding 支持 LAW-/CASE- 前缀分支；词表扩充
core/statemachine/statemachine.go 新增：refs 不参与状态升级（显式拒绝）
core/retrieve/retrieve.go        【V2】删 LawEntry/CaseEntry 与 Kind=LAW/CASE；
                                 包定位收窄为「本案卷宗内检索」，包注释写明边界
config/config.go                 localcases 配置块 + env 覆盖 + validate
config/settings.example.yaml     localcases 段
plugins/law_openlaw.go           【V1】删 openlawStatus 自行映射，改透传 OpenLaw
                                 返回的 effect_status（未就绪前保守返回 UNKNOWN）
plugins/law_openlaw_verdict.go   CheckCitation / FindRelation / ResolveVersion 透传
neutral/reader.go                【V3】related_element 候选集由要件库注入，模型只能
                                 选或留空；无要件库时字段强制置空
main.go                          案件上下文、WithCase 全面接入、新绑定、审计事件
profiles/loader.go               data_dir 未启用告警注释
tests/compliance/compliance_test.go  新增 4 条合规用例（见 §7）
docs/architecture.md / README.md     指向本文
```

**审计事件新增**：`CASE_CREATE` / `CASE_SWITCH` / `LAW_SEARCH` / `LAW_VERIFY` /
`LAW_VERIFY_FAIL` / `CASE_SEARCH` / `CASE_SEARCH_DISABLED` / `REF_GATE_BLOCK`。

## 7. 验收标准（必须全绿）

**包内单测**

- `core/storage`：迁移幂等（重复 Open 不报错、存量归 `CASE-DEFAULT`）；
  `WithCase` 跨案件读不到事实；refs 表按案件隔离。
- `neutral/lawassist`：正则抽取表驱动（《》全称/简称、条/款/项、多引用）；
  核验判定六分支（ok / OL-1001 / OL-1002 / OL-1003 / OL-1004 / OL-1006）；
  `pending` 关系与 `confirm_status` 原样透传。
- `plugins/case_localcases`：未启用即报错；token 缺失 401 处置；`/api/health`
  失败返回 `LocalCasesGuidance`；查询参数映射正确；超时不挂死。
- `neutral/caseassist`：要旨脱敏后才落 `case_refs`；`Disclaimer` 恒非空。

**合规测试（`tests/compliance/`）**

1. `TestCrossCaseReferenceBlocked`：A 案件事实被 B 案件结论引用 → 门禁拦截。
2. `TestCaseRefCannotBecomeFact`：类案要旨无法进入 `facts`，无法升级到强态。
3. `TestLawRefUnverifiedSurfaces`：断链/`pending` 法条引用必须显示 UNVERIFIED /
   待人工确认，不得静默通过。
4. `TestProbabilityPhrasingBlocked`：新增概率词（胜算/几成/倾向判决）被拦截；
   分布统计文案（带标注）不被误拦。

**端到端**

- `make test` 全绿；`make lint` 通过（`gofmt` 干净）。
- 真机：OpenLaw 数据目录 → 检索《民法典》条文 → 文书引用核验出 OL-1003（条款不存在）
  与 OL-1004（引文不符）用例；
  LocalCases `serve` → 检索民间借贷类案 → 分布统计 + 标注文案可见。

## 8. 排期与顺序（约 2.5 周）

```
D1-3   M1 案件实体（domain → storage 迁移 → WithCase → main 绑定 → CaseBar）
D4-8   M2 法条链路（plugins 透传 → lawassist → 绑定 → LawSearch → 合规用例 2/3）
D9-13  M3 类案接入（case_localcases → caseassist → 绑定 → SimilarCases → 合规用例 1/4）
D14-15 M4-接口（ElementSource + MatchElementsFrom，无数据显式降级）
D16    App.vue 面板拆分、CHANGELOG「预览版边界」更新、README 能力表更新
```

串行依赖：M1 未完成不得开始 M2/M3（refs 表要挂 `case_id`）。M2 与 M3 可并行分人。

## 9. 风险与对策

| 风险 | 对策 |
|---|---|
| 存量库迁移出错丢数据 | 迁移前自动备份 `opencourt.db.bak-<ts>`；迁移幂等；单测覆盖重复迁移 |
| `WithCase` 改造遗漏调用点导致跨案件泄漏 | 编译期强制：旧的全局方法从 `Store` 移除，只保留在 `CaseScope` 上 |
| LocalCases 契约变动 | 适配层集中一处；`Health` 探测 + 契约字段缺失时如实报错，不做猜测默认值 |
| OpenLaw `CheckCitation` 对简称/别名匹配弱 | 抽取层维护「法名规范化表」（全称/简称），核验失败时并列展示候选法名交人工，不自动采信 |
| gate 词表扩充引入误拦 | 每条新词配正反用例，误拦即回退该词 |


## 10. 跨项目契约要求（提给 OpenLaw / LocalCases）

> 完整对接契约已单独成文：[openlaw-integration.md](openlaw-integration.md)、
> [localcases-integration.md](localcases-integration.md)。本节是摘要，细节以该文为准。

OpenCourt 侧不实现对方职责，只提契约；对方未就绪时 OpenCourt 一律**保守降级 + 显式
标注**，绝不自建替代能力。

### 10.1 提给 OpenLaw

| # | 要求 | 用途 | 未就绪时 OpenCourt 的行为 |
|---|---|---|---|
| OL-1 | `Search`/`GetArticle` 返回显式 `effect_status`（`EFFECTIVE/AMENDED/REPEALED_PENDING/UNKNOWN`），由 OpenLaw 依据 `law_version` 链 + `relation`（仅 `confirmed`）判定 | 废除 V1：OpenCourt 不再自行映射效力 | 显示 `UNKNOWN` + 「效力状态待确认」，不猜 |
| OL-2 | `openlaw-element` CLI：按案由导入构成要件（`pending → 人工确认`，`review_log` 留痕） | M4 要件数据基础（当前 `element` 表 0 行） | 要件匹配降级为「手输要件」，并显式提示无要件库 |
| OL-3 | `AnalyzeElements` 的 `element_id/name` 可作为阅卷抽取的封闭候选集返回 | 废除 V3：要件名不得由模型自由生成 | `related_element` 一律置空 |
| OL-4 | `CheckCitation` 支持法名规范化/别名解析（全称↔简称，如《民法典》↔《中华人民共和国民法典》），解析不了时返回候选列表而非单值 | 文书引用核验可用性 | 并列展示候选法名交人工，不自动采信 |
| OL-5 | 期限规则查询（举证/上诉/送达等期限，带法条引用与 `confirm_status`） | 废除 V4：程序规则数据归语料层（v0.3） | `DeadlineRule` 仍由人工构造，UI 标注「规则未溯源」 |
| OL-6 | 保持 `sdk` 接口稳定（破坏性变更须主版本 + 迁移脚本） | 进程内 import，编译期耦合 | — |
| **OL-7** | `sdk.OpenReadOnly(dataDir)`：`query_only` + 不跑迁移 + 不写 `meta`；上层必须用它 | 修复 H1：现状 `store.Open()` 读写打开并无条件迁移，消费方会改动生产者语料 | M2 不得在用户机器上用读写入口打开共享语料 |
| **OL-8** | schema 版本协商：导出 `SchemaMin/SchemaMax`，先读后判、超范围报错；manifest 增 `producer_version`/`schema_version` 并纳入签名 | 修复 H2：`migrate()` 末尾把 `meta.schema_version` 回写为内嵌 SDK 版本号，可能把 v4 语料标成 v3 | 打开前先只读读 `schema_version`，高于当前 `SchemaVersion` 常量即拒绝 |

### 10.2 提给 LocalCases

| # | 要求 | 用途 | 未就绪时 OpenCourt 的行为 |
|---|---|---|---|
| LC-1 | `/api/search` 查询参数与响应字段（`Hit`/`Meta`/`facets`）作为稳定契约，字段增删须版本化 | 适配层不碎 | 字段缺失如实报错，不做猜测默认值 |
| LC-2 | `x-localcases-token` 鉴权语义稳定；`/api/health` 返回 `indexed_documents` | 启用探测与「语料规模」可见 | 探测失败 → `LocalCasesGuidance`，类案面板不显示空列表 |
| LC-3 | 精确过滤支持 `legal_basis`（法条号）与 `cause`（案由） | 「法条 → 类案」导航（能力在 LocalCases，OpenCourt 只组查询） | 该导航项置灰并说明原因 |
| LC-4 | `group_by` facet 计数与命中同源（同查询同过滤），并给出近似项误差说明 | D6 分布统计的可信基础 | 不展示无标注的计数 |
| LC-5 | **不需要** LocalCases 做脱敏、不需要它做相关性重排、不需要它理解案件上下文 | 边界不扩散 | OpenCourt 自己过 `core/redact` |
| **LC-6** | serve 启动时写发现文件 `<state dir>/localcases.json`（`addr/token/pid/version/schema/indexed_documents`，0600，退出删除） | 修复 H3：桌面版 token 每次随机不落盘，一并安装时连不上 | 降级为手动 `base_url` + `token_env`，UI 标注临时通道 |
| LC-7 | `/api/health` 返回 `version` 与索引 `schema` | 契约不匹配时报错而非误解析 | 缺字段按「版本未知」显示，功能可用但标注 |

### 10.3 明确不由 OpenCourt 承担的事项

法条/类案的索引构建与检索算法、语料清洗与去重、法条效力与废止关系判定、构成要件
数据生产与人工确认、裁判文书抓取、语料完整性与签名校验、类案相关性重排（除非 OpenLaw
或 LocalCases 自身提供）。

