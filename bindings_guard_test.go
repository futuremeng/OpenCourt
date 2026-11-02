package main

// 绑定漂移守卫：Wails 的前端绑定（frontend/wailsjs/go/main/App.d.ts）由
// `wails build`/`wails dev` 生成，容易在增删 App 方法后忘记重生成——前端调用
// 一个不存在的绑定只会在运行时炸。此测试用 go/ast 直接比对
// 「Go 侧 *App 导出方法」与「App.d.ts 里的 export function」，漂移即 CI 失败。

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestAppBindingsMatchFrontendDTS(t *testing.T) {
	goMethods, err := exportedAppMethods(".")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join("frontend", "wailsjs", "go", "main", "App.d.ts"))
	if err != nil {
		t.Fatalf("读取前端绑定失败（wails 生成物缺失？）: %v", err)
	}
	missingInTS, staleInTS := diffBindings(goMethods, string(data))
	if len(missingInTS) > 0 || len(staleInTS) > 0 {
		t.Errorf("前端绑定与 Go 方法漂移，需重跑 `wails build`（或 wails dev）重生成 frontend/wailsjs：\n"+
			"  Go 有但 App.d.ts 缺失: %v\n  App.d.ts 有但 Go 已无: %v", missingInTS, staleInTS)
	}
	if len(goMethods) < 20 {
		t.Errorf("导出的 App 方法数异常偏少（%d），解析可能失效", len(goMethods))
	}
}

// diffBindings 比对 Go 方法集合与 App.d.ts 文本，返回（前端缺失, 前端多余）。
func diffBindings(goMethods map[string]bool, dts string) (missingInTS, staleInTS []string) {
	tsMethods := map[string]bool{}
	for _, m := range regexp.MustCompile(`export function (\w+)\(`).FindAllStringSubmatch(dts, -1) {
		tsMethods[m[1]] = true
	}
	for name := range goMethods {
		if !tsMethods[name] {
			missingInTS = append(missingInTS, name)
		}
	}
	for name := range tsMethods {
		if !goMethods[name] {
			staleInTS = append(staleInTS, name)
		}
	}
	sort.Strings(missingInTS)
	sort.Strings(staleInTS)
	return missingInTS, staleInTS
}

// TestDiffBindingsDetectsDrift 守卫自检：漏生成/多余绑定都必须被检出，
// 否则守卫会在真正的漂移上静默放行。
func TestDiffBindingsDetectsDrift(t *testing.T) {
	goMethods := map[string]bool{"ListFacts": true, "CreateCase": true}
	dts := "export function ListFacts():Promise<Array<any>>;\n" +
		"export function RemovedGoMethod():Promise<void>;\n"
	missing, stale := diffBindings(goMethods, dts)
	if len(missing) != 1 || missing[0] != "CreateCase" {
		t.Errorf("应检出 Go 有而前端缺失的 CreateCase，实际 %v", missing)
	}
	if len(stale) != 1 || stale[0] != "RemovedGoMethod" {
		t.Errorf("应检出前端多余的 RemovedGoMethod，实际 %v", stale)
	}
}

// exportedAppMethods 解析 dir 下 package main 的非测试文件，返回接收者为 *App
// 的导出方法集合。
func exportedAppMethods(dir string) (map[string]bool, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Recv == nil || !fd.Name.IsExported() {
					continue
				}
				if !isAppReceiver(fd.Recv.List[0].Type) {
					continue
				}
				out[fd.Name.Name] = true
			}
		}
	}
	return out, nil
}

// isAppReceiver 判定 *(t *App) 形式的接收者（含写全的 *main.App）。
func isAppReceiver(e ast.Expr) bool {
	star, ok := e.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch t := star.X.(type) {
	case *ast.Ident:
		return t.Name == "App"
	case *ast.SelectorExpr:
		return t.Sel.Name == "App"
	}
	return false
}
