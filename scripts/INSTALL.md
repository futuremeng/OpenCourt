# 安装说明（OpenCourt + 法条数据）

本磁盘镜像包含 **OpenCourt.app** 与 **law-data/**（离线法条语料库，约 3.3G）。

## 安装（两步）

1. 把本卷中的 **OpenCourt.app 和 law-data 两个项目一起**拖到本机任意同一级目录
   （例如「应用程序」文件夹，或「桌面/OpenCourt」文件夹）。
   也可以不拷贝，直接在本卷中双击 OpenCourt。
2. 双击 **OpenCourt.app** 首次启动：应用会自动把 law-data 安装到本机
   （`~/Library/Application Support/OpenCourt/law-data/`），约需 1~2 分钟，
   窗口出现「法条数据安装」日志输出即在进行中，完成后即可使用。

> macOS 提示「无法验证开发者」：右键 app →「打开」→「打开」。
> （正式对外分发版会完成签名与公证，无此提示。）

## 已装过 OpenCourt 的机器

数据只安装一次；再次启动直接复用本机数据。想更换语料库（如更新后的法条数据），
把新的 law-data 与本卷同样操作前，先删除本机
`~/Library/Application Support/OpenCourt/law-data/`。

## 高级配置（可跳过）

无需任何配置即可使用。如需自定义（模型端点、指向已有语料库路径等），
编辑 `~/Library/Application Support/OpenCourt/settings.yaml`，
参考应用内文档或 deploy/README.md。
