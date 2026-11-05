// 模型阅卷指标（开发文档 11.1 验收项的可测形态）：
//   - 逃逸幻觉率 = 已入库事实中复验失败（引用落不回原文）的比例，目标 < 1%；
//   - 溯源准确率 = 已入库事实中引用满足最低溯源要求且定位成功的比例，目标 > 98%；
//   - 候选拒绝率 = 核验器/门禁拦下的候选占比——模型幻觉由核验器兜底的直接证据。
//
// 指标只从 neutral.ReaderReport 聚合，评测器不另设第二套判定逻辑——
// 判定标准与生产路径完全一致（同一核验器、同一门禁）。
package eval

import (
	"strings"

	"github.com/futuremeng/OpenCourt/core/verify"
	"github.com/futuremeng/OpenCourt/neutral"
)

// DocMetrics 单份文书的阅卷指标。
type DocMetrics struct {
	DocID          string `json:"doc_id"`
	Candidates     int    `json:"candidates"`      // 模型提交候选总数
	Accepted       int    `json:"accepted"`        // 入库事实
	RejectedVerify int    `json:"rejected_verify"` // 定位失败被拒（拦下的幻觉）
	GateBlocked    int    `json:"gate_blocked"`    // 门禁拦截（倾向性等表述）
	EscapedVerify  int    `json:"escaped_verify"`  // 逃逸幻觉：入库但复验定位失败
	TracedOK       int    `json:"traced_ok"`       // 入库且引用有效并定位成功
	Error          string `json:"error,omitempty"` // 该份取文/阅卷失败原因（失败不计入聚合）
}

// MetricsFromReport 对一份阅卷结果做复验并计算指标。
// 复验用与生产同一个 verify.Locate——评测器不引入第二套判定。
func MetricsFromReport(rep neutral.ReaderReport, doc *verify.DocumentText) DocMetrics {
	docs := map[string]*verify.DocumentText{doc.File: doc}
	m := DocMetrics{DocID: rep.DocID, Accepted: len(rep.Accepted)}
	for _, r := range rep.Rejected {
		if strings.HasPrefix(r.RejectReason, "GATE:") {
			m.GateBlocked++
		} else {
			m.RejectedVerify++
		}
	}
	m.Candidates = len(rep.Accepted) + len(rep.Rejected)
	for i := range rep.Accepted {
		f := &rep.Accepted[i]
		if f.Citation != nil && f.Citation.Valid() && verify.Locate(*f.Citation, docs) {
			m.TracedOK++
			continue
		}
		m.EscapedVerify++
	}
	return m
}

// Aggregate 跨文书聚合。
type Aggregate struct {
	Docs           int `json:"docs"`
	FailedDocs     int `json:"failed_docs"`
	Candidates     int `json:"candidates"`
	Accepted       int `json:"accepted"`
	RejectedVerify int `json:"rejected_verify"`
	GateBlocked    int `json:"gate_blocked"`
	EscapedVerify  int `json:"escaped_verify"`
	TracedOK       int `json:"traced_ok"`
}

func AggregateOf(ms []DocMetrics) Aggregate {
	a := Aggregate{}
	for _, m := range ms {
		if m.Error != "" {
			a.FailedDocs++
			continue
		}
		a.Docs++
		a.Candidates += m.Candidates
		a.Accepted += m.Accepted
		a.RejectedVerify += m.RejectedVerify
		a.GateBlocked += m.GateBlocked
		a.EscapedVerify += m.EscapedVerify
		a.TracedOK += m.TracedOK
	}
	return a
}

// EscapedRate 逃逸幻觉率（入库后复验失败 / 入库数）。
func (a Aggregate) EscapedRate() float64 {
	if a.Accepted == 0 {
		return 0
	}
	return float64(a.EscapedVerify) / float64(a.Accepted)
}

// TraceAccuracy 溯源准确率（引用有效且定位成功 / 入库数）。
func (a Aggregate) TraceAccuracy() float64 {
	if a.Accepted == 0 {
		return 0
	}
	return float64(a.TracedOK) / float64(a.Accepted)
}

// CandidateRejectRate 候选拒绝率（被拦候选 / 全部候选）。
func (a Aggregate) CandidateRejectRate() float64 {
	if a.Candidates == 0 {
		return 0
	}
	return float64(a.RejectedVerify+a.GateBlocked) / float64(a.Candidates)
}
