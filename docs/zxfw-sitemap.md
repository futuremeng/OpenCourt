# zxfw.court.gov.cn（人民法院在线服务网）站点地图 v1

勘察日期：2026-10-25 · 方式：BrowserOS neo MCP 只读勘察（用户已授权并登录）
状态：v1 = 路由表 + 导航结构（登录后深度勘察待续，见 §6）

## 1. 技术画像

- uni-app（Vue3 + Vite）H5 单页应用，hash 路由：`https://zxfw.court.gov.cn/zxfw/#/<route>`
- 完整路由表可从 `window.__uniRoutes` 读出：**181 条路由**（`/pages/pc/*` 仅 9 条，PC 大量复用 `pagesXxx/common/*` 与 `pagesXxx/pc/*` 模块路由）
- **登录态为纯内存**（localStorage/sessionStorage/可见 cookie 均无 token）：
  - 刷新页面或 `uni.reLaunch` 跨模块跳转可能触发登录守卫跳回 `#/pagesGrxx/pc/login/index`
  - ⚠️ 操作纪律：**只用用户级点击（等同 navigateTo）导航，绝不 reLaunch/刷新**；会话丢失只能人工重新登录
- 页面元素为 `UNI-VIEW/UNI-TEXT` 自定义元素；可交互快照中多为无标签 generic——按文本定位（`querySelectorAll('*')` 精确文本匹配）比 ref 更可靠

## 2. 服务指引页导航（#/pages/pc/fwzy/index，登录后）

分组（文本顺序即导航顺序）：

- **诉讼服务**：参与调解 / 申请司法确认 / **申请立案** / 参与庭审 / 送达签收 / 联系法官 / 申请执行 / 立案执行 / 执行线索举报 / 提交执行悬赏 / 申请恢复执行 / 申请申诉信访 / 补充提交材料 / 申请视频接谈 / 查询信访进展 / 申请保全 / 保全交费 / 申请担保 / 查询保单 / 委托鉴定 / 查阅结果
- **公开服务**（外链）：裁判文书网 / 审判流程公开 / 庭审公开 / 执行公开 / 企业破产重整 / 国际商事法庭 / 知识产权法庭 / 海事审判 / 减刑假释 等
- **在线立案主区**：首页 / 我的立案 / 我的案件 / 文书送达 / 在线交费 / 在线阅卷 / 诉讼工具 / 文书制作 / 其他服务 / 常用资料 / 担保服务 / 债权申报 / 执行悬赏 / 签章验证

## 3. 核心模块（路由前缀 → 功能）

| 前缀 | 功能 | 关键 PC 路由 |
|---|---|---|
| `pages/pc/*` | 外壳 | `home/index` 首页 · `fwzy/index` 服务指引 · `case-list/index` 我的案件 · `zxjf/index` 在线交费 · `zxyj/index` 在线阅卷 · `wszz/*` 文书制作 · `smqm/index` 实名签名 · `components/tbaj/index` 填表立案 |
| `pagesWsla/*` | **网上立案**（核心） | `pc/zxla/pick-case-type/index` 选择案件类型 · `pc/zxla/pick-lafs/index` 选择立案方式 · `pc/list/index` 我的立案 · `common/wsla/index` 立案主流程 · `common/wsla/components/ajxx/index` 案件信息 · `common/wsla/components/file-upload/index` 材料上传 · `common/wsla/detail/index` 立案详情 · `common/dbz/index` 诉状书? · `common/qsz/index` 签章 |
| `pagesAjkj/*` | 案件空间 | `common/case-space` · `wssd` 文书送达 · `wsyj` 在线阅卷 · `jzcy` 参加庭审 · `chat` 与法官沟通 · `zjjh` 调解? · `wtjd` 委托鉴定 · `xsjb` 线索举报 · `sxsq` 事项申请 · `components/tjtjcl` 补充提交材料 |
| `pagesGrxx/*` | 个人信息/认证 | `pc/login/index` 登录 · `pc/personal-info/index` 个人信息 · `pc/lssfrz/*` 律师身份认证 · `pc/frzcsq` 法人注册 · `pc/rlsb` 人脸识别 · `pc/sign` 签名(canvas) |
| `pagesOther/*` | 诉讼工具等 | `pc/ssgj/*` 诉讼工具/材料指引 · `common/wsdb` 文书模板 · `common/sfgk` 诉讼费减免? · `pc/pochan` 破产债权申报 · `pc/dxqm` 电子签名 · `app/drhq` 视频开庭? · `app/gxkj` 在线调解? · `zxxs` 执行悬赏 |
| `pagesWsxf/*` | 申诉信访（仅 app 路由） | `app/wsxfList` 等 |
| `pagesCustom/*` | 排期/开庭 | `pc/ktpt` 开庭排期平台 · `pc/csyy` 庭审预约? |
| `pagesAjkj/common/case-space` 等 | 案件空间 | — |

（标 ? = 拼音缩写推断，待登录后逐页核实）

## 4. 网上立案流程骨架（推断，待核实）

```
申请立案(点击) → pagesWsla/pc/zxla/pick-case-type 选案件类型
  → pagesWsla/pc/zxla/pick-lafs 选立案方式
  → pagesWsla/common/wsla/index 立案主流程
      ├─ components/ajxx 案件信息（当事人/诉讼请求/事实理由）
      ├─ components/file-upload 材料上传（upload 工具可定位 input[type=file]）
      └─ qsz 签章（红线：人做）
  → 提交（红线：人做） → pagesWsla/pc/list 我的立案（状态跟踪）
```

## 5. 操作纪律（勘察实测得出）

1. **绝不 reLaunch/刷新/直接改 hash**——会丢内存登录态（已实测踩坑）
2. 导航=按文本精确点击（等同用户点击，走 SPA navigateTo）
3. 表单填写走两段式候选（P2 已实现）；提交/签章/验证码/人脸永远交人
4. 登录丢失 → request_human_help（长轮询形态：客户端需保持连接或周期性 await_human_help）
5. `read` 抓回内容含当事人姓名等 PII——一律走脱敏管道后才可见于 UI/模型

## 6. 立案流程实测（2026-10-25 二次勘察）

### 导航机制（实测）
- 服务指引「申请立案」→ 同标签 navigateTo 到 pick-case-type（选案件类型页，含省份 picker，默认按登录省市）
- 类型卡片点击 = **`window.open` 开新标签**（源码 `pick-case-type` chunk 证实）：
  - 民事一审 → `/zxfw/index.html#/pagesWsla/common/wsla/index?ajlx=1501_000001-0301&ajlxMc=民事一审`（直达立案主流程，无 pick-lafs）
  - 有 `lafs` 属性的类型先过 `pick-lafs`（立案方式选择，带 `?ajlx=&lafs=&ajlxMc=`）
  - 跳转前把 `{ajcx, ajlx}` 写入**新标签 sessionStorage `layyParam`**
- **会话是按标签独立的**：新标签打开后向导 UI 可渲染，但首个需鉴权 API 返回 401 → 整页跳登录。源码显示登录后 `setStorageSync("zxfwtoken"/"userId"/"username"/"role")`，但实测 localStorage 无这些键（会话实际靠 per-tab 服务端会话）。**跨标签不共享登录** → 自动化必须在新标签内完成登录（人工）或全程单标签。

### 案件类型清单（pick-case-type 实测卡片）
保全：在线保全 | 民事：民事一审、民事二审、申请司法确认 | 行政：知产行政一审 | 刑事：刑事自诉 | 执行：申请执行 | 破产：强制清算申请审查、破产申请审查 | 申诉信访：民事申请再审、刑事申诉、执行监督申请

### 网上立案向导（wsla/index，民事一审实测）
步骤条：**选择受理法院 → 阅读须知 → 选择立案案由 → 上传诉讼材料 → 完善案件信息 → 预览和提交 → 提交成功**

步骤 1「选择受理法院」表单元素（DOM 实测）：
- 法院搜索 input（placeholder「请输入搜索关键词」）+「搜索」按钮 + 法院列表（按省，北京=高院/4家中院/16家基层/知产/金融/互联网法院）
- radio：申请人类型（本人申请 / 为他人或公司等组织申请）
- radio：是否为知识产权立案
- 「下一步」按钮

### 表单可访问性结论（对 OpenCourt 校准重要）
- zxfw 的 a11y 树几乎无标签（generic/textbox 无 name）→ `ExtractFormFields` 基于 AX 的解析在此站不可用
- 可靠定位方式：DOM 文本定位（`querySelectorAll('*')` 精确文本）+ `input.uni-input-input` + 邻近文本标签；后续映射层应基于「字段标签文本 → input 索引」而非 ref/AX name

## 7. 待续（需在新标签内登录后）

- [x] 申请立案入口与类型清单（已实测，§6）
- [ ] 步骤 2-6 表单字段清单（阅读须知/案由树/材料清单/案件信息大表单/预览）
- [ ] 「我的立案」列表状态机
- [ ] 各步表单元素 → 喂给映射层校准（DOM 文本定位方案）

