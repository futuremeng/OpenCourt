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
