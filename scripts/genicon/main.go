// Command genicon 生成应用图标 build/appicon.png（512x512）。
// 设计：深色圆角底 + 金色天平，呼应「中立法律辅助」定位。
// 用法: go run ./scripts/genicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

const size = 512

var (
	bg    = color.RGBA{15, 23, 42, 255}    // 深海军蓝
	gold  = color.RGBA{232, 184, 75, 255}  // 天平金
	plate = color.RGBA{203, 213, 225, 255} // 底座浅灰
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	radius := 96.0

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if inRoundedRect(float64(x), float64(y), 0, 0, size, size, radius) {
				img.Set(x, y, bg)
			}
		}
	}

	// 天平
	drawRect(img, 176, 404, 336, 424, plate) // 底座
	drawRect(img, 208, 388, 304, 404, plate) // 底座上层
	drawRect(img, 248, 148, 264, 388, gold)  // 立柱
	drawRect(img, 112, 148, 400, 164, gold)  // 横梁
	drawCircle(img, 256, 140, 18, gold)      // 支点

	// 左右吊盘：悬线 + 半圆盆
	drawPan(img, 128, gold)
	drawPan(img, 384, gold)

	f, err := os.Create("build/appicon.png")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}

func inRoundedRect(x, y, x0, y0, w, h, r float64) bool {
	if x < x0 || x >= x0+w || y < y0 || y >= y0+h {
		return false
	}
	cx, cy := x0+r, y0+r
	nx, ny := math.Max(cx, math.Min(x, x0+w-r)), math.Max(cy, math.Min(y, y0+h-r))
	dx, dy := x-nx, y-ny
	return dx*dx+dy*dy <= r*r || (x >= x0+r && x <= x0+w-r) || (y >= y0+r && y <= y0+h-r)
}

func drawRect(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			img.Set(x, y, c)
		}
	}
}

func drawCircle(img *image.RGBA, cx, cy, r int, c color.Color) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, c)
			}
		}
	}
}

// drawPan 在横梁端点 px 处绘制悬线与半圆吊盆。
func drawPan(img *image.RGBA, px int, c color.Color) {
	topY, panY := 164, 250
	// 悬线（V 形）
	for t := 0; t <= 100; t++ {
		fy := float64(t) / 100
		// 左线: (px, topY) -> (px-40, panY)
		img.Set(int(float64(px)+(-40)*fy), int(float64(topY)+(float64(panY)-float64(topY))*fy), c)
		img.Set(int(float64(px)+(-40)*fy)+1, int(float64(topY)+(float64(panY)-float64(topY))*fy), c)
		// 右线: (px, topY) -> (px+40, panY)
		img.Set(int(float64(px)+(40)*fy), int(float64(topY)+(float64(panY)-float64(topY))*fy), c)
		img.Set(int(float64(px)+(40)*fy)-1, int(float64(topY)+(float64(panY)-float64(topY))*fy), c)
	}
	// 半圆盆（下半圆，半径 44）
	r := 44
	for y := panY; y <= panY+r; y++ {
		dy := y - panY
		half := int(math.Sqrt(float64(r*r - dy*dy)))
		for x := px - half; x <= px+half; x++ {
			img.Set(x, y, c)
		}
	}
	// 盆口横线
	drawRect(img, px-r, panY-2, px+r, panY+1, c)
}
