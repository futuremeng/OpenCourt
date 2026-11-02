// 本文件由 main.go 按领域拆分而来（同包 main：Wails 绑定方法必须
// 留在 main 包，这里只是物理分文件，便于按领域阅读与评审）。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
)

// statePath 应用状态文件（当前案件 + 用户选择的语料目录，重启恢复）。
func (a *App) statePath() string { return filepath.Join(a.dataDir, "state.json") }

// appState state.json 的内容。law_data_dir 为用户显式加载的语料目录
// （修 P2：原先 LoadLawData 只改内存值，重启后回落到自动发现）。
type appState struct {
	CurrentCase string `json:"current_case"`
	LawDataDir  string `json:"law_data_dir,omitempty"`
}

// loadState 读 state.json；缺失/损坏返回零值（启动照常，不阻断）。
func loadState(path string) appState {
	var st appState
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

// writeState 原子落盘 state.json（调用方自备快照，不取锁）。
func (a *App) writeState(st appState) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.statePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.statePath())
}

// saveState 取当前状态快照并落盘（调用方不得持 a.mu）。
func (a *App) saveState() error {
	a.mu.RLock()
	st := appState{CurrentCase: a.currentCaseID, LawDataDir: a.lawDataDirOverride}
	a.mu.RUnlock()
	return a.writeState(st)
}

// validatedCaseID 校验 state.json 里的案件仍然存在（删除/损坏回落默认案件）。
func (a *App) validatedCaseID(id string) string {
	if id == "" {
		return domain.DefaultCaseID
	}
	if _, err := a.store.GetCase(id); err == nil {
		return id
	}
	log.Printf("state.json 中的案件不存在，回落默认案件: %s", id)
	return domain.DefaultCaseID
}

// CreateCase 新建案件并即时切换为当前案件。标题必填；案由为自由文本
// （不自动判定案由）；stage 缺省 CONSULT，非法阶段直接报错。
func (a *App) CreateCase(title, caseType, stage string) (domain.Case, error) {
	if strings.TrimSpace(title) == "" {
		return domain.Case{}, fmt.Errorf("案件标题不能为空")
	}
	st := domain.StageConsult
	if stage != "" {
		st = domain.CaseStage(stage)
		if !st.Valid() {
			return domain.Case{}, fmt.Errorf("非法案件阶段: %q（允许: CONSULT/FILING/FIRST_TRIAL/APPEAL/SECOND_TRIAL）", stage)
		}
	}
	c := domain.Case{
		CaseID: newID("CASE"), Title: strings.TrimSpace(title), CaseType: strings.TrimSpace(caseType),
		Stage: st, Visibility: domain.VisBoth, CreatedAt: time.Now().UTC(),
	}
	if err := a.store.CreateCase(c); err != nil {
		return domain.Case{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "CASE_CREATE", c.CaseID, c.Title)
	if err := a.SwitchCase(c.CaseID); err != nil {
		return c, err
	}
	return c, nil
}

// ListCases 列出全部案件（创建时间倒序）。
func (a *App) ListCases() ([]domain.Case, error) {
	return a.store.ListCases()
}

// SwitchCase 切换当前案件（不存在的案件报错；审计 CASE_SWITCH；
// state.json 持久化——切换失败即回滚，绝不出现内存与磁盘状态分裂）。
// 「读旧值 → 改 → 落盘」在写锁内完成：与并发中的 scope() 读者互斥，
// 不会出现内存已切、磁盘未切（或反之）的撕裂状态。
func (a *App) SwitchCase(caseID string) error {
	c, err := a.store.GetCase(caseID)
	if err != nil {
		return err
	}
	a.mu.Lock()
	if caseID == a.currentCaseID {
		a.mu.Unlock()
		return nil
	}
	prev := a.currentCaseID
	a.currentCaseID = caseID
	if err := a.writeState(appState{CurrentCase: caseID, LawDataDir: a.lawDataDirOverride}); err != nil {
		a.currentCaseID = prev
		a.mu.Unlock()
		return err
	}
	a.mu.Unlock()
	_ = a.auditLog.Log(a.profile.Role, "CASE_SWITCH", caseID, c.Title)
	return nil
}

// CurrentCase 当前活动案件。
func (a *App) CurrentCase() (domain.Case, error) {
	return a.store.GetCase(a.caseID())
}

// --- 法条检索与引用核验（v0.2 M2，plan-v0.2 4.1）---
// 纪律：检索/核验全部经 plugins 适配层走 OpenLaw sdk（禁直连 DB）；核验
// 判定透传 OpenLaw 错误码（OK / OL-1001..OL-1006），不另建同义枚举；
// pending 确认状态原样上浮——OpenCourt 不自动确认、不自行判定效力。
