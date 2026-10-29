# 更新日志

本项目遵循 Semantic Versioning，版本语义见开发文档 10.4。

## Unreleased（阶段二开发中）

### 新增
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
