package ingest

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"

	"github.com/futuremeng/OpenCourt/core/verify"
)

// parseDocumentXML 抽取正文（含表格线性化）与修订痕迹标注。
// 修订文本**不进正文**：w:ins 内容进「插入」标注，w:del 内容进「删除」标注。
func parseDocumentXML(data []byte) (string, []verify.Annotation, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var (
		sb        strings.Builder
		inTable   int
		inCell    bool
		inIns     int
		inDel     int
		insAuthor string
		delAuthor string
		insBuf    strings.Builder
		delBuf    strings.Builder
		revisions []verify.Annotation
	)
	for {
		tk, tokErr := dec.Token()
		if tokErr == io.EOF {
			break
		}
		if tokErr != nil {
			return "", nil, tokErr
		}
		switch el := tk.(type) {
		case xml.StartElement:
			switch wlocal(el.Name) {
			case "tbl":
				inTable++
				if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
					sb.WriteByte('\n')
				}
				sb.WriteString("[表格已线性化]\n")
			case "tr":
				if inTable > 0 && !strings.HasSuffix(sb.String(), "\n") {
					sb.WriteByte('\n')
				}
				inCell = false
			case "tc":
				if inTable > 0 && inCell {
					sb.WriteString(" | ")
				}
				inCell = true
			case "ins":
				inIns++
				if inIns == 1 {
					insAuthor = wattr(el.Attr, "author")
					insBuf.Reset()
				}
			case "del":
				inDel++
				if inDel == 1 {
					delBuf.Reset()
					delAuthor = wattr(el.Attr, "author")
				}
			case "tab":
				writeTo(&sb, inIns, inDel, &insBuf, &delBuf, "\t")
			case "br", "cr":
				writeTo(&sb, inIns, inDel, &insBuf, &delBuf, "\n")
			case "t":
				var txt string
				if err := dec.DecodeElement(&txt, &el); err != nil {
					return "", nil, err
				}
				writeTo(&sb, inIns, inDel, &insBuf, &delBuf, txt)
			case "delText":
				var txt string
				if err := dec.DecodeElement(&txt, &el); err != nil {
					return "", nil, err
				}
				if inDel > 0 {
					delBuf.WriteString(txt)
				}
			}
		case xml.EndElement:
			switch wlocal(el.Name) {
			case "tbl":
				if inTable > 0 {
					inTable--
				}
				sb.WriteByte('\n')
			case "tr":
				if inTable > 0 {
					sb.WriteByte('\n')
				}
			case "p":
				if inTable == 0 {
					sb.WriteByte('\n')
				}
			case "ins":
				if inIns == 1 {
					if t := strings.TrimSpace(insBuf.String()); t != "" {
						revisions = append(revisions, verify.Annotation{
							Kind: "revision_insert", Author: insAuthor, Text: t,
							Meta: "修订痕迹（插入，未接受），仅作线索",
						})
					}
				}
				if inIns > 0 {
					inIns--
				}
			case "del":
				if inDel == 1 {
					if t := strings.TrimSpace(delBuf.String()); t != "" {
						revisions = append(revisions, verify.Annotation{
							Kind: "revision_delete", Author: delAuthor, Text: t,
							Meta: "修订痕迹（被删除文本），仅作线索",
						})
					}
				}
				if inDel > 0 {
					inDel--
				}
			}
		}
	}
	return sb.String(), revisions, nil
}

// writeTo 把文本路由到当前目标：修订删除优先，其次插入，否则正文。
// 嵌套修订（ins 内 del）按删除优先——插入块里的删除线文本不算插入内容。
func writeTo(body *strings.Builder, inIns, inDel int, insBuf, delBuf *strings.Builder, txt string) {
	switch {
	case inDel > 0:
		delBuf.WriteString(txt)
	case inIns > 0:
		insBuf.WriteString(txt)
	default:
		body.WriteString(txt)
	}
}

// parseCommentsXML 抽取批注（作者 + 全文）。
func parseCommentsXML(data []byte) ([]verify.Annotation, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var (
		out       []verify.Annotation
		inComment bool
		author    string
		buf       strings.Builder
	)
	for {
		tk, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch el := tk.(type) {
		case xml.StartElement:
			switch wlocal(el.Name) {
			case "comment":
				inComment = true
				author = wattr(el.Attr, "author")
				buf.Reset()
			case "p":
				if inComment {
					buf.WriteByte('\n')
				}
			case "t":
				if inComment {
					var txt string
					if err := dec.DecodeElement(&txt, &el); err != nil {
						return nil, err
					}
					buf.WriteString(txt)
				}
			}
		case xml.EndElement:
			if wlocal(el.Name) == "comment" {
				if t := strings.TrimSpace(buf.String()); t != "" {
					out = append(out, verify.Annotation{
						Kind: "comment", Author: author, Text: t,
						Meta: "批注，仅作线索，不作正文引用来源",
					})
				}
				inComment = false
			}
		}
	}
	return out, nil
}
