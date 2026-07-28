package layout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"congmingpay/internal/errcode"
	"congmingpay/internal/escpos"
)

// Render 把 JSON 小票排版数组渲染为 ESC/POS 字节。widthMM 为纸宽(58/80)。
// compat=true(云盒兼容)时接受 type=cut 并返回切纸意图;compat=false 时 cut 为未知元素。
// 元素本身不写入 GS V,由调用方交给 Finish 收尾。
func Render(contents []byte, widthMM int, compat bool) (data []byte, contentCut *bool, err error) {
	var elems []Element
	if err := json.Unmarshal(contents, &elems); err != nil {
		return nil, nil, errcode.Wrap(errcode.RenderNotArray, err)
	}
	r := &renderer{
		b:           escpos.NewBuilder(),
		width:       widthMM,
		cpl:         escpos.CharsPerLine(widthMM),
		yunheCompat: compat,
	}
	for i := range elems {
		if err := r.element(&elems[i]); err != nil {
			return nil, nil, errcode.Wrap(errcode.RenderInvalid, fmt.Errorf("contents[%d](%s): %v", i, kindOf(&elems[i]), err))
		}
	}
	data, err = r.b.Bytes()
	if err != nil {
		return nil, nil, errcode.Wrap(errcode.EncodeFailed, err)
	}
	return data, r.contentCut, nil
}

type renderer struct {
	b            *escpos.Builder
	width        int
	cpl          int
	qrSizeWarned bool
	yunheCompat  bool  // 云盒兼容:允许 contents cut
	contentCut   *bool // 切纸意图(仅 compat)
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
	case "cut": // 云盒兼容 C3:接受不拒;不写 GS V,只记切纸意图
		if !r.yunheCompat {
			return fmt.Errorf("未知元素 type %q(支持 text/title/div_line/div_star/qrcode/bc128/code39/png)", e.Type)
		}
		on, err := parseCutCont(e)
		if err != nil {
			return err
		}
		r.contentCut = &on
		return nil
	default:
		sup := "text/title/div_line/div_star/qrcode/bc128/code39/png"
		if r.yunheCompat {
			sup += "/cut"
		}
		return fmt.Errorf("未知元素 type %q(支持 %s)", e.Type, sup)
	}
}

// parseCutCont LegacyCompat C3: cont 为 1/"1"/true → 切;0/"0"/false → 不切;缺失 → 切。
func parseCutCont(e *Element) (bool, error) {
	t := bytes.TrimSpace(e.Cont)
	if len(t) == 0 || string(t) == "null" {
		return true, nil
	}
	var s string
	if err := json.Unmarshal(t, &s); err == nil {
		switch strings.TrimSpace(s) {
		case "1", "true", "on":
			return true, nil
		case "0", "false", "off":
			return false, nil
		default:
			return false, fmt.Errorf("cut.cont 需为 0 或 1")
		}
	}
	var n int
	if err := json.Unmarshal(t, &n); err == nil {
		if n == 0 || n == 1 {
			return n == 1, nil
		}
		return false, fmt.Errorf("cut.cont 需为 0 或 1")
	}
	var b bool
	if err := json.Unmarshal(t, &b); err == nil {
		return b, nil
	}
	return false, fmt.Errorf("cut.cont 需为 0 或 1")
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
