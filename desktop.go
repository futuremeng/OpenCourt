// 桌面集成（macOS HIG 对齐）：菜单栏、单实例锁、拖放导入、窗口状态持久化。
// 与 main.go 同包（Wails 绑定方法必须在 main 包）。
//
// 菜单项 → 前端事件约定：菜单是快捷键的唯一权威入口（原生 key equivalent
// 先于 WebView 拦截），点击/快捷键统一经 EventsEmit("menu:*") 交给前端消费，
// 前端在 App.vue 里集中订阅——菜单与 UI 永远指向同一动作（HIG Rule 1.3）。
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// menuEvent 生成把菜单动作转发给前端的回调。
func (a *App) menuEvent(name string) menu.Callback {
	return func(*menu.CallbackData) {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, name)
		}
	}
}

// buildMenu 标准菜单栏：App / 文件 / 编辑 / 检索 / 显示 / 窗口 / 帮助
// （HIG Rule 1.1；应用专属「检索」菜单置于编辑与显示之间）。
func buildMenu(app *App) *menu.Menu {
	m := menu.NewMenu()

	// App 菜单用原生角色（macOS 自动给出 关于/服务/隐藏 Cmd+H/退出 Cmd+Q；
	// 关于面板读取 wails.json info，即系统标准 About 面板）。Wails v2.17 的
	// Hide/Quit 单项角色未导出，故设置项落在「文件」菜单（Cmd+, 快捷键不变）。
	m.Append(menu.AppMenu())

	fileMenu := m.AddSubmenu("文件")
	fileMenu.AddText("设置…", keys.CmdOrCtrl(","), app.menuEvent("menu:settings"))
	fileMenu.AddSeparator()
	fileMenu.AddText("新建案件…", keys.CmdOrCtrl("n"), app.menuEvent("menu:new-case"))
	fileMenu.AddText("导入卷宗文档…", keys.CmdOrCtrl("o"), app.menuEvent("menu:import-doc"))
	fileMenu.AddSeparator()
	fileMenu.AddText("打开数据目录", nil, func(*menu.CallbackData) { app.OpenDataDir() })

	m.Append(menu.EditMenu()) // 原生 Undo/Cut/Copy/Paste/Select All（Cmd+Z 安全网）

	searchMenu := m.AddSubmenu("检索")
	searchMenu.AddText("法条检索", keys.CmdOrCtrl("k"), app.menuEvent("menu:law-search"))
	searchMenu.AddText("卷宗检索", keys.Combo("K", keys.CmdOrCtrlKey, keys.ShiftKey), app.menuEvent("menu:case-search"))

	viewMenu := m.AddSubmenu("显示")
	viewMenu.AddText("缩放窗口", keys.Combo("F", keys.CmdOrCtrlKey, keys.ControlKey), func(*menu.CallbackData) {
		wailsRuntime.WindowToggleMaximise(app.ctx)
	})

	m.Append(menu.WindowMenu()) // 原生 最小化 Cmd+M / 缩放

	helpMenu := m.AddSubmenu("帮助")
	helpMenu.AddText("项目主页与文档", nil, func(*menu.CallbackData) {
		wailsRuntime.BrowserOpenURL(app.ctx, "https://github.com/futuremeng/OpenCourt")
	})
	return m
}

// OpenDataDir 在系统文件管理器中打开应用数据目录（设置面板与「文件」菜单共用）。
func (a *App) OpenDataDir() {
	if a.dataDir != "" {
		wailsRuntime.BrowserOpenURL(a.ctx, "file://"+a.dataDir)
	}
}

// onSecondInstance 单实例锁：第二次启动不再开新进程（两份实例会争抢同一
// SQLite/state.json）——唤起既有窗口并置于前台。回调只在既有实例运行期
// 经 IPC 触发，此时 startup 必已完成，a.ctx 已就绪。
func (a *App) onSecondInstance(options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	wailsRuntime.WindowUnminimise(a.ctx)
	wailsRuntime.WindowShow(a.ctx)
	wailsRuntime.EventsEmit(a.ctx, "app:second-instance")
}

// onFileDrop 拖放导入（HIG：用户能看到文件，就期望能拖进来）。
// 拖入的文件逐个走与「导入卷宗文档」完全相同的 IngestDocument 管道
// （OCR 门禁/审计/五态封顶一个不少），结果经 docs:ingest 事件回前端呈现。
func (a *App) onFileDrop(x, y int, paths []string) {
	for _, p := range paths {
		res := map[string]any{"path": p}
		doc, err := a.IngestDocument(p)
		if err != nil {
			res["ok"] = false
			res["error"] = err.Error()
		} else {
			res["ok"] = true
			res["file"] = doc.File
			res["pages"] = doc.PageCount
			res["ocr_engine"] = doc.OCREngine
		}
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "docs:ingest", res)
		}
	}
}

// --- 窗口状态持久化（macOS 惯例：重启后回到上次的位置/大小/缩放态） ---

// 与 main.go 中 options.App 的 MinWidth/MinHeight 保持一致。
const (
	minWindowWidth  = 960
	minWindowHeight = 600
)

type windowState struct {
	X, Y, W, H int
	Maximised  bool
}

func (a *App) windowStatePath() string { return filepath.Join(a.dataDir, "window.json") }

// restoreWindow 恢复上次窗口几何（OnDomReady：窗口已创建，可安全调整）。
// 无记录则保持 wails.json 默认（1280×800 居中）。
func (a *App) restoreWindow(ctx context.Context) {
	b, err := os.ReadFile(a.windowStatePath())
	if err != nil {
		return
	}
	var st windowState
	if json.Unmarshal(b, &st) != nil {
		return
	}
	if st.W >= minWindowWidth && st.H >= minWindowHeight {
		wailsRuntime.WindowSetSize(a.ctx, st.W, st.H)
	}
	if st.X > 0 || st.Y > 0 {
		wailsRuntime.WindowSetPosition(a.ctx, st.X, st.Y)
	}
	if st.Maximised {
		wailsRuntime.WindowMaximise(a.ctx)
	}
}

// saveWindow 记录当前几何（OnShutdown）。最大化时仍记录当前框架值并带
// Maximised 标志，恢复时先复位再最大化，行为与 AppKit 状态恢复一致。
func (a *App) saveWindow() {
	if a.ctx == nil {
		return
	}
	st := windowState{}
	st.W, st.H = wailsRuntime.WindowGetSize(a.ctx)
	st.X, st.Y = wailsRuntime.WindowGetPosition(a.ctx)
	st.Maximised = wailsRuntime.WindowIsMaximised(a.ctx)
	if b, err := json.MarshalIndent(st, "", "  "); err == nil {
		_ = os.WriteFile(a.windowStatePath(), b, 0o600)
	}
}
