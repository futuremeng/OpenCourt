// docx 辅助：媒体 OCR、zip 读取（含 zip 炸弹防护）、OOXML 命名空间工具。
package ingest

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// recognizeMediaFile 解出媒体文件到临时目录并 OCR（引擎只认文件路径）。
func (d *DocxParser) recognizeMediaFile(f *zip.File) (string, float64, error) {
	rc, err := f.Open()
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()
	tmp, err := os.CreateTemp("", "opencourt-docx-img-*"+path.Ext(f.Name))
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(rc, docxEntryLimit)); err != nil {
		tmp.Close()
		return "", 0, err
	}
	tmp.Close()
	pages, err := d.OCR.Recognize(context.Background(), tmp.Name())
	if err != nil {
		return "", 0, err
	}
	var sb strings.Builder
	var conf float64
	for _, pg := range pages {
		sb.WriteString(pg.Text)
		if pg.Confidence > 0 {
			conf = pg.Confidence
		}
	}
	if sb.Len() == 0 {
		return "", 0, errors.New("OCR 未产出文本")
	}
	return sb.String(), conf, nil
}

func isImageName(name string) bool {
	l := strings.ToLower(name)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff"} {
		if strings.HasSuffix(l, ext) {
			return true
		}
	}
	return false
}

func isOLEFile(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	var head [4]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return false
	}
	return head[0] == 0xD0 && head[1] == 0xCF && head[2] == 0x11 && head[3] == 0xE0
}

func readZipEntry(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, docxEntryLimit+1))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > docxEntryLimit {
			return nil, fmt.Errorf("%s 超过 %d MB 解压上限（zip 炸弹防护）", name, docxEntryLimit>>20)
		}
		return data, nil
	}
	return nil, errors.New("部件不存在: " + name)
}

const wmlNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

func wlocal(name xml.Name) string {
	if name.Space == wmlNS || name.Space == "" {
		return name.Local
	}
	return ""
}

func wattr(attrs []xml.Attr, local string) string {
	for _, a := range attrs {
		if wlocal(a.Name) == local {
			return a.Value
		}
	}
	return ""
}
