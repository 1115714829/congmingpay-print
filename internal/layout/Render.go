package layout

import (
	"bytes"
	"encoding/json"
	"fmt"

	"congmingpay/internal/errcode"
	"congmingpay/internal/escpos"
)

// Render 把 JSON 小票排版数组渲染为 ESC/POS 字节。widthMM 为纸宽(58/80)。
// 全部严格:任一元素非法即整单拒绝,error 带 contents[i](种类) 定位与全局错误码,不产出部分字节。
func Render(contents []byte, widthMM int) ([]byte, error) {
	var elems []Element
	if err := json.Unmarshal(contents, &elems); err != nil {
		return nil, errcode.Wrap(errcode.RenderNotArray, err)
	}
	r := &renderer{
		b:     escpos.NewBuilder(),
		width: widthMM,
		cpl:   escpos.CharsPerLine(widthMM),
	}
	for i := range elems {
		if err := r.element(&elems[i]); err != nil {
			return nil, errcode.Wrap(errcode.RenderInvalid, fmt.Errorf("contents[%d](%s): %v", i, kindOf(&elems[i]), err))
		}
	}
	data, err := r.b.Bytes()
	if err != nil {
		return nil, errcode.Wrap(errcode.EncodeFailed, err) // GB18030 编码失败(无元素定位)
	}
	return data, nil
}

type renderer struct {
	b            *escpos.Builder
	width        int
	cpl          int
	qrSizeWarned bool // qrcode 的 size 暂不支持:每次渲染至多落一条警告日志
}

func (r *renderer) element(e *Element) error {
	switch {
	case hasJSON(e.Thead):
		return r.table(e)
	case len(e.Tbody) > 0:
		return fmt.Errorf("有 tbody 但缺 thead")
	case e.BothSides != nil:
		return r.bothSides(e)
	}
	switch e.Type {
	case "", "text": // 无 type = text(文档声明的默认)
		return r.text(e)
	case "title":
		return r.title(e)
	case "div_line":
		return r.divider(e, "-")
	case "div_star":
		return r.divider(e, "*")
	case "qrcode":
		return r.qrcode(e)
	case "bc128":
		return r.barcode(e, "128")
	case "code39":
		return r.barcode(e, "39")
	case "png":
		return r.png(e)
	default:
		return fmt.Errorf("未知元素 type %q(支持 text/title/div_line/div_star/qrcode/bc128/code39/png)", e.Type)
	}
}

// kindOf 返回元素的种类标签,用于错误定位展示。
func kindOf(e *Element) string {
	switch {
	case hasJSON(e.Thead) || len(e.Tbody) > 0:
		return "table"
	case e.BothSides != nil:
		return "both_sides"
	case e.Type == "":
		return "text"
	}
	return e.Type
}

// hasJSON 判断 RawMessage 是否为实质内容(非缺失、非 JSON null)。
// json.RawMessage 对显式 "null" 保留 4 字节,故不能只看 len——须排除 null,
// 与 Element.contString/contArray 对 cont 的 null=缺失处理保持一致。
func hasJSON(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && string(t) != "null"
}
