package layout

import (
	"fmt"
	"strings"

	"congmingpay/internal/escpos"
)

// applyAlign 按 align 字符串设置对齐。缺失/空=左(合法);非法值整单拒。
func (r *renderer) applyAlign(align string) error {
	switch align {
	case "", "left":
		r.b.SetAlign(escpos.AlignLeft)
	case "center":
		r.b.SetAlign(escpos.AlignCenter)
	case "right":
		r.b.SetAlign(escpos.AlignRight)
	default:
		return fmt.Errorf("align %q 非法(可选 left/center/right)", align)
	}
	return nil
}

// sizeMag 解析 size "WH" → (宽放大, 高放大)。缺失/空=不放大(合法);
// 存在则必须恰为两位 0-7 数字,否则整单拒。
func sizeMag(size string) (int, int, error) {
	if size == "" {
		return 0, 0, nil
	}
	if len(size) == 2 {
		w := int(size[0] - '0')
		h := int(size[1] - '0')
		if w >= 0 && w <= 7 && h >= 0 && h <= 7 {
			return w, h, nil
		}
	}
	return 0, 0, fmt.Errorf("size %q 非法(需两位 0-7 数字,如 \"11\")", size)
}

func (r *renderer) title(e *Element) error {
	s, err := e.contString()
	if err != nil {
		return err
	}
	r.b.SetAlign(escpos.AlignCenter).SetEmphasize(true).SetSize(1, 1).
		Line(s).
		SetSize(0, 0).SetEmphasize(false).SetAlign(escpos.AlignLeft)
	return nil
}

func (r *renderer) text(e *Element) error {
	if err := r.applyAlign(e.Align); err != nil {
		return err
	}
	w, h, err := sizeMag(e.Size)
	if err != nil {
		return err
	}
	s, err := e.contString()
	if err != nil {
		return err
	}
	if e.Bold {
		r.b.SetEmphasize(true)
	}
	if w > 0 || h > 0 {
		r.b.SetSize(w, h)
	}
	r.b.Line(s)
	r.b.SetSize(0, 0)
	if e.Bold {
		r.b.SetEmphasize(false)
	}
	r.b.SetAlign(escpos.AlignLeft)
	return nil
}

func (r *renderer) divider(e *Element, ch string) error {
	s, err := e.contString()
	if err != nil {
		return err
	}
	r.b.SetAlign(escpos.AlignLeft).Line(buildDivider(s, ch, r.cpl))
	return nil
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

func (r *renderer) bothSides(e *Element) error {
	if len(e.BothSides) != 2 {
		return fmt.Errorf("both_sides 需恰为 2 段文本,实得 %d 段", len(e.BothSides))
	}
	w, h, err := sizeMag(e.Size)
	if err != nil {
		return err
	}
	cpl := r.cpl
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
	return nil
}

// padBetween 左右两段之间填空格,使总显示宽=cpl。
func padBetween(left, right string, cpl int) string {
	pad := cpl - escpos.DisplayWidth(left) - escpos.DisplayWidth(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}
