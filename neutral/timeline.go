// 时间线构建（开发文档 4.9）：事实按关联的时间要素排序；
// 无法定位日期的事实排在末尾并显式标注，不做猜测性归位。
package neutral

import (
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/extract"
)

var (
	reISODate = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	reCNDate  = regexp.MustCompile(`(\d{4})年(\d{1,2})月(\d{1,2})日`)
)

// parseDate 将 ISO 或中文日期解析为时间；失败返回 ok=false。
func parseDate(s string) (time.Time, bool) {
	if m := reISODate.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC), true
	}
	if m := reCNDate.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC), true
	}
	return time.Time{}, false
}

// BuildTimeline 构建时间线：每条事实查找与其引用同页的时间要素，
// 取该页第一个时间作为时间锚点；无锚点的事实排末尾（Date 为空）。
// 绝不虚构日期。
func BuildTimeline(facts []domain.Fact, elements []extract.Element) []TimelineItem {
	timesByPage := map[string]map[int][]extract.Element{}
	for _, e := range elements {
		if e.Type != extract.ElementTime {
			continue
		}
		if timesByPage[e.SourceFile] == nil {
			timesByPage[e.SourceFile] = map[int][]extract.Element{}
		}
		timesByPage[e.SourceFile][e.PageNum] = append(timesByPage[e.SourceFile][e.PageNum], e)
	}

	out := make([]TimelineItem, 0, len(facts))
	for _, f := range facts {
		item := TimelineItem{Fact: f}
		if f.Citation != nil {
			if pageTimes := timesByPage[f.Citation.SourceFile][f.Citation.PageNum]; len(pageTimes) > 0 {
				item.Date = pageTimes[0].Value
			}
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti, iok := parseDate(out[i].Date)
		tj, jok := parseDate(out[j].Date)
		if iok != jok {
			return iok // 有日期的在前
		}
		if iok {
			return ti.Before(tj)
		}
		return out[i].Fact.FactID < out[j].Fact.FactID
	})
	return out
}
