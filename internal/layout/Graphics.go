package layout

import (
	"bytes"
	"encoding/base64"
	"fmt"
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
	"congmingpay/internal/logger"

	qrcode "github.com/skip2/go-qrcode"
)

func (r *renderer) qrcode(e *Element) error {
	if err := r.applyAlign(e.Align); err != nil {
		return err
	}
	// size 暂不支持(demo 的 "60"/"06" 含义未明,恒按模块大小 6 渲染):不拒,每次渲染至多警告一次。
	if e.Size != "" && !r.qrSizeWarned {
		r.qrSizeWarned = true
		logger.Warnf("排版警告: qrcode 的 size %q 暂不支持,按默认模块大小 6 渲染", e.Size)
	}
	arr, err := e.contArray()
	if err != nil {
		return err
	}
	if arr != nil {
		if len(arr) != 2 {
			return fmt.Errorf("cont 为数组时需恰含 2 项(双码),实得 %d 项", len(arr))
		}
		if err := r.doubleQR(arr[0], arr[1]); err != nil {
			return err
		}
	} else {
		s, err := e.contString()
		if err != nil {
			return err
		}
		if s == "" {
			return fmt.Errorf("二维码内容为空")
		}
		// QR Model2 容量上限约 2953 字节;超出既无法编码、又会让 GS ( k 存储长度字节回绕,直接拒。
		if len(s) > 2953 {
			return fmt.Errorf("二维码内容过长(%d 字节,上限 2953)", len(s))
		}
		r.b.QRCode(s, 6) // 单码走 native
	}
	r.b.SetAlign(escpos.AlignLeft)
	return nil
}

// doubleQR 并排双码 → 合成位图光栅。
func (r *renderer) doubleQR(a, b string) error {
	ia, err := genQR(a)
	if err != nil {
		return fmt.Errorf("第 1 个二维码生成失败: %v", err)
	}
	ib, err := genQR(b)
	if err != nil {
		return fmt.Errorf("第 2 个二维码生成失败: %v", err)
	}
	r.b.Raw(escpos.Raster(composeSideBySide(ia, ib, 24), escpos.WidthDots(r.width)))
	return nil
}

func genQR(data string) (image.Image, error) {
	q, err := qrcode.New(data, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	return q.Image(180), nil
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

func (r *renderer) barcode(e *Element, kind string) error {
	if err := r.applyAlign(e.Align); err != nil {
		return err
	}
	h, mw, err := barcodeSize(e.Size)
	if err != nil {
		return err
	}
	if e.Hri < 0 || e.Hri > 3 {
		return fmt.Errorf("hri %d 非法(0=无 1=上 2=下 3=上下)", e.Hri)
	}
	s, err := e.contString()
	if err != nil {
		return err
	}
	if s == "" {
		return fmt.Errorf("条码内容为空")
	}
	// GS k 的长度字节为单字节:内容过长会回绕产出损坏指令流,直接拒(厨房条码=订单号,远短于此)。
	if len(s) > 250 {
		return fmt.Errorf("条码内容过长(%d 字节,上限 250)", len(s))
	}
	if kind == "39" {
		r.b.Code39(s, h, mw, e.Hri)
	} else {
		r.b.Code128(s, h, mw, e.Hri)
	}
	r.b.SetAlign(escpos.AlignLeft)
	return nil
}

// barcodeSize 条码规格:缺失=默认(高60,模块宽2);"22"/"33" 放大;其余非法即拒。
func barcodeSize(size string) (heightDots, moduleWidth int, err error) {
	switch size {
	case "":
		return 60, 2, nil
	case "22":
		return 90, 3, nil
	case "33":
		return 120, 4, nil
	default:
		return 0, 0, fmt.Errorf("size %q 非法(条码仅支持 \"22\"/\"33\" 或缺省)", size)
	}
}

func (r *renderer) png(e *Element) error {
	if err := r.applyAlign(e.Align); err != nil {
		return err
	}
	s, err := e.contString()
	if err != nil {
		return err
	}
	img, err := loadImage(s)
	if err != nil {
		return err
	}
	r.b.Raw(escpos.Raster(img, escpos.WidthDots(r.width)))
	r.b.SetAlign(escpos.AlignLeft)
	return nil
}

// loadImage 支持 http(s) URL 与 data:image/...;base64, 两种来源;任何失败即报错(整单拒)。
func loadImage(src string) (image.Image, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("png 内容为空")
	}
	var data []byte
	switch {
	case strings.HasPrefix(src, "data:"):
		i := strings.Index(src, ",")
		if i < 0 {
			return nil, fmt.Errorf("png 来源非法(data: 需为 data:image/...;base64,<数据>)")
		}
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src[i+1:]))
		if err != nil {
			return nil, fmt.Errorf("png base64 解码失败: %v", err)
		}
		data = b
	case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
		cli := &http.Client{Timeout: 10 * time.Second}
		resp, err := cli.Get(src)
		if err != nil {
			return nil, fmt.Errorf("png 下载失败: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("png 下载失败: HTTP %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil, fmt.Errorf("png 下载读取失败: %v", err)
		}
		data = b
	default:
		return nil, fmt.Errorf("png 来源非法(需 http(s) URL 或 data:image/...;base64,)")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("png 解码失败: %v", err)
	}
	return img, nil
}
