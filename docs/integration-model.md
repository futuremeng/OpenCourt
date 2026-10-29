# 跨项目调用方式设计（三项目独立分发 + 通常一并安装）

> 前提（已明确）：OpenCourt、OpenLaw、LocalCases **各自可独立分发、自带管理界面与客户端**，
> 但用户通常是**一并安装**。本文定案调用方式、语料所有权、发现机制与兼容矩阵。
> 逐项字段契约见 [openlaw-integration.md](openlaw-integration.md) 与
> [localcases-integration.md](localcases-integration.md)。

## 1. 总原则

1. **代码各自分发，语料只有一份。** 集成面是「语料 + 契约」，不是「代码」。
2. **谁生产语料，谁拥有写权。** 消费方一律**只读**打开。
3. **分发 ≠ 编辑。** 消费方可以铺设一份不可变语料包（下载 + sha256 + 首次落盘），
   但不得修改已有语料（不跑迁移、不写库、不改标记）。
4. **版本不匹配一律 fail-closed**：明确报错 + 行动指引，绝不静默兼容、绝不猜。
5. **消费方不启动生产方进程。** 连不上就给指引，不代管生命周期。

## 2. 调用方式定案

| 对接 | 方式 | 理由 | 不采纳的替代 |
|---|---|---|---|
| OpenCourt → OpenLaw | **进程内 SDK**（只读模式） | 130 万条文级检索走 IPC 不划算；OpenCourt 无监听端口的安全模型；编译期已知版本优于运行期协商；两项目同为 Go/Apache-2.0，静态链接无障碍 | OpenLaw 常驻 serve + HTTP：新增端口与鉴权、多一个可挂掉的活动部件，且 OpenLaw 自身定位就是「不默认暴露 HTTP」 |
| OpenCourt → LocalCases | **回环 HTTP + token** | 异构（Rust）唯一合理解；对方本就是独立服务进程；契约以 HTTP 参数为面，版本解耦 | 进程内绑定：跨语言成本过高；OpenCourt 自行拉起 serve：会与用户已开实例争 db/index 锁（源码无单实例保护，行为未验证） |
| OpenCourt → BrowserOS | 回环 MCP（已实现） | 同上「外部可选组件」纪律 | — |

## 3. 语料所有权与位置（一并安装的核心问题）

**问题**：OpenCourt 现在会把语料装到 `~/Library/Application Support/OpenCourt/law-data/`，
而 OpenLaw CLI 用的是自己的 `OpenLaw/data/`。一并安装 = **两份 ~3GB 语料**，
且两边确认状态、版本链各自漂移。

**定案**：

| 项 | 约定 |
|---|---|
| 语料所有者 | **OpenLaw**。导入、人工确认、关系判定、schema 迁移、更新，只发生在 OpenLaw 侧 |
| 共享位置 | `~/.openlaw/data`（OpenLaw 首次初始化时写入；可被 `OPENLAW_DATA_DIR` 覆盖） |
| 位置注册 | OpenLaw 在 `~/.openlaw/location` 写入实际数据目录绝对路径（单行文本），供消费方发现 |
| OpenCourt 发现顺序 | ① 配置 `openlaw.data_dir` → ② `OPENLAW_DATA_DIR` → ③ `~/.openlaw/location` 指向的目录 → ④ `~/.openlaw/data` → ⑤ dmg 卷内 `law-data/` → ⑥ Downloads 中 `law-data*` → ⑦ 未找到 |
| 命中 ①–④ | **直接只读使用，不复制、不安装、不迁移** |
| 命中 ⑤–⑥ | 首次铺设到 `~/.openlaw/data`（幂等；已存在则不动），然后按 ③ 只读使用 |
| 未找到 | 法条功能整体不可用 + 指引（安装 OpenLaw 或下载语料包）；**不再往 OpenCourt app-data 私有副本装** |

> 迁移说明：现有 OpenCourt 用户 app-data 下的 `law-data/` 副本，在升级时**保留不动**，
> 发现顺序里排在共享位置之后；不做自动搬迁（避免误删用户数据），UI 提示可手动清理。

## 4. 只读闸口与版本协商（必须先修）

### 4.1 发现的隐患（源码取证）

| # | 隐患 | 证据 | 后果 |
|---|---|---|---|
| **H1** | OpenCourt 打开语料即**写库** | `OpenLaw/store/db.go: Open()` 无条件 `migrate()`；DSN `file:...journal_mode(WAL)` 读写打开，无 `query_only` | 消费方修改生产者数据 |
| **H2** | schema 版本被**回写降级** | `migrate()` 末尾 `INSERT ... ON CONFLICT DO UPDATE` 把 `meta.schema_version` 写成内嵌 SDK 的 `SchemaVersion`（当前 `"3"`） | 语料结构是 v4、标记声称 v3；新版 OpenLaw CLI 会重跑迁移——数据完整性事故 |
| **H3** | LocalCases token **不可发现** | `desktop/src/main.rs:56` `random_token()` 每次随机、不落盘、不进 env；`config.rs` 无 token 字段；CLI 只读 `LOCALCASES_TOKEN` | 一并安装场景下 OpenCourt 永远连不上 LocalCases，类案功能形同虚设 |

### 4.2 修法（定案）

- **OL-7 sdk 只读入口**：新增 `sdk.OpenReadOnly(dataDir)`——`_pragma=query_only(1)`、
  **不执行 migrate**、不写 `meta`。OpenCourt 必须使用它；`sdk.New` 保留给 OpenLaw 自己的
  写入路径。（不用 `mode=ro`：WAL 库只读打开需要 `-shm/-wal` 可写，易失败；
  `query_only` 按连接生效，兼容 WAL。）
- **OL-8 版本协商**：打开前先读 `meta.schema_version` 与 manifest 的 `producer_version`，
  与 SDK 声明的 `SchemaMin/SchemaMax` 比对。**语料 schema > SDK 支持上限 ⇒ 直接报错**，
  文案给出两条出路（升级 OpenCourt / 换兼容语料包），不迁移、不部分读、不猜。
- **LC-6 发现文件**：LocalCases 启动 serve 时写 `<state dir>/localcases.json`
  （`{addr, token, pid, version, schema, indexed_documents}`，权限 0600，退出时删除）；
  OpenCourt 在 `token_env` 未配置时读取它。CLI 用户仍可用 `LOCALCASES_TOKEN`（两条路都通）。
- **LC-7 health 带版本**：`/api/health` 返回 `version` 与索引 `schema`，OpenCourt 据此
  判定契约兼容，不匹配时报错而非误解析。

## 5. 三种安装形态的行为矩阵

| 安装形态 | 法条来源 | 类案来源 | 语料写入者 | OpenCourt 法条面板 |
|---|---|---|---|---|
| 只装 OpenCourt | 首次铺设语料包到 `~/.openlaw/data`（只读使用） | 不可用（指引） | 仅铺设，不编辑 | 状态 + 首次铺设 + 下载更新指引 |
| OpenCourt + OpenLaw | 共享位置只读使用 | 不可用（指引） | **OpenLaw** | 只读状态显示 + 指引去 OpenLaw 更新 |
| 三个都装 | 共享位置只读使用 | LocalCases 回环（发现文件） | OpenLaw / LocalCases 各自 | 只读状态显示 |

## 6. 管理界面归属（避免双入口打架）

一并安装会出现两个都能「加载/更新语料」的入口，必须切清：

| 界面 | 职责 | 禁止 |
|---|---|---|
| OpenLaw（CLI + 其自身界面） | 语料导入、人工确认、关系判定、schema 迁移、更新包生成 | — |
| OpenCourt「法条数据」面板 | **状态显示**（计数/manifest/许可/producer_version/schema）、首次铺设、指向 OpenLaw 的更新指引 | 修改已有语料、跑迁移、改写确认状态 |
| LocalCases（桌面 + CLI） | 裁判文书导入、索引构建、serve | — |
| OpenCourt「类案」面板 | 检索、引用留痕、脱敏、分布呈现 | 建索引、抓文书、代管 serve 生命周期 |

## 7. 兼容矩阵与失败文案

| 情况 | OpenCourt 行为 |
|---|---|
| 语料 schema > SDK 上限 | 阻断法条功能：「语料 schema v<N> 高于本版本支持范围（≤ v<M>）。请升级 OpenCourt，或使用 OpenLaw 生成兼容语料包」 |
| 语料 schema < SDK 下限 | 阻断并指引：「语料过旧，请用 OpenLaw 升级」——**OpenCourt 不代跑迁移** |
| manifest 缺 `producer_version` | 显示「语料来源版本未知」，功能可用但标注（向后兼容） |
| LocalCases 契约版本不匹配 | 阻断类案功能，显示双方版本，不猜测字段 |
| 发现文件存在但进程已死（pid 不在） | 提示「LocalCases 未在运行」，不尝试启动它 |

## 8. 对 v0.2 计划的影响（据此更正）

| 项 | 原计划 | 更正 |
|---|---|---|
| M2 法条链路 | `NewOpenLawSource(dataDir)`（读写 + 自动迁移） | 改 **`OpenReadOnly`**；打开前先版本协商；不匹配 fail-closed |
| 语料发现 | 只认 `openlaw.data_dir` + app-data 副本 | 按 §3 发现链，**共享位置优先，命中即不复制** |
| 「法条数据」面板 | 状态 + 下载并安装 + 手动加载 | 职责收缩为 §6：状态显示 + 首次铺设 + 更新指引；已有共享语料时不提供写操作 |
| M3 类案接入 | 只支持 `token_env` | 增加**发现文件**读取（`~/.localcases/localcases.json`），`token_env` 作为显式覆盖 |
| M3 前置 | 无 | 依赖 **LC-6/LC-7**；未就绪时降级为「手动填 `base_url` + `token_env`」并在 UI 说明这是临时通道 |
| 新增依赖项 | — | **OL-7 / OL-8 / LC-6 / LC-7** 进入跨项目清单（见各 integration 文档） |

**开工顺序不变**（M1 案件实体不依赖任何跨项目项），但 M2 的插件层第一步就是
`OpenReadOnly` + 版本协商，而不是直接接检索绑定。

## 9. 一句话总结

进程内 SDK 与回环 HTTP 两种调用方式**都保留且合理**——需要修的不是调用形态，
而是**所有权**：语料只有一份、生产者独占写权、消费者只读且版本不匹配就拒绝工作。

