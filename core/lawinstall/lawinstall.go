// Package lawinstall 桌面分发的法条数据安装（OpenLaw 数据目录随 dmg 打包）。
// 分发形态：dmg 卷内 OpenCourt.app 旁放置 law-data/（已签名 manifest 的
// OpenLaw 数据目录）。首次启动时一次性拷贝到本机应用数据目录——SQLite 需要
// 本地读写（WAL），只读卷上无法直接运行。拷贝幂等：目标已存在即复用。
//
// 除自动发现外，还提供显式数据包加载（前端「法条数据」面板）：
// Classify 识别 core/full/evidence/update 四类包，NormalizeSource 处理
// zip 解压与根目录归一，Replace 全量替换语料（更新语义），Status 汇总
// 语料概况供 UI 展示与升级提示。
package lawinstall

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/futuremeng/OpenLaw/store"
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

// FindEvidence 证据层包（law-evidence-*.zip 解压产物）的发现：Downloads
// 直接子目录中名字以 law-evidence 开头、含 docx/ md/ provenance/ 之一者。
func FindEvidence(downloadsDir string) string {
	entries, err := os.ReadDir(downloadsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "law-evidence") {
			continue
		}
		p := filepath.Join(downloadsDir, e.Name())
		for _, layer := range []string{"docx", "md", "provenance"} {
			if st, err := os.Stat(filepath.Join(p, layer)); err == nil && st.IsDir() {
				return p
			}
		}
	}
	return ""
}

// MergeEvidence 把证据层包内容（docx/ md/ provenance/）合并进已安装数据
// 目录：仅补缺失文件（同名跳过——manifest 哈希一致性由读取链路持续校验），
// 幂等可重跑。合并后 GetArticle 断链解除（前提是文件与 manifest 一致）。
func MergeEvidence(evidenceDir, dataDir string, progress func(string)) error {
	if !isLawData(dataDir) {
		return fmt.Errorf("目标数据目录不合法: %s", dataDir)
	}
	report := func(s string) {
		if progress != nil {
			progress(s)
		}
	}
	merged := 0
	for _, layer := range []string{"docx", "md", "provenance"} {
		src := filepath.Join(evidenceDir, layer)
		if st, err := os.Stat(src); err != nil || !st.IsDir() {
			continue
		}
		n, err := mergeTree(src, filepath.Join(dataDir, layer))
		if err != nil {
			return fmt.Errorf("证据层合并失败（%s）: %w", layer, err)
		}
		merged += n
		report(fmt.Sprintf("合并 %s/：%d 个文件", layer, n))
	}
	if merged == 0 {
		report("证据层已是最新，无需合并")
	} else {
		report(fmt.Sprintf("证据层合并完成（新增 %d 个文件）", merged))
	}
	return nil
}

// mergeTree 递归合并复制：已存在的文件跳过（不覆盖本机数据）。
func mergeTree(src, dst string) (int, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if e.IsDir() {
			m, err := mergeTree(s, d)
			n += m
			if err != nil {
				return n, err
			}
			continue
		}
		if _, err := os.Stat(d); err == nil {
			continue // 已有文件不覆盖
		}
		if err := copyFile(s, d); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// isLawData OpenLaw 数据目录判据：db/openlaw.db 存在（防误把普通目录当语料）。
func isLawData(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "db", "openlaw.db"))
	return err == nil && !st.IsDir()
}

// dirExists 目录存在判据。
func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
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

// --- 显式数据包加载（前端「法条数据」面板） ---

// Installed 本机是否已安装法条语料（dataDir 为合法 OpenLaw 数据目录）。
func Installed(dir string) bool { return isLawData(dir) }

// Classify 识别数据包目录类型（NormalizeSource 归一后调用）：
//   - "core"     含 db/openlaw.db（检索库 + 签名 manifest）
//   - "full"     core + 证据层（单包全量）
//   - "evidence" 仅 docx/ md/ provenance/（证据层包）
//   - "update"   含 changes.csv（openlaw-pack 增量更新包）
//   - ""         无法识别
func Classify(dir string) string {
	hasDB := isLawData(dir)
	hasEvidence := false
	for _, layer := range []string{"docx", "md", "provenance"} {
		if dirHasFiles(filepath.Join(dir, layer)) {
			hasEvidence = true
			break
		}
	}
	if hasDB {
		if hasEvidence {
			return "full"
		}
		return "core"
	}
	if _, err := os.Stat(filepath.Join(dir, "changes.csv")); err == nil {
		return "update"
	}
	if hasEvidence {
		return "evidence"
	}
	return ""
}

// NormalizeSource 归一化用户选择的数据包路径：zip → 解压到临时目录（带
// zip-slip 防护）并进入唯一根目录（忽略 __MACOSX 等归档杂项）。
// 返回（实际目录, 需清理的临时目录或 "", error）。
func NormalizeSource(path string) (string, string, error) {
	if !strings.HasSuffix(strings.ToLower(path), ".zip") {
		if !dirExists(path) {
			return "", "", fmt.Errorf("路径不存在或不是目录: %s", path)
		}
		return path, "", nil
	}
	tmp, err := os.MkdirTemp("", "opencourt-lawdata-")
	if err != nil {
		return "", "", err
	}
	if err := ExtractZip(path, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", "", err
	}
	return descendSingleDir(tmp), tmp, nil
}

// descendSingleDir 解压产物若只有一个业务根目录（打包脚本约定 law-data/
// 或 law-evidence/ 为根），进入该层；macOS 归档杂项（__MACOSX、._*）忽略。
func descendSingleDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir
	}
	var only string
	n := 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "__MACOSX") {
			continue
		}
		only = filepath.Join(dir, e.Name())
		n++
	}
	if n == 1 {
		return only
	}
	return dir
}

// ExtractZip 解压 zip 到 destDir。zip-slip 防护：拒绝 .. / 绝对路径条目。
func ExtractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer r.Close()
	root := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, f := range r.File {
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || filepath.IsAbs(name) {
			return fmt.Errorf("zip 含非法路径条目，拒绝解压: %s", f.Name)
		}
		target := filepath.Join(destDir, name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), root) {
			return fmt.Errorf("zip 条目越出目标目录，拒绝解压: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		if err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Replace 用 src 数据目录全量替换 targetDir（新版 core/full 包的更新语义）。
// 先整体拷到 .installing，再原子换目录；任一步失败回滚，旧语料原样保留。
// 调用前须确保没有打开的 SQLite 句柄（WAL 文件锁）。
func Replace(src, targetDir string, progress func(string)) error {
	report := func(s string) {
		if progress != nil {
			progress(s)
		}
	}
	if !isLawData(src) {
		return fmt.Errorf("更新源不是合法数据目录（缺 db/openlaw.db）: %s", src)
	}
	tmp := targetDir + ".installing"
	old := targetDir + ".old"
	_ = os.RemoveAll(tmp)
	report("开始替换法条数据（首次拷贝，约数分钟）…")
	if err := copyTree(src, tmp, report); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("法条数据替换失败（旧语料未动）: %w", err)
	}
	existed := dirExists(targetDir)
	if existed {
		_ = os.RemoveAll(old)
		if err := os.Rename(targetDir, old); err != nil {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("旧语料移出失败: %w", err)
		}
	}
	if err := os.Rename(tmp, targetDir); err != nil {
		if existed {
			_ = os.Rename(old, targetDir) // 回滚
		}
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("新语料落位失败（已回滚旧语料）: %w", err)
	}
	_ = os.RemoveAll(old)
	report("法条数据替换完成")
	return nil
}

// DataStatus 语料概况（前端「法条数据」面板展示与升级提示依据）。
type DataStatus struct {
	Installed      bool   `json:"installed"`
	DataDir        string `json:"data_dir"`
	Docs           int64  `json:"docs"`
	Articles       int64  `json:"articles"`
	EvidenceOK     bool   `json:"evidence_ok"`
	ManifestSigned bool   `json:"manifest_signed"`
	CorpusDate     string `json:"corpus_date"` // manifest generated_at（YYYYMMDD 化）
}

// Status 汇总语料概况。db 打不开（损坏/占用的极端情形）不报错，
// 计数为 0——前端如实显示，读取链路的真实错误在检索时暴露。
func Status(dataDir string) DataStatus {
	st := DataStatus{DataDir: dataDir}
	if dataDir == "" || !isLawData(dataDir) {
		return st
	}
	st.Installed = true
	if db, err := store.Open(filepath.Join(dataDir, "db", "openlaw.db")); err == nil {
		_ = db.QueryRow("SELECT COUNT(*) FROM document").Scan(&st.Docs)
		_ = db.QueryRow("SELECT COUNT(*) FROM article").Scan(&st.Articles)
		db.Close()
	}
	for _, layer := range []string{"docx", "md", "provenance"} {
		if dirHasFiles(filepath.Join(dataDir, layer)) {
			st.EvidenceOK = true
			break
		}
	}
	_, err := os.Stat(filepath.Join(dataDir, "manifest", "manifest.sig"))
	st.ManifestSigned = err == nil
	st.CorpusDate = corpusDate(dataDir)
	return st
}

// corpusDate 本地语料版本日期：manifest.json 的 generated_at 取数字部分
// （YYYY-MM-DD…→YYYYMMDD），供与发布索引的包日期比较是否有更新。
func corpusDate(dataDir string) string {
	body, err := os.ReadFile(filepath.Join(dataDir, "manifest", "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		GeneratedAt string `json:"generated_at"`
	}
	if json.Unmarshal(body, &m) != nil {
		return ""
	}
	var digits strings.Builder
	for _, r := range m.GeneratedAt {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
		if len(digits.String()) >= 8 {
			return digits.String()
		}
	}
	return ""
}

// dirHasFiles 目录存在且至少一个条目。
func dirHasFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}
