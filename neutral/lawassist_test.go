package neutral

import (
	"context"
	"errors"
	"testing"

	"github.com/futuremeng/OpenCourt/plugins"
)

// --- 引用抽取（确定性正则，无模型参与）---

func TestExtractLawCitations(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []LawCitationDraft
	}{
		{
			"全称+中文数字条号",
			"依据《中华人民共和国民法典》第五百七十七条，被告构成违约。",
			[]LawCitationDraft{{LawName: "中华人民共和国民法典", ArticleNo: "第五百七十七条"}},
		},
		{
			"简称+阿拉伯数字+之N",
			"适用《民法典》第136条之2的规定。",
			[]LawCitationDraft{{LawName: "民法典", ArticleNo: "第136条之2"}},
		},
		{
			"多引用+重复去重（保持出现顺序）",
			"《民法典》第一百七十九条、《劳动法》第三十三条、《民法典》第一百七十九条再次援引。",
			[]LawCitationDraft{{LawName: "民法典", ArticleNo: "第一百七十九条"}, {LawName: "劳动法", ArticleNo: "第三十三条"}},
		},
		{
			"无书名号的裸条号不抽取（无法核验不猜）",
			"本合同第十七条约定违约金。",
			[]LawCitationDraft{},
		},
		{"无引用", "本文书不含任何法条引用。", []LawCitationDraft{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractLawCitations(c.text)
			if len(got) != len(c.want) {
				t.Fatalf("抽取数量错误: got %d want %d (%+v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i].LawName != c.want[i].LawName || got[i].ArticleNo != c.want[i].ArticleNo {
					t.Fatalf("第 %d 条抽取错误: got %+v want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
}

// --- 核验管线（六分支透传 + pending 原样上浮）---

type fakeVerifier struct {
	verdicts     map[string]plugins.CitationVerdict
	transportErr error
	calls        []string
}

func (f *fakeVerifier) CheckCitation(_ context.Context, lawName, articleNo, _, _ string) (plugins.CitationVerdict, error) {
	key := lawName + " " + articleNo
	f.calls = append(f.calls, key)
	if f.transportErr != nil {
		return plugins.CitationVerdict{}, f.transportErr
	}
	if v, ok := f.verdicts[key]; ok {
		return v, nil
	}
	return plugins.CitationVerdict{VerifyResult: "OL-1001", Status: "UNKNOWN", ConfirmStatus: "pending"}, nil
}

func (f *fakeVerifier) FindRelation(_ context.Context, _ string) ([]plugins.Relation, error) {
	return nil, nil
}

func TestVerifyCitationsSixBranches(t *testing.T) {
	v := &fakeVerifier{verdicts: map[string]plugins.CitationVerdict{
		"民法典 第五百七十七条": {VerifyResult: "OK", Status: "UNKNOWN", ConfirmStatus: "confirmed", ArticleID: "flk_minfadian_577"},
		"民法典 第九百九十九条": {VerifyResult: "OL-1003", Status: "UNKNOWN", ConfirmStatus: "pending"},
		"劳动合同法 第八十七条": {VerifyResult: "OL-1006", Status: "UNKNOWN", ConfirmStatus: "pending", ArticleID: "flk_ldht_87"},
	}}
	drafts := []LawCitationDraft{
		{LawName: "民法典", ArticleNo: "第五百七十七条"},
		{LawName: "民法典", ArticleNo: "第九百九十九条"},
		{LawName: "劳动合同法", ArticleNo: "第八十七条"},
		{LawName: "消失的法", ArticleNo: "第一条"},
	}
	refs, err := VerifyCitations(context.Background(), v, "2024-01-01", drafts)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 4 {
		t.Fatalf("留痕数量错误: %d", len(refs))
	}
	wantResults := []string{"OK", "OL-1003", "OL-1006", "OL-1001"}
	for i, want := range wantResults {
		if refs[i].VerifyResult != want {
			t.Fatalf("第 %d 条核验判定错误: got %s want %s", i, refs[i].VerifyResult, want)
		}
		if refs[i].AtDate != "2024-01-01" {
			t.Fatalf("时间点必须随行: %+v", refs[i])
		}
	}
	// pending 原样上浮，绝不自动确认；断链条文标识照常返回（数据不吞）
	if refs[1].ConfirmStatus != "pending" {
		t.Fatalf("pending 必须原样上浮: %+v", refs[1])
	}
	if refs[2].ArticleID == "" {
		t.Fatal("OL-1006 断链数据照常返回，不得吞掉")
	}
}

func TestVerifyCitationsTransportError(t *testing.T) {
	v := &fakeVerifier{transportErr: errors.New("数据目录损坏")}
	_, err := VerifyCitations(context.Background(), v, "", []LawCitationDraft{{LawName: "民法典", ArticleNo: "第一条"}})
	if err == nil {
		t.Fatal("传输层失败必须报错，不得伪装成核验不通过")
	}
}

func TestVerifyCitationsNilVerifier(t *testing.T) {
	if _, err := VerifyCitations(context.Background(), nil, "", nil); err == nil {
		t.Fatal("法条源未配置必须显式报错")
	}
}

// --- 检索守卫 ---

type fakeLawSource struct{ hits []plugins.LawHit }

func (s *fakeLawSource) Name() string { return "fake" }
func (s *fakeLawSource) SearchLaw(_ context.Context, _ string, _ int) ([]plugins.LawHit, error) {
	return s.hits, nil
}
func (s *fakeLawSource) GetLawDetail(_ context.Context, _ string) (plugins.LawHit, error) {
	return plugins.LawHit{}, nil
}

func TestSearchLawGuards(t *testing.T) {
	if _, err := SearchLaw(context.Background(), nil, "违约金", 10); err == nil {
		t.Fatal("法条源未配置必须显式报错（不静默冒充空结果）")
	}
	if _, err := SearchLaw(context.Background(), &fakeLawSource{}, "  ", 10); err == nil {
		t.Fatal("空检索词必须报错")
	}
	hits, err := SearchLaw(context.Background(), &fakeLawSource{
		hits: []plugins.LawHit{{LawID: "A1", Status: "UNKNOWN", ConfirmStatus: "pending"}},
	}, "违约金", 0)
	if err != nil || len(hits) != 1 {
		t.Fatalf("检索失败: %v %+v", err, hits)
	}
}
