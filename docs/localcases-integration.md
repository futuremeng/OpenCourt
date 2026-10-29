# OpenCourt ↔ LocalCases 对接契约

> 状态：**待对接（v0.2 M3）**。本文记录 LocalCases 的真实 HTTP 契约（参数名、字段名、
> 默认值均取自源码）与 OpenCourt 侧使用规则。边界总则见
> [architecture.md](architecture.md) §跨项目边界 与 [plan-v0.2.md](plan-v0.2.md) §D0。

## 1. 边界（先于一切）

**裁判文书语料的索引、检索、过滤与计数归 LocalCases；OpenCourt 只拥有案件。**

- LocalCases 回答：「哪些文书匹配、分布如何、原文是什么」。
- OpenCourt 回答：「用什么词去检索、哪些结果作为引用留痕、要不要脱敏、能否呈现、
  分布统计怎么标注」。

OpenCourt **不抓取裁判文书、不建类案索引、不做相关性重排、不统计裁判倾向**。
`case_refs.summary` 是引用留痕快照（含 `retrieved_at`），**不是语料副本，不得用于再检索
召回**；需要更多内容回源调 `/api/case/{id}`。

## 2. 对接方式

| 项 | 约定 |
|---|---|
| 形态 | **外部可选组件**：用户自装自启的 LocalCases `serve`，回环 HTTP |
| 客户端 | OpenCourt 侧 stdlib `net/http`（零新增依赖），**全部出站连接**，OpenCourt 仍无监听端口 |
| 鉴权 | 请求头 `x-localcases-token`；token 只经环境变量注入，严禁写入配置文件 |
| **token 获取** | 优先读 LocalCases 的发现文件（LC-6：`~/.localcases/localcases.json`，含 `addr`/`token`/`pid`/`version`）；`token_env` 为显式覆盖。桌面版每次运行随机生成 token 且不落盘（`desktop/src/main.rs:56`），**没有发现文件就连不上**（隐患 H3） |
| 进程生命周期 | **OpenCourt 绝不启动 LocalCases**：源码无单实例/锁保护，自行拉起会与用户已开实例争 db/index。连不上只给指引 |
| 默认 | **禁用**：`localcases.enabled=false` 或 `base_url` 为空 → 类案检索整体不可用 |
| 失败 | 返回 `LocalCasesGuidance` 指引，**绝不降级到其他抓取方式**；不自动拉起对方进程 |
| 探测 | 启用后调 `GET /api/health` → `{ok, indexed_documents}`，用于「语料规模」显示与可用性判定 |

OpenCourt 配置（`config/settings.yaml`）：

```yaml
localcases:
  base_url: ""        # 例 http://127.0.0.1:5010（LocalCases config.toml 的 addr）
  token_env: ""       # 例 LOCALCASES_TOKEN；token 严禁写入本文件
  enabled: false      # 默认离线安全
```

环境变量覆盖：`OPENCOURT_LOCALCASES_BASE_URL` / `OPENCOURT_LOCALCASES_TOKEN_ENV` /
`OPENCOURT_LOCALCASES_ENABLED`。

## 3. 端点

| 端点 | 用途 | OpenCourt 使用 |
|---|---|---|
| `GET /api/health` | 就绪探测，返回 `indexed_documents` | 启动/使用时探测 |
| `GET /api/search` | 检索 + 过滤 + 分页 + facet | 类案检索主入口 |
| `GET /api/facets` | 仅聚合（同查询同过滤） | 分布统计面板 |
| `GET /api/case/{id}` | 整条记录（`record` + `derived`） | 「点开核对原文」 |
| `GET /api/export.csv` | 导出（上限 `export_limit`） | v0.2 不用 |

## 4. `/api/search` 参数（真实参数名）

**语义**：同一参数用逗号分隔多值 = **OR**；不同参数之间 = **AND**；关键词多词也是 AND。
未知参数被忽略（旧客户端不受影响）；**坏值返回 400，不会静默放宽查询**。

| 参数 | 取值 | 说明 |
|---|---|---|
| `search` | 关键词，空格分词 AND | 空关键词 + 仅过滤/聚合也是合法查询 |
| `court` `region` `cause` `case_type` `case_type_code` `procedure` `procedure_group` `procedure_kind` `source` **`law`** `court_code` | 精确值，逗号多值 | ⚠️ 法律依据过滤参数是 **`law`**，不是 `legal_basis`（后者是响应字段名） |
| `year_from` `year_to` | 整数年 | 数值层 |
| `date_from` `date_to` | `YYYY-MM-DD` 或裸年份 | 裁判日期；裸年份自动展开为该年首/末日 |
| `pub_from` `pub_to` | 同上 | 公开日期 |
| `has_text` `has_html` | bool | — |
| `min_text_len` `max_text_len` | 整数 | 正文长度 |
| `offset` | 整数，被 `max_results`（默认 50000）截断 | 页偏移 |
| `id_gt` | 整数游标 | **深翻页推荐路径**（比 offset 便宜） |
| `sort` | `relevance`（默认）/ `date` / `id` | — |
| `group_by` | 逗号分隔：`court` `region` `cause` `case_type` `case_type_code` `procedure` `procedure_group` `procedure_kind` `source` `law` `court_code` `year` `case_year` | 未知值 → 400 |
| `size` | facet 桶数，默认 **20**，上限 **1000** | — |
| `export` | bool | 导出模式（上限 `export_limit`，默认 10000） |

⚠️ **没有 `limit` 参数**：`/api/search` 每页固定 **20 条**（`PAGE_SIZE`），分页只能靠
`offset` 或 `id_gt` 游标。OpenCourt 的 `CaseQuery` 不得设计 `Limit` 字段，改为
`Offset` + `IDAfter`。

## 5. 响应结构

```jsonc
// GET /api/search
{
  "search_meta": {
    "offset": 0, "search": "借款 合同", "search_type": "keyword",
    "limit": 20, "total": 137, "export": false,
    "structured": true              // ★ 见下
  },
  "cases": [{
    "id": 42, "url": "...", "case_id": "（2020）川01民终1234号",
    "case_name": "...", "court": "...", "region": "...", "case_type": "...",
    "procedure": "...", "judgment_date": "2020-05-11", "public_date": "...",
    "parties": "...", "cause": "民间借贷纠纷", "legal_basis": "...",
    "preview": "…前 240 字…"        // 全文不返回，取 /api/case/{id}
  }],
  "facets": {                       // 仅当 group_by 至少一个
    "year": { "buckets": [ { "key": 2020, "doc_count": 41 } ] }
  }
}
```

### ★ 必须检查 `search_meta.structured`

索引**缺精确匹配层**时，LocalCases 会**静默忽略所有过滤条件**，只回关键词结果
（诚实降级，不假装过滤过），并把 `structured` 置为 `false`。

**OpenCourt 规则**：`structured == false` 时，UI 必须显示「当前索引不支持精确过滤，
以下结果未按法院/案由等条件筛选」，并且**不展示 facet 分布**（无精确层时聚合不可信）。
绝不把降级结果当过滤结果呈现。

### facet 的可信度

facet 与命中**同源**（同一 query 同一过滤），桶计数描述的是筛选后的集合——这是 D6
允许的分布统计的唯一来源。但 terms 聚合走 `segment_size` 近似策略，LocalCases
**未返回误差上界字段**（见 LC-4）。OpenCourt 呈现时：只展示桶与计数，不换算百分比
排名结论，并附固定标注（见 §7）。

## 6. OpenCourt 侧使用规则（定案）

1. **要旨/预览进 UI 或模型前必须过 `core/redact`**——LocalCases 不做脱敏，也不该做。
2. 检索结果落 `case_refs`（引用留痕），**不落 `facts`**；`case_refs` 不参与五态与
   状态机升级，`statemachine` 显式拒绝。
3. 引用形态：`case:<source_case_id>@<retrieved_at>`；UI 可跳 `/api/case/{id}` 核对原文。
4. 检索策略归 OpenCourt：用案由、争点词、法条号（`law` 参数）组查询；组词过程可用
   模型产候选，但候选只在组查询时用，不发给对方做「语义扩展」。
5. 「法条 → 类案」导航：`law=<法条名>` 过滤能力在 LocalCases，OpenCourt 只组参数。

## 7. 分布统计的合规呈现（D6）

facet 是**唯一合法的概率类输出**，且必须：

- 附固定标注：「以上为本地语料检索结果的分布，**不是裁判概率或胜诉可能性**」；
- 不换算成「胜诉率 / 几成」等表述——`core/gate` 词表已拦截此类表述并配正反用例；
- 语料规模（`indexed_documents`）必须同时显示，避免小样本被误读为趋势。

## 8. 提给 LocalCases 的契约要求

| # | 要求 | 用途 | 未就绪时 OpenCourt 的行为 |
|---|---|---|---|
| **LC-1** | 参数与响应字段（`Hit` / `Meta` / `facets`）作为稳定契约，字段增删版本化 | 适配层不碎 | 字段缺失如实报错，不做猜测默认值 |
| **LC-2** | `x-localcases-token` 语义与 `/api/health` 返回 `indexed_documents` 稳定 | 启用探测与语料规模可见 | 探测失败 → `LocalCasesGuidance`，类案面板不显示空列表 |
| **LC-3** | `law` / `cause` 精确过滤保持可用（**已具备**） | 「法条 → 类案」导航 | 该导航项置灰并说明原因 |
| **LC-4** | facet 返回近似误差上界（如 `doc_count_error_upper_bound`）或明确声明为精确 | 分布统计可信度标注 | 只展示桶与计数，不展示百分比排名 |
| **LC-5** | **不需要** LocalCases 做脱敏、相关性重排、案件上下文理解 | 边界不扩散 | OpenCourt 自己过 `core/redact` |
| **LC-6** | serve 启动时写发现文件 `<state dir>/localcases.json`：`{addr, token, pid, version, schema, indexed_documents}`，权限 0600，退出时删除 | **修复 H3**：桌面版 `random_token()` 每次随机且不落盘（`desktop/src/main.rs:56`），一并安装场景 OpenCourt 永远拿不到 token | 降级为「手动填 `base_url` + `token_env`」，UI 说明这是临时通道 |
| **LC-7** | `/api/health` 返回 `version` 与索引 `schema` | 契约不匹配时报错而非误解析 | 缺字段时按「版本未知」处理，功能可用但标注 |

## 9. 错误处置

| 情况 | OpenCourt 行为 |
|---|---|
| 连接失败 / 未启动 | `LocalCasesGuidance`：启动 LocalCases `serve` → 配置 `base_url` → 置 `enabled=true`；不重试到其他来源 |
| 401 | 「token 缺失或不匹配」，提示检查 `token_env` 指向的环境变量 |
| 400 | 原样展示 LocalCases 的错误信息（坏参数不猜、不自动改写重试） |
| `structured=false` | 显示降级提示，隐藏 facet（见 §5） |
| 超时 | 显式超时错误 + 建议收窄过滤（`year_from` / `cause`），不静默返回部分结果 |

## 10. 联调样例

```bash
# 就绪探测
curl -s localhost:5010/api/health

# 民间借贷类案 + 年份/法院分布
curl -s localhost:5010/api/search \
  --data-urlencode 'search=借款 合同' \
  --data-urlencode 'cause=民间借贷纠纷' \
  --data-urlencode 'year_from=2019' \
  --data-urlencode 'group_by=year,court' \
  --data-urlencode 'size=10'

# 按法条导航（参数名是 law，不是 legal_basis）
curl -s localhost:5010/api/search --data-urlencode 'law=中华人民共和国民法典'

# 深翻页游标（优于大 offset）
curl -s localhost:5010/api/search --data-urlencode 'search=借款' --data-urlencode 'id_gt=100000'
```

## 11. 对 plan-v0.2 的修正

`plan-v0.2.md` §4.2 的 `CaseQuery` 字段据此更正：

```go
type CaseQuery struct {
    Search      string
    Court       string   // 逗号多值 = OR
    Cause       string
    Law         string   // ★ 法条过滤（不是 LegalBasis）
    Procedure   string
    YearFrom    int
    YearTo      int
    DateFrom    string   // YYYY-MM-DD
    DateTo      string
    GroupBy     []string // 分布统计
    FacetSize   int      // 默认 20，≤1000
    Offset      int      // ★ 无 Limit：每页固定 20 条
    IDAfter     int      // 深翻页游标
    Sort        string   // relevance / date / id
}
```

