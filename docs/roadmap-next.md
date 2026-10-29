# 下一阶段开发规划（v0.2 → v0.3）

> **v0.2 范围已定案**，直接按 [plan-v0.2.md](plan-v0.2.md) 开工（M1 案件实体 +
> M2 法条链路 + M3 类案接入 + M4 要件接口）。本文保留为盘点与 v0.3 候选池。

本文基于 2026-10-28 的代码盘点，规划「智能庭审模拟 + 案件推演」方向的落地路径。
外部参考方案（多智能体 + 法律知识图谱 + 向量检索 + 规则引擎 + 本地大模型）在此
**按当前架构做映射与取舍**，不是照搬：本项目是 Go/Wails 桌面单进程、纯 Go 无 CGO、
无监听端口、默认离线、代码仲裁优先于模型判断，任何引入都必须服从这套纪律。

## 0. 结论先行

- 反幻觉内核（五态 / 状态机 / 门禁 / 脱敏 / 哈希链审计）**已经可用且经过测试**，
  它是本项目相对参考方案最大的差异化资产，下一阶段不重写、只加固。
- 当前最大的问题不是「缺智能体」，而是**已有能力没有连成链路**：法条源已接入进程内
  SDK 但没有任何检索入口，`core/retrieve` 是死代码，`CaseSourcePlugin` 零实现，
  要件匹配没有要件数据，`adversary` 只有监控没有推演。**先接线，再加智能。**
- 「庭审模拟」在本项目的合规形态是：**多角色结构化推演 + 攻击点/争点/补证方向**，
  不是胜负预测。参考方案中的「量刑区间 / 裁判倾向 / 置信度 / 概率参考」与
  `core/gate` 的 `NO_PROBABILITY` / `NO_PREDICTION` / `NO_VERDICT` 硬拦截直接冲突，
  必须降级为「语料内检索结果的分布统计」并强制标注，否则等于自毁内核。
- 里程碑顺序：M1 案件实体 → M2 法条检索与引用校验 → M3 类案检索（LocalCases）
  → M4 构成要件数据链路 → M5 混合检索 → M6 对抗推演与庭审流程引擎
  → M7 审校 Agent 与推演报告 → M8 评测验收。

## 1. 现状盘点

### 1.1 能力矩阵

| 能力面 | 状态 | 落点 | 说明 |
|---|---|---|---|
| 事实五态模型 | ✅ 可用 | `core/domain` | 五态 + Fact/Conflict/Assumption/Conclusion + Citation 溯源 |
| 状态机三硬规则 | ✅ 可用 | `core/statemachine` | 证据驱动升级 / 假设标签贯穿 / 冲突不自动消解，模型无裁决权 |
| 输出门禁 | ✅ 可用 | `core/gate` | 引导性词表、幻觉实体落地、废止法条标红；动作红线 `gate/action.go` |
| 强制脱敏 | ✅ 可用 | `core/redact` | 姓名一致替换 + 还原表、掩码、地址泛化、案号保留 |
| 哈希链审计 | ✅ 可用 | `core/audit` | JSONL 追加，`Verify` 可检出篡改 |
| 引用定位 | ✅ 可用 | `core/verify` | 片段回原文页码定位，核验器基础能力 |
| 文档接入 | 🟡 部分 | `core/ingest` | 文本 + 文本型 PDF 逐页；OCR 接口就位但**无引擎**；无 docx |
| 阅卷 Agent | ✅ 可用 | `neutral/reader` | 模型产候选 → 核验器逐条仲裁 → 定位失败拒绝入库并留痕 |
| 时间线 / 汇聚 / 冲突 | 🟡 部分 | `neutral` | 时间锚定排序已有；冲突判定仅「同要件不同片段」，偏保守 |
| 要件匹配 / 盲区 | 🟡 空转 | `neutral/neutral.go` | 代码就绪，**要件由前端手输**，无要件库支撑 |
| 程序期限守护 | ✅ 可用 | `neutral/procedure_guard` | 规则需外部构造，未接 UI |
| 法条源 | 🟡 半接线 | `plugins/law_openlaw.go` | 进程内 SDK 已通，**但 App 层无任何检索绑定** |
| 类案源 | ❌ 未实现 | `plugins/plugins.go` | `CaseSourcePlugin` 接口存在，零实现，LocalCases 未对接 |
| 检索层 | ❌ 死代码 | `core/retrieve` | 关键词实现存在但**全仓无引用**；无向量、无重排 |
| 模型适配 | ✅ 可用 | `plugins/model_openai.go` | OpenAI 兼容 chat + embeddings，默认禁用，禁训头 |
| 浏览器辅助 | ✅ 可用 | `plugins/browser_browseros.go` | MCP 回环，只读 + 两段式动作，默认禁用 |
| 立案辅助 | ✅ 可用 | `neutral/filingassist.go` | 候选→确认→执行，提交/支付/签名代码级红线 |
| 对抗推演 | ❌ 未实现 | `adversary/` | 仅 `TrajectoryMonitor` 抖动判定；`OpponentSimulator` / `DriftDetector` 是接口 |
| Profile 立场层 | 🟡 部分 | `profiles/` | 四份 yaml + 默认拒绝加载器；`data_dir` 字段**未用于数据隔离** |
| 案件实体 | ❌ 缺失 | `core/storage` | 无 `cases` 表，事实全局挂载，无 `case_id` |
| 庭审流程 | ❌ 缺失 | — | `domain.CaseStage` 枚举存在但无流程引擎 |
| 推演报告 / 审校 | ❌ 缺失 | — | 无导出、无引用核验报告 |
| 前端 | 🟡 演示级 | `frontend/src/App.vue` | 695 行单文件，6 个面板；无推演/庭审界面 |

### 1.2 断点清单（按影响排序）

1. **无案件实体**：所有事实/冲突/文档全局混在一库，多案件无法并行，推演无边界。
   Profile 的 `data_dir` 形同虚设（`main.go` 固定 `opencourt.db`）。
2. **法条检索链路断裂**：`a.lawSource` 只被打开与关闭，没有任何 Wails 绑定调用它；
   `LawHit` 到不了前端。OpenLaw 的 `ResolveVersion` / `CheckCitation` / `FindRelation`
   三个高价值能力**完全未接**。
3. **类案检索为零**：`CaseSourcePlugin` 无实现，LocalCases（Rust + loopback HTTP +
   token）未对接。推演缺先例支撑。
4. **要件库为空**：OpenLaw `element` 表 0 行，且无导入 CLI。要件匹配、盲区生成、
   构成要件推演全部失去数据基础。
5. **`core/retrieve` 死代码**：关键词检索未接入任何链路；`Embed` 有接口但无向量存储、
   无混合融合、无重排。
6. **`Citation.LawVersion` / `LawRepealed` 无填充路径**：门禁能标红废止法条，但没人
   设置这两个字段——标红能力实际处于休眠。
7. **adversary 空壳**：立场漂移检测与对方模拟未实现；`TrajectoryMonitor` 已实现但
   未接入 App 与 UI。
8. **无庭审/推演流程引擎**：`CaseStage` 只是枚举，阶段推进无守卫、无产出校验。
9. **无审校与报告**：无引用核验报告、无推演报告导出、无推理链留痕视图。
10. **模型真机评测未做**：开发文档 11.1 的「100 份真实判决书 / 幻觉率 < 1% /
    溯源准确率 > 98%」验收未执行。所有模型能力的扩展都缺基线。

## 2. 与参考架构的映射与取舍

### 2.1 逐项映射

| 参考方案 | 本项目落点 | 取舍 |
|---|---|---|
| 接入层 Web 工作台 / API | Wails bindings + Vue 面板（现有） | ✅ 保持。不引入 HTTP 服务与端口 |
| 编排层 LangGraph / 状态机 | 新建 `core/flow`（Go 状态机） | ⚠️ 用 Go 自研，不引 LangGraph。庭审阶段是有限状态，Go 更可控可审计 |
| 智能体层（法官/律师/检察官/证人/书记员/审校） | `neutral/` + `adversary/` + `profiles/` | ⚠️ 角色映射见 §3；检察官与独立证人 Agent 本阶段不做（见 §6） |
| 推理层：类案检索 | `plugins/case_localcases.go` → LocalCases | ✅ 直接对接兄弟项目 |
| 推理层：知识图谱（Neo4j/NebulaGraph） | OpenLaw `relation` + `element` + `article` 表 | ❌ 不引图数据库。OpenLaw 关系表已是「法条—修改—废止—解释」结构层，且带 `confirm_status` 人工闸口；图数据库会破坏单进程与单目录可迁移数据 |
| 推理层：规则引擎（Drools） | `core/gate` + `core/statemachine` + `neutral/procedure_guard` | ❌ 不引 Drools。现有确定性代码就是规则引擎，且已带测试与审计 |
| 推理层：事实核验 | `core/verify` + `neutral/VerifyFact` | ✅ 已有，加固 |
| 数据层：向量库（Chroma/Qdrant/Milvus） | SQLite 内嵌向量列 + 暴力余弦 | ⚠️ 不引外部向量服务。纯 Go、无端口、数据目录单目录可迁移 |
| 数据层：图数据库 | OpenLaw SQLite | ❌ 同上 |
| 数据层：案件状态库 / 日志审计库 | `core/storage` + `core/audit` | ✅ 已有，补案件表 |
| 模型层：Ollama / vLLM / TGI | `plugins/model_openai.go`（OpenAI 兼容） | ✅ 已覆盖（Ollama `/v1`、LM Studio、vLLM 均可指向），不假定服务商 |
| 模型层：Embedding / 重排序 | 同适配器 `/embeddings`；重排走本地 rerank 端点（可选） | 🟡 需补向量存储与融合 |
| 模型层：OCR / ASR | `core/ingest/OCRAdapter` 注入本地引擎 | 🟡 OCR 接本地引擎；ASR（庭审语音）不做 |
| 运维层：Docker Compose 微服务 | 桌面单进程为主，`deploy/` 保留机构模式 | ❌ 不拆微服务。拆分会破坏「无端口 + 数据不出域」模型 |
| 隐私：本地存储 / 脱敏索引 / 结论留痕 | 已实现（redact + audit + citation 强制） | ✅ 保持并扩展到新链路 |

### 2.2 必须修正的冲突项（红线）

参考方案里以下四项与本项目内核直接冲突，**按原样实现等于自毁差异化**：

| 参考项 | 冲突点 | 合规替代 |
|---|---|---|
| 量刑/裁判参考、量刑区间 | `gate.NO_PREDICTION` 拦截「量刑预测/估计刑期」；OpenLaw 纪律「不输出胜诉概率」 | 只输出「法条的法定幅度原文」+ 引用坐标，不做区间建议 |
| 概率参考、置信度 | `gate.NO_PROBABILITY` 拦截「胜诉率/胜诉概率」 | 输出**语料内检索分布**（命中数、年份分布、法院分布），强制标注「语料检索结果的分布，不是裁判概率」 |
| 裁判倾向分析 | court profile `disabled_tools: verdict_tendency` | 不做。类案只呈现「该条被引用于哪些案件」的事实清单 |
| 反事实推演 | 极易滑向「假设→结论」链 | 只允许经由 `domain.Assumption` + `TagConclusionWithAssumptions` 显式登记，假设标签必须贯穿到最终输出，推翻时回填受影响结论 |


## 3. 智能体角色映射（本项目形态）

| 参考角色 | 本项目实现 | 阶段 | 边界 |
|---|---|---|---|
| 案件解析 Agent | `neutral/reader`（已有） | — | 模型只产候选，核验器仲裁 |
| 书记员 Agent | `neutral/timeline` + `Aggregate`（已有）+ 结构化摘要（M7） | M7 | 摘要只由已入库事实拼装，无来源句禁止出现 |
| 法律适用 Agent | 新建 `neutral/lawassist`：OpenLaw `Search` + `ResolveVersion` + `FindRelation` + `CheckCitation` | M2 | 候选法律路径必须带 `article_id` 与版本区间；断链 OL-1006 上浮标注「无法溯源」 |
| 类案 Agent | 新建 `neutral/caseassist`：LocalCases `/api/search` + 结构化过滤 | M3 | 类案要旨**永不进强态**（同联网源纪律），引用 = `case:<id>@<检索时间>` |
| 要件/盲区 Agent | `MatchElements` + `GenerateBlindspots`（已有）+ OpenLaw `AnalyzeElements` | M4 | 只有「已溯源确认」事实可覆盖要件 |
| 原告/被告律师 Agent | `adversary.OpponentSimulator`（输入严格按 `Visibility` 过滤） | M6 | 输出必经 `gate.Check`；`disabled_tools` 命中即禁 |
| 立场漂移检测 | `adversary.DriftDetector` | M6 | 每轮重新注入立场，漂移立即报警并停止该角色续写 |
| 法官 Agent | `core/flow` 流程守卫 + `procedure_guard` + 争点归纳（结构化拼装） | M6 | 只归纳争点与检查程序节点，不输出裁断性表述 |
| 审校 Agent | 新建 `neutral/review`：引用核验（OpenLaw `CheckCitation`）+ 回原文一致性 + 五态检查 + 脱敏复查 + 推理链完整性 | M7 | 全部确定性检查优先，模型仅补候选说明 |
| 证人/鉴定人 Agent | **不做独立生成 Agent** | — | 风险最高（生成超出卷宗的陈述）。改为「笔录对齐核验器」：只做已有笔录与事实的五态对齐 |
| 检察官 Agent | **本阶段不做** | — | 当前四类角色（court/plaintiff/defendant/party）无刑事控方；产品边界先聚焦民事 |

## 4. 里程碑

> 每个里程碑必须自带测试（包内单测 + `tests/compliance/` 合规用例）与审计留痕，
> 否则不算完成。`make test` 必须全绿。

### M1 案件实体与数据隔离（地基，1 周）

- 目标：一切推演以案件为边界。
- 落点：`core/domain`（`Case` 类型）、`core/storage`（`cases` 表 + 各表 `case_id` 列 +
  迁移）、`main.go`（当前案件上下文，切换案件）。
- 关键设计：`case_id` 迁移时现有数据归入 `CASE-DEFAULT`（不丢历史）；Profile 的
  `data_dir` 真正生效——立场切换即切换数据根目录，跨立场数据物理隔离。
- 验收：多案件事实互不可见；跨案件引用被门禁拦截；旧库自动迁移且审计留痕。
- 合规：可见性标签在检索层前置过滤（先过滤再召回），不靠 UI 隐藏。

### M2 法条检索链路与引用校验（最高性价比，1 周）

- 目标：把已接入的 OpenLaw 能力真正送到用户与门禁面前。
- 落点：`neutral/lawassist.go`（新）、`main.go`（`SearchLaw` / `GetLawDetail` /
  `CheckLawCitation` 绑定）、前端「法条检索」面板。
- 关键设计：
  - `SearchLaw` → `LawHit` → 前端展示版本区间与 `EFFECTIVE/AMENDED/UNVERIFIED`；
  - `ResolveVersion(lawName, atDate)` 填 `Citation.LawVersion`，让休眠的废止标红真正生效；
  - `CheckCitation(lawName, articleNo, quote, atDate)` 做**文书引用核验**：条文是否存在、
    当时生效版本为何、引文是否逐字一致——这同时是 M7 审校的确定性内核；
  - `FindRelation` 呈现修改/废止/解释关系，`confirm_status=pending` 必须显示「待人工确认」。
- 验收：文书中虚假法条引用被核验器标出；断链条文显示 UNVERIFIED 而非静默通过；
  pending 关系不被当作已确认事实。

### M3 类案检索接入 LocalCases（1 周）

- 目标：推演有先例支撑，且先例可点开核对原文。
- 落点：`plugins/case_localcases.go`（实现 `CaseSourcePlugin`）、`neutral/caseassist.go`、
  `config`（`localcases.base_url/token/enabled`，默认禁用）、前端「类案」面板。
- 关键设计：
  - 与 BrowserOS 同一「外部可选组件」纪律：本机回环 HTTP + `x-localcases-token`，
    出站连接，本项目仍无监听端口；未启用即报错，绝不降级到其他抓取；
  - 检索参数映射：关键词 AND + `court/year/cause/legal_basis/procedure` 精确过滤 +
    `group_by` facet（分布统计的唯一合法来源）；
  - 类案要旨入库上限为 `SINGLE_SOURCE`，**永不进 `TRACED_CONFIRMED`**；
    引用形态 `case:<id>@<retrieved_at>`，UI 可跳原文记录；
  - 呈现「检索语料的分布」而非「裁判概率」，强制标注措辞。
- 验收：未启用时调用报错；类案事实无法被升级到强态；分布统计文案不触发 gate 拦截、
  概率化表述被拦截。

### M4 构成要件数据链路（跨项目，1–2 周）

- 目标：要件匹配与盲区从「手输」变成「有据」。
- 落点：OpenLaw 侧新增 `openlaw-element` CLI（要件导入，pending 人工闸口 + review_log
  审计）；OpenCourt 侧 `MatchElements` 改为按案由从 `AnalyzeElements` 取要件。
- 关键设计：要件一律 `pending → 人工确认`，机器不自动生成要件；`how_to_attack`
  （对方突破方向）只作为「补证方向」呈现，不作为策略建议。
- 验收：至少一个民事案由（如民间借贷）要件闭环；无要件数据时明确降级提示，
  不回退到模型自由生成要件。


### M5 混合检索（向量 + 关键词 + 重排）（1 周）

- 目标：补语义召回，但**范围严格限定为本案卷宗内检索**——法条检索归 OpenLaw、
  类案检索归 LocalCases，`core/retrieve` 不得自称拥有后两者（删 `LawEntry`/
  `CaseEntry`/`Kind=LAW/CASE`，见 plan-v0.2 §D0 违例 V2）。
- 落点：`core/storage`（`embeddings` 表，BLOB + 元数据）、`core/retrieve`
  （向量召回 + 关键词召回 + RRF 融合 + 可选模型重排）、`neutral/reader` 复用。
- 关键设计：纯 Go 暴力余弦（万级文档量足够，避免引外部向量库）；embedding 走现有
  `ModelAdapter.Embed`，模型未启用时自动降级为纯关键词并在结果里显式标注「未启用语义召回」；
  **检索结果只是候选**，进事实库仍需 `verify.Locate` 定位。
- 验收：向量与关键词结果融合可复现；未启用模型时链路可用且标注明确；
  检索命中不能直接成为强态事实。

### M6 对抗推演与庭审流程引擎（2 周，参考方案的核心诉求）

- 落点：`core/flow`（新：案件阶段 + 庭内阶段状态机）、`adversary/simulate.go`
  （`OpponentSimulator` 实现）、`adversary/drift.go`（`DriftDetector` 实现）、
  `neutral/moderator.go`（争点归纳）、前端「推演」面板（轮次视图 + 轨迹抖动告警）。
- 关键设计：
  - 流程硬守卫（确定性代码）：阶段推进需前置产出——无已入库证据事实不得进入质证；
    未质证不得进入辩论；每轮产出写审计并挂 `round_id`；
  - 对方模拟输入严格按 `Visibility` 过滤（本方可见材料不得进入对方模拟）；
  - 每轮发言必须挂 `fact_ids`，未挂或挂到 `UNSUPPORTED` 即被 `gate.CheckGrounding` 拦截；
  - 结论继承假设标签（`TagConclusionWithAssumptions`），假设被推翻时回填受影响结论；
  - `TrajectoryMonitor` 接入：连续翻转且无新证据 → 停止自动续写并触发人工复核；
  - 全程 UI 标注「模拟推演，非预测」，`court` profile 禁用 `defense_rehearsal` /
    `verdict_tendency`。
- 验收：合规测试新增——本方私有材料不得进入对方模拟；倾向性发言被拦截并改写；
  无新证据的结论翻转触发人工复核；立场漂移报警。

### M7 审校 Agent 与推演报告（1–2 周）

- 落点：`neutral/review.go`（新）、`neutral/report.go`（新）、前端「审校与报告」面板。
- 关键设计：审校以确定性检查为主——引用核验（M2）、回原文一致性（`verify`）、
  五态合规（弱态不得进事实段落）、脱敏复查、推理链完整性（每个结论可回溯到事实与要件）；
  模型仅补「风险说明」文本且必过 gate。报告导出 Markdown/PDF，含事实清单（五态 +
  引用坐标）、争点、盲区、模拟轮次轨迹、风险项、待人工确认清单。
- 验收：任一结论缺来源即报告显式标红；导出内容默认脱敏，未脱敏导出需二次确认并留痕。

### M8 评测验收与发布（1 周 + 持续）

- 目标：补上 README 承认的验收缺口，给所有模型能力一条基线。
- 落点：`scripts/modelcheck` 扩展为评测 runner；评测集与指标落 `tests/eval/`。
- 指标：100 份真实判决书上的幻觉率 < 1%、溯源准确率 > 98%、要件覆盖准确率、
  门禁误拦率。结果写入 CHANGELOG「已知限制」，未达标项如实保留。
- 发布：v0.2 打包（dmg + 语料分层包 + sha256），更新「预览版边界」段落。

## 5. 依赖顺序

```
M1 案件实体 ──┬── M2 法条链路 ──┐
              ├── M3 类案接入 ──┼── M5 混合检索 ──┐
              └── M4 要件数据 ──┘                ├── M6 对抗推演 ── M7 审校报告 ── M8 评测
OpenLaw: openlaw-element CLI ──→ M4        LocalCases: 保持 API/token 契约稳定 ──→ M3
```

M1 是所有后续工作的前置（无案件边界则推演无意义）。M2 与 M3 可并行。
M4 依赖 OpenLaw 侧配合，可后置但不宜久拖——要件库为空会让 M6 的争点归纳失去骨架。

## 6. 明确不做

- 不预测判决结果、不输出胜诉概率、不做量刑区间建议、不做裁判倾向分析。
- 不引入 Neo4j / NebulaGraph / Chroma / Qdrant / Milvus / Drools / LangGraph。
- 不拆微服务、不新增监听端口、不引入任何把案情送往第三方 API 的默认路径。
- 不做独立证人/鉴定人生成 Agent、不做检察官 Agent（超出当前角色边界）。
- 不做 ASR 庭审语音转写（另议）。
- 不自动判定法条废止/修改关系（OpenLaw 人工闸口纪律）。
- 不恢复隐名信息、不用模型幻觉填补事实空白。

## 7. 风险与前置

| 风险 | 处置 |
|---|---|
| 庭审模拟天然诱导「结论化」输出 | gate 词表随 M6 同步扩充（模拟场景新词）；合规测试先行编写，实现后必须通过 |
| LocalCases 为 Rust 外部组件，契约可能变动 | 插件层做版本/健康探测（`/api/health`）；契约变更走适配层，不改 core |
| 要件库需人工确认，进度受制于 OpenLaw | M4 拆为「OpenCourt 侧接口 + OpenLaw 侧 CLI」两半，前者可先行合入并显式降级 |
| 向量检索在离线端性能 | 先暴力余弦 + 分案库；规模超阈值再评估 ANN，不提前优化 |
| 模型未评测就扩链路 | M8 的评测 runner 尽量前移到 M6 之前跑一次基线，哪怕样本不足也要有数 |

