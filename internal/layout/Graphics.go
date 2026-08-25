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

	qrcode "github.com/skip2/go-qrcode"
)

const (
	qrMaxBytes  = 512       // 二维码内容上限(字节)
	qrDefault   = 4         // qrcode size 缺省 = 厂商默认 "04"
	maxPNGBytes = 60 * 1024 // png 解码后字节上限
	maxBMPBytes = 6 * 1024  // bmp 解码后字节上限
)

func (r *renderer) qrcode(e *Element) error {
	if err := r.applyAlign(e.Align); err != nil {
		return err
	}
	m, err := qrSize(e.Size)
	if err != nil {
		return err
	}
	arr, err := e.contArray()
	if err != nil {
		return err
	}
	if arr != nil {
		if len(arr) != 2 {
			return fmt.Errorf("cont 为数组时需恰含 2 项(双码),实得 %d 项", len(arr))
		}
		for _, s := range arr {
			if len(s) > qrMaxBytes {
				return fmt.Errorf("二维码内容过长(%d 字节,上限 %d)", len(s), qrMaxBytes)
			}
		}
		if err := r.doubleQR(arr[0], arr[1], m); err != nil {
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
		if len(s) > qrMaxBytes {
			return fmt.Errorf("二维码内容过长(%d 字节,上限 %d)", len(s), qrMaxBytes)
		}
		r.b.QRCode(s, m) // 单码走 native
	}
	r.b.SetAlign(escpos.AlignLeft)
	return nil
}

// qrSize 解析 qrcode 的 size "01"-"09" → 模块大小;缺省 = "04"。
func qrSize(size string) (int, error) {
	if size == "" {
		return qrDefault, nil
	}
	if len(size) == 2 && size[0] == '0' && size[1] >= '1' && size[1] <= '9' {
		return int(size[1] - '0'), nil
	}
	return 0, fmt.Errorf("size %q 非法(qrcode 需两位 01-09 数字,如 \"04\")", size)
}

// doubleQR 并排双码 → 合成位图光栅。moduleSize 决定位图边长(30×模块,模块 6=180 与旧默认一致)。
func (r *renderer) doubleQR(a, b string, moduleSize int) error {
	ia, err := genQR(a, moduleSize)
	if err != nil {
		return fmt.Errorf("第 1 个二维码生成失败: %v", err)
	}
	ib, err := genQR(b, moduleSize)
	if err != nil {
		return fmt.Errorf("第 2 个二维码生成失败: %v", err)
	}
	r.b.Raw(escpos.Raster(composeSideBySide(ia, ib, 24), escpos.WidthDots(r.width)))
	return nil
}

func genQR(data string, moduleSize int) (image.Image, error) {
	q, err := qrcode.New(data, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	return q.Image(30 * moduleSize), nil
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

func (r *renderer) barcode(e *Element, typ string) error {
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
	switch typ {
	case "code39":
		r.b.Code39(s, h, mw, e.Hri)
	case "bc128c":
		if !allDigits(s) {
			return fmt.Errorf("bc128c 内容仅支持数字(0-9)")
		}
		if len(s) > 28 {
			return fmt.Errorf("bc128c 内容过长(%d 位,上限 28)", len(s))
		}
		r.b.Code128(s, 'C', h, mw, e.Hri)
	default: // bc128 / bc128a:Code128 码集 A
		if !upperDigits(s) {
			return fmt.Errorf("%s 内容仅支持大写字母与数字(0-9/A-Z)", typ)
		}
		if len(s) > 14 {
			return fmt.Errorf("%s 内容过长(%d 位,上限 14)", typ, len(s))
		}
		r.b.Code128(s, 'A', h, mw, e.Hri)
	}
	r.b.SetAlign(escpos.AlignLeft)
	return nil
}

// barcodeSize 条码规格 "AB":A=模块宽 1-6(默认 2,GS w 下限按 2 夹取)、B=高度档 0-9
// (高度=(B+1)×24 点,8 点=1mm,默认 B=2)。缺省 = A2 B2 → (72,2)。
func barcodeSize(size string) (heightDots, moduleWidth int, err error) {
	if size == "" {
		return 72, 2, nil
	}
	if len(size) != 2 || size[0] < '1' || size[0] > '6' || size[1] < '0' || size[1] > '9' {
		return 0, 0, fmt.Errorf("size %q 非法(条码需两位:模块宽 1-6 + 高度档 0-9,如 \"22\")", size)
	}
	mw := int(size[0] - '0')
	if mw < 2 {
		mw = 2
	}
	h := (int(size[1]-'0') + 1) * 24
	return h, mw, nil
}

// upperDigits 是否仅含大写字母与数字(Code128 码集 A 的常见内容)。
func upperDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// allDigits 是否仅含数字(Code128 码集 C)。
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func (r *renderer) png(e *Element) error {
	if err := r.applyAlign(e.Align); err != nil {
		return err
	}
	s, err := e.contString()
	if err != nil {
		return err
	}
	data, err := FetchImageBytes(s, "png")
	if err != nil {
		return err
	}
	if len(data) > maxPNGBytes {
		return fmt.Errorf("png 内容过大(%d 字节,上限 60K)", len(data))
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("png 解码失败: %v", err)
	}
	r.b.Raw(escpos.Raster(img, escpos.WidthDots(r.width)))
	r.b.SetAlign(escpos.AlignLeft)
	return nil
}

// bmp 元素:24/32 位未压缩 BMP(自研解码器),来源与 png 相同;字节上限 6K。
func (r *renderer) bmp(e *Element) error {
	if err := r.applyAlign(e.Align); err != nil {
		return err
	}
	s, err := e.contString()
	if err != nil {
		return err
	}
	data, err := FetchImageBytes(s, "bmp")
	if err != nil {
		return err
	}
	if len(data) > maxBMPBytes {
		return fmt.Errorf("bmp 内容过大(%d 字节,上限 6K)", len(data))
	}
	img, err := DecodeBMP(data)
	if err != nil {
		return err
	}
	r.b.Raw(escpos.Raster(img, escpos.WidthDots(r.width)))
	r.b.SetAlign(escpos.AlignLeft)
	return nil
}

// FetchImageBytes 按来源取图片原始字节(data URI 或 http(s) URL);任何失败即报错(整单拒)。
// kind 用于错误文案(如 "png"/"bmp")。URL 下载仍保留 8MB 截断兜底,具体字节上限由调用方校验。
func FetchImageBytes(src, kind string) ([]byte, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("%s 内容为空", kind)
	}
	switch {
	case strings.HasPrefix(src, "data:"):
		i := strings.Index(src, ",")
		if i < 0 {
			return nil, fmt.Errorf("%s 来源非法(data: 需为 data:image/...;base64,<数据>)", kind)
		}
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src[i+1:]))
		if err != nil {
			return nil, fmt.Errorf("%s base64 解码失败: %v", kind, err)
		}
		return b, nil
	case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
		cli := &http.Client{Timeout: 10 * time.Second}
		resp, err := cli.Get(src)
		if err != nil {
			return nil, fmt.Errorf("%s 下载失败: %v", kind, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s 下载失败: HTTP %d", kind, resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil, fmt.Errorf("%s 下载读取失败: %v", kind, err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("%s 来源非法(需 http(s) URL 或 data:image/...;base64,)", kind)
	}
}
