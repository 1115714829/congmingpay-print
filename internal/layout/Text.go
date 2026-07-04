package layout

import (
	"strings"

	"congmingpay/internal/escpos"
)

// applyAlign 按 align 字符串设置对齐(默认左)。
func (r *renderer) applyAlign(align string) {
	switch align {
	case "center":
		r.b.SetAlign(escpos.AlignCenter)
	case "right":
		r.b.SetAlign(escpos.AlignRight)
	default:
		r.b.SetAlign(escpos.AlignLeft)
	}
}

// sizeMag 解析 size "WH" → (宽放大, 高放大),各 0-7;空/非法 → 0,0。
func sizeMag(size string) (int, int) {
	if len(size) >= 2 {
		w := int(size[0] - '0')
		h := int(size[1] - '0')
		if w >= 0 && w <= 7 && h >= 0 && h <= 7 {
			return w, h
		}
	}
	return 0, 0
}

func (r *renderer) title(e *Element) {
	r.b.SetAlign(escpos.AlignCenter).SetEmphasize(true).SetSize(1, 1).
		Line(e.contString()).
		SetSize(0, 0).SetEmphasize(false).SetAlign(escpos.AlignLeft)
}

func (r *renderer) text(e *Element) {
	r.applyAlign(e.Align)
	if e.Bold {
		r.b.SetEmphasize(true)
	}
	w, h := sizeMag(e.Size)
	if w > 0 || h > 0 {
		r.b.SetSize(w, h)
	}
	r.b.Line(e.contString())
	r.b.SetSize(0, 0)
	if e.Bold {
		r.b.SetEmphasize(false)
	}
	r.b.SetAlign(escpos.AlignLeft)
}

func (r *renderer) divider(e *Element, ch string) {
	r.b.SetAlign(escpos.AlignLeft).Line(buildDivider(e.contString(), ch, r.cpl))
}

// buildDivider 生成带居中标题的分割线;无标题则整行填充。
func buildDivider(label, ch string, cpl int) string {
	if label == "" {
		return strings.Repeat(ch, cpl)
	}
	lw := escpos.DisplayWidth(label) + 2 // 标题两侧留空
	if lw >= cpl {
		return label
	}
	total := cpl - lw
	left := total / 2
	right := total - left
	return strings.Repeat(ch, left) + " " + label + " " + strings.Repeat(ch, right)
}

func (r *renderer) bothSides(e *Element) {
	if len(e.BothSides) < 2 {
		return
	}
	cpl := r.cpl
	w, h := sizeMag(e.Size)
	if w > 0 {
		cpl = cpl / (w + 1) // 放大后每行字符变少
	}
	r.b.SetAlign(escpos.AlignLeft)
	if w > 0 || h > 0 {
		r.b.SetSize(w, h)
	}
	r.b.Line(padBetween(e.BothSides[0], e.BothSides[1], cpl))
	if w > 0 || h > 0 {
		r.b.SetSize(0, 0)
	}
}

// padBetween 左右两段之间填空格,使总显示宽=cpl。
func padBetween(left, right string, cpl int) string {
	pad := cpl - escpos.DisplayWidth(left) - escpos.DisplayWidth(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}
