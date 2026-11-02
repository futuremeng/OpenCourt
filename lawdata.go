// 本文件由 main.go 按领域拆分而来（同包 main：Wails 绑定方法必须
// 留在 main 包，这里只是物理分文件，便于按领域阅读与评审）。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/futuremeng/OpenLaw/pack"
	"github.com/futuremeng/OpenLaw/store"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/futuremeng/OpenCourt/core/lawinstall"
	"github.com/futuremeng/OpenCourt/plugins"
)

// openLawSource 按当前生效的语料目录打开法条源（启动时与数据包加载后复用）。
// 打开失败不阻断（法条检索不可用 + 审计留痕），其余功能不受影响。
func (a *App) openLawSource() {
	a.closeLawSource() // 先释放旧句柄（SQLite 文件锁），再开新的
	src, err := plugins.NewOpenLawSource(a.lawDataDir())
	if err != nil {
		log.Printf("OpenLaw 法条源初始化失败（法条检索不可用）: %v", err)
		_ = a.auditLog.Log(a.settings.Profile.Active, "LAW_SOURCE_FAIL", "openlaw", err.Error())
	}
	a.setLawSource(src)
	if src != nil {
		_ = a.auditLog.Log(a.settings.Profile.Active, "LAW_SOURCE_OK", "openlaw", src.Name())
	}
}

// closeLawSource 关闭法条源句柄（替换语料前必须释放 SQLite 文件锁）。
// 取写锁：withLaw 的读者持 RLock 跨整个调用，故拿到写锁即无人正在查询。
func (a *App) closeLawSource() {
	a.mu.Lock()
	src := a.lawSource
	a.lawSource = nil
	a.mu.Unlock()
	if src == nil {
		return
	}
	if c, ok := src.(interface{ Close() error }); ok {
		_ = c.Close()
	}
}

// ensureLawData 把发现的语料源（分发包内 / 下载目录解压）安装到本机应用
// 数据目录并返回应使用的路径；用户已配置有效路径 → 原样返回不动。
// ensureLawData 把发现的语料源（分发包内 / 下载目录解压）安装到本机应用
// 数据目录并返回应使用的路径；已有生效路径且存在 → 原样返回不动。
func (a *App) ensureLawData(source string) string {
	if cur := a.lawDataDir(); cur != "" && dirExists(cur) {
		return cur
	}
	target := filepath.Join(a.dataDir, "law-data")
	if err := lawinstall.Install(source, target, func(s string) {
		log.Printf("法条数据安装: %s", s)
	}); err != nil {
		log.Printf("法条数据安装失败: %v", err)
		_ = a.auditLog.Log(a.settings.Profile.Active, "LAW_DATA_INSTALL_FAIL", "lawinstall", err.Error())
		return a.lawDataDir()
	}
	_ = a.auditLog.Log(a.settings.Profile.Active, "LAW_DATA_INSTALLED", "lawinstall", target)
	return target
}

// dirExists 目录存在判据（配置路径有效性检查）。
func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// --- 法条数据管理（数据包加载与更新，前端「法条数据」面板） ---

// LawDataStatus 语料概况：是否已装、文档/条文计数、证据层与签名 manifest
// 状态。前端据此在首屏提示「加载数据包」或「加载更新包」。
func (a *App) LawDataStatus() map[string]any {
	st := lawinstall.Status(a.lawDataDir())
	return map[string]any{
		"installed":       st.Installed,
		"data_dir":        st.DataDir,
		"docs":            st.Docs,
		"articles":        st.Articles,
		"evidence_ok":     st.EvidenceOK,
		"manifest_signed": st.ManifestSigned,
	}
}

// PickLawDataPackage 弹出文件选择框：语料/更新包 zip。
func (a *App) PickLawDataPackage() (string, error) {
	return a.pickLawDataPath("选择法条数据包（core/evidence/更新包 zip）", "*.zip")
}

// PickLawDataDir 弹出目录选择框：已解压的数据目录。
func (a *App) PickLawDataDir() (string, error) {
	return a.pickLawDataPath("选择法条数据目录（已解压的 law-data / law-evidence / 更新包）", "")
}

func (a *App) pickLawDataPath(title, pattern string) (string, error) {
	if pattern == "" {
		return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{Title: title})
	}
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:   title,
		Filters: []wailsRuntime.FileFilter{{DisplayName: "数据包 (zip)", Pattern: pattern}},
	})
}

// LoadLawData 加载数据包（zip 或目录），自动识别类型并执行：
//   - core / full：首次安装，或全量替换现有语料（更新语义；失败回滚旧语料）
//   - evidence   ：合并证据层（只补缺失，溯源链补全）
//   - update     ：openlaw-pack 增量包——签名/哈希校验通过后逐条导入，
//     校验失败拒绝导入并保留旧数据（6.4 增量包契约）
//
// 全程通过 "lawdata:progress" 事件向前端报告进度；结果审计留痕。
func (a *App) LoadLawData(path string) (map[string]any, error) {
	if path == "" {
		return nil, fmt.Errorf("未选择数据包路径")
	}
	role := a.settings.Profile.Active
	report := func(s string) {
		log.Printf("法条数据: %s", s)
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "lawdata:progress", s)
		}
	}
	fail := func(err error) (map[string]any, error) {
		log.Printf("法条数据加载失败: %v", err)
		_ = a.auditLog.Log(role, "LAW_DATA_LOAD_FAIL", path, err.Error())
		return nil, err
	}

	src, tmp, err := lawinstall.NormalizeSource(path)
	if tmp != "" {
		defer os.RemoveAll(tmp)
	}
	if err != nil {
		return fail(err)
	}
	kind := lawinstall.Classify(src)
	if kind == "" {
		return fail(fmt.Errorf("无法识别的数据包（需为 core/evidence/更新包）: %s", filepath.Base(path)))
	}
	target := a.lawDataDir()
	if target == "" {
		target = filepath.Join(a.dataDir, "law-data")
	}
	report(fmt.Sprintf("识别为 %s 包: %s", kind, filepath.Base(path)))

	switch kind {
	case "core", "full":
		a.closeLawSource() // 释放旧 SQLite 句柄再换目录
		if lawinstall.Installed(target) {
			if err := lawinstall.Replace(src, target, report); err != nil {
				return fail(err)
			}
			_ = a.auditLog.Log(role, "LAW_DATA_REPLACED", target, path)
		} else {
			if err := lawinstall.Install(src, target, report); err != nil {
				return fail(err)
			}
			_ = a.auditLog.Log(role, "LAW_DATA_INSTALLED", target, path)
		}
	case "evidence":
		if !lawinstall.Installed(target) {
			return fail(fmt.Errorf("尚未安装语料：请先加载 core 数据包，再补装证据层"))
		}
		if err := lawinstall.MergeEvidence(src, target, report); err != nil {
			return fail(err)
		}
		_ = a.auditLog.Log(role, "LAW_EVIDENCE_MERGED", target, path)
	case "update":
		if !lawinstall.Installed(target) {
			return fail(fmt.Errorf("尚未安装语料：增量更新包需要基础语料先就位"))
		}
		a.closeLawSource() // 导入前释放句柄，避免 SQLite 写冲突
		db, err := store.Open(filepath.Join(target, "db", "openlaw.db"))
		if err != nil {
			return fail(fmt.Errorf("打开语料库失败: %w", err))
		}
		res, err := pack.Import(db, target, src)
		db.Close()
		if err != nil {
			return fail(err)
		}
		_ = a.auditLog.Log(role, "LAW_PACK_IMPORTED", target, fmt.Sprintf("%d 条文档", len(res)))
		report(fmt.Sprintf("增量导入完成：新增 %d 条文档", len(res)))
	}

	a.setLawDataDir(target)
	if err := a.saveState(); err != nil {
		log.Printf("语料目录持久化失败（本次生效、重启后按配置重新发现）: %v", err)
	}
	a.openLawSource() // 语料已变，重开法条源
	return a.LawDataStatus(), nil
}

// CheckLawDataUpdates 拉取发布索引并与本机语料比对，返回可下载包列表：
// action = install（本机缺）/ update（有更新版本）/ optional（可重装）/
// skip（不适用，如未装 core 时的 evidence）。前端据此渲染勾选项。
func (a *App) CheckLawDataUpdates() map[string]any {
	indexURL := a.settings.OpenLaw.IndexURL
	if indexURL == "" {
		return map[string]any{"ok": false, "error": "未配置数据包索引（openlaw.index_url）"}
	}
	ctx := context.Background()
	if a.ctx != nil {
		ctx = a.ctx
	}
	idx, err := lawinstall.FetchIndex(ctx, indexURL)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	cur := lawinstall.Status(a.lawDataDir())
	hasNewer := func(date string) bool {
		return cur.CorpusDate != "" && date > cur.CorpusDate
	}
	pkgs := make([]map[string]any, 0, len(idx.Packages))
	for _, p := range idx.Packages {
		action := "optional"
		switch p.Kind {
		case "core", "full":
			if !cur.Installed {
				action = "install"
			} else if hasNewer(p.Date) {
				action = "update"
			}
		case "evidence":
			switch {
			case !cur.Installed:
				action = "skip"
			case !cur.EvidenceOK:
				action = "install"
			case hasNewer(p.Date):
				action = "update"
			}
		case "update":
			if cur.Installed && hasNewer(p.Date) {
				action = "install"
			} else {
				action = "skip"
			}
		}
		pkgs = append(pkgs, map[string]any{
			"kind": p.Kind, "name": p.Name, "date": p.Date,
			"size": p.Size, "action": action,
		})
	}
	return map[string]any{
		"ok": true, "generated": idx.GeneratedAt, "packages": pkgs,
		"current": a.LawDataStatus(),
	}
}

// DownloadAndInstallLawData 按名称下载所选包（自动排序：core/full →
// evidence → update），每个包下载后先按索引 sha256 校验完整性，再走
// LoadLawData 完整流水线（内容真实性由包内签名 manifest 独立校验）。
// 成功即清缓存；失败保留已装结果并报错（审计 LAW_DATA_LOAD_FAIL）。
func (a *App) DownloadAndInstallLawData(names []string) (map[string]any, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("未勾选任何数据包")
	}
	indexURL := a.settings.OpenLaw.IndexURL
	if indexURL == "" {
		return nil, fmt.Errorf("未配置数据包索引（openlaw.index_url）")
	}
	ctx := context.Background()
	if a.ctx != nil {
		ctx = a.ctx
	}
	idx, err := lawinstall.FetchIndex(ctx, indexURL)
	if err != nil {
		return nil, err
	}
	byName := map[string]lawinstall.PackageInfo{}
	for _, p := range idx.Packages {
		byName[p.Name] = p
	}
	order := map[string]int{"core": 0, "full": 0, "evidence": 1, "update": 2}
	sel := make([]lawinstall.PackageInfo, 0, len(names))
	for _, n := range names {
		p, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("所选包不在发布索引中: %s", n)
		}
		sel = append(sel, p)
	}
	sort.SliceStable(sel, func(i, j int) bool { return order[sel[i].Kind] < order[sel[j].Kind] })

	cache := filepath.Join(a.dataDir, "lawdata-cache")
	emit := func(s string) {
		log.Printf("法条数据: %s", s)
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "lawdata:progress", s)
		}
	}
	for _, p := range sel {
		url := lawinstall.PackageURL(idx, p, indexURL)
		if url == "" {
			return nil, fmt.Errorf("包 %s 无法解析下载地址", p.Name)
		}
		dest := filepath.Join(cache, p.Name)
		emit(fmt.Sprintf("下载 %s（%s）…", p.Name, humanBytes(p.Size)))
		err := lawinstall.DownloadFile(ctx, url, dest, p.SHA256, func(done, total int64) {
			if total > 0 {
				emit(fmt.Sprintf("下载 %s：%d%%（%s / %s）",
					p.Name, done*100/total, humanBytes(done), humanBytes(total)))
			} else {
				emit(fmt.Sprintf("下载 %s：%s…", p.Name, humanBytes(done)))
			}
		})
		if err != nil {
			return nil, err
		}
		_ = a.auditLog.Log(a.settings.Profile.Active, "LAW_DATA_DOWNLOADED", p.Name, "sha256 校验通过")
		if _, err := a.LoadLawData(dest); err != nil {
			return nil, err
		}
		_ = os.Remove(dest) // 安装成功即清缓存
	}
	return a.LawDataStatus(), nil
}

// humanBytes 人类可读字节数。
func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// --- 程序自身更新（GitHub Releases，macOS 自动替换重启） ---
