// Package lawinstall 桌面分发的法条数据安装（OpenLaw 数据目录随 dmg 打包）。
// 分发形态：dmg 卷内 OpenCourt.app 旁放置 law-data/（已签名 manifest 的
// OpenLaw 数据目录）。首次启动时一次性拷贝到本机应用数据目录——SQLite 需要
// 本地读写（WAL），只读卷上无法直接运行。拷贝幂等：目标已存在即复用。
package lawinstall

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FindBundled 从可执行文件位置推导 dmg 卷内的 law-data 目录。
// 布局：<vol>/OpenCourt.app/Contents/MacOS/OpenCourt 与 <vol>/law-data/。
// 找不到（开发模式运行、或用户只拷了 .app）返回 ""。
func FindBundled(exePath string) string {
	// exe → MacOS → Contents → OpenCourt.app → 卷根
	vol := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(exePath))))
	candidate := filepath.Join(vol, "law-data")
	if isLawData(candidate) {
		return candidate
	}
	// 备用布局：law-data 与 OpenCourt.app 同级放在任意目录（用户整卷拷贝到本地）
	appDir := filepath.Dir(filepath.Dir(filepath.Dir(exePath))) // <parent>/OpenCourt.app
	candidate = filepath.Join(appDir, "law-data")
	if isLawData(candidate) {
		return candidate
	}
	return ""
}

// FindDownloaded 语料独立包的自动发现：用户下载 lawdata-*.zip 双击解压
// （默认落 Downloads）后，启动时自动识别。匹配 Downloads 直接子目录中
// 名字以 law-data 开头的合法数据目录（归档工具重名会加 " 2" 后缀）。
func FindDownloaded(downloadsDir string) string {
	entries, err := os.ReadDir(downloadsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "law-data") {
			continue
		}
		if p := filepath.Join(downloadsDir, e.Name()); isLawData(p) {
			return p
		}
	}
	return ""
}

// isLawData OpenLaw 数据目录判据：db/openlaw.db 存在（防误把普通目录当语料）。
func isLawData(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "db", "openlaw.db"))
	return err == nil && !st.IsDir()
}

// Install 把 bundledDir 数据目录安装到 targetDir（一次性、幂等）。
// targetDir 已含合法数据目录 → 直接返回 targetDir 不重拷。
// progress 回调报告阶段（可 nil）。
func Install(bundledDir, targetDir string, progress func(string)) error {
	report := func(s string) {
		if progress != nil {
			progress(s)
		}
	}
	if isLawData(targetDir) {
		report("本机已有法条数据，跳过安装")
		return nil
	}
	if !isLawData(bundledDir) {
		return fmt.Errorf("分发包内未找到 law-data（缺 db/openlaw.db）: %s", bundledDir)
	}
	tmp := targetDir + ".installing"
	_ = os.RemoveAll(tmp)
	report("开始安装法条数据（首次，约数分钟）…")
	if err := copyTree(bundledDir, tmp, report); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("法条数据安装失败（保留现场不覆盖）: %w", err)
	}
	if err := os.Rename(tmp, targetDir); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("法条数据落位失败: %w", err)
	}
	report("法条数据安装完成")
	return nil
}

// copyTree 递归复制（文件级：数据目录为静态快照，无需并发）。
// 跳过 logs/（运行产物不随包分发）。
func copyTree(src, dst string, report func(string)) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() == "logs" {
			continue
		}
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyTree(s, d, report); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d); err != nil {
			return err
		}
		report("复制 " + e.Name() + "/…")
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, st.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
