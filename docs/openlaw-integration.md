# OpenCourt ↔ OpenLaw 对接契约

> 状态：**已对接（进程内 SDK）**，本文记录真实契约与处置规则，并列出提给 OpenLaw 的
> 契约要求。边界总则见 [architecture.md](architecture.md) §跨项目边界 与
> [plan-v0.2.md](plan-v0.2.md) §D0。

## 1. 边界（先于一切）

**法规语料的索引、检索与语料层校验归 OpenLaw；OpenCourt 只拥有案件。**

- OpenLaw 回答：「这条引用真不真、当时哪个版本生效、引文与原文是否一致、溯源链断没断、
  要件确认了没有」。只有它有 PDF + SHA-256 + manifest 签名 + 版本链，只有它能判。
- OpenCourt 回答：「这条引用在本案里能不能用、挂到哪个事实、处于哪个五态、能否经门禁
  输出、是否脱敏、是否留痕」。

单向读：OpenCourt **永不写回** OpenLaw 数据目录。OpenCourt 的 `law_refs` 存的是引用
留痕（我引用了什么 + 核验结论 + 时间），**不是语料副本，不得用于再检索召回**。

## 2. 对接方式

| 项 | 约定 |
|---|---|
| 形态 | **进程内 SDK**：`import github.com/futuremeng/OpenLaw/sdk` |
| 打开方式 | **必须只读**：`sdk.OpenReadOnly(dataDir)`（`query_only` + 不跑迁移）。`sdk.New` 只属于 OpenLaw 自己的写入路径——用 `New` 打开语料是消费方越权（隐患 H1/H2，见 [integration-model.md](integration-model.md) §4） |
| 版本协商 | 打开前读 `meta.schema_version` 与 manifest `producer_version`，与 SDK `SchemaMin/SchemaMax` 比对；**超范围即报错**，不迁移、不部分读、不猜 |
| 依赖 | `go.mod` 中 `replace github.com/futuremeng/OpenLaw => ../OpenLaw`（开发期本地联调） |
| 禁止 | 直连 SQLite、绕过 sdk 读 `data/db`、让模型自行生成法条、修改语料 |
| 语料位置 | **共享位置优先，命中即不复制**：`openlaw.data_dir` → `OPENLAW_DATA_DIR` → `~/.openlaw/location` → `~/.openlaw/data` → dmg 卷内 → Downloads。详见 [integration-model.md](integration-model.md) §3 |
| 配置 | `openlaw.data_dir`；env `OPENCOURT_OPENLAW_DATA_DIR` |
| 未配置 / 未找到 | 法条检索整体不可用 + 指引（安装 OpenLaw 或下载语料包），其余功能不受影响（降级不猜） |
| 语料分发 | 分层包 `lawdata-core-<日期>.zip`（db+manifest）+ `lawdata-evidence-<日期>.zip`（docx/md/provenance）；各附 `.sha256`。**铺设 ≠ 编辑**：只允许首次落盘，不允许改已有语料 |
| 完整性 | 每日 manifest + Ed25519 签名；未生成 manifest 时 `UNVERIFIED` 是闸口的正常表现，不是 bug |

## 3. 通用返回结构（sdk.Result）

```go
type Result struct {
    Status    Status         // ok / expired / broken_chain / not_found / need_manual
    Data      map[string]any
    Trace     *Trace         // doc_id / pdf_path / chain_ok / break_reason
    Warnings  []string       // 只提示不报错（12.3）
    ErrorCode string         // OL-1001..OL-1009
}
```

**判定必须走 `Status` / `ErrorCode`，禁止靠字符串模糊匹配。**

### 错误码与 OpenCourt 处置（定案）

| 码 | 含义 | OpenCourt 处置 |
|---|---|---|
| — (`ok`) | 命中且一致 | 记 `law_refs.verify_result = ok`，可参与要件比对 |
| OL-1001 | 未找到法规 | 展示「核对名称与简称」；并列候选法名交人工，**不自动采信** |
| OL-1002 | 指定日期无生效版本 | 展示「确认时间范围」；不得用当前版本顶替 |
| OL-1003 | 条款号不存在 | 门禁级标红「该条款不存在」；文书引用核验的典型失败用例 |
| OL-1004 | 引文与原文差异大 | 并列展示 OpenLaw 原文与文书引文，交人工比对，不判谁对 |
| OL-1005 | 版本已失效 | 提示失效 + 给出当前有效版本；`Citation.LawRepealed = true` 触发门禁标红 |
| OL-1006 | 溯源断链 | **数据照常入库与展示**，UI 必须标「无法溯源」；禁止输出判错结论 |
| OL-1007 | 哈希校验失败 | 立即告警并隔离文件；OpenCourt 显示语料异常提示 |
| OL-1008 | manifest 签名失败 | 停止使用该数据目录；OpenCourt 法条面板红标并阻断检索 |
| OL-1009 | OCR 置信度过低 | 转人工确认（OpenCourt 侧对应 `core/ingest` OCR 纪律） |
| `confirm_status != confirmed` | 待人工确认 | 橙标「待人工确认」，仅提示不报错 |

## 4. 已用能力与返回字段（真实字段名）

### 4.1 `Search(query, topK) → Data{query, hits[]}`

`topK` 默认 10、上限 50（超出被 sdk 收敛）。中文多词空格分词走 LIKE 多词 AND，
英文多词走 FTS 短语。零命中是 `ok` + warning，**不是错误**。

每条 hit：`article_id / article_no / doc_id / law_name / version_id / version_no /
effective_date / expire_date / confirm_status / lang / snippet`

### 4.2 `GetArticle(articleID) → Data{}`

`article_id / article_no / content（全文）/ doc_id / law_name / version_id / version_no /
effective_date / expire_date / confirm_status / lang` + `Trace`。断链时返回
`broken_chain` 但 `Data` 完整，供人工核对。

### 4.3 `ResolveVersion(lawName, atDate) → Data{}`

`version_id / version_no / effective_date / expire_date / confirm_status`。
用途：填 `Citation.LawVersion`，让门禁的废止标红真正生效（v0.2 M2）。

### 4.4 `CheckCitation(lawName, articleNo, quote, atDate) → Data{}`

`hit / similarity / expired / article_id / article_no / content / version_no /
current_version / confirm_status` + `Trace`。
用途：文书引用核验（v0.2 M2 的核心，审校 M7 的确定性内核）。

### 4.5 `FindRelation(lawID) → Data{law_id, relations[]}`

每条 relation：`src_law_id / dst_law_id / relation_type / based_on_doc_id / confirm_status`。
`pending` 计数进 `Warnings`。**机器不自动判定修改/废止关系**——OpenCourt 原样上浮，
绝不据此推断废止。

### 4.6 `AnalyzeElements(caseType, facts[]) → Data{case_type, covered[], missing_elements[]}`

`covered`: `element_id / name / fact`；`missing_elements`: `element_id / name / how_to_attack`。
`how_to_attack`（对方突破方向）在 OpenCourt 只作为「补证方向」呈现，**不作为策略建议**。

### 4.7 `DetectBreak(articleID) → Data{article_id, chain_ok, breaks[]}`

`breaks[]`: `step / reason / error_code`。

### 4.8 `Desensitize(text, extraNames...) → Data{text, restore_map}`

**OpenCourt 不调用它做强制脱敏**——强制关口是 `core/redact`（见 §6）。

## 5. 提给 OpenLaw 的契约要求

OpenCourt 侧不实现对方职责；对方未就绪时 OpenCourt 一律**保守降级 + 显式标注**，
绝不自建替代能力。可直接开工的任务清单见 OpenLaw 仓库
`docs/tasks-opencourt-bridge.md`。

| # | 要求 | 用途 | 未就绪时 OpenCourt 的行为 |
|---|---|---|---|
| **OL-1** | `Search` / `GetArticle` / `ResolveVersion` 返回显式 `effect_status`（`EFFECTIVE / AMENDED / REPEALED_PENDING / UNKNOWN`），由 OpenLaw 依据 `law_version` 链 + `relation`（仅 `confirm_status=confirmed`）判定 | 废除违例 V1：OpenCourt 不再自行映射效力 | 显示 `UNKNOWN` + 「效力状态待确认」，不猜 |
| **OL-2** | `openlaw-element` CLI：按案由导入构成要件，`pending → 人工确认`，`review_log` 留痕 | v0.2 M4 的数据基础（**当前 `element` 表 0 行**） | 要件匹配降级为「前端手输要件」+ 显式提示无要件库 |
| **OL-3** | 要件名可作为阅卷抽取的**封闭候选集**返回（`ListElements(caseType)`） | 废除违例 V3：要件名不得由模型自由生成 | `related_element` 一律置空，不参与要件比对 |
| **OL-4** | `CheckCitation` / `ResolveVersion` 支持法名规范化与别名解析（全称 ↔ 简称，如《民法典》↔《中华人民共和国民法典》）；解析不确定时返回**候选列表**而非单值 | 文书引用核验可用性（简称引用极常见） | OpenCourt 并列展示候选法名交人工，不自动采信 |
| **OL-5** | 期限规则查询：举证/上诉/送达等程序期限，带法条引用与 `confirm_status` | 废除违例 V4：程序规则数据归语料层（v0.3） | `DeadlineRule` 仍由人工构造，UI 标注「规则未溯源」 |
| **OL-6** | `sdk` 接口稳定：破坏性变更走主版本 + 迁移脚本（OpenLaw 10.4 版本语义已如此约定） | 进程内 import 是编译期耦合 | — |

## 6. 违例与修法（v0.2 内处理）

| # | 违例 | 证据 | 修法 |
|---|---|---|---|
| **V1** | OpenCourt 自行推导法条效力状态 | `plugins/law_openlaw.go: openlawStatus()` 以 `expire_date` 是否为空判 `AMENDED/EFFECTIVE` | 删除该映射，改透传 OpenLaw 的 `effect_status`；OL-1 未就绪前保守返回 `UNKNOWN` |
| **V3** | 要件名由模型自由生成 | `neutral/reader.go` prompt：`"related_element":"可选要件名"` | 要件候选集由 OL-3 注入，模型只能选其一或留空；无要件库时字段强制置空 |
| **V4** | 程序期限规则无出处、硬编码在 OpenCourt | `neutral/procedure_guard.go: DeadlineRule{Name, Days}` 由调用方手填 | 规则数据归 OpenLaw（OL-5）；OpenCourt 只保留引擎 `CheckDeadlines` 与本案事件日期（v0.3） |

**脱敏边界**：OpenLaw 的 `Desensitize` 是其自身语料的可选预处理，**不能替代** OpenCourt
的 `core/redact`。OpenCourt 内容进模型或进 UI 前必须过 `core/redact`，与是否已调过
OpenLaw 脱敏无关。

**禁止事项**：自建法条索引或检索（`core/retrieve` 只服务本案卷宗内检索）、缓存条文
正文供再召回、自行判定废止/修改关系、用模型生成或补全要件、绕过 `CheckCitation`
采信模型给出的法条。

## 7. 联调与验收

```bash
# 依赖与编译（replace 指向本地 OpenLaw）
cd OpenCourt && go build ./... && go test ./plugins/ -run OpenLaw

# 语料就位验证
#   配置 openlaw.data_dir 指向 OpenLaw/data
#   预期：LawDataStatus 显示文档/条文计数与 manifest 状态；无 manifest → UNVERIFIED
```

**真机验收用例（v0.2 M2）**

1. 检索「民法典 合同」→ 命中携带 `law_name / effective_date / expire_date /
   confirm_status`；`pending` 命中显示橙标「待人工确认」。
2. 文书引用一个不存在的条款 → `CheckCitation` 返回 OL-1003 → `law_refs` 记录该码 →
   UI 红标「该条款不存在」。
3. 文书引文与原文不符 → OL-1004 → 并列展示双方原文，不判谁对。
4. 断链条文 → `broken_chain` + `Trace.chain_ok=false` → UI 标「无法溯源」，
   数据照常展示，**不出现「引用错误」结论**。
5. 未配置 `data_dir` → 法条面板显示「未配置法条源」，其余功能正常，无崩溃。

