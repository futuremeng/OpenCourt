package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAdapter 内存 OCR 引擎桩。
type fakeAdapter struct {
	pages []OCRPage
	err   error
	name  string
}

func (f *fakeAdapter) Name() string {
	if f.name != "" {
		return f.name
	}
	return "fake"
}

func (f *fakeAdapter) Recognize(_ context.Context, _ string) ([]OCRPage, error) {
	return f.pages, f.err
}

func TestOCRParserMarksOCRAndConfidence(t *testing.T) {
	p := &OCRParser{
		Adapter: &fakeAdapter{pages: []OCRPage{{PageNum: 1, Text: "合同原件扫描", Confidence: 0.42}, {PageNum: 2, Text: "第二页", Confidence: 0.95}}},
	}
	doc, err := p.Parse("scan.png")
	if err != nil {
		t.Fatal(err)
	}
	if !doc.OCR || doc.OCREngine != "fake" {
		t.Errorf("OCR 文档必须带标记与引擎名: %+v", doc)
	}
	if doc.OCRConf[1] != 0.42 {
		t.Errorf("置信度未透传: %v", doc.OCRConf)
	}
	low := NewRegistry().LowConfidencePages(doc, 0.6)
	if len(low) != 1 || low[0] != 1 {
		t.Errorf("低置信页判定错误: %v", low)
	}
}

func TestOCRParserRejectsEmptyOutput(t *testing.T) {
	p := &OCRParser{Adapter: &fakeAdapter{}}
	if _, err := p.Parse("blank.png"); err == nil {
		t.Fatal("OCR 无产出必须拒绝入库")
	}
	p2 := &OCRParser{Adapter: &fakeAdapter{pages: []OCRPage{{PageNum: 0, Text: "x"}}}}
	if _, err := p2.Parse("bad.png"); err == nil {
		t.Fatal("非法页码必须拒绝入库")
	}
}

func TestParseTesseractTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t0\t0\t10\t10\t92.5\t合同\n" +
		"5\t1\t1\t1\t1\t2\t10\t0\t10\t10\t87.5\t原件\n" +
		"5\t1\t1\t1\t1\t3\t20\t0\t10\t10\t-1\t \n"
	text, conf, ok := parseTesseractTSV(tsv)
	if !ok || text != "合同原件" {
		t.Fatalf("TSV 解析错误: %q %v", text, ok)
	}
	if conf < 0.89 || conf > 0.91 {
		t.Errorf("页级置信应为词均值 ~0.90: %v", conf)
	}
	if _, _, ok := parseTesseractTSV("level\n5\t1\t1\t1\t1\t1\t0\t0\t1\t1\t-1\t\n"); ok {
		t.Error("无非文本以外词行时必须报告无文本")
	}
}

// writeFakeBinary 生成输出固定内容的 shell 桩程序。
func writeFakeBinary(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fakebin")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat <<'EOF'\n"+content+"EOF\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVisionAdapterViaHelper(t *testing.T) {
	helper := writeFakeBinary(t, `{"text":"扫描文本","confidence":0.91}`+"\n")
	v := &VisionAdapter{HelperPath: helper}
	if !v.Available() {
		t.Fatal("helper 应可用")
	}
	pages, err := v.Recognize(context.Background(), "x.png")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Text != "扫描文本" || pages[0].Confidence != 0.91 {
		t.Errorf("Vision 产出错误: %+v", pages)
	}
	missing := &VisionAdapter{HelperPath: filepath.Join(t.TempDir(), "nope")}
	if _, err := missing.Recognize(context.Background(), "x.png"); err == nil {
		t.Fatal("helper 缺失必须报错，绝不静默")
	}
}

func TestTesseractAdapterViaBinary(t *testing.T) {
	bin := writeFakeBinary(t, "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"+
		"5\t1\t1\t1\t1\t1\t0\t0\t9\t9\t95.0\t借款\n")
	ad := &TesseractAdapter{Binary: bin}
	pages, err := ad.Recognize(context.Background(), "x.png")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Text != "借款" {
		t.Errorf("tesseract 产出错误: %+v", pages)
	}
	if !(&TesseractAdapter{Binary: bin}).Available() {
		t.Error("指定二进制应可用")
	}
}

func TestDetectOCRDefaults(t *testing.T) {
	if eng, note := DetectOCR(OCRConfig{}); eng != nil || note != "" {
		t.Errorf("默认必须无引擎无提示: %v %q", eng, note)
	}
	if eng, note := DetectOCR(OCRConfig{Engine: "bogus"}); eng != nil || note == "" {
		t.Errorf("未知引擎必须维持拒绝并给出说明: %v %q", eng, note)
	}
	if eng, note := DetectOCR(OCRConfig{Engine: "vision", Path: filepath.Join(t.TempDir(), "missing")}); eng != nil || note == "" {
		t.Errorf("指定引擎不可用不得降级到其他引擎: %v %q", eng, note)
	}
}

// fakeExtractor 扫描取图桩：每页一张临时图（cleanup 为空操作）。
func fakeExtractor(pages int) ImageExtractor {
	return func(string) ([]PageImages, func(), error) {
		out := []PageImages{}
		for i := 0; i < pages; i++ {
			out = append(out, PageImages{Page: i + 1, Paths: []string{fmt.Sprintf("page-%d.png", i+1)}})
		}
		return out, func() {}, nil
	}
}

func TestScannedPDFGoesThroughOCR(t *testing.T) {
	// 第 2 页引擎空白产出 → 整体拒绝（部分入库比拒绝更危险）。
	seq := &seqAdapter{calls: [][]OCRPage{
		{{PageNum: 1, Text: "第一页", Confidence: 0.8}},
		{},
	}}
	p := &PDFParser{OCR: seq, ExtractImages: fakeExtractor(2)}
	if _, err := p.parseScanned("scan.pdf", 2); err == nil {
		t.Fatal("任一页空白必须整体拒绝")
	}
	p.OCR = &fakeAdapter{pages: []OCRPage{{PageNum: 1, Text: "扫描件文本", Confidence: 0.8}}}
	p.ExtractImages = fakeExtractor(1)
	doc, err := p.parseScanned("scan.pdf", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.OCR || doc.Pages[1] != "扫描件文本" {
		t.Errorf("扫描 PDF 应产出 OCR 标记文档: %+v", doc)
	}
}

// 同页多内嵌图（彩色层+mask 等）必须按序拼接为一页，而非误拒整文档；
// 缺图像层的页必须明确拒绝；cleanup 必须被调用（临时目录不泄漏）。
func TestScannedPDFMultiImagePages(t *testing.T) {
	cleaned := false
	p := &PDFParser{
		OCR: &fakeAdapter{pages: []OCRPage{{PageNum: 1, Text: "段", Confidence: 0.9}}},
		ExtractImages: func(string) ([]PageImages, func(), error) {
			return []PageImages{
				{Page: 1, Paths: []string{"p1a.png"}},
				{Page: 2, Paths: []string{"p2a.png", "p2mask.png"}},
			}, func() { cleaned = true }, nil
		},
	}
	doc, err := p.parseScanned("scan.pdf", 2)
	if err != nil {
		t.Fatalf("同页多图不得误拒: %v", err)
	}
	if doc.Pages[2] != "段段" {
		t.Errorf("同页多图应拼接: %q", doc.Pages[2])
	}
	if !cleaned {
		t.Error("取图临时目录必须经 cleanup 清理")
	}
	if _, err := p.parseScanned("scan.pdf", 3); err == nil ||
		!strings.Contains(err.Error(), "第 3 页无图像层") {
		t.Errorf("缺图像层的页必须明确拒绝: %v", err)
	}
}

// seqAdapter 按调用次序返回不同产出（模拟逐页差异）。
type seqAdapter struct {
	calls [][]OCRPage
	i     int
}

func (s *seqAdapter) Name() string { return "seq" }

func (s *seqAdapter) Recognize(_ context.Context, _ string) ([]OCRPage, error) {
	if s.i >= len(s.calls) {
		return nil, nil
	}
	out := s.calls[s.i]
	s.i++
	return out, nil
}

func TestScannedPDFRejectsWithoutExtractor(t *testing.T) {
	p := &PDFParser{OCR: &fakeAdapter{}}
	_, err := p.parseScanned("scan.pdf", 1)
	if err == nil || !strings.Contains(err.Error(), "拒绝") {
		t.Fatalf("有引擎但取图组件缺失必须明确拒绝: %v", err)
	}
}

func TestRegistrySetOCRWiresParsers(t *testing.T) {
	r := NewRegistry()
	ad := &fakeAdapter{pages: []OCRPage{{PageNum: 1, Text: "图", Confidence: 0.9}}}
	r.SetOCR(ad, fakeExtractor(1))
	doc, err := r.Parse("scan.png")
	if err != nil {
		t.Fatal(err)
	}
	if !doc.OCR {
		t.Error("注入后图片应经 OCR 入库并带标记")
	}
}
