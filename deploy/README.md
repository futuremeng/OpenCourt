# 部署

## 桌面端（主分发形态）

```bash
make build          # 当前平台安装包
make build-all      # darwin universal / linux / windows
```

产物位于 `build/bin/`：

- macOS：`OpenCourt.app`（配合 `wails build -clean` 后可用 `create-dmg` 打包 .dmg）
- Windows：`.exe` / `.msi`（需在 Windows 主机或 CI 中构建）
- Linux：ELF 单文件 / `.AppImage`

签名与公证（macOS）：`wails build -codesign "Developer ID: ..."`，发布版应完成 notarization。

## 配置与法条数据（桌面分发）

配置加载顺序（`loadSettings`）：

1. `<应用数据目录>/settings.yaml`——正式位置（macOS 为
   `~/Library/Application Support/OpenCourt/settings.yaml`），双击 .app 启动也可读
2. `config/settings.yaml`（相对工作目录）——开发模式
3. 都没有 → 默认配置（`OPENCOURT_*` 环境变量覆盖始终生效）

首次桌面使用：把 `config/settings.example.yaml` 复制到应用数据目录为
`settings.yaml`，按需填写 `openlaw.data_dir`（指向 OpenLaw 数据目录，
含 `db/ pdf/ docx/ md/ provenance/`，约 3.3G，需单独拷贝分发，不随 .app 走）。
也可不改文件、仅设环境变量 `OPENCOURT_OPENLAW_DATA_DIR`。

立场（profiles）已内置进二进制，双击启动不依赖仓库目录；工作目录下
`profiles/` 存在时优先（允许自定义立场覆盖内置）。

> 开发期 `go.mod` 以 `replace` 指向本地 OpenLaw 仓库；正式分发前给
> OpenLaw 打 tag 并移除 replace。法条详情若显示 `UNVERIFIED`，是
> OpenLaw 数据目录尚未生成 manifest（防篡改闸口），先在 OpenLaw 侧
> 运行 `openlaw-check -build-manifest`。

## 机构服务器模式（可选）

法院/律所等机构如需服务器集中部署（对应开发文档第九章），沿用容器策略：

- 容器默认 `network_mode: "none"`，联网需显式改为 bridge 并配置白名单
- `privileged: false`、`cap_drop: ALL`、`read_only` 根文件系统、仅数据卷可写
- 内存限制 4G、仅监听 `127.0.0.1:8000`（反向代理置于 Nginx 后）
- 审计日志 JSON 输出，保留 ≥180 天；每日增量备份数据目录

> 服务器模式 API 层（HTTP）尚未实现，属阶段二交付物；当前服务器用户可直接运行桌面版（headless 场景待 `cmd/opencourt-cli` 补充）。

## 合规检查清单（发布前必测）

| 检查项 | 方法 | 通过标准 |
|---|---|---|
| 离线验证 | 断网启动应用 | 核心功能正常，无对外请求 |
| 遥测验证 | 抓包出站流量 | 无非白名单出站请求 |
| 溯源测试 | 注入虚假事实 | 被门禁拦截（`tests/compliance` 覆盖） |
| 脱敏测试 | 导入含敏感信息文本 | 敏感字段已处理（`tests/compliance` 覆盖） |
