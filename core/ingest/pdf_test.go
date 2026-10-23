package ingest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildTextPDF 构造最小可解析 PDF：每页一行 Helvetica 文本。
// xref 偏移按对象写入顺序记录，保证结构合法。
func buildTextPDF(lines ...string) []byte {
	n := len(lines)
	fontNum := 3 + 2*n
	total := fontNum // 对象总数
	var buf bytes.Buffer
	offsets := []int{}
	write := func(format string, args ...any) {
		fmt.Fprintf(&buf, format, args...)
	}
	addObj := func(num int, body string) {
		offsets = append(offsets, buf.Len())
		write("%d 0 obj\n%s\nendobj\n", num, body)
	}

	write("%%PDF-1.2\n")
	addObj(1, "<< /Type /Catalog /Pages 2 0 R >>")

	kids := ""
	for i := 0; i < n; i++ {
		kids += fmt.Sprintf("%d 0 R ", 3+2*i)
	}
	addObj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, n))

	for i, line := range lines {
		pageNum := 3 + 2*i
		contentNum := pageNum + 1
		escaped := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(line)
		stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", escaped)
		addObj(pageNum, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>",
			contentNum, fontNum))
		addObj(contentNum, fmt.Sprintf(
			"<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	addObj(fontNum, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	xrefStart := buf.Len()
	write("xref\n0 %d\n0000000000 65535 f \n", total+1)
	for _, off := range offsets {
		write("%010d 00000 n \n", off)
	}
	write("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", total+1, xrefStart)
	return buf.Bytes()
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPDFParserExtractsPerPageText(t *testing.T) {
	path := writeTemp(t, "case.pdf", buildTextPDF(
		"2023 nian 5 yue 10 ri, yuan-gao xiang bei-gao zhi-fu ding-jin 50000 yuan.",
		"bei-gao wei yue-ding qi-huo, gou-cheng wei-yue.",
	))
	p := &PDFParser{}
	if !p.CanParse(path) {
		t.Fatal("CanParse 应识别 .pdf")
	}
	doc, err := p.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Pages) != 2 {
		t.Fatalf("页数错误: %d", len(doc.Pages))
	}
	if !strings.Contains(doc.Pages[1], "50000") {
		t.Errorf("第 1 页文本缺失: %q", doc.Pages[1])
	}
	if !strings.Contains(doc.Pages[2], "wei-yue") {
		t.Errorf("第 2 页文本缺失: %q", doc.Pages[2])
	}
}

func TestPDFParserRejectsScannedPDF(t *testing.T) {
	// 页面无任何文本（模拟扫描件）：必须拒绝并提示需经 OCR，绝不产出空文本。
	path := writeTemp(t, "scan.pdf", buildTextPDF(""))
	_, err := (&PDFParser{}).Parse(path)
	if err == nil {
		t.Fatal("扫描型 PDF 必须被拒绝")
	}
	if !strings.Contains(err.Error(), "OCR") {
		t.Errorf("错误应明确指向需经 OCR: %v", err)
	}
}

func TestRegistryRoutesPDF(t *testing.T) {
	path := writeTemp(t, "a.pdf", buildTextPDF("hello"))
	r := NewRegistry()
	doc, err := r.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.File != path {
		t.Errorf("File 字段错误: %s", doc.File)
	}
}

func TestOCRParserRefusesWithoutEngine(t *testing.T) {
	r := NewRegistry()
	_, err := r.Parse(filepath.Join(t.TempDir(), "scan.png"))
	if err == nil || !strings.Contains(err.Error(), "OCR") {
		t.Fatalf("未配置 OCR 引擎时图片必须被拒绝: %v", err)
	}
}
