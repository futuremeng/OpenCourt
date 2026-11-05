// 扫描型 PDF 取图（plan-v0.4 D1）：pdfcpu（纯 Go，Apache-2.0）抽取每页
// 内嵌图像——绕开 PDFium/CGO 渲染。取不到图即报错，绝不猜测内容。
package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ExtractEmbeddedImages 按页归组返回内嵌图像的临时文件路径。
// pdfcpu 产物命名 <base>_<页码零填充>_<资源名>.<ext>——base 可能含下划线，
// 故从尾部倒数第 2 字段取页码；同页多图按资源名排序保证确定性。
func ExtractEmbeddedImages(path string) ([]PageImages, func(), error) {
	outDir, err := os.MkdirTemp("", "opencourt-scan-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { os.RemoveAll(outDir) }
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	if err := api.ExtractImagesFile(context.Background(), path, outDir, nil, conf); err != nil {
		cleanup()
		return nil, nil, err
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	type item struct {
		path string
		page int
		name string
	}
	items := []item{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		fields := strings.Split(stem, "_")
		if len(fields) < 2 {
			continue
		}
		page, err := strconv.Atoi(fields[len(fields)-2])
		if err != nil || page < 1 {
			continue
		}
		items = append(items, item{path: filepath.Join(outDir, name), page: page, name: name})
	}
	if len(items) == 0 {
		cleanup()
		return nil, nil, fmt.Errorf("PDF 无内嵌图像（既无文本层也无图像层——拒绝猜测内容）")
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].page != items[j].page {
			return items[i].page < items[j].page
		}
		return items[i].name < items[j].name
	})
	pages := []PageImages{}
	for _, it := range items {
		if n := len(pages); n > 0 && pages[n-1].Page == it.page {
			pages[n-1].Paths = append(pages[n-1].Paths, it.path)
			continue
		}
		pages = append(pages, PageImages{Page: it.page, Paths: []string{it.path}})
	}
	return pages, cleanup, nil
}
