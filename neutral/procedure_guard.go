// Package neutral 程序守护（开发文档 4.13）：
// 内置程序节点监控引擎，支持自定义期限规则。临近或超期直接触发预警，
// 防止因程序瑕疵导致实体权利丧失。
package neutral

import (
	"time"
)

// DeadlineRule 一条期限规则（如举证期限、上诉期限）。
type DeadlineRule struct {
	Name      string    // 规则名：举证期限 / 上诉期限 / 送达回证 ...
	EventDate time.Time // 起算事件日期
	Days      int       // 期限天数
	WarnDays  int       // 提前预警天数
}

// DeadlineWarning 预警输出。
type DeadlineWarning struct {
	Rule     string    `json:"rule"`
	DueDate  time.Time `json:"due_date"`
	DaysLeft int       `json:"days_left"` // 负数表示已超期
	Level    string    `json:"level"`     // WARN / OVERDUE
}

// CheckDeadlines 按当前日期检查全部期限规则，返回临近与超期预警。
func CheckDeadlines(rules []DeadlineRule, now time.Time) []DeadlineWarning {
	out := []DeadlineWarning{}
	for _, r := range rules {
		due := r.EventDate.AddDate(0, 0, r.Days)
		left := int(due.Sub(now).Hours() / 24)
		switch {
		case left < 0:
			out = append(out, DeadlineWarning{Rule: r.Name, DueDate: due, DaysLeft: left, Level: "OVERDUE"})
		case left <= r.WarnDays:
			out = append(out, DeadlineWarning{Rule: r.Name, DueDate: due, DaysLeft: left, Level: "WARN"})
		}
	}
	return out
}
