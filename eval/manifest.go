// Package eval 评测验收 runner（v0.4 E0，roadmap M8 前移）。
// 两部分：
//  1. 门禁对抗集——离线确定性，任何时候可跑，产出误拦率/漏拦率；
//  2. 模型阅卷指标——需要显式启用模型与 LocalCases（正常对接通道：
//     manifest 固化 id 清单 + 逐条 /api/case/{id} 回源取文），未启用时
//     如实标注「未运行」，绝不以规则/模板冒充模型评测结果。
//
// 纪律见 docs/plan-v0.4.md §3-E0 与 docs/localcases-integration.md §8
// （LC-8 快照 / LC-9 确定性圈定 / LC-10 全文取回）。
package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ManifestEntry 评测集清单的一行（tests/eval/corpus_manifest.jsonl）。
// Query 是圈定查询原文（LC-9 语义：同过滤 + sort=id 升序取前 N），
// 留档是为了「任何人可复跑同一集合」可核查。
type ManifestEntry struct {
	ID           int64  `json:"id"` // LocalCases 内部 id
	SourceCaseID string `json:"source_case_id,omitempty"`
	Source       string `json:"source,omitempty"`
	Query        string `json:"query,omitempty"`
	SHA256       string `json:"sha256,omitempty"` // 空 = 未固化，runner 现算并写入报告
}

// SHA256Hex 对正文计算 sha256（十六进制），与 manifest 比对用。
func SHA256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// LoadManifest 解析 JSONL 清单。校验：id 必填且唯一、sha256 为合法 hex。
// 空输入返回空清单（合法——runner 据此只跑门禁对抗集并如实标注）。
func LoadManifest(r io.Reader) ([]ManifestEntry, error) {
	entries := []ManifestEntry{}
	seen := map[int64]bool{}
	dec := json.NewDecoder(r)
	line := 0
	for dec.More() {
		line++
		var e ManifestEntry
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("评测清单第 %d 行解析失败: %w", line, err)
		}
		if e.ID <= 0 {
			return nil, fmt.Errorf("评测清单第 %d 行：id 必须为正整数", line)
		}
		if seen[e.ID] {
			return nil, fmt.Errorf("评测清单第 %d 行：id %d 重复，清单必须是确定集合", line, e.ID)
		}
		seen[e.ID] = true
		if e.SHA256 != "" {
			if len(e.SHA256) != 64 || strings.TrimRight(e.SHA256, "0123456789abcdefABCDEF") != "" {
				return nil, fmt.Errorf("评测清单第 %d 行：sha256 不是 64 位十六进制", line)
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}
