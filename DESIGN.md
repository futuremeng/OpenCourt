# OpenCourt 设计系统（DESIGN.md）

由 Impeccable 审计后的 `/document` 步骤建立。令牌的**唯一代码来源**是
`frontend/src/style.css` 的 `:root`；本文件解释语义与使用规则。任何组件
scoped 样式只允许引用 `var(--…)`，禁止硬编码十六进制色值。

## 品牌基调

- 定位：法律从业者的证据纪律工具。视觉身份 = 靛蓝（`--accent` #4338ca）+ 中性灰阶，
  克制、文书感，不追潮流。
- 一个字：可信。颜色只用来编码状态与后果，不做装饰。

## 颜色令牌

### 深色模式

- 由 `style.css` 在 `prefers-color-scheme: dark` 下**整体换值**（跟随系统外观）；
  组件层规则不变——只引用 `var(--…)`，禁止针对深色单独写色。
- 换值原则：表面按 底 < 面板 < 沉底 递进提亮；文字反相；强调色与反馈前景
  提亮保对比度，底色/边框改深底同色相；事实五态色相不变、只换深底。

### 表面与文字
| 令牌 | 值 | 用途 |
|---|---|---|
| `--bg` / `--surface` / `--surface-sunken` | #f6f7f9 / #fff / #f3f4f6 | 页面底 / 面板 / 沉底 |
| `--border` / `--border-strong` | #e5e7eb / #d1d5db | 分隔线 / 控件描边 |
| `--text-1/2/3` `--text-inv` | #1f2937 / #374151 / #6b7280 / #fff | 主文 / 次文 / 辅助文 / 反白 |
| `--accent` `-soft` `-ink` | #4338ca / #eef2ff / #3730a3 | 主行动、当前态、焦点环 |

### 反馈语义（全局唯一，禁止组件另立）
| 语义 | 前景 | 底 | 边框 | 旧值（已废除） |
|---|---|---|---|---|
| 成功 ok | `--ok` #047857 | `--ok-bg` #ecfdf5 | — | #065f46 / #1a7f37 |
| 警告 warn | `--warn` #b45309 | `--warn-bg` #fffbeb | `--warn-border` #fde68a | #92400e / #9a6700 / #fbbf24 |
| 错误 bad | `--bad` #b91c1c | `--bad-bg` #fef2f2 | `--bad-border` #fecaca | #c0392b |
| 信息 info | `--info` #1a5fb4 | `--info-bg` #eef5ff | `--info-border` #bcd4f5 | — |

「诚实降级黄标」统一使用 warn 三元组（`--warn-bg` + `--warn-border` + `--warn`），
全产品只允许这一种黄色。

### 事实五态（产品核心语义）
`--state-confirmed`（已溯源确认·绿）/ `--state-single`（单源未补强·蓝）/
`--state-opponent`（对方主张·黄）/ `--state-conflicted`（冲突·红）/
`--state-unsupported`（无依据·灰）。五态色只允许出现在事实看板列底与状态标注。

## 字体

- 家族：PingFang SC / Microsoft YaHei / system-ui（中文优先，不引入西文字体）。
- 阶梯（禁止半整数）：`--fs-xs` 12 / `--fs-sm` 13 / `--fs-md` 14 / `--fs-lg` 16 / `--fs-xl` 20。
- 面板标题 h2 用 14px 加粗；正文 13px；辅助说明 12px。

## 组件规则

- `.badge`：pill 形（`--r-pill`），浅底彩字（ok/warn/bad 三元组）；顶栏与法条统计
  用深底反白变体（`--topbar-badge` 底）。同一含义在任何面板必须同色。
- `.panel`：白底 + `--border` + `--r-3`，由内容组件自带（App.vue 不再代包）。
- 按钮：默认描边按钮；主行动 `.primary` 每面板至多一个；`.sm` 用于表格行内。
- 提示框 `.notice`：warn 三元组；错误框用 bad 三元组；一律带可执行文案。

## 布局

- 主区按办案阶段组织（v0.4 P3，docs/design-workflow.md）：阶段导航常驻，
  每阶段内部 `repeat(auto-fit, minmax(460px, 1fr))`：宽屏双栏、窄窗自动单栏；
  非当前阶段 v-show 隐藏（保留面板状态与暴露方法）。
- 顶栏只留对办案有后果的状态（角色/智能模型/外网检索/使用统计）；
  已启用=accent，离线/未启用=中性，仅使用统计开启显 warn；其余系统状态
  迁入「工具与数据 → 运行状态」。
- 事实看板 `repeat(auto-fit, minmax(180px, 1fr))`。
- 控件最小高度 32px（`.sm` 28px），满足 WCAG 2.5.8。

## 可访问性底线

- 每个输入控件必须有 `aria-label`（placeholder 不算）。
- 动态错误/结果容器 `role="status"`（拦截、拒绝、状态机反馈必须可被朗读）。
- 统一 `:focus-visible` 焦点环；`prefers-reduced-motion` 已全局处理。
- 装饰性 emoji/符号 `aria-hidden="true"`；✓/✗ 必须伴随文字标签。

## 交互范式

- 高危确认统一为**页内两步式**（按钮 → 原位「确认/取消」），不使用原生 `confirm()`。
- 状态永远由后端状态机判定，前端只呈现；被拦截必须可见、可追溯。
