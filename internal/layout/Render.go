package layout

import (
	"encoding/json"

	"congmingpay/internal/escpos"
)

// Render 把 JSON 小票排版数组渲染为 ESC/POS 字节。widthMM 为纸宽(58/80)。
func Render(contents []byte, widthMM int) ([]byte, error) {
	var elems []Element
	if err := json.Unmarshal(contents, &elems); err != nil {
		return nil, err
	}
	r := &renderer{
		b:     escpos.NewBuilder(),
		width: widthMM,
		cpl:   escpos.CharsPerLine(widthMM),
	}
	for i := range elems {
		r.element(&elems[i])
	}
	return r.b.Bytes()
}

type renderer struct {
	b     *escpos.Builder
	width int
	cpl   int
}

func (r *renderer) element(e *Element) {
	switch {
	case len(e.Thead) > 0:
		r.table(e)
	case e.BothSides != nil:
		r.bothSides(e)
	default:
		switch e.Type {
		case "title":
			r.title(e)
		case "div_line":
			r.divider(e, "-")
		case "div_star":
			r.divider(e, "*")
		case "qrcode":
			r.qrcode(e)
		case "bc128":
			r.barcode(e, "128")
		case "code39":
			r.barcode(e, "39")
		case "png":
			r.png(e)
		case "cut":
			// 收尾切纸由 printsvc 按打印机「切刀」设置统一执行,这里跳过
			_ = e
		default: // "text" 或无 type
			r.text(e)
		}
	}
}
