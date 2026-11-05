# v0.4 实施方案（提案，待定案）

> 本文承接 [roadmap-next.md](roadmap-next.md)：M1–M6 已合入主干（见 CHANGELOG），
> 本文对**剩余里程碑与新增工作包做执行裁定**。产品评审结论：反幻觉内核已可用，
> 当前卡产品化的不是「缺智能体」，而是三块拼图——**真实卷宗进得来、效果有数据
> 证明、结果带得走**。本方案按此重排序，并首次引入非代码工作流（试点与分发）。

## 1. 范围

**做（v0.4）**

| 编号 | 内容 | 工期 | 对应评审缺口 |
|---|---|---|---|
| E0 | 评测 runner 前移 + 基线报告（roadmap M8 前移，兑现 §7 承诺） | 3–4 天 | 效果实证 |
| O1 | OCR 引擎接入（`OCRAdapter` 注入本地引擎，接口已就位） | 4–5 天 | 卷宗进得来 |
| O2 | docx 解析（纯 Go，zip + document.xml） | 2–3 天 | 卷宗进得来 |
| M7 | 审校 Agent + 推演报告导出（Markdown 优先，PDF 走打印） | 1–2 周 | 结果带得走 |
| P1 | 法院场景包：引导动线 + 卷宗接入向导 + App.vue 拆分 | 1 周 | 滩头阵地 |

**不做（明确推到 v0.5 或永不）**

- M4 要件库数据闭环——OpenCourt 侧接口与显式降级已在 v0.2 落地，数据侧继续与
  OpenLaw 并行推进，但**不阻塞本版本**（roadmap §7 既定拆分）。
- ANN 向量索引、Web 版、多用户协作、云同步、ASR（roadmap §6 全部继续有效）。
- 签名分发属发布工程而非功能，随 E0 报告一起排进 release 流程，不单列里程碑。

**为什么是这个顺序**：E0 零新代码依赖、即刻可跑，且是所有后续模型能力的标尺——
roadmap §7 已自定纪律「评测 runner 尽量前移」，本方案正式执行。O1/O2 决定真实
用户能否把卷宗喂进系统（当前仅纯文本/文本型 PDF，在真实工作流中占比极低）。
M7 补价值闭环：没有可带走的产出物，前述一切能力都停在「过程演示」。P1 最后做，
因为向导要展示的内容（OCR 入库、报告导出）必须先存在。

## 2. 设计决策（定案）

### D0 评测先行纪律

任何依赖模型的能力，上线前必须有一条可复现的评测数字，哪怕样本不足也要有数。
`scripts/modelcheck` 扩展为评测 runner：固定模型配置与输入集 → 输出指标 JSON +
人读报告。结果**如实**写入 CHANGELOG「已知限制」，未达标项保留不降标准。

### D1 OCR 引擎纪律：本地进程、永不联网、弱态入库

- 引擎经 `core/ingest/ocr.go` 既有 `OCRAdapter` 注入，两个内置适配：
  `ocr_tesseract`（本机 tesseract 二进制，subprocess 调用）与
  `ocr_vision`（macOS 系统视觉，经本地 helper，中文精度优先默认）。
  均为本地进程，**不引入任何网络路径**；外部引擎缺失时维持现状——
  `ErrOCRNotConfigured` 明确拒绝，绝不静默、绝不降级到模型看图。
- **引擎选型调研（2026-11）**：
  - `ocr_vision`：macOS 系统内置 Vision（11+），随 .app 打包约百 KB 的 Swift CLI
    helper，subprocess 输出逐行文本+行级置信度——中文精度最高、体积近零；
  - `ocr_tesseract`：跨平台兜底，subprocess `--tsv` 自带词级置信度；模型用
    tessdata_fast 的 chi_sim（数 MB，Apache-2.0），印刷体扫描件够用；
  - 扫描 PDF 取图：pdfcpu（纯 Go，Apache-2.0）直接抽取每页内嵌 JPEG/PNG，
    绕开 PDFium（CGO）渲染；
  - **不采用**：gosseract / onnxruntime_go（CGO）、生成式多模态看图识字
    （违反「模型永不改字」）；RapidOCR（PP-OCR 系，Apache-2.0，中文最强）
    以 sidecar 形态列入 v0.5「按需下载高精度 OCR 包」候选，复用语料包
    sha256 分发模式，不进 v0.4 主体。
- 探测顺序：显式指定 → Vision helper 存在 → PATH 有 tesseract → 皆无 =
  维持 `ErrOCRNotConfigured` 明确拒绝。
- **OCR 文本入库上限为 `SINGLE_SOURCE`，永不进强态**（同联网源纪律）；仍必须经
  `verify.Locate` 回原文定位，定位失败拒绝入库并记审计。
- 引擎置信度低于阈值的页标「OCR 低置信」黄标上浮 UI，**不得由模型自动改字**；
  纠正只允许人工编辑后重新核验。
- 配置 `ingest.ocr.engine / path / min_confidence`，默认 `none`（= 拒绝）。

### D2 docx 解析：纯 Go，修订与批注不自动采信

zip 解包 + `word/document.xml` 文本抽取，不引 CGO。`w:ins`/`w:del` 修订痕迹与
`w:comment` 批注**保留为独立标注区块**上浮（证据线索），不并入正文；内嵌图片走
OCR 管道（未配置即拒绝该图，其余照常）。表格线性化为文本并标注「表格已线性化」。

### D3 报告导出：确定性拼装 + 默认脱敏 + 免责固定

- `neutral/review.go`：审校全部确定性检查优先——引用核验（M2 `LawVerifier`）、
  回原文一致性（`verify`）、五态合规（弱态不得进事实段落）、脱敏复查、推理链
  完整性（每个结论可回溯到 fact_ids 与要件）；模型仅补「风险说明」文本且必过 `gate`。
- `neutral/report.go`：Markdown 导出——事实清单（五态 + 引用坐标）、争点（冲突
  原样并列）、盲区（仅「无法判断 + 补证方向」）、模拟轮次轨迹（含被拦截轮）、
  待人工确认清单；固定头部免责标注「AI 辅助整理，不构成裁判或法律意见」。
- **导出内容默认脱敏**；未脱敏导出需二次确认并写审计（`REPORT_EXPORT_RAW`）。
- PDF 不引重依赖：Markdown + 系统打印路径；报告即留痕，导出动作本身写审计。

### D4 场景聚焦：用 Profile 表达，不加代码分支

法院场景包 = court Profile 的引导动线与文案，不产生任何 `if role == court` 分支。
首屏向导四步：建案 → 卷宗接入（拖入文件，实时显示 OCR/解析状态与弱态标注）→
阅卷与事实板 → 产出（审校报告）。其余三个 Profile 保持现有自由动线。

### D5 前端：App.vue 拆分随 P1 进行

784 行 App.vue 拆为 `views/`（Onboard / Workspace / Settings）+ 既有 components；
补空状态与下一步引导文案；不引 UI 框架，沿用现有样式纪律。

## 3. 里程碑细节

### E0 评测 runner 与基线报告（3–4 天）

- 落点：`scripts/modelcheck`（扩展）、`tests/eval/`（评测集清单 + 指标定义 + 报告模板）。
- 指标：100 份真实判决书（公有领域来源，清单入库）上的**幻觉率 < 1%、溯源准确率
  > 98%**、要件覆盖准确率、门禁误拦率；另跑门禁对抗集（引导性/预测性表述样本）。
- **评测集获取（定案：LocalCases 正常对接，不要求任何评测专用端点）**：
  1. 圈定：`GET /api/search`（案由/年份过滤 + `sort=id` + `id_gt` 游标）取前 100 条——
     确定性查询，任何人可复跑同一集合（依赖 LC-9）；
  2. 取文：逐条 `GET /api/case/{id}` 取正文全文，OpenCourt 侧计算 sha256 固化进
     `tests/eval/corpus_manifest.jsonl`（id + sha256 + source + 圈定查询参数原文）；
  3. 快照：报告头记 `corpus_digest`（LC-8，缺失时降级记 `indexed_documents` + 日期并
     如实标注复现性受限）；
  4. 纪律：全程走既有 `plugins/case_localcases.go` 通道与 token 规则，评测数据同样
     遵守「要旨永不进强态」；评测取文是**只读回源**，不建本地语料副本、不再召回。
- LC 配合项：LC-8 / LC-9 / LC-10（见 localcases-integration.md §8）；三项未就绪时
  评测仍可运行（manifest 固化 + 逐条回源是完备降级路径），仅复现性标注降级。
- 验收：一条命令产出可复现报告；结果与未达标项如实进 CHANGELOG「已知限制」；
  README「版本边界」的「未做验收」条目改写为「评测报告见 …」。

### O1 OCR 接入（4–5 天）——已落地（2026-11-04）

- 落点：`core/ingest/ocr_tesseract.go`、`core/ingest/ocr_vision.go`（新）、
  `config`（`ingest.ocr.*`）、`main.go` 装配注入、前端卷宗面板 OCR 状态徽章。
- 合规用例：扫描件不得绕过 OCR 入库；OCR 文本无法升级到强态；引擎未配置报错文案；
  低置信页标黄且不触发模型改字。
- 落地实况：`ocr_vision.go`（macOS Vision helper subprocess，`scripts/ocr_helper`
  Swift 源码 + `scripts/build-ocr-helper.sh`，本机中文冒烟通过）、
  `ocr_tesseract.go`（`--tsv` 词级置信均值）、`ocr_detect.go`（探测顺序
  显式→Vision→tesseract→nil，指定引擎不可用不降级）、
  `pdf_images.go`（pdfcpu 纯 Go 抽扫描页内嵌图，零 CGO）、
  `verify.DocumentText` 带 OCR 标记与逐页置信度、`statemachine` 拒绝
  OCR 来源进入已溯源确认（D1 硬封顶）、documents 表 `ocr_engine` 列
  （幂等迁移）、前端 OCR 徽章与低置信提示；合规用例
  `TestOCRCompliance` 锁死三纪律。
- 待办（不阻塞）：helper 随 .app 打包进 `Contents/Resources/`（分发脚本）；
  RapidOCR 高精度 sidecar 按 D1 留 v0.5。

### O2 docx 解析（2–3 天）——已落地（2026-11-04）

- 落点：`core/ingest/docx.go`（新）、装配注册。
- 验收：修订/批注独立区块上浮且不进正文；内嵌图片走 OCR 管道；损坏 docx 明确报错。
- 落地实况：纯 Go `archive/zip` + `encoding/xml`（零新依赖、零 CGO）；
  `w:ins`/`w:del`/`w:comment` 进 `verify.DocumentText.Annotations` 独立标注
  区块——**不进 Pages**，回原文定位对标注文本一律失败（不自动采信，
  `TestDocxBodyTableAndRevisions` 用 `verify.Locate` 正反锁定）；表格线性化
  并标注「表格已线性化」（单元格 ` | ` 分隔、行换行）；内嵌图片走 OCR
  管道——未配置引擎只拒绝该图并标注原因，正文照常（D2），识别成功进
  第 2 页起顺排并带置信度；损坏 zip / 缺 `document.xml` / 空正文均明确
  报错，`.doc` 与 OLE 伪装 `.docx` 给出「另存为 .docx」指引；zip 炸弹
  防护（单条目 64MB 上限）；`documents.annotations_json` 列（幂等迁移）+
  `SaveDocumentFull` 往返不丢标注；前端卷宗行「标注」按钮展开独立区块。
- 待办（不阻塞）：`w:footnote`/文本框/域代码等次要部件的抽取精度
  （现行为「解析存疑段落不静默丢弃」——正文抽不到即整体报错）。

### M7 审校与报告（1–2 周）——已落地（2026-11-04）

- 落点：`neutral/review.go`、`neutral/report.go`（新）、Wails 绑定
  `ReviewCase` / `ExportReport`、前端「审校与报告」面板。
- 验收：任一结论缺来源即报告显式标红；导出默认脱敏、原样导出留痕；报告全文过
  `gate.Check`（倾向性表述在报告层二次拦截）；合规用例——弱态事实出现在报告
  事实段落即测试失败。
- 落地实况：审校五项确定性检查（`CheckLawCitation` 引用核验透传判定、
  `CheckSourceLocate` 回原文一致性 + 强态必须有坐标、`CheckStateCompliance`
  无依据事实入库即红、`CheckReasoningChain` 轮次结论必须可回溯到存在事实、
  未解决冲突 warn 上浮），模型零参与；报告 Markdown 确定性拼装——固定免责
  头部、强态才进事实段落（弱态一律进「待人工确认清单」，`TestReportCompliance`
  锁定「弱态出现在事实段落即失败」）、缺来源显式 `**[缺来源]**`、冲突原样并列、
  盲区只给「无法判断 + 补证方向」、被拦截轮显式标注；导出默认脱敏（零命中时
  上浮「请确认脱敏名单完整」），原样导出必须 `confirmRaw=true` 显式二次确认，
  两种导出都写审计（`REPORT_EXPORT` / `REPORT_EXPORT_RAW`）；导出前报告全文过
  `gate.CheckGuidance`，违例拒绝导出并写 `REPORT_EXPORT_BLOCKED`；报告落
  `dataDir/reports/`（0600）。
- 待办（不阻塞）：PDF 走系统打印路径的用户指引文案；审校「风险说明」叙述层
  的模型补充（当前报告全文零模型文本）。

### P1 法院场景包（1 周）

- 落点：`frontend/src/views/`（新）、court Profile 文案、空状态组件。
- 验收：新用户从安装包到看到第一份事实板 ≤ 3 步；每步有空状态与下一步指引；
  跨案件隔离在向导中显式可见。


## 4. 依赖与顺序

```
E0 评测基线 ──────────────┐（基线数字是后续所有模型能力的标尺）
O1 OCR ────┐              ├── M7 审校报告 ── P1 法院场景包 ── v0.4 发布
O2 docx ───┘（O1/O2 并行）┘
OpenLaw: openlaw-element CLI ──→ M4 要件数据（并行推进，不阻塞）
```

E0 即刻可开工（零新代码依赖）；O1/O2 并行；M7 依赖的 M2/M6 已在主干；P1 收尾。
预计总工期 4–5 周，产出 v0.4.0 预览版（面向真实法律从业者的第一个可试用版本）。

## 5. 明确不做

roadmap-next §6 全部继续有效。本版本另加：不做 Web 版、不做多用户协作与云同步、
不做 OCR 结果的模型自动纠错、不做报告模板市场（先打穿一个场景）。

## 6. 产品工作流（非代码，与里程碑并行）

| 事项 | 内容 | 挂靠 |
|---|---|---|
| 评测公开 | E0 报告 + 门禁对抗集结果发在仓库（README 链接） | E0 |
| 签名分发 | macOS Developer ID 签名 + 公证，消除「右键→打开」门槛；Windows 代码签名评估 | E0 |
| 试点 | 1–2 家法院立案庭/法官助理或律所团队内测，目标 20 份真实卷宗走通全流程 | M7 后 |
| 反馈闭环 | 试点问题按「合规红线 / 效果 / 体验」三类归档，红线类一票否决发版 | P1 后 |
| 北极星指标 | 周活跃案件数（走完「接入→报告」全流程的案件）；辅助指标：报告导出率、OCR 入库成功率、门禁误拦率 | 持续 |

## 7. 风险与处置

| 风险 | 处置 |
|---|---|
| OCR 质量差污染事实库 | 弱态入库上限 + `verify.Locate` 定位失败即拒 + 低置信黄标；模型永不改字 |
| 评测不达标（幻觉率 ≥1%） | 如实公布 + 出专项修复清单，不降标准不发「达标」声明；参照 roadmap「未达标项如实保留」 |
| docx 复杂版式解析失真 | 表格线性化标注 + 修订批注独立区块；解析存疑段落标「解析降级」而非静默丢弃 |
| 报告被误当法律意见 | 固定免责头部 + 全文过 gate + 弱态标注；DISCLAIMER 随报告副本分发 |
| 试点卷宗数据合规 | 全程本地不出域 + 默认脱敏 + 哈希链审计；试点协议明确数据不出机器 |
| 法院场景文案诱导性表述 | 向导与空状态文案进 gate 词表扫描的合规用例，UI 文案纳入 CI |

## 8. 定案与待确认

1. ~~E0 评测集来源~~ **已定案**：LocalCases 正常对接调用（`/api/search` 圈定 +
   `/api/case/{id}` 取文，见 §3-E0），不要求评测专用端点；需对方配合的契约项为
   LC-8 / LC-9 / LC-10，已按既有编号提入
   [localcases-integration.md](localcases-integration.md) §8，未就绪时有完备降级路径。
2. `ocr_vision` 是否 v0.4 必做，还是先 tesseract 单引擎 + 接口留位？
3. 试点渠道：走 OpenLaw 社区还是直接机构联系？
