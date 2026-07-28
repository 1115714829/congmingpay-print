// Package preview 把 JSON 小票排版软渲染为纸宽点阵位图(供队列预览)。
// 为近似 WYSIWYG:单码 QR/条码为本机生成,非打印机 ROM 像素。
package preview

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"congmingpay/internal/escpos"
	"congmingpay/internal/printsvc"
)

// Params 描述一次预览所需的纸面参数与源。
type Params struct {
	WidthMM    int
	SourceJSON []byte
	Reprint    bool
	HeadLines  int
	TailLines  int
	Buzzer     bool
	Cut        bool
}

// FromJob 由 printsvc.JobPreview 构造 Params。
func FromJob(j *printsvc.JobPreview) Params {
	return Params{
		WidthMM: j.WidthMM, SourceJSON: j.SourceJSON,
		Reprint: j.Reprint, HeadLines: j.HeadLines, TailLines: j.TailLines,
		Buzzer: j.Buzzer, Cut: j.Cut,
	}
}

// CanPreview 返回是否可出图及不可时的原因。
func CanPreview(j *printsvc.JobPreview) (ok bool, reason string) {
	if j == nil {
		return false, "无任务"
	}
	switch j.ContentType {
	case printsvc.ContentESC:
		return false, "原始 ESC/POS(type=1)无法还原排版预览"
	case printsvc.ContentJSON:
		if len(j.SourceJSON) == 0 {
			return false, "无排版源数据"
		}
		return true, ""
	default:
		if len(j.SourceJSON) > 0 {
			return true, ""
		}
		return false, "该任务无排版源(升级前历史任务或本地测试页无法预览)"
	}
}

// Render 按纸宽渲成卷纸位图(白底黑字)。含重打抬头、首尾空行示意。
func Render(p Params) (image.Image, error) {
	if len(p.SourceJSON) == 0 {
		return nil, fmt.Errorf("无排版源")
	}
	w := p.WidthMM
	if w != 58 && w != 80 {
		w = 80
	}
	dots := escpos.WidthDots(w)
	cpl := escpos.CharsPerLine(w)
	face, err := loadFace(float64(dots / cpl)) // ~12px
	if err != nil {
		return nil, err
	}
	c := newCanvas(dots, cpl, face)

	if p.Buzzer {
		c.noteLine("(蜂鸣已开·预览无声)")
	}
	if p.Reprint {
		line := repeatChar("*", cpl)
		c.textLine(line, alignCenter, false, 0, 0)
		c.textLine("重打小票", alignCenter, true, 1, 1)
		c.textLine("打印服务器 重印", alignCenter, false, 0, 0)
		c.textLine(line, alignCenter, false, 0, 0)
		c.feed(1)
	}
	for i := 0; i < p.HeadLines; i++ {
		c.feed(1)
	}
	if err := c.renderContents(p.SourceJSON); err != nil {
		return nil, err
	}
	tail := escpos.TailFeed(w, p.TailLines)
	for i := 0; i < tail; i++ {
		c.feed(1)
	}
	if p.Cut {
		c.noteLine("— 切纸 —")
	}
	return c.image(), nil
}

func repeatChar(ch string, n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, 0, n*len(ch))
	for i := 0; i < n; i++ {
		b = append(b, ch...)
	}
	return string(b)
}

// blankCanvas 生成白底占位(错误路径不用)。
func blankCanvas(w, h int) *image.RGBA {
	if h < 1 {
		h = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	return img
}
