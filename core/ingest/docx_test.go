package ingest

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/verify"
)

// buildDocx 构造最小 docx：parts 为部件名 → 内容。
func buildDocx(t *testing.T, parts map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "case.docx")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const wHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`

func TestDocxBodyTableAndRevisions(t *testing.T) {
	docXML := wHeader +
		`<w:p><w:r><w:t>借款合同约定</w:t></w:r>` +
		`<w:ins w:id="1" w:author="张三"><w:r><w:t>插入的利率条款</w:t></w:r></w:ins>` +
		`<w:del w:id="2" w:author="李四"><w:r><w:delText>被删除的担保条款</w:delText></w:r></w:del>` +
		`</w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>甲方</w:t></w:r></w:p></w:tc>` +
		`<w:tc><w:p><w:r><w:t>乙方</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`</w:body></w:document>`
	path := buildDocx(t, map[string]string{"word/document.xml": docXML})

	doc, err := (&DocxParser{}).Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	body := doc.Pages[1]
	if !strings.Contains(body, "借款合同约定") {
		t.Errorf("正文缺失: %q", body)
	}
	if strings.Contains(body, "插入的利率条款") || strings.Contains(body, "被删除的担保条款") {
		t.Errorf("修订文本不得进正文: %q", body)
	}
	if !strings.Contains(body, "[表格已线性化]") || !strings.Contains(body, "甲方 | 乙方") {
		t.Errorf("表格线性化标注/分隔缺失: %q", body)
	}
	var ins, del int
	for _, a := range doc.Annotations {
		switch a.Kind {
		case "revision_insert":
			ins++
			if a.Author != "张三" || a.Text != "插入的利率条款" {
				t.Errorf("插入标注错误: %+v", a)
			}
		case "revision_delete":
			del++
			if a.Author != "李四" || a.Text != "被删除的担保条款" {
				t.Errorf("删除标注错误: %+v", a)
			}
		}
	}
	if ins != 1 || del != 1 {
		t.Errorf("修订标注数量错误: ins=%d del=%d", ins, del)
	}
	// 标注不进 Pages：回原文定位必须失败（不自动采信）。
	cite := domain.Citation{SourceFile: "x", PageNum: 1, Snippet: "插入的利率条款"}
	if verify.Locate(cite, map[string]*verify.DocumentText{"x": doc}) {
		t.Error("标注文本必须无法被 Locate 定位")
	}
	// 正文可定位。
	bodyCite := domain.Citation{SourceFile: "x", PageNum: 1, Snippet: "借款合同约定"}
	if !verify.Locate(bodyCite, map[string]*verify.DocumentText{"x": doc}) {
		t.Error("正文必须可被 Locate 定位")
	}
}

func TestDocxComments(t *testing.T) {
	docXML := wHeader + `<w:p><w:r><w:t>正文</w:t></w:r></w:p></w:body></w:document>`
	commentsXML := `<?xml version="1.0"?>` +
		`<w:comments xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:comment w:id="1" w:author="王法官"><w:p><w:r><w:t>请补充交付凭证</w:t></w:r></w:p></w:comment>` +
		`</w:comments>`
	path := buildDocx(t, map[string]string{
		"word/document.xml": docXML,
		"word/comments.xml": commentsXML,
	})
	doc, err := (&DocxParser{}).Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc.Pages[1], "请补充交付凭证") {
		t.Errorf("批注文本不得进正文: %q", doc.Pages[1])
	}
	found := false
	for _, a := range doc.Annotations {
		if a.Kind == "comment" && a.Author == "王法官" && a.Text == "请补充交付凭证" {
			found = true
		}
	}
	if !found {
		t.Errorf("批注未进标注区块: %+v", doc.Annotations)
	}
}

func TestDocxCorruptedAndLegacy(t *testing.T) {
	// zip 完好但缺 document.xml → 明确报错。
	noDoc := buildDocx(t, map[string]string{"word/styles.xml": "<x/>"})
	if _, err := (&DocxParser{}).Parse(noDoc); err == nil ||
		!strings.Contains(err.Error(), "document.xml") {
		t.Fatalf("缺正文部件必须明确报错: %v", err)
	}
	// 损坏 zip。
	broken := filepath.Join(t.TempDir(), "broken.docx")
	if err := os.WriteFile(broken, []byte("not a zip at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&DocxParser{}).Parse(broken); err == nil ||
		!strings.Contains(err.Error(), "docx") {
		t.Fatalf("损坏 docx 必须明确报错: %v", err)
	}
	// 扩展名 .doc → 明确指引另存。
	legacy := filepath.Join(t.TempDir(), "old.doc")
	if err := os.WriteFile(legacy, []byte("whatever"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&DocxParser{}).Parse(legacy); err == nil ||
		!strings.Contains(err.Error(), ".docx") {
		t.Fatalf(".doc 必须给出另存指引: %v", err)
	}
	// OLE 伪装 .docx。
	ole := filepath.Join(t.TempDir(), "fake.docx")
	head := append([]byte{0xD0, 0xCF, 0x11, 0xE0}, make([]byte, 512)...)
	if err := os.WriteFile(ole, head, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&DocxParser{}).Parse(ole); err == nil ||
		!strings.Contains(err.Error(), ".docx") {
		t.Fatalf("OLE 伪装必须给出另存指引: %v", err)
	}
}

func TestDocxEmbeddedImagesViaOCR(t *testing.T) {
	docXML := wHeader + `<w:p><w:r><w:t>正文</w:t></w:r></w:p></w:body></w:document>`
	p := &DocxParser{}
	path := buildDocx(t, map[string]string{
		"word/document.xml":   docXML,
		"word/media/img1.png": "fakepng",
	})
	// 未配置引擎：正文照常入库，图片标注拒绝原因。
	doc, err := p.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.OCR {
		t.Error("图片未识别时不得整篇标 OCR")
	}
	imgNote := false
	for _, a := range doc.Annotations {
		if a.Kind == "image" && strings.Contains(a.Text, "未配置 OCR") {
			imgNote = true
		}
	}
	if !imgNote {
		t.Errorf("未配置引擎的图片必须标注拒绝原因: %+v", doc.Annotations)
	}
	// 配置引擎：图片文本进页 2，文档带 OCR 标记。
	p.OCR = &fakeAdapter{name: "vision", pages: []OCRPage{{PageNum: 1, Text: "收据照片文字", Confidence: 0.7}}}
	doc, err = p.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.OCR || doc.OCREngine != "vision" || doc.Pages[2] != "收据照片文字" {
		t.Errorf("内嵌图片应经 OCR 进第 2 页: %+v", doc)
	}
	if doc.OCRConf[2] != 0.7 {
		t.Errorf("图片 OCR 置信度未透传: %v", doc.OCRConf)
	}
}

func TestRegistryRoutesDocx(t *testing.T) {
	docXML := wHeader + `<w:p><w:r><w:t>合同</w:t></w:r></w:p></w:body></w:document>`
	path := buildDocx(t, map[string]string{"word/document.xml": docXML})
	r := NewRegistry()
	doc, err := r.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Pages[1], "合同") {
		t.Errorf("Registry 未正确路由 docx: %q", doc.Pages[1])
	}
	// SetOCR 注入后 docx 图片走 OCR。
	picXML := wHeader + `<w:p><w:r><w:t>合同</w:t></w:r></w:p></w:body></w:document>`
	path2 := buildDocx(t, map[string]string{
		"word/document.xml": picXML,
		"word/media/x.jpeg": "fakejpeg",
	})
	r.SetOCR(&fakeAdapter{name: "tesseract", pages: []OCRPage{{PageNum: 1, Text: "图内文字", Confidence: 0.9}}}, nil)
	doc2, err := r.Parse(path2)
	if err != nil {
		t.Fatal(err)
	}
	if !doc2.OCR || doc2.Pages[2] != "图内文字" {
		t.Errorf("SetOCR 未注入 DocxParser: %+v", doc2)
	}
}
