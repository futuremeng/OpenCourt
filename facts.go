// 本文件由 main.go 按领域拆分而来（同包 main：Wails 绑定方法必须
// 留在 main 包，这里只是物理分文件，便于按领域阅读与评审）。
package main

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/gate"
	"github.com/futuremeng/OpenCourt/core/redact"
	"github.com/futuremeng/OpenCourt/core/statemachine"
	"github.com/futuremeng/OpenCourt/core/verify"
	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
)

// ListFacts 列出全部事实（先过可见性过滤——本部署仅本方数据）。
func (a *App) ListFacts() ([]domain.Fact, error) {
	facts, err := a.scope().ListFacts()
	if err != nil {
		return nil, err
	}
	_ = a.auditLog.Log(a.profile.Role, "QUERY", "facts", "列出事实 "+fmt.Sprint(len(facts))+" 条")
	return facts, nil
}

// CreateFact 录入一条事实。初始状态由 statemachine.InitialState 判定：
// 有可定位引用→单源未补强；对方来源→对方主张未验证；无来源→无依据。
// 手工录入的引用同样必须回原文定位（与阅卷候选同一纪律）：定位失败
// 拒绝入库并留审计；「已溯源确认」只能通过 UpgradeFact 的证据驱动路径达成。
func (a *App) CreateFact(description, sourceFile string, pageNum int, snippet string, fromOpponent bool) (domain.Fact, error) {
	if description == "" {
		return domain.Fact{}, fmt.Errorf("事实描述不能为空")
	}
	f := domain.Fact{
		FactID:      newID("FACT"),
		Description: description,
		Visibility:  domain.VisBoth,
	}
	if sourceFile != "" {
		c := domain.Citation{SourceFile: sourceFile, PageNum: pageNum, Snippet: snippet}
		if !c.Valid() {
			return domain.Fact{}, fmt.Errorf("引用不完整：填了来源文书就必须给页码与原文片段，否则无法回原文定位；也可留空来源按「无依据」录入")
		}
		docs := a.documentTexts()
		if _, ok := docs[sourceFile]; !ok {
			_ = a.auditLog.Log(a.profile.Role, "CREATE_REJECTED", filepath.Base(sourceFile), "来源文书不在本案已接入文档中")
			return domain.Fact{}, fmt.Errorf("来源文书《%s》尚未接入本案：先在「阅卷」接入该文书，或留空来源按「无依据」录入", filepath.Base(sourceFile))
		}
		if !verify.Locate(c, docs) {
			_ = a.auditLog.Log(a.profile.Role, "CREATE_REJECTED", filepath.Base(sourceFile),
				fmt.Sprintf("引用片段未能在第 %d 页原文定位", pageNum))
			return domain.Fact{}, fmt.Errorf("引用片段未能在来源文书第 %d 页原文中定位：可在「阅卷」检索卷宗找到该页后复制原句重新提交，或留空来源按「无依据」录入（未入库）", pageNum)
		}
		f.Citation = &c
	}
	f.State = statemachine.InitialState(f, fromOpponent)
	if err := a.scope().CreateFact(f); err != nil {
		return domain.Fact{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "CREATE", f.FactID, "初始状态 "+f.State.DisplayName())
	return f, nil
}

// UpgradeFact 提交状态升级候选，由 statemachine 确定性仲裁。
// 模型或前端无权直接改状态——这里就是“代码仲裁优先于模型判断”的入口。
func (a *App) UpgradeFact(factID string, targetState string, evidenceImported, locatedInSource, noInternalConflict bool) (domain.Fact, error) {
	target, err := domain.ParseFactState(targetState)
	if err != nil {
		return domain.Fact{}, err
	}
	f, err := a.scope().GetFact(factID)
	if err != nil {
		return domain.Fact{}, err
	}
	// D1 接线：升级仲裁前反查来源文档的 OCR 标记——OCR 文本永不进强态。
	// 标记查不到即按非 OCR 处理的前提是查询成功；查询失败一律 fail-closed
	// 拒绝升级，绝不带着未知来源进强态。
	sourceFromOCR := false
	if target == domain.StateTracedConfirmed && f.Citation != nil && f.Citation.SourceFile != "" {
		ocr, err := a.scope().IsOCRSource(f.Citation.SourceFile)
		if err != nil {
			return domain.Fact{}, err
		}
		sourceFromOCR = ocr
	}
	// v0.4 P13：进强态三条件由后端确定性重算——前端声明只作报报，
	// 重算只能收紧不能放宽（fail-closed）：引用必须回原文定位得上、
	// 来源文书必须仍在本案、本案不得有涉及该事实的未解决冲突；
	// 查询失败一律按不满足处理，绝不带着未知进强态。
	if target == domain.StateTracedConfirmed {
		if f.Citation != nil && f.Citation.SourceFile != "" {
			docs := a.documentTexts()
			if _, ok := docs[f.Citation.SourceFile]; !ok {
				evidenceImported = false // 来源文书已不在本案，等于证据缺失
			} else {
				locatedInSource = locatedInSource && verify.Locate(*f.Citation, docs)
			}
		} else {
			evidenceImported = false
			locatedInSource = false
		}
		conflicts, cerr := a.scope().ListConflicts()
		if cerr != nil {
			noInternalConflict = false
		} else {
			for _, c := range conflicts {
				if c.IsResolved {
					continue
				}
				for _, id := range c.FactIDs {
					if id == f.FactID {
						noInternalConflict = false
						break
					}
				}
			}
		}
	}
	updated, err := statemachine.ApplyUpgrade(statemachine.UpgradeRequest{
		Fact:               f,
		Target:             target,
		EvidenceImported:   evidenceImported,
		LocatedInSource:    locatedInSource,
		NoInternalConflict: noInternalConflict,
		SourceFromOCR:      sourceFromOCR,
	})
	if err != nil {
		_ = a.auditLog.Log(a.profile.Role, "STATE_UPGRADE_BLOCKED", factID, err.Error())
		return domain.Fact{}, err
	}
	if err := a.scope().UpdateFactState(updated.FactID, updated.State); err != nil {
		return domain.Fact{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "STATE_UPGRADE", factID, f.State.DisplayName()+" -> "+updated.State.DisplayName())
	return updated, nil
}

// ListConflicts 列出全部冲突（并列展示，不自动消解）。
func (a *App) ListConflicts() ([]domain.Conflict, error) {
	return a.scope().ListConflicts()
}

// refIDRe 从待检文本中抽取引用标识（FACT- / LAW- / CASE-）。门禁据此做落地
// 检查：文本里出现、但本案索引/留痕里没有的标识即幻觉。CASE-DEFAULT 是案件
// 标识不是引用，显式排除（否则提到默认案件就误报）。
var refIDRe = regexp.MustCompile(`(?:FACT|LAW|CASE)-[0-9A-Za-z][0-9A-Za-z_\-]*`)

func extractRefIDs(text string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range refIDRe.FindAllString(text, -1) {
		if id == "CASE-"+strings.TrimPrefix(domain.DefaultCaseID, "CASE-") || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// CheckOutput 输出门禁（开发文档 7.4 三层门禁的对外入口）：
//  1. 引导性/预测性/裁判性表述——词表拦截；
//  2. 文本中出现的 FACT- 引用——必须落在本案事实索引，且不得是「无依据」；
//  3. 文本中出现的 LAW- / CASE- 引用——必须落在本案引用留痕，未核验或
//     待人工确认一律上浮（REF_UNVERIFIED），跨案件引用天然 not-found。
//
// 前端预检与合规演示走同一条代码路径：门禁是代码级拦截，不是提示词自觉。
func (a *App) CheckOutput(text string) []gate.Violation {
	out := gate.OutputDraft{Text: text, FactIDs: extractRefIDs(text)}
	index := gate.FactIndex{}
	if facts, err := a.scope().ListFacts(); err == nil {
		for _, f := range facts {
			index[f.FactID] = f
		}
	}
	refs := gate.RefIndex{}
	if laws, err := a.scope().ListLawRefs(); err == nil {
		for _, r := range laws {
			refs[r.RefID] = gate.RefInfo{Kind: "LAW", Status: r.Status,
				ConfirmStatus: r.ConfirmStatus, VerifyResult: r.VerifyResult}
		}
	}
	if cases, err := a.scope().ListCaseRefs(); err == nil {
		for _, r := range cases {
			// 类案引用没有「效力」概念：检索留痕（URL + 抓取时间 + 哈希）即
			// 溯源凭据，Status 记 UNKNOWN（不冒充 EFFECTIVE）。
			refs[r.RefID] = gate.RefInfo{Kind: "CASE", Status: "UNKNOWN",
				ConfirmStatus: "confirmed", VerifyResult: "OK"}
		}
	}
	violations := gate.Check(out, index, refs)
	if len(violations) > 0 {
		_ = a.auditLog.Log(a.profile.Role, "GATE_BLOCK", "output", fmt.Sprint(len(violations))+" 条违规")
	}
	return violations
}

// RedactText 对文本执行强制脱敏，返回脱敏文本与操作审计记录。
func (a *App) RedactText(text string, names []string) redact.Result {
	r := redact.New(names)
	res := r.RedactText(text)
	for _, act := range res.Actions {
		_ = a.auditLog.Log(a.profile.Role, "REDACT", act.RuleID, act.Strategy)
	}
	return res
}

// --- 阶段二：文档接入与阅卷（开发文档 4.1、4.8-4.12、11.2） ---

// BuildTimeline 构建案件时间线（按时间要素锚定，无锚点排末尾，不虚构日期）。
func (a *App) BuildTimeline() ([]neutral.TimelineItem, error) {
	facts, err := a.scope().ListFacts()
	if err != nil {
		return nil, err
	}
	elements, err := a.scope().ListElements()
	if err != nil {
		return nil, err
	}
	return neutral.BuildTimeline(facts, elements), nil
}

// RunAggregate 汇聚全部事实：检测冲突并列输出入库（不自动消解）。
func (a *App) RunAggregate() ([]domain.Conflict, error) {
	facts, err := a.scope().ListFacts()
	if err != nil {
		return nil, err
	}
	_, conflicts := neutral.Aggregate(facts)
	for _, c := range conflicts {
		if err := a.scope().CreateConflict(c); err != nil {
			return nil, err
		}
	}
	_ = a.auditLog.Log(a.profile.Role, "AGGREGATE", "facts", fmt.Sprintf("发现冲突 %d 组（并列展示，不自动消解）", len(conflicts)))
	return conflicts, nil
}

// RunElementMatch 要件匹配：只有「已溯源确认」的事实才能覆盖要件。
func (a *App) RunElementMatch(requirements []string) (neutral.MatchResult, error) {
	facts, err := a.scope().ListFacts()
	if err != nil {
		return neutral.MatchResult{}, err
	}
	return neutral.MatchElements(facts, requirements), nil
}

// RunElementMatchByCaseType 按案由要件匹配（v0.2 M4）：要件清单只从 OpenLaw
// 要件库取（封闭集，要件经人工确认导入）。无要件库显式报错并给出出路——
// 绝不回退到模型自由生成要件（V3）。审计 ELEMENT_MATCH。
func (a *App) RunElementMatchByCaseType(caseType string) (neutral.MatchResult, error) {
	facts, err := a.scope().ListFacts()
	if err != nil {
		return neutral.MatchResult{}, err
	}
	var res neutral.MatchResult
	err = a.withLaw(func(src plugins.LawSourcePlugin) error {
		es, ok := src.(neutral.ElementSource)
		if !ok {
			return fmt.Errorf("无要件库：请手输要件（要件匹配面板），或经 OpenLaw（openlaw-element）人工确认导入要件")
		}
		var verr error
		res, verr = neutral.MatchElementsFrom(facts, es, caseType)
		return verr
	})
	if err != nil {
		return neutral.MatchResult{}, err
	}
	_ = a.auditLog.Log(a.profile.Role, "ELEMENT_MATCH", caseType,
		fmt.Sprintf("覆盖 %d / 缺失 %d", len(res.Covered), len(res.Missing)))
	return res, nil
}

// elementNamesForCurrentCase 当前案件的要件名封闭候选集（V3 阅卷注入）：
// 法条源未配置、案由为空或取数失败一律返回空集——阅卷照常但要件字段
// 强制置空，绝不因要件库失败而中断阅卷，也绝不放模型自由生成。
func (a *App) elementNamesForCurrentCase() []string {
	c, err := a.store.GetCase(a.caseID())
	if err != nil || strings.TrimSpace(c.CaseType) == "" {
		return nil
	}
	var specs []plugins.ElementSpec
	if err := a.withLaw(func(src plugins.LawSourcePlugin) error {
		es, ok := src.(neutral.ElementSource)
		if !ok {
			return fmt.Errorf("法条源未配置要件库")
		}
		var verr error
		specs, verr = es.ElementsByCaseType(context.Background(), c.CaseType)
		return verr
	}); err != nil {
		return nil
	}
	names := make([]string, 0, len(specs))
	for _, sp := range specs {
		names = append(names, sp.Name)
	}
	return names
}

// RunBlindspots 盲区生成：输出无法判断的问题与补证方向，绝不幻觉填补。
func (a *App) RunBlindspots(requirements []string) ([]neutral.Blindspot, error) {
	match, err := a.RunElementMatch(requirements)
	if err != nil {
		return nil, err
	}
	return neutral.GenerateBlindspots(match), nil
}
