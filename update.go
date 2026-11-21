// 本文件由 main.go 按领域拆分而来（同包 main：Wails 绑定方法必须
// 留在 main 包，这里只是物理分文件，便于按领域阅读与评审）。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/futuremeng/OpenCourt/core/appupdate"
)

// CheckAppUpdate 检查程序新版本：比较 GitHub Release tag 与内置版本号。
// 返回 available / notes / 包信息，供前端横幅提示。
func (a *App) CheckAppUpdate() map[string]any {
	if a.settings.App.UpdateAPI == "" {
		return map[string]any{"ok": false, "error": "未配置更新源（app.update_api）"}
	}
	ctx := context.Background()
	if a.ctx != nil {
		ctx = a.ctx
	}
	rel, err := appupdate.CheckLatest(ctx, a.settings.App.UpdateAPI)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	avail := appupdate.CompareVersions(rel.TagName, version) > 0
	dmg, sha := appupdate.PickAssets(rel, goosForUpdate(), goarchForUpdate())
	notes := rel.Body
	if len(notes) > 4000 {
		notes = notes[:4000]
	}
	res := map[string]any{
		"ok": true, "current": version, "latest": rel.TagName,
		"available": avail, "notes": notes,
	}
	if dmg != nil {
		res["package"] = dmg.Name
		res["size"] = dmg.Size
		res["has_sha256"] = sha != nil
	}
	return res
}

// InstallAppUpdate 下载并校验最新程序包（sha256 资产强制），随后由分离
// 脚本完成替换并重启新版本。仅支持 macOS 应用包（.app）形态。
func (a *App) InstallAppUpdate() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("自动更新目前仅支持 macOS，请从发布页手动更新")
	}
	if a.settings.App.UpdateAPI == "" {
		return fmt.Errorf("未配置更新源（app.update_api）")
	}
	// 定位自身 .app 包（开发模式 go run 不在 bundle 内 → 拒绝）
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	idx := strings.LastIndex(exe, ".app")
	if idx < 0 {
		return fmt.Errorf("当前不是应用包形态（开发模式），无法自动更新")
	}
	appPath := exe[:idx+4]

	emit := func(s string) {
		log.Printf("程序更新: %s", s)
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "appupdate:progress", s)
		}
	}
	ctx := context.Background()
	if a.ctx != nil {
		ctx = a.ctx
	}
	emit("下载新版本安装包并校验…")
	dmgPath, err := appupdate.DownloadRelease(ctx, a.settings.App.UpdateAPI,
		filepath.Join(a.dataDir, "update-cache"), func(done, total int64) {
			if total > 0 {
				emit(fmt.Sprintf("下载更新包：%d%%（%s / %s）",
					done*100/total, humanBytes(done), humanBytes(total)))
			}
		})
	if err != nil {
		_ = a.auditLog.Log(a.settings.Profile.Active, "APP_UPDATE_FAIL", "updater", err.Error())
		return err
	}
	script := appupdate.UpdaterScript(dmgPath, appPath, os.Getpid(), a.settings.App.AllowUnsigned)
	scriptPath := filepath.Join(a.dataDir, "update-cache", "updater.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return err
	}
	// 审计必须记录来源校验强度：跳过 Gatekeeper 的更新是明确的风险决定。
	gateNote := "gatekeeper=on"
	if a.settings.App.AllowUnsigned {
		gateNote = "gatekeeper=skipped(allow_unsigned)"
	}
	_ = a.auditLog.Log(a.settings.Profile.Active, "APP_UPDATE_APPLYING", "updater",
		fmt.Sprintf("%s %s origin=%s", dmgPath, gateNote, a.settings.App.UpdateAPI))
	emit("安装包校验通过，即将退出并完成替换重启…")
	cmd := exec.Command("/bin/sh", scriptPath)
	detachCommand(cmd) // 脱离应用进程组，应用退出后继续（按平台拆分：Windows 无 Setsid 字段）
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新脚本失败: %w", err)
	}
	if a.ctx != nil {
		wailsRuntime.Quit(a.ctx)
	}
	return nil
}

// goosForUpdate / goarchForUpdate 平台标识（拆出便于测试资产挑选）。
func goosForUpdate() string { return runtime.GOOS }

func goarchForUpdate() string { return runtime.GOARCH }
