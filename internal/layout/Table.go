package layout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"congmingpay/internal/escpos"
)

func (r *renderer) table(e *Element) error {
	names, pcts, err := parseThead(e.Thead)
	if err != nil {
		return err
	}
	sum := 0
	for _, p := range pcts {
		sum += p
	}
	if sum != 100 {
		return fmt.Errorf("thead 列宽总和 %d%% ≠ 100%%", sum)
	}

	cpl := r.cpl
	w, _, err := sizeMag(e.Size)
	if err != nil {
		return err
	}
	if w > 0 {
		cpl = cpl / (w + 1) // 放大后每行字符变少
	}

	// 按百分比分配列字符宽,末列吸收误差补足到 cpl(数据已校验合法,此处仅为取整补偿)
	cols := make([]int, len(pcts))
	used := 0
	for i, p := range pcts {
		cols[i] = p * cpl / 100
		if cols[i] < 1 {
			cols[i] = 1
		}
		used += cols[i]
	}
	cols[len(cols)-1] += cpl - used
	if cols[len(cols)-1] < 1 {
		cols[len(cols)-1] = 1
	}

	// 渲染前逐行校验:单元格数须等于列数、值须为标量(string/number/bool)
	rows := make([][]string, len(e.Tbody))
	for ri, row := range e.Tbody {
		if len(row) != len(pcts) {
			return fmt.Errorf("tbody 第 %d 行有 %d 个单元格,与列数 %d 不符", ri+1, len(row), len(pcts))
		}
		cells := make([]string, len(row))
		for ci, v := range row {
			s, err := cellString(v)
			if err != nil {
				return fmt.Errorf("tbody 第 %d 行第 %d 列%v", ri+1, ci+1, err)
			}
			cells[ci] = s
		}
		rows[ri] = cells
	}

	if w > 0 {
		r.b.SetSize(w, w)
	}
	if names != nil {
		r.tableRow(names, cols)
		if e.LineDiv == 1 {
			r.b.Line(strings.Repeat("-", cpl))
		}
	}
	for _, cells := range rows {
		r.tableRow(cells, cols)
		if e.LineDiv == 1 {
			r.b.Line(strings.Repeat("-", cpl))
		}
		for i := 0; i < e.LineSpace; i++ {
			r.b.Feed(1)
		}
	}
	if w > 0 {
		r.b.SetSize(0, 0)
	}
	return nil
}

// tableRow 渲染一行:各单元格按列宽换行,行内多行左对齐拼接。
func (r *renderer) tableRow(cells []string, cols []int) {
	wrapped := make([][]string, len(cols))
	maxLines := 1
	for i := range cols {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		wrapped[i] = wrapByWidth(cell, cols[i])
		if len(wrapped[i]) > maxLines {
			maxLines = len(wrapped[i])
		}
	}
	for ln := 0; ln < maxLines; ln++ {
		var sb strings.Builder
		for i, cw := range cols {
			seg := ""
			if ln < len(wrapped[i]) {
				seg = wrapped[i][ln]
			}
			sb.WriteString(padCell(seg, cw))
		}
		r.b.SetAlign(escpos.AlignLeft).Line(strings.TrimRight(sb.String(), " "))
	}
}

// padCell 把 s 右侧补空格到显示宽 cw(左对齐)。
func padCell(s string, cw int) string {
	pad := cw - escpos.DisplayWidth(s)
	if pad < 0 {
		pad = 0
	}
	return s + strings.Repeat(" ", pad)
}

// wrapByWidth 按显示宽把 s 折成多行,每行不超过 cw(中文按 2 计)。
func wrapByWidth(s string, cw int) []string {
	if s == "" {
		return []string{""}
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

// parseThead 解析 thead:对象{列名:宽%}(保序)或字符串数组[宽%];形态/取值非法即报错。
func parseThead(raw json.RawMessage) (names []string, pcts []int, err error) {
	// 数组:["50%","20%",...]
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		for _, p := range arr {
			n, err := parsePct(p)
			if err != nil {
				return nil, nil, err
			}
			pcts = append(pcts, n)
		}
		if len(pcts) == 0 {
			return nil, nil, fmt.Errorf("thead 为空(至少 1 列)")
		}
		return nil, pcts, nil
	}
	// 对象:保序读取键与值
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err2 := dec.Token(); err2 != nil || t != json.Delim('{') {
		return nil, nil, fmt.Errorf("thead 需为对象{列名:\"宽%%\"}或字符串数组[\"宽%%\"]")
	}
	for dec.More() {
		keyTok, err2 := dec.Token()
		if err2 != nil {
			return nil, nil, fmt.Errorf("thead 解析失败: %v", err2)
		}
		key, _ := keyTok.(string)
		valTok, err2 := dec.Token()
		if err2 != nil {
			return nil, nil, fmt.Errorf("thead 解析失败: %v", err2)
		}
		val, ok := valTok.(string)
		if !ok {
			return nil, nil, fmt.Errorf("thead 第 %d 列宽度需为字符串(如 \"50%%\")", len(pcts)+1)
		}
		n, err2 := parsePct(val)
		if err2 != nil {
			return nil, nil, err2
		}
		names = append(names, key)
		pcts = append(pcts, n)
	}
	if len(pcts) == 0 {
		return nil, nil, fmt.Errorf("thead 为空(至少 1 列)")
	}
	return names, pcts, nil
}

// parsePct 解析 "85%"/"85" 为整数百分比;非整数或超出 1-100 即报错(容忍前后空白与 % 后缀)。
func parsePct(s string) (int, error) {
	t := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	n, err := strconv.Atoi(t)
	if err != nil {
		return 0, fmt.Errorf("thead 列宽 %q 非法(需 1-100 的整数百分比)", s)
	}
	if n < 1 || n > 100 {
		return 0, fmt.Errorf("thead 列宽 %d%% 超出 1-100", n)
	}
	return n, nil
}

// cellString 把单元格值转为字符串;仅 string/number/bool 合法,null/对象/数组报错。
func cellString(v interface{}) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10), nil
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("值非法(需字符串/数字/布尔)")
	}
}
