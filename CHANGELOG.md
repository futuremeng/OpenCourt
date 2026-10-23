# 更新日志

本项目遵循 Semantic Versioning，版本语义见开发文档 10.4。

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
