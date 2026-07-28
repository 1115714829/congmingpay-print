package preview

import (
	"image"
	"image/color"
	"image/draw"
	"strings"

	"congmingpay/internal/escpos"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	alignLeft = iota
	alignCenter
	alignRight
)

type canvas struct {
	width, cpl, colW int
	face             font.Face
	rows             []image.Image // each row strip
	yHint            int
}

func newCanvas(dots, cpl int, face font.Face) *canvas {
	colW := dots / cpl
	if colW < 8 {
		colW = 8
	}
	return &canvas{width: dots, cpl: cpl, colW: colW, face: face}
}

func (c *canvas) lineH(magH int) int {
	h := c.colW * 2 // Font A ~24 dots tall
	if magH > 0 {
		h *= magH + 1
	}
	m := c.face.Metrics().Height.Ceil()
	if m > h {
		h = m + 2
	}
	return h
}

func (c *canvas) feed(n int) {
	for i := 0; i < n; i++ {
		c.appendStrip(c.blankStrip(c.lineH(0)))
	}
}

func (c *canvas) blankStrip(h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.width, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	return img
}

func (c *canvas) appendStrip(img image.Image) {
	c.rows = append(c.rows, img)
	c.yHint += img.Bounds().Dy()
}

func (c *canvas) noteLine(s string) {
	h := c.lineH(0)
	img := c.blankStrip(h)
	c.drawString(img, s, 0, h-4, color.RGBA{R: 120, G: 120, B: 120, A: 255}, false)
	c.appendStrip(img)
}

func (c *canvas) textLine(s string, align int, bold bool, magW, magH int) {
	// 放大:按显示列折行后逐行画;mag 通过字号近似
	cpl := c.cpl
	if magW > 0 {
		cpl = cpl / (magW + 1)
		if cpl < 1 {
			cpl = 1
		}
	}
	lines := wrapByWidth(s, cpl)
	h := c.lineH(magH)
	face := c.face
	for _, ln := range lines {
		img := c.blankStrip(h)
		x := c.alignX(ln, align, magW)
		c.drawStringFace(img, ln, x, h-3, color.Black, face, bold, magW, magH)
		c.appendStrip(img)
	}
}

func (c *canvas) alignX(s string, align, magW int) int {
	dw := escpos.DisplayWidth(s)
	cell := c.colW
	if magW > 0 {
		cell *= magW + 1
	}
	textW := dw * cell
	switch align {
	case alignCenter:
		x := (c.width - textW) / 2
		if x < 0 {
			return 0
		}
		return x
	case alignRight:
		x := c.width - textW
		if x < 0 {
			return 0
		}
		return x
	default:
		return 0
	}
}

func (c *canvas) drawString(img *image.RGBA, s string, x, baseline int, col color.Color, bold bool) {
	c.drawStringFace(img, s, x, baseline, col, c.face, bold, 0, 0)
}

func (c *canvas) drawStringFace(img *image.RGBA, s string, x, baseline int, col color.Color, face font.Face, bold bool, magW, magH int) {
	if s == "" {
		return
	}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(s)
	if bold {
		d.Dot = fixed.P(x+1, baseline)
		d.DrawString(s)
	}
	_ = magW
	_ = magH
}

func (c *canvas) drawImage(src image.Image, align int) {
	if src == nil {
		return
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	// 限宽:不超过纸宽
	scale := 1.0
	if sw > c.width {
		scale = float64(c.width) / float64(sw)
	}
	dw := int(float64(sw) * scale)
	dh := int(float64(sh) * scale)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	strip := c.blankStrip(dh + 4)
	x := 0
	switch align {
	case alignCenter:
		x = (c.width - dw) / 2
	case alignRight:
		x = c.width - dw
	}
	if x < 0 {
		x = 0
	}
	dst := image.Rect(x, 2, x+dw, 2+dh)
	xdraw.ApproxBiLinear.Scale(strip, dst, src, b, xdraw.Src, nil)
	c.appendStrip(strip)
}

func (c *canvas) image() *image.RGBA {
	h := c.yHint
	if h < 1 {
		h = c.lineH(0)
	}
	out := blankCanvas(c.width, h)
	y := 0
	for _, r := range c.rows {
		b := r.Bounds()
		draw.Draw(out, image.Rect(0, y, c.width, y+b.Dy()), r, b.Min, draw.Src)
		y += b.Dy()
	}
	return out
}

func wrapByWidth(s string, cw int) []string {
	if s == "" {
		return []string{""}
	}
	if cw < 1 {
		cw = 1
	}
	var lines []string
	var cur strings.Builder
	curw := 0
	for _, ru := range s {
		rw := 1
		if ru > 0x7F {
			rw = 2
		}
		if curw+rw > cw && curw > 0 {
			lines = append(lines, cur.String())
			cur.Reset()
			curw = 0
		}
		cur.WriteRune(ru)
		curw += rw
	}
	if cur.Len() > 0 || len(lines) == 0 {
		lines = append(lines, cur.String())
	}
	return lines
}
