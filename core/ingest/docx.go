// docx 解析（plan-v0.4 D2）：纯 Go zip + word/document.xml，零 CGO。
// 修订痕迹（w:ins/w:del）与批注（w:comment）**保留为独立标注区块**上浮——
// 不并入正文，回原文定位对标注文本一律失败（不自动采信）；表格线性化并
// 标注「表格已线性化」；内嵌图片走 OCR 管道——未配置引擎只拒绝该图，
// 其余照常（图片识别不了不等于文档读不了，但绝不静默丢弃）。
package ingest

import (
	"archive/zip"
	"fmt"
	"path"
	"strings"

	"github.com/futuremeng/OpenCourt/core/verify"
)

// docxEntryLimit 单条目解压上限（zip 炸弹防护——超限明确报错，绝不硬解）。
const docxEntryLimit = 64 << 20 // 64 MB

// DocxParser Office Open XML（.docx）解析器。
// 低置信黄标判定统一走 Registry.LowConfidencePages（阈值来自配置）。
type DocxParser struct {
	OCR OCRAdapter // nil = 内嵌图片不识别（标注拒绝原因），正文照常
}

// CanParse 实现 Parser 接口。.doc（旧二进制格式）也返回 true——
// 为的是给出**明确**的错误文案，而不是「没有解析器处理」。
func (d *DocxParser) CanParse(p string) bool {
	l := strings.ToLower(p)
	return strings.HasSuffix(l, ".docx") || strings.HasSuffix(l, ".doc")
}

// Parse 实现 Parser 接口。
func (d *DocxParser) Parse(p string) (*verify.DocumentText, error) {
	if strings.HasSuffix(strings.ToLower(p), ".doc") {
		return nil, fmt.Errorf("旧版二进制 .doc 无法解析：请在 Word/WPS 中另存为 .docx 后重试: %s", p)
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		// OLE 复合文档伪装成 .docx 的常见情形：给出可执行的纠正指引。
		if isOLEFile(p) {
			return nil, fmt.Errorf("该文件实为旧版 .doc（OLE 格式）：请另存为 .docx 后重试: %s", p)
		}
		return nil, fmt.Errorf("docx 无法打开（zip 损坏或不是 Office 文档）: %w", err)
	}
	defer zr.Close()

	docXML, err := readZipEntry(&zr.Reader, "word/document.xml")
	if err != nil {
		return nil, fmt.Errorf("docx 缺少 word/document.xml（文件已损坏或结构异常）: %s", p)
	}
	body, revisions, err := parseDocumentXML(docXML)
	if err != nil {
		return nil, fmt.Errorf("docx 正文解析失败: %w", err)
	}
	if strings.TrimSpace(body) == "" && len(revisions) == 0 {
		return nil, fmt.Errorf("docx 无可抽取正文（空文档或内容全部在不受支持的部件中）: %s", p)
	}

	doc := &verify.DocumentText{
		File:        p,
		Pages:       map[int]string{1: body},
		Annotations: revisions,
	}

	// 批注部件（可选）：word/comments.xml。
	if cx, err := readZipEntry(&zr.Reader, "word/comments.xml"); err == nil {
		comments, err := parseCommentsXML(cx)
		if err != nil {
			return nil, fmt.Errorf("docx 批注解析失败: %w", err)
		}
		doc.Annotations = append(doc.Annotations, comments...)
	}

	// 内嵌图片：走 OCR 管道（页码从 2 起顺排）；未配置/失败只拒绝该图。
	// 注意（D1 保守推论）：任一图 OCR 成功即整文档标记 OCR——原生正文随
	// 整档封顶弱态。宁可保守，不可让「正文看起来非 OCR」而图片页混入强态；
	// 页码顺排是展示约定，不代表文档真实页码（标注中已注明）。
	page := 2
	for _, f := range zr.File {
		name := f.Name
		if !strings.HasPrefix(name, "word/media/") || !isImageName(name) {
			continue
		}
		if d.OCR == nil {
			doc.Annotations = append(doc.Annotations, verify.Annotation{
				Kind: "image",
				Text: fmt.Sprintf("内嵌图片 %s：未配置 OCR 引擎，未识别（正文不受影响）", path.Base(name)),
			})
			continue
		}
		text, conf, err := d.recognizeMediaFile(f)
		if err != nil {
			doc.Annotations = append(doc.Annotations, verify.Annotation{
				Kind: "image",
				Text: fmt.Sprintf("内嵌图片 %s：OCR 失败（%v），未识别（正文不受影响）", path.Base(name), err),
			})
			continue
		}
		doc.Pages[page] = text
		if conf > 0 {
			if doc.OCRConf == nil {
				doc.OCRConf = map[int]float64{}
			}
			doc.OCRConf[page] = conf
		}
		doc.OCR = true
		if doc.OCREngine == "" {
			doc.OCREngine = d.OCR.Name()
		}
		doc.Annotations = append(doc.Annotations, verify.Annotation{
			Kind: "image",
			Text: fmt.Sprintf("内嵌图片 %s 已经 OCR 进入第 %d 页（引擎 %s，封顶弱态）", path.Base(name), page, d.OCR.Name()),
		})
		page++
	}
	return doc, nil
}
