# 产品体验重构：从功能清单到办案动线（v0.4 P3）

本文档是 OpenCourt 工作台体验重构的设计定稿，规则编号引用产品设计规则集
（`rule/…`）。设计令牌与组件规则仍以 `DESIGN.md` 为唯一来源，本文只裁决
**信息架构与动线**；与 DESIGN.md 冲突处以 DESIGN.md 的成文规则为准并在此登记。

## 诊断：当前 UI 是一张功能清单

v0.4 之前的工作台把 11 个面板平铺在同一个 auto-fit 网格里
（法条数据 / 法条检索 / 类案 / 卷宗检索 / 对抗推演 / 录入事实 / 门禁自检 /
文档接入 / BrowserOS / 审校报告 / 事实看板），面板之间零层级：

- 每个面板权重相同，用户必须自己知道「哪个面板干什么、现在该用哪个」
  ——违反 `rule/one-primary-action`（每屏应有一个主行动）。
- 顶栏 6 枚徽章（Profile、遥测、模型、BrowserOS、脱敏级别）是系统黑话
  堆叠，与办案动作无关——违反项目诚实纪律的「法言法语」要求，且
  「已启用=黄色告警」把状态语义用反。
- 用户的心智模型是**办案流程**（建案 → 卷宗 → 事实 → 核验 → 出报告），
  不是工具货架。违反 `rule/preserve-mental-model`。

## 决策：按办案阶段重组，而不是按工具重组

新增「办案阶段」导航（案件条下方、常驻）：

**案件总览 · 阅卷 · 事实 · 核验 · 报告 · 工具与数据**

对应真实动线（court Profile 的四步 guide 数据：new-case / import-doc /
document / report），代码仍不判断 role（D4 契约不变），阶段完成状态
仍由真实计数推导。

### 阶段归属（面板 → 阶段）

| 阶段 | 内容 | 主行动 | 依据 |
|---|---|---|---|
| 案件总览 | 向导（guide 数据）、本案计数条（当前案件/文档数/事实数/五态计数）；未选案件时先给「新建案件」空态 | 向导的下一步 | `rule/empty-state-action`、`rule/structure-before-containers` |
| 阅卷 | DocsReader（文档接入与阅卷）、CaseSearch（卷宗检索·本案） | 导入卷宗文档 | `rule/one-primary-action` |
| 事实 | 录入事实表单、事实看板（五态） | 录入 | `rule/one-primary-action` |
| 核验 | LawSearch（法条检索与引用核验）、SimilarCases（类案检索） | 核验 | 同上 |
| 报告 | GateCheck（出报告前检查）、ReviewPanel（审校与报告，锚点 sec-report） | 审校 | 同上 |
| 工具与数据 | LawDataPanel（法条数据）、BrowserOSPanel、SimPanel（对抗推演，court 如实禁用）、运行状态 | 面板各自 | 渐进披露：非办案主链的工具折叠到一处（`rule/smallest-intervention`） |

- 阶段切换用常驻按钮（`aria-current="page"`，键盘可达、Tab+Enter 可切换），
  满足 `rule/keyboard-complete-flow`；不改 desktop.go 菜单，既有
  Cmd+N/O/K/Shift+K 落点不变，只是先切到目标阶段再聚焦
  （`rule/no-redundant-entry`：快捷键仍是唯一入口）。
- 面板一律 `v-show` 隐藏而非销毁：保留各面板内部状态与暴露的 ref 方法，
  案件切换刷新契约（onCaseSwitched）不变。
- 一个视图一个主行动；面板内部至多一个 `.primary`（DESIGN.md 组件规则，
  项目成文优先于规则集）。
- 高危操作保持页内两步式确认（DESIGN.md 交互范式，`rule/inline-before-modal`、
  `rule/irreversible-action-safeguard`）。

### 顶栏收敛

顶栏只留：标题+版本、使用角色（审判模式/律师模式）、三个诚实状态：
智能模型（已启用/离线）、外网检索（已启用/未启用）、使用统计（开启/关闭）。
- 「已启用」不再用告警黄：已启用=accent，离线/未启用=中性灰；
  使用统计（遥测）开启时仍用 warn（隐私相关，需注意）。
- 脱敏级别与 Profile 原始值移入「工具与数据 → 运行状态」，逐项配一句
  白话说明；设置面板入口不变。数据缺失/未知仍如实显示，不渲染假状态。

## 状态覆盖矩阵（每视图必须成立的态）

| 视图 | 空态 | 已加载 | 错误/拦截 |
|---|---|---|---|
| 总览 | 未选案件→EmptyState+新建案件；无文档→导入按钮 | 计数条+五态计数 | 案件数据读取失败沿用外壳 error |
| 阅卷 | 本案无文档→导入按钮 | 文档列表+时间线+要件盲区 | OCR/解析失败如实列出 |
| 事实 | 看板五列各自「暂无」 | 五态计数 | 状态机拒绝必须可见 |
| 核验 | 检索词为空不代猜 | 结果+UNKNOWN/OL-1006 显式标注 | 核验失败原样透传 |
| 报告 | 无事实→提示审校需至少一条事实 | 门禁自检+报告 | 门禁未过不得静默放行 |
| 工具 | BrowserOS 未配置→指引 | 各面板真实状态 | court 禁用工具显示禁用文案，不假装可用 |

## 兼容性契约（重构不得破坏）

1. Workspace `defineExpose`：`onCaseSwitched / importFile / focusLawSearch /
   focusCaseSearch / focusDocSearch` 签名不变（App.vue 菜单/快捷键依赖）。
2. guide stepStatus 由真实数据计算；未识别 action 不给按钮；代码零 role 分支。
3. court Profile 对 strategy/win_eval/defense_rehearsal/verdict_tendency
   的禁用文案保留在 SimPanel 内。
4. dist/ 为跟踪文件：改完前端必须 `npm run build` 并同提交入库。
