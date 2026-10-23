## OpenCourt v0.1.0-preview — 技术预览版（Tech Preview / Alpha）

> ⚠️ **面向开发者的技术预览，不是面向法律从业者的产品，请勿用于真实案件。**
> 本版本验证的是「反幻觉、可溯源、代码仲裁」安全内核的可运行性。

### 已可用
- **事实五态模型 + 状态机三硬规则**：证据驱动升级、假设标签贯穿、冲突不自动消解——状态变更由确定性代码仲裁，模型仅提交候选
- **幻觉门禁与引导性检测**：虚假事实引用拦截、「建议起诉 / 大概率胜诉 / 应当调解」等倾向性表述强制拦截、废止法条标红
- **强制脱敏**：姓名一致替换（本地还原表）、证件号/银行账号掩码、地址泛化、案号保留
- **审计日志**：JSONL 哈希链，篡改可检出
- **四套 Profile**（court / plaintiff / defendant / party）与默认拒绝式加载器
- **桌面应用**：数据全本地、默认离线、遥测不存在；实测启动后零出站连接、零监听端口
- **VS Code 一键调试**：桌面应用 Delve 直接启动 / wails dev 自动附加 / Chrome 前端附加 / 全栈组合

### 尚不可用（阶段二+）
- PDF/扫描件/OCR 文档接入、阅卷链路、模型接入真机评测、案件管理、跨部署交换包
- 开发文档 11.1 验收（100 份判决书、幻觉率 < 1%、溯源准确率 > 98%）尚未执行

### 下载与安装
| 平台 | 文件 | 说明 |
|---|---|---|
| macOS (Universal: Apple Silicon + Intel) | `OpenCourt-0.1.0-preview-darwin-universal.dmg` | 自签名未公证：首次打开请右键 → 打开，或 `xattr -dr com.apple.quarantine /Applications/OpenCourt.app` |
| Windows (x64) | `OpenCourt.exe` | CI 构建，未代码签名，SmartScreen 可能警告 → 仍要运行 |
| Linux (x64) | `OpenCourt-linux.zip` | 需系统安装 `libgtk-3` 与 `libwebkit2gtk-4.1`（webkit2_41 构建） |

### 合规自检（已通过）
- 溯源测试：注入虚假事实 → 门禁拦截（`tests/compliance/`）
- 脱敏测试：敏感字段处理后输出（`tests/compliance/`）
- 引导性检测：倾向性表述拦截（`tests/compliance/`）
- 离线验证：启动应用后 `lsof` 无任何监听/出站连接

完整变更见 CHANGELOG.md。免责声明见 DISCLAIMER.md。
