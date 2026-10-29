# 架构设计

本文对应《OpenCourt 开发文档》第二、三、八章，并记录桌面化改造带来的偏离。

## 与原文档的偏离（桌面优先）

| 原文档 | 本实现 | 原因 |
|---|---|---|
| Python 3.10+ | Go 1.21+（Wails v2） | 桌面单二进制分发、三平台安装包、无运行时依赖 |
| docker-compose 部署 | 桌面应用为主；Docker 保留为机构服务器模式（`deploy/`） | 用户下载安装包即用 |
| pytest / poetry | go test / go mod；Makefile | 语言随栈更换 |
| tests/unit + integration | Go 惯例：单测在各包内；合规测试集中于 `tests/compliance/` | Go 社区规范 |
| HTTP API | Wails bindings 进程内直连 | 无端口暴露，攻击面更小，更贴合「默认拒绝」安全原则 |

保留不变的核心：四层架构、依赖单向、事实五态、状态机硬规则、门禁、脱敏、审计、Profile 机制、OPENCOURT_ 环境变量覆盖、默认离线。

## 包结构与依赖方向

```
main.go ──────────→ config, core, profiles（组装层）
profiles/ ──┐
adversary/ ─┼───→ neutral/ ───→ core/
            └───────────────────↗
plugins/ ──(仅接口)──→ core/domain, core/gate(确定性红线)
core/* 禁止依赖任何上层包
```

## 跨项目边界（语料所有权决定能力归属）

**规则**：谁拥有语料，谁拥有该语料的索引、检索与语料层校验；OpenCourt 只拥有**案件**，
负责调用、取舍、案件层检验与合规输出。单向读，永不写回数据项目。

| 语料 / 能力 | 归属 |
|---|---|
| 法规语料、条文索引、版本链、效力状态、断链与签名 | OpenLaw（进程内 sdk，禁直连 DB） |
| 「引用是否真实 / 当时生效版本 / 引文是否一致 / 溯源是否断」 | OpenLaw（只有它有 PDF + 哈希 + manifest） |
| 构成要件库与人工确认 | OpenLaw |
| 裁判文书语料、检索、精确过滤、facet 计数 | LocalCases（回环 HTTP + token，外部可选组件） |
| 案件、卷宗、五态事实、引用留痕、门禁、脱敏、审计、推演 | OpenCourt |

两类「检验」不可混：**语料层校验**（引用真不真）归数据项目；**案件层检验**
（在本案能不能用、挂到哪个事实、能否经门禁输出）归 OpenCourt。
`core/retrieve` 只服务**本案卷宗内检索**，不得实现法条/类案检索。
脱敏的唯一强制关口是 `core/redact`，外部返回内容进模型或 UI 前必须再过一次。

**分发与调用形态**（三项目各自可独立分发、自带界面，通常一并安装）：
**代码各自分发，语料只有一份**；生产者独占写权，消费者一律**只读**打开语料并做
**版本协商 fail-closed**（不匹配即报错，不迁移、不猜）。调用形态定案为
OpenCourt→OpenLaw **进程内 SDK（只读入口）**、OpenCourt→LocalCases **回环 HTTP**
（只连接、不启动对方进程）。

违例与修法、跨项目契约要求见 [plan-v0.2.md](plan-v0.2.md) §D0 与 §10；
调用方式、语料所有权、发现机制、安装形态矩阵与兼容矩阵见
[integration-model.md](integration-model.md)；
逐项真实契约（字段名、参数名、错误码、降级行为）见
[openlaw-integration.md](openlaw-integration.md) 与
[localcases-integration.md](localcases-integration.md)。


## 各层落点

- **core/domain**：五态枚举（`FactState`）、Fact/Conflict/Assumption/Conclusion、可见性、案件阶段、引用溯源（Citation）
- **core/statemachine**：三条硬规则的唯一仲裁入口（`ArbitrateUpgrade` / `ResolveConflict` / `RejectAssumption`）。任何状态变更必须经过此包
- **core/gate**：输出三层门禁——引导性词表拦截、幻觉实体落地检查、废止法条标红
- **core/redact**：姓名一致替换（还原表）、证件/账号掩码、地址泛化、案号保留；动作含原始哈希供审计
- **core/audit**：JSONL 追加 + 哈希链（`Verify` 可检出任意篡改）
- **core/verify**：引用原文定位（核验器基础能力）
- **core/ingest / extract / retrieve**：文档接入、确定性要素抽取、关键词检索+重排（向量检索待 embedding 插件）
- **core/storage**：SQLite（第六章数据模型：facts / conflicts / assumptions）
- **neutral/**：汇聚器（时间线+冲突标记）、核验器、要件匹配器、盲区生成器、程序守护
- **profiles/**：court / plaintiff / defendant / party 四份 profile.yaml + 加载器（默认拒绝：disabled_tools 命中即禁）
- **adversary/**：对方模拟与漂移检测（接口，阶段三实现）、轨迹监控（已实现抖动判定）
- **plugins/**：LawSourcePlugin / CaseSourcePlugin / ModelAdapter / BrowserPlugin 接口；OpenAI 兼容适配器（chat/completions + embeddings，默认禁用，Key 走环境变量，强制「禁止训练」头；不假定服务商）；BrowserOS 浏览器辅助插件（默认禁用，两段式 Propose→确认→Execute，红线由 core/gate 代码级强制）

## 桌面端安全模型

- 无 HTTP 服务、无监听端口；前端经 Wails IPC 调用白名单方法
- 数据目录：`os.UserConfigDir()/OpenCourt`（0700），SQLite + audit.jsonl 全部本地
- 默认离线：`model.enabled=false` 时任何模型适配器调用直接报错
- 遥测：不存在遥测代码路径；配置项保留以兼容机构模式审计
- 审计：所有查询/创建/门禁拦截/状态变更均写入哈希链日志

## 路线图对应（第十一章）

- Phase 1（当前）：core/ 可运行 + CLI 等价能力经桌面 App 暴露 ✅
- Phase 2：neutral/ 完整阅卷链路（依赖 OCR 与模型插件）
- Phase 3：profiles/ 推理链路 + adversary/ 完整实现
- Phase 4：案件交换包（`internal/exchange`，AES-256-GCM + 签名）、插件加载器

下一阶段（庭审模拟与案件推演方向）的现状盘点、与外部参考方案的取舍、里程碑
M1–M8 与依赖顺序见 [roadmap-next.md](roadmap-next.md)；
v0.2 已定案实施方案见 [plan-v0.2.md](plan-v0.2.md)。

