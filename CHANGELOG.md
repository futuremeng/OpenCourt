# 更新日志

本项目遵循 Semantic Versioning，版本语义见开发文档 10.4。

## Unreleased

### 变更
- **v0.4 P3 产品体验重构（功能清单→办案动线）**：工作台由 11 面板平铺改为六个
  办案阶段（案件总览/阅卷/事实/核验/报告/工具与数据，`docs/design-workflow.md`
  定稿）：阶段导航常驻、键盘 Tab+Enter 可达、当前阶段 `aria-current`；非当前
  阶段 v-show 隐藏（保留面板内部状态与暴露方法，案件切换刷新契约不变）；向导
  落点先切阶段再执行/聚焦，菜单快捷键落点不变；总览展示真实计数（当前案件/
  文档/事实五态）与未选案件空态；顶栏 6 枚黑话徽章收敛为 4 个法言法语状态
  （使用角色/智能模型/外网检索/使用统计，已启用不再用告警黄），脱敏级别与
  运行详情迁入工具区「运行状态」逐项白话如实；对抗推演 court 禁用文案保留。
  实机验证（Orca）：六阶段内容各自渲染且隐藏阶段不泄漏进无障碍树、Cmd+K
  切至核验、向导真实计数、court 禁用提示均确认；webview 焦点行为后台模式
  不可证（逻辑未改）。`vue-tsc` 0 错误、`vite build` 通过、Go 全门禁绿

### 新增
- **v0.4 P1 法院场景包骨架**：① 引导动线入 Profile——`court/profile.yaml` 声明四步
  向导（建案→卷宗接入→阅卷与事实板→产出审校报告），`profiles.GuideStep` 随
  `AppInfo.guide` 透出；向导按数据渲染，代码零 `role` 分支，其余三个 Profile 未声明即
  保持自由动线（plan-v0.4 D4）；② `views/Onboard.vue` 首屏向导：步骤完成状态一律
  来自真实计数（当前案件/本案文档/已录事实），不自称完成；每步空状态由
  `components/EmptyState.vue` 呈现并给下一步按钮，未识别的 action 只展示文字不给按钮；
  跨案件隔离在建案步文案显式可见；③ `App.vue` 拆为外壳 + `views/Workspace.vue`：
  菜单/快捷键落点经暴露方法转发，外壳不再持有功能面板 ref；滚动锚点
  `sec-facts`/`sec-report`；验收标准「安装→首份事实板 ≤3 步」在向导内达成
  （建案→接入→看板上板）。`vue-tsc` 0 错误、`vite build` 通过、`go test ./profiles/`
  新增 guide 声明与「其余 Profile 无 guide」两用例

  ### 修复
  - **P1 向导序列化键名**：`GuideStep` 只有 yaml 标签，经 Wails 绑定序列化后键名变成
    大写的 `Title/Text/Action`，前端按小写读取致四步全部渲染成空壳（标题/文案为空、
    无动作按钮、完成状态永不点亮）——GUI 实走首次暴露；补小写 json 标签与序列化
    契约用例（`TestGuideJSONKeys`），重建后向导四步渲染与完成态实测正常

## v0.3.0（2026-11-20）

### 变更
- **UI 设计系统落地（Impeccable 审计修复，9/20→Good）**：① **设计令牌唯一来源**——
  `frontend/src/style.css` `:root` 令牌 + 根目录 `DESIGN.md`（五态/ok-warn-bad 语义色统一，
  废除此前三套并行反馈色与两套「诚实降级黄标」；组件 scoped 样式只允许 `var(--…)`，
  硬编码色值清零）；② **App.vue 871→255 行拆分**——法条数据/门禁自检/文档阅卷/
  审校报告/BrowserOS 五个功能区抽出为独立组件，案件切换统一走各组件 `refresh()` 契约；
  ③ **可访问性底线**——输入控件补 `aria-label`、错误与拦截反馈 `role="status"`、
  全局 `:focus-visible` 焦点环、触控目标 ≥28px、表头 `scope`、`prefers-reduced-motion`；
  ④ 主布局与事实看板改 `auto-fit` 网格（窄窗自动单栏）；FactBoard 模板内重复
  `filter` 收敛为 computed 一次分组；高危确认（应用安装）由原生 `confirm()` 统一为
  页内两步式，与「原样导出」范式一致；`vue-tsc` 0 错误、`vite build` 通过

### 加固
- **v0.4 工作集代码 review 加固轮**：① **D1 生产接线**——`App.UpgradeFact`
  升级仲裁前经 `storage.IsOCRSource` 反查来源文档 OCR 标记置
  `SourceFromOCR`（此前该字段只在测试中置位，生产升级路径 OCR 事实可进
  强态；查询失败 fail-closed 拒绝升级），App 级用例
  `TestUpgradeFactOCRCapped` 走生产入口锁定；② **门禁组合规则去误伤**——
  「建议」+决策词改为**同句**同现且决策词后紧跟名词化后缀（诉讼时效/
  仲裁协议/起诉状等）不计入同现，「建议核实诉讼时效…」等补证方向不再
  误拦，对抗集补 3 条必放样本（21 条，误拦/漏拦均 0）；③ **审校红项阻断
  导出落地**——`ExportReport` 兑现「Red>0 必须阻断」不变量（红项拒绝
  导出 + `REPORT_EXPORT_BLOCKED` 审计；原样导出自身的红项标注发生在阻断
  之后不受影响），并修正脱敏顺序（先并入复查说明再渲染、对最终全文
  脱敏）——导出报告必带「脱敏复查」说明；④ **扫描 PDF 取图**——按页
  归组（同页多内嵌图拼接为一页，不再因取图数≠页数误拒整文档；缺图像层
  的页明确拒绝），临时目录经 cleanup 回收不再泄漏，页置信度取各图最低值；
  ⑤ **tesseract 词间拼接**——相邻 ASCII 词补空格（中文逐字不插），
  中英混排不再粘连且不影响回原文定位；⑥ 收敛近重复：删除
  `OCRParser.LowConfidencePages` 与各解析器无人消费的 `MinConfidence`
  字段，低置信判定单一落在 `Registry.LowConfidencePages`；
  `migrateOCREngine` 列顺序确定化；docx「整档封顶弱态」保守推论注释补全

### 新增
- **M7 审校与报告导出（v0.4 方案，plan-v0.4 D3）**：`neutral/review.go` 审校
  五项**确定性**检查（模型零参与）——法条引用核验透传判定（未核验 warn /
  核验失败红）、事实引用回原文一致性（`verify.Locate` 失败或强态缺坐标即红）、
  五态合规（「无依据」事实入库即红）、推理链完整性（轮次结论必须可回溯到
  存在的事实，被拦截轮除外）、未解决冲突上浮；`neutral/report.go` Markdown
  报告确定性拼装——固定免责头部「AI 辅助整理，不构成裁判或法律意见」、
  **强态才进事实段落**（弱态一律进「待人工确认清单」）、缺来源结论显式
  `**[缺来源]**` 标红、冲突原样并列不消解、盲区只给「无法判断 + 补证方向」、
  模拟轨迹含被拦截轮标注；绑定 `ReviewCase` / `ExportReport`——**导出默认
  脱敏**（脱敏零命中时上浮「请确认脱敏名单完整」），原样导出必须显式二次
  确认（`confirmRaw`）并写 `REPORT_EXPORT_RAW` 审计，两种导出均留痕；
  **报告也是输出**：导出全文过 `gate.CheckGuidance`，倾向性表述在报告层
  二次拦截（拒绝导出 + `REPORT_EXPORT_BLOCKED` 审计）；前端「审校与报告」
  面板（审校结果红/黄分层 + 脱敏导出 + 勾选确认的原样导出）；合规用例
  `TestReportCompliance` 锁定「弱态事实出现在报告事实段落即失败」
- **O2 docx 解析（v0.4 方案，plan-v0.4 D2）**：`core/ingest/docx.go` 纯 Go
  `archive/zip` + `encoding/xml`（零新依赖、零 CGO）——修订痕迹（`w:ins`/`w:del`）
  与批注（`w:comment`）**保留为独立标注区块**（`verify.DocumentText.Annotations`）
  上浮，**不进正文文本层**：回原文定位（`verify.Locate`）对标注文本一律失败，
  标注只能作证据线索、不能作事实的正文引用来源（不自动采信）；表格线性化为
  文本并标注「表格已线性化」；内嵌图片走 OCR 管道——未配置引擎**只拒绝该图**
  并标注原因，正文照常入库（图片识别不了不等于文档读不了，但绝不静默丢弃），
  识别成功进第 2 页起顺排并带置信度；损坏 zip / 缺 `word/document.xml` /
  空正文一律明确报错，`.doc` 与 OLE 伪装 `.docx` 给出「另存为 .docx」的
  可执行指引；zip 炸弹防护（单条目 64MB 解压上限）；`documents` 表新增
  `annotations_json` 列（幂等迁移，存量文档按无标注处理不猜测）、
  `SaveDocumentFull` 保存解析产出全部语义（OCR 标记 + 标注）往返不丢；
  前端卷宗行「标注」按钮展开独立标注区块（kind 徽章 + 作者 + 线索说明）；
  绑定 `DocumentAnnotations`；测试锁定：修订/批注不进正文且 Locate 必失败、
  正文可定位、表格线性化、损坏/伪装明确报错、内嵌图 OCR 管道与
  Registry/SetOCR 装配注入
- **O1 OCR 引擎接入（v0.4 方案，plan-v0.4 D1）**：`core/ingest` 两个本地引擎
  adapter——`ocr_vision.go`（macOS 系统 Vision，经极小 Swift helper
  subprocess，`scripts/ocr_helper/main.swift` + `scripts/build-ocr-helper.sh`，
  本机中文识别冒烟通过）、`ocr_tesseract.go`（本机 tesseract `--tsv`，词级
  置信度取均值，跨平台兜底）；`ocr_detect.go` 探测顺序 = 显式指定 → Vision
  helper → PATH tesseract → 无（扫描件维持明确拒绝，**指定引擎不可用绝不
  降级到其他引擎**）；扫描型 PDF 经 `pdf_images.go`（pdfcpu 纯 Go 抽每页内嵌
  图，零 CGO/PDFium）逐页 OCR，任一页失败或空白即整体拒绝——部分入库会让
  回原文定位出现无法解释的空页，比拒绝更危险；`config` 新增
  `ingest.ocr.engine/path/lang/min_confidence`（默认 engine=none 安全默认，
  `OPENCOURT_OCR_*` 环境变量覆盖）；`verify.DocumentText` 带 OCR 标记与逐页
  置信度，`documents` 表新增 `ocr_engine` 列（幂等迁移，存量文档一律按非 OCR
  处理不猜测）；**D1 硬纪律**：`statemachine` 拒绝 OCR 来源事实进入
  「已溯源确认」——OCR 文本入库上限「单源未补强」，纠正只允许人工编辑后
  重新核验，绝不由模型自动改字；低置信页（默认 <0.6）审计留痕并上浮 UI，
  前端卷宗面板 OCR 徽章；合规用例 `TestOCRCompliance` 锁死三纪律（扫描件
  不得绕过 OCR / OCR 文本封顶弱态 / 引擎缺失明确报错）。RapidOCR 高精度
  sidecar 按 D1 决策留 v0.5；helper 随 .app 打包（`Contents/Resources/`）
  待分发脚本接入
- **E0 评测 runner 与门禁对抗集（v0.4 方案，roadmap M8 前移）**：`eval/`（新包）
  门禁对抗集 18 条（必拦 10 / 必放 8，含 D6 固定标注措辞必放用例）对
  `gate.CheckGuidance` 出**误拦率/漏拦率**数字——离线确定性，`make eval` 即跑，
  基线报告 `tests/eval/report-2026-11-04.md`：误拦 0.00% / 漏拦 0.00%；模型阅卷
  指标（逃逸幻觉率 <1%、溯源准确率 >98%）走 **LocalCases 正常对接通道**——
  manifest 固化 id 清单 + sha256，逐条 `/api/case/{id}` 回源取文（LC-10），
  语料快照记 `corpus_digest`（LC-8，缺失时如实标注复现性受限）；正文与清单
  sha256 不一致即**剔除并计数**（评测集改版 evalset vN，不静默换集）；模型/类案源
  未启用时报告显式「未运行 + 原因」，绝不静默出假数字；`plugins` 补
  `GetCaseText` 与 health `corpus_digest` 解析；评测器与生产共用同一
  `verify.Locate`/`gate` 判定，不引入第二套标准。提给 LocalCases 的契约项
  LC-8/LC-9/LC-10 见 localcases-integration.md §8
- **gate 组合规则（对抗集暴露的真实缺口）**：「建议」与起诉/诉讼/上诉/撤诉/调解/
  仲裁同现即拦截——堵住「建议对被告提起诉讼」这类间隔写法绕过精确词表；
  补证方向类程序建议（不含决策词）与叙述性「原告提起诉讼」明确放行
  （`TestGuidancePairRule` 正反用例锁定）
- **对抗推演与庭审流程引擎（v0.3 M6，roadmap-next M6 / 开发文档 2.2、3.6、3.7）**：`core/flow`（新包）庭审阶段状态机（STATEMENT→EVIDENCE→DEBATE→FINAL）——阶段推进由确定性代码硬守卫（`AdvanceGuard`/`CheckPhaseEntry`）：当事人陈述无有效轮次或事实库无任何「有证据支撑」状态即不得进质证（无证据不进质证）、质证/辩论未完成不得进下一阶段、跳跃阶段一律拒绝（`ErrSkipPhase`），不靠提示词约定；`adversary/simulate.go` `Simulator` 实现 `OpponentSimulator`——对方模拟输入按 `Visibility` 硬过滤（`ViewFor`：本方保密材料在数据层进不了 prompt，不是 UI 隐藏；COURT_ONLY 对任何一方模拟不可见），模型未启用显式报错（`ErrModelDisabled`，绝不用模板/规则冒充「AI 模拟对方」），每条攻击点候选必须带 `fact_ids` 且过 `gate.Check`（无来源即拦截，`Blocked` 条目照常上浮不静默丢弃）；`adversary/drift.go` `LexicalDrift` 规则式立场漂移检测（极性词对确定性比对，漂移报警不依赖另一个会漂移的模型）；`neutral/moderator.go` `SummarizeIssues` 中立争点归纳——事实按要件分组、未消解冲突原样并列挂争点下不代作取舍、「对方主张」只标注不采信、要件缺失直通证据缺口不自动补全；App 绑定 `RunSimRound`（阶段守卫→发言门禁→立场漂移比对→对方模拟→轨迹监控；被拦截轮次也入库留痕但不计阶段完成度，审计 `SIM_ROUND`/`SIM_ROUND_BLOCK`/`SIM_GUARD_BLOCK`/`SIM_DRIFT`）、`SimReview`（轨迹抖动停止自动续演后恢复的唯一出口）、`SimStatus`、`ListSimRounds`、`SummarizeCaseIssues`；`core/storage` 新增 `sim_rounds` 表（轮次留痕挂 `case_id`，被拦截轮次可追溯）；`TrajectoryMonitor` 接入——连续翻转且均无新证据即触发人工复核（有新证据支撑的立场变化不触发）；法院/当事人 Profile 经 `disabled_tools: defense_rehearsal` 默认拒绝；全程固定免责标注「这是模拟推演，不是审判结果预测」；前端「推演」面板 `SimPanel.vue`（阶段完成度、轮次轨迹含被拦截轮、攻击点候选 Blocked 标红、漂移/待复核告警、争点归纳并列视图）；合规用例先行：本方保密材料不得进对方 prompt（prompt 捕获断言）、无 fact_ids/引用「无依据」发言拦截、倾向性攻击点拦截待改写、流程硬守卫四态、抖动无新证据必触发人工复核、立场极性翻转报警、争点归纳中立性、法院 Profile 禁用推演工具

- **本案卷宗混合检索（v0.3 M5，roadmap-next M5 / plan-v0.2 V2 收窄落地）**：`core/storage` 新增 `embeddings` 表（页级向量 BLOB + dim + text_hash + model，挂 `case_id`，`(case,doc,page,model)` 唯一——跨案件天然隔离、模型间向量空间不混用）；`core/retrieve` 补暴力余弦 `VectorSearch`（纯 Go，不引外部向量库）与 `FuseRRF`（RRF k=60 只取排名，分数尺度免疫；关键词/向量/融合三处同分一律按 Ref 升序——结果可复现是验收项）；`neutral/caseindex.go`：`IndexCasePages`（text_hash 未变跳过、改文只重嵌缺页）+ `SearchCaseDocs`（关键词榜+向量榜→RRF；降级四态——模型未启用/不支持 embedding/索引未建/运行期失败——一律 `Mode=keyword_only` 并 Note 显式说明原因，绝不静默冒充向量化过）；App 绑定 `SearchCaseDocs`/`CaseIndexStatus`（审计 `DOC_SEARCH`/`DOC_SEARCH_FAIL`，记录实际召回路径与向量新建/复用数）；前端「卷宗检索」面板（语义召回徽章、降级黄标、命中来源 keyword/vector 标注、页码定位）；**范围纪律**：只搜本案卷宗——法条归 OpenLaw、类案归 LocalCases，检索结果只是候选，无任何写事实库路径、永不产生五态变化（合规用例锁死）；单测/合规：RRF 融合可复现、模型/案件双隔离、text_hash 增量、四态降级标注、检索零写事实零改状态

### 已知限制
- **模型阅卷指标未运行**：开发文档 11.1 的幻觉率 <1%、溯源准确率 >98% 需显式启用
  模型与 LocalCases 后 `make eval`；当前报告仅含门禁对抗集（21 条，误拦率/漏拦率
  均 0.00%，见 `tests/eval/`）。LocalCases LC-8（`corpus_digest`）未就绪，评测
  复现性按降级路径如实标注（记 `indexed_documents` + 运行日期）。
- **法院场景包（P1）未做**：首屏向导「≤3 步到事实板」验收属 v0.4（plan-v0.4 §P1）。
- **分发**：macOS 构建自签名（首次打开需右键 → 打开）；OCR helper 尚未随 .app
  打包进 `Contents/Resources/`（分发脚本待接入）。

## v0.2.0（2026-11-02）

### 加固（代码 review 修复轮）
- **CI 就位并修好依赖**：新增 `.github/workflows/ci.yml`（gofmt/vet/build + `go test -race ./...` + 前端 typecheck/build + Wails 桌面壳冒烟）；`go.mod` 的 `replace ../OpenLaw` 是仓库外路径，CI 改为先把 OpenLaw 检出到 `.deps/OpenLaw` 再 `go mod edit -replace` 改写——不再因模块解析失败整条流水线红；`release.yml` 同步该策略并补 `-ldflags -X main.version=<git tag>`（版本单一来源）
- **运行时并发保护（P0）**：Wails 每次前端调用在独立 goroutine 中执行绑定方法，`App` 的共享可变状态（当前案件 ID、法条源句柄、语料目录、配置）改为 `sync.RWMutex` 保护，读写统一走 `scope()`/`caseID()`/`withLaw()`/`setLawSource()`/`lawDataDir()`；法条查询一律持读锁跨整个插件调用，换语料方取写锁后才关句柄——**杜绝在已关闭 SQLite 句柄上查询**；`SwitchCase` 的「改内存 + 写 state.json」移入写锁，失败回滚，不留撕裂状态。注意纪律：`withLaw` 闭包内不得再调 `scope()`/`caseID()`（RWMutex 不可重入）
- **审计日志并发写修复（真机竞态，`-race` 实测检出）**：`audit.Logger.Log` 无锁——并发写不仅数据竞争，还会写出两条相同 `prev_hash` 的记录把哈希链**自己写断**（`Verify` 从此报「疑似篡改」）。改为「取链尾 → 追加 → 推进链尾」全程持锁，并补并发链完整性用例
- **`lawDataDir` 持久化**：用户显式选择的语料目录此前只在内存，重启即回落配置值（表现为「语料又没了」）；现写入 `state.json` 并在启动时恢复
- **SQLite 与迁移**：连接串补 `journal_mode=WAL`/`busy_timeout=5000`/`foreign_keys=ON`/`synchronous=NORMAL`，最大连接数限 4；案件 `case_id` 迁移备份优先 `VACUUM INTO`（WAL 下的一致性快照），失败回落 checkpoint + 文件复制，并只保留最近数份备份
- **脱敏覆盖面**：新增手机号、统一社会信用代码（先掩身份证再匹配 + 必须含字母，防纯数字误伤）、邮箱、街道门牌地址；补误伤反面用例（案号/日期/金额不得被动）
- **输出门禁接入生产路径**：`neutral.Reader` 候选事实入库前先过 gate（含预测/引导性表述一律拒收并记 `GateBlocked`，拒绝原因 `GATE:<rule>:<phrase>` 上浮 UI）；`CheckOutput` 提取 FACT/LAW/CASE 引用 ID 并对照当前案件留痕核验（跨案件引用天然拦截），违规写审计；gate 预测词表扩充
- **自更新信任链收紧**：下载源白名单 + 强制 https + 可信主机匹配；资产名按 tag/平台/架构匹配（不再「取第一个资产」）；安装脚本默认执行 macOS Gatekeeper `spctl` 评估，自构建/内部分发需显式 `AllowUnsigned`
- **测试提速**：govlist 翻页/重试/刷新路径改虚拟时钟 `pacer`（原用真实 sleep，单测耗时分钟级），限流语义改为断言而非挂钟等待
- **入口拆分**：`main.go`（约 1000 行）按域拆为 `case.go`/`lawdata.go`/`lawsearch.go`/`similar.go`/`facts.go`/`documents.go`/`browser.go`/`update.go`（同 package main，Wails 绑定不受影响），入口只保留装配与生命周期
- **新增守卫与空白补齐**：绑定漂移守卫（go/ast 比对 `*App` 导出方法与 `frontend/wailsjs/go/main/App.d.ts`，漂移即 CI 失败并提示重跑 `wails build`）；并发回归（切案 × 读事实、换语料 × 检索）；`core/domain`/`core/extract`/`core/retrieve`/`adversary` 补齐首份单测（此前零测试）；前端新增 `make typecheck`（`vue-tsc --noEmit`）并修其检出的真实缺陷——`FactBoard` 引用取值键错误（`source_file`→`SourceFile`，此前页码/原文在界面上恒为空）
- **测试稳定性与 OL-1 对齐**：`TestConcurrentLawSourceSwap` 修调度竞态——全量套件负载下读者 goroutine 可能未被调度即被 stop，造成与产品无关的假失败；现启动 barrier（每读者至少完成一次检索再换语料）+ 换完保留读者活跃窗口才停。OpenLaw 侧 OL-1 已就绪（SDK 显式返回 `effect_status`），`TestOpenLawSourceSearchLaw` 期望从「一律 UNKNOWN」更新为按 fixture 透传语义（现行版本 EFFECTIVE、到期无关系佐证如实 UNKNOWN）；前端效力标签补 `REPEALED_PENDING`（废止待人工确认）

### 新增
- **类案检索接入（v0.2 M3 降级通道，plan-v0.2 §4.2/§4.3 / D3/D6）**：`plugins/case_localcases.go` `LocalCasesSource`（`CaseSourcePlugin` + `FilteredCaseSource`）——契约严格对齐 LocalCases `/api/search`（法条过滤参数 `law`、无 `limit`、分页 `offset`/`id_gt`、token 走 `x-localcases-token` 头）；错误处置按 §9（连接失败→`LocalCasesGuidance` 不降级、401→提示查 token_env、400→原样展示不改写重试）；`structured=false`（索引缺精确层的诚实降级）保守透传并隐藏 facet，`structured` 字段缺失按 false（LC-7 未就绪不冒充）；回环护栏（类案语料含未脱敏文书，拒远程地址）；`neutral/caseassist.go` `RunCaseSearch` 管道：检索→要旨强制过 `core/redact`（唯一强制关口）→`case_refs` 留痕（不进 facts、永不进五态）+ 固定标注 `CaseSearchDisclaimer`（D6）；config `localcases` 段（`base_url`/`token_env`/`enabled`，token 严禁入文件）+ `OPENCOURT_LOCALCASES_*` 覆盖——LC-6 发现文件未就绪前为手动临时通道（UI 已标注）；App 绑定 `SearchSimilarCases`/`ListCaseRefs`/`LocalCasesStatus`（health 探测 + 语料规模同显防小样本误读；审计 `CASE_SEARCH`/`CASE_SEARCH_FAIL`）；前端 `SimilarCases` 面板（未启用显指引不显空列表、降级提示、facet 分布 + 固定标注、本案留痕）；gate 词表补 D6 概率化表述（胜算/几成/可能性多大/倾向判决）并合规用例先行；单测：契约参数断言（law≠legal_basis、无 limit）、structured 三态、401/400 处置、非回环拒绝、脱敏一致替换、facet 降级不留痕、绑定守卫
- **按案由要件匹配与要件名封闭候选集（v0.2 M4，plan-v0.2 4.4 / V3 修法）**：`neutral.ElementSource` 窄接口（`ElementsByCaseType`）+ `MatchElementsFrom`——要件清单只从 OpenLaw 要件库取（封闭集，要件经人工确认导入、review_log 留痕归 OpenLaw），`src==nil`/案由为空/案由无要件一律显式报错给出出路，绝不回退模型自由生成要件；`plugins/law_openlaw_elements.go` 经 `AnalyzeElements(caseType, nil)` 适配要件清单读取（SDK 无独立列举 API，空事实集全量落 missing 即清单；案由无要件 not_found 显式报错不冒充空集）；【V3】阅卷要件名废除自由生成：`readerPrompt` 按案由注入要件库候选集（模型只能原样选或留空），后置校验兜底——模型给出的要件名不在封闭集内一律置空（不匹配不入库），无要件库时字段强制置空且 prompt 不携带任何要件候选；App 绑定 `RunElementMatchByCaseType`（无要件库 fail-closed，审计 `ELEMENT_MATCH`）；前端要件面板新增「按案由匹配」按钮（当前案由展示 + 覆盖/缺失清单 + 显式降级提示）；单测：封闭集匹配/四类显式降级、要件库 fixture 读取与无要件报错、V3 集外置空与无库强制置空（prompt 断言）、绑定守卫
- **法条检索与文书引用核验（v0.2 M2，plan-v0.2 4.1）**：`neutral/lawassist.go` 检索守卫 + 确定性引用抽取（正则「《法名》第X条之Y」，无模型参与、不猜简称）+ 核验管线（`LawVerifier` 窄接口，fake 可测）；`plugins/law_openlaw_verdict.go` 透传 OpenLaw `CheckCitation`/`FindRelation`/`ResolveVersion` 与 DTO——核验判定直接采用 OpenLaw 错误码（OK / OL-1001..OL-1006，不另建同义枚举），断链数据照常返回；`law_refs` 补 `law_id` 列（幂等迁移 + 自动备份；OpenLaw SDK 现不返回 law_id，`GetLawRelations` 对此 fail-closed 报错不猜——桥接缺口已记录）；`gate.CheckGrounding` 引用表分支：`FACT-` 走事实索引、`LAW-`/`CASE-` 走引用留痕索引（不存在即幻觉——跨案件引用天然拦截；UNVERIFIED/pending/非 OK 即违规 `REF_UNVERIFIED`，未核验不得静默通过）；App 绑定 `SearchLaw`/`VerifyDocLawCitations`/`ListLawRefs`/`GetLawRelations`（审计 `LAW_SEARCH`/`LAW_VERIFY`/`LAW_VERIFY_FAIL`）；前端 `LawSearch` 面板（检索结果与核验留痕，UNKNOWN「效力待确认」橙标、pending「待人工确认」橙标、OL-1006「无法溯源」红标、关系 pending 橙标）；合规用例 `TestCrossCaseReferenceBlocked`（存储隔离 + 门禁双防线）与 `TestLawRefUnverifiedSurfaces`（未核验上浮）
- **案件实体与案件级隔离（v0.2 M1，plan-v0.2 D1/D5）**：`cases` 表（标题/案由自由文本/阶段枚举/可见性）与 `law_refs`/`case_refs` 引用留痕表就位（D2：法条不是事实、类案要旨不是本案事实——不进 facts、永不进五态）；存量五表幂等迁移补 `case_id`（PRAGMA table_info 探测，迁移前自动备份 `<db>.bak-<ts>`，存量一律归 `CASE-DEFAULT` 不丢历史）；仓储改为 `Store.WithCase(caseID)` → `CaseScope` 案件限定视图，**旧全局方法已从 Store 移除（编译期强制）**——跨案件读写在 SQL 层不可能（WHERE 强制），跨案件引用天然 not-found；App 层新增 `CreateCase`/`ListCases`/`SwitchCase`/`CurrentCase` 绑定（新建即切换；非法阶段报错；当前案件持久化 `state.json` 重启恢复；审计 `CASE_CREATE`/`CASE_SWITCH`）；前端新增 `CaseBar` 案件切换栏（当前案件 + 切换 + 新建，切换后各面板联动刷新）；Profile `data_dir` 明确未启用（字段注释 + 加载告警，防误认为按立场分库）；单测覆盖：迁移幂等（重复打开 no-op 且不重复备份）、跨案件隔离（事实/冲突/文档/要素/refs 互不可见互改不动）、未核验引用保守缺省（UNVERIFIED/pending）

### 变更
- **V1 修法（plan-v0.2 D0）**：`plugins/law_openlaw.go` 删除 `openlawStatus` 自行映射——效力状态是语料层判断归 OpenLaw，只透传 `effect_status`（OL-1 未就绪前一律 UNKNOWN）；OpenLaw 打开方式改为 `sdk.OpenReadOnly`（OL-7 已就位：query_only、不跑迁移、不写 meta；OL-8 schema 协商 fail-closed）
- **V2 修法**：`core/retrieve` 删除 `LawEntry`/`CaseEntry`/`FilterRepealed`（全仓无引用的死代码）——法条/类案是外部语料，检索归 plugins 适配层 + OpenLaw sdk，效力归 OpenLaw；本包定位收窄为「本案卷宗内检索」

### 变更
- **既有绑定签名不变、内部一律走当前案件视图**（`ListFacts`/`CreateFact`/`UpgradeFact`/`RunReader`/`BuildTimeline`/`RunAggregate`/`RunElementMatch`/`RunBlindspots` 等经 `store.WithCase(currentCaseID)`），前端无感；`scanFact`/冲突扫描加固 NULL 列读取（存量库健壮性）

### 修复
- **条目 URL 硬校验 http/https 双收（重大真机教训）**：官方清单 `docurl` 大量为 `http://` 主站形态（301 → https）——只收 https 把整页 7 条全滤光、把满页误判成空页：所谓「大段稀疏空区」（472–499 等）实为 http 条目页，全库因此只收到 1970/约 7000 条。修复后同区段 6.9 条/页（曾 1.3）；主机仍硬锁 `.gov.cn` 域名族。教训已回写：页大小实测为 7 条/页（1000 页 ≈ 7000 条），「空页」判定前必须先排除协议过滤误伤

### 变更
- **规章库清单跨会话续接 + CLI 批量采集**：全库约 1000 页 > 单会话 200 页硬上限——起始页经官方「跳转」控件（`input.jumpText` + 「确定」）到达并以分页控件选中页码核验（等价人工输入页码；核验不过即报错，绝不在错误页采集）；`BrowseGovList` 增加 `startPage` 参数（前端加「起始页」输入）；新增 `cmd/govlistcollect` CLI（与应用面板同一代码路径与护栏）与 `scripts/govcollect.sh`（分批串行、会话间冷却 30s）
- **空页停止阈值可配（EmptyStop / -emptystop）**：`GovListOptions.EmptyStop`（默认 10）+ CLI `-emptystop`；`cmd/govlistcollect` 修复有条目会话误 exit 1 丢弃产物的 bug（注：早前「大段真实稀疏空区」结论已被证伪——见上「修复」条目，系 http 过滤误伤而非真空页）
- **采集改为跳转控件驱动（真机全量试跑校准）**：顺序点「下一页」只覆盖首页窗口前 ~97 页（其后按钮返回空壳渲染），跳转控件可达全部约 1000 页——主循环改为每页官方跳转 + 页码核验（弃用 `turnPage.next-page` 点击；页码核验不过 = 越过末页 `end_of_list` 正常结束）；SPA 空壳渲染防误停：DOM 重读 ≤5 次 → 官方跳转重刷当前页（等价人工刷新）≤2 次 → 连续 3 页真空才停（`empty_page`）；单测改为跳转驱动语义（含越界 `end_of_list`、虚拟时钟跳页间隔 ≥2s 断言）
- **国家规章库清单浏览（真机校准，neo 0.0.80）**：默认列表即混合清单（部门规章+地方政府规章同列，共约 1000 页，无独立分类页签）——分类改为按发布机关**逐条推断**（puborg 含「人民政府」→ 地方政府规章，与 OpenLaw `gvgzResolveLevel` 同规则），`cat` 参数改为可选过滤（空 = 全收）；翻页改为官方分页按钮 `button.turnPage.next-page` DOM click（等价人工点击；快照 ref 在大列表下不可靠，弃用两段式点击路径，evaluate 仍仅两段固定脚本）；URL 硬校验放宽至 `.gov.cn` 域名族（地方政府规章详情页直链地方政府网站——清单如实记录，OpenLaw 主机锁使其在导入报告可见转人工）；新增 SPA 就绪重试（只重读 DOM，零额外网络请求）与翻页间隔 ≥2s 虚拟时钟断言测试；`evaluate` 传 `maxChars=100000`（neo 默认 5000 字节即截断落本地文件，远端读回会丢数据）；`evaluate` code 语义修正为函数体（IIFE 返回 undefined）。真机端到端通过：2 页 13 条混合清单、翻页与 cat 推断正确
- **立案最终提交绝对红线（代码级强化）**：`core/gate/action.go` 提交类拦截词表扩展至 zxfw 实测词汇（递交/重新提起/提起申请/正式提交/pay/submit/delete 等，英文加词边界）；执行链三重硬约束写入文档——被拦候选不进 pending 登记表、ExecuteAction 复查 gate、一次性消费，两段式确认不可绕过红线（`docs/browseros-integration.md` 5.3）；gate 测试新增 zxfw 提交/非提交用例 12 条
- **语料许可可见性**：「法条数据」面板显示 manifest 内的机器可读许可声明（无声明时如实提示自行确认来源合法性）；`LawDataStatus` 新增 `license` 字段
- **代码许可证统一为 Apache-2.0**（OpenCourt 由 MIT 变更，与 OpenLaw 一致；新增明示专利授权条款）；语料数据另行许可：官方文本公有领域 + 本项目汇编产物 CC BY 4.0 + 欧盟部分保留 © European Union 署名（OpenLaw `DATA-LICENSE.md`，随每个语料包分发副本；`index.json` 附 license 字段）；新增第三方依赖许可清单（dmg 内附副本）；DISCLAIMER 第 5 条对齐「官方语料包单独发布」的新分发模式

### 新增
- **zxfw 站点地图勘察文档（docs/zxfw-sitemap.md）**：BrowserOS 只读实测人民法院在线服务网——完整路由表（181 条）、网上立案 7 步向导、33 类要素式案由与民间借贷要素式起诉状全字段图、材料清单与必传门控、登录态/会话/倒计时门控等操作纪律；为 filing-assist 映射层提供真实校准基准
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
