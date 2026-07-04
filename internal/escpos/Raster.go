package escpos

import "image"

// Raster 把图像转为单色位图并生成 GS v 0 打印指令(1D 76 30 m ...)。
// maxWidthDots 为纸宽点数(58=384,80=576);图像等比缩放到不超过该宽,宽取 8 的倍数。
// 采用 Floyd-Steinberg 误差扩散抖动,适合 logo/照片。
func Raster(src image.Image, maxWidthDots int) []byte {
	bnd := src.Bounds()
	sw, sh := bnd.Dx(), bnd.Dy()
	if sw <= 0 || sh <= 0 {
		return nil
	}
	w := sw
	if w > maxWidthDots {
		w = maxWidthDots
	}
	w = w / 8 * 8
	if w < 8 {
		w = 8
	}
	h := sh * w / sw
	if h < 1 {
		h = 1
	}

	// 缩放取样 + 灰度
	gray := make([]float64, w*h)
	for y := 0; y < h; y++ {
		sy := bnd.Min.Y + y*sh/h
		for x := 0; x < w; x++ {
			sx := bnd.Min.X + x*sw/w
			cr, cg, cb, ca := src.At(sx, sy).RGBA()
			r8, g8, b8 := float64(cr>>8), float64(cg>>8), float64(cb>>8)
			a := float64(ca>>8) / 255
			lum := 0.299*r8 + 0.587*g8 + 0.114*b8
			gray[y*w+x] = lum*a + 255*(1-a) // 透明视为白
		}
	}

	// Floyd-Steinberg 二值化
	rowBytes := w / 8
	out := make([]byte, rowBytes*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			old := gray[y*w+x]
			nv := 255.0
			if old < 128 {
				nv = 0
				out[y*rowBytes+x/8] |= 0x80 >> uint(x%8)
			}
			e := old - nv
			if x+1 < w {
				gray[y*w+x+1] += e * 7 / 16
			}
			if y+1 < h {
				if x > 0 {
					gray[(y+1)*w+x-1] += e * 3 / 16
				}
				gray[(y+1)*w+x] += e * 5 / 16
				if x+1 < w {
					gray[(y+1)*w+x+1] += e * 1 / 16
				}
			}
		}
	}

	cmd := []byte{
		0x1D, 0x76, 0x30, 0x00,
		byte(rowBytes % 256), byte(rowBytes / 256),
		byte(h % 256), byte(h / 256),
	}
	return append(cmd, out...)
}
