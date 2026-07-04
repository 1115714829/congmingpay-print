package layout

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"  // 解码器
	_ "image/jpeg" // 解码器
	_ "image/png"  // 解码器
	"io"
	"net/http"
	"strings"
	"time"

	"congmingpay/internal/escpos"

	qrcode "github.com/skip2/go-qrcode"
)

func (r *renderer) qrcode(e *Element) {
	r.applyAlign(e.Align)
	if arr := e.contArray(); len(arr) >= 2 {
		r.doubleQR(arr[0], arr[1]) // 并排双码 → 合成位图光栅
	} else {
		r.b.QRCode(e.contString(), qrModuleSize(e.Size)) // 单码走 native
	}
	r.b.SetAlign(escpos.AlignLeft)
}

// qrModuleSize:demo 的 "60"/"06" 含义未明,先取默认模块点大小 6;真机再调。
func qrModuleSize(size string) int { return 6 }

func (r *renderer) doubleQR(a, b string) {
	ia, ib := genQR(a), genQR(b)
	if ia == nil || ib == nil {
		return
	}
	r.b.Raw(escpos.Raster(composeSideBySide(ia, ib, 24), escpos.WidthDots(r.width)))
}

func genQR(data string) image.Image {
	q, err := qrcode.New(data, qrcode.Medium)
	if err != nil {
		return nil
	}
	return q.Image(180)
}

func composeSideBySide(a, b image.Image, gap int) image.Image {
	aw, ah := a.Bounds().Dx(), a.Bounds().Dy()
	bw, bh := b.Bounds().Dx(), b.Bounds().Dy()
	h := ah
	if bh > h {
		h = bh
	}
	w := aw + gap + bw
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(0, 0, aw, ah), a, a.Bounds().Min, draw.Over)
	draw.Draw(dst, image.Rect(aw+gap, 0, aw+gap+bw, bh), b, b.Bounds().Min, draw.Over)
	return dst
}

func (r *renderer) barcode(e *Element, kind string) {
	r.applyAlign(e.Align)
	h, mw := barcodeSize(e.Size)
	if kind == "39" {
		r.b.Code39(e.contString(), h, mw, e.Hri)
	} else {
		r.b.Code128(e.contString(), h, mw, e.Hri)
	}
	r.b.SetAlign(escpos.AlignLeft)
}

func barcodeSize(size string) (heightDots, moduleWidth int) {
	switch size {
	case "22":
		return 90, 3
	case "33":
		return 120, 4
	default:
		return 60, 2
	}
}

func (r *renderer) png(e *Element) {
	img := loadImage(e.contString())
	if img == nil {
		return
	}
	r.applyAlign(e.Align)
	r.b.Raw(escpos.Raster(img, escpos.WidthDots(r.width)))
	r.b.SetAlign(escpos.AlignLeft)
}

// loadImage 支持 http(s) URL 与 data:image/...;base64, 两种来源。
func loadImage(src string) image.Image {
	var data []byte
	switch {
	case strings.HasPrefix(src, "data:"):
		if i := strings.Index(src, ","); i >= 0 {
			b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src[i+1:]))
			if err != nil {
				return nil
			}
			data = b
		}
	case strings.HasPrefix(src, "http"):
		cli := &http.Client{Timeout: 10 * time.Second}
		resp, err := cli.Get(src)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil
		}
		data = b
	default:
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}
