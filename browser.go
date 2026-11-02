// 本文件由 main.go 按领域拆分而来（同包 main：Wails 绑定方法必须
// 留在 main 包，这里只是物理分文件，便于按领域阅读与评审）。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/futuremeng/OpenCourt/core/redact"
	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
)

// ResearchWebPage 网络查阅：BrowserOS 抓取 → 强制脱敏 → 「单源未补强」事实
// 入库（Citation 含 URL/抓取时间/快照哈希）。返回文本已脱敏；原始页面内容
// 不回流模型。names 为已知当事人姓名（一致替换用）。
func (a *App) ResearchWebPage(url string, names []string) (neutral.ResearchResult, error) {
	if url == "" {
		return neutral.ResearchResult{}, fmt.Errorf("未提供 URL")
	}
	res, err := neutral.RunResearch(a.ctx, a.browser, names, url)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_RESEARCH_FAIL", url, err.Error())
		return neutral.ResearchResult{}, err
	}
	if err := a.scope().CreateFact(res.Fact); err != nil {
		return neutral.ResearchResult{}, err
	}
	for _, act := range res.RedactActions {
		_ = a.auditLog.Log(a.profile.Role, "REDACT", act.RuleID, act.Strategy)
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_RESEARCH", url,
		fmt.Sprintf("快照哈希 %s；脱敏 %d 项；事实 %s 初始状态 %s（联网源至多单源未补强）",
			res.ContentSHA[:12], len(res.RedactActions), res.Fact.FactID, res.Fact.State.DisplayName()))
	return res, nil
}

// --- 网上立案辅助（两段式，设计见 docs/browseros-integration.md 第 6 节）---

// BrowserOpenPage 在用户 BrowserOS 中打开页面（辅助模式；不抓回整页内容）。
func (a *App) BrowserOpenPage(url string) (plugins.BrowserPage, error) {
	if url == "" {
		return plugins.BrowserPage{}, fmt.Errorf("未提供 URL")
	}
	if a.browser == nil || !a.browser.Enabled() {
		return plugins.BrowserPage{}, fmt.Errorf("浏览器辅助插件未启用：%s", plugins.BrowserOSGuidance)
	}
	page, err := a.browser.Open(a.ctx, url)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_OPEN_FAIL", url, err.Error())
		return plugins.BrowserPage{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_OPEN", url, fmt.Sprintf("page=%d", page.PageID))
	return page, nil
}

// BrowserPlanFill 生成当前页填写计划（候选，不执行）：可交互快照 → 表单解析
// → 值映射 → gate 红线预筛。UI 必须逐条展示全部候选——含被红线拦截项
// （Allowed=false，附原因）与弱态事实字段（FactState 标黄），
// 经用户逐条确认后方可 BrowserExecuteAction。
func (a *App) BrowserPlanFill(pageID int, values map[string]string, factStates map[string]string) ([]plugins.ActionCandidate, error) {
	cands, elements, err := neutral.PlanPage(a.ctx, a.browser, pageID, values, factStates)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_PLAN_FAIL", fmt.Sprintf("page=%d", pageID), err.Error())
		return nil, err
	}
	blocked := 0
	for _, c := range cands {
		if !c.Allowed {
			blocked++
			_ = a.auditLog.Log(a.profile.Role, "BROWSER_ACTION_BLOCKED", c.ActionID,
				fmt.Sprintf("%s: %s", c.Rule, c.Reason))
		}
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_PLAN", fmt.Sprintf("page=%d", pageID),
		fmt.Sprintf("元素 %d 个；候选 %d 项；红线拦截 %d 项", len(elements), len(cands), blocked))
	return cands, nil
}

// BrowserExecuteAction 执行单个「已经 Propose 且经用户确认」的候选动作。
// 插件侧三重硬约束（必须来自 Propose / gate 放行 / 一次性消费）；
// 回执 diff 为页面内容，返回 UI 前强制脱敏。
func (a *App) BrowserExecuteAction(actionID string, names []string) (plugins.ActionReceipt, error) {
	if a.browser == nil || !a.browser.Enabled() {
		return plugins.ActionReceipt{}, fmt.Errorf("浏览器辅助插件未启用：%s", plugins.BrowserOSGuidance)
	}
	rec, err := a.browser.ExecuteAction(a.ctx, actionID)
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_EXECUTE_FAIL", actionID, err.Error())
		return plugins.ActionReceipt{}, err
	}
	sum := sha256.Sum256([]byte(rec.Diff))
	red := redact.New(names)
	redRes := red.RedactText(rec.Diff)
	rec.Diff = redRes.Text
	for _, act := range redRes.Actions {
		_ = a.auditLog.Log(a.profile.Role, "REDACT", act.RuleID, act.Strategy)
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_EXECUTE", actionID,
		fmt.Sprintf("回执哈希 %s；脱敏 %d 项", hex.EncodeToString(sum[:])[:12], len(redRes.Actions)))
	return rec, nil
}

// BrowserHandToHuman 登录/验证码/实名/最终提交等环节交还给人接管——
// 这些环节工具永不代做（红线，见 docs/browseros-integration.md 5.3）。
func (a *App) BrowserHandToHuman(pageID int, kind, reason string) (plugins.HumanHelpStatus, error) {
	st, err := a.browser.HandToHuman(a.ctx, pageID, kind, reason, "")
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_HUMAN_FAIL", fmt.Sprintf("page=%d", pageID), err.Error())
		return st, err
	}
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_HUMAN", fmt.Sprintf("page=%d", pageID), st.Status)
	return st, nil
}

// BrowserAwaitHuman 等待人接管结果（resolved 后才允许继续辅助操作）。
func (a *App) BrowserAwaitHuman() (plugins.HumanHelpStatus, error) {
	st, err := a.browser.AwaitHuman(a.ctx)
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_HUMAN_WAIT", "-", st.Status)
	return st, err
}

// --- 国家规章库清单浏览（人在环代理浏览，neutral/govlistassist.go）---

// BrowseGovList 用用户自己的 BrowserOS neo 浏览国家规章库官方列表页并导出清单
// （混合清单；cat = 可选分类过滤 部门规章/地方政府规章/空 = 全收；maxPages 单会话上限；
// startPage 起始页——跨会话续接，经官方「跳转」控件到达并核验页码）。清单是人工浏览的
// 代理等价物：只走官方 UI 翻页（限速 ≥2s、红线审计），绝不触碰检索 API/鉴权。
// 产物写 appData/incoming/govgz/list_agent_<ts>.json，交 OpenLaw
// openlaw-govcn -source gzk -list <此文件> 走详情页合规抓取 + 人工确认闸口。
func (a *App) BrowseGovList(cat string, maxPages int, startPage int) (neutral.GovListReport, error) {
	dir, err := appDataDir()
	if err != nil {
		return neutral.GovListReport{}, err
	}
	rep, err := neutral.RunGovListBrowse(a.ctx, a.browser, neutral.GovListOptions{
		Cat: cat, MaxPages: maxPages, StartPage: startPage,
	})
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "BROWSER_GOVLIST_FAIL", cat, err.Error())
		return rep, err
	}
	outDir := filepath.Join(dir, "incoming", "govgz")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return rep, err
	}
	out := filepath.Join(outDir, "list_agent_"+time.Now().UTC().Format("20060102T150405")+".json")
	b, err := json.MarshalIndent(rep.Entries, "", "  ")
	if err != nil {
		return rep, err
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return rep, err
	}
	rep.OutPath = out
	sum := sha256.Sum256(b)
	_ = a.auditLog.Log(a.profile.Role, "BROWSER_GOVLIST", cat,
		fmt.Sprintf("浏览 %d 页（起始页 %d）得 %d 条（%s）→ %s sha256=%s；待 openlaw-govcn -source gzk 抓取+人工确认",
			rep.Pages, rep.StartPage, len(rep.Entries), rep.StoppedAt, out, hex.EncodeToString(sum[:])[:12]))
	return rep, nil
}
