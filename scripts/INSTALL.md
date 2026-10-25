# 安装说明（OpenCourt + 法条语料）

需要下载两个文件（分开下载，升级程序不必重下语料）：

| 文件 | 大小 | 说明 |
|---|---|---|
| `OpenCourt-<版本>-darwin-<架构>.dmg` | ~15MB | 应用本体 |
| `lawdata-<日期>.zip` | ~3G | 离线法条语料库（含防篡改签名清单） |

## 安装（三步）

1. 打开 dmg，把 **OpenCourt.app 拖到「应用程序」**（或任意位置）。
2. 双击 **lawdata-<日期>.zip** 解压（默认解压到「下载」文件夹，得到 `law-data` 文件夹，**放那里不用动**）。
3. 双击 **OpenCourt.app** 启动：首次会自动发现下载文件夹里的语料并安装到本机
   （约 1~2 分钟，有进度日志），完成后即可使用，**无需任何配置**。

> macOS 提示「无法验证开发者」：右键 app →「打开」→「打开」。
> （正式对外分发版会完成签名与公证，无此提示。）

## 校验下载完整性（可选）

发布页公示 `lawdata-<日期>.zip.sha256`。终端执行：
```bash
shasum -a 256 -c lawdata-<日期>.zip.sha256
```
语料内部另带 Ed25519 签名清单，应用读取时会持续校验——任何渠道
（网盘/镜像/断点续传）获得的语料被篡改都会被发现并如实标注。

## 已装过语料的机器

语料只安装一次；程序升级（新 dmg）直接替换 app 即可，语料不动。
想更换新版语料：删除本机
`~/Library/Application Support/OpenCourt/law-data/`，再下载新 lawdata zip 重复上面步骤。

## 高级配置（可跳过）

无需任何配置即可使用。如需指向已有语料库路径、配置模型端点等，
编辑 `~/Library/Application Support/OpenCourt/settings.yaml`
（参考 deploy/README.md）。
