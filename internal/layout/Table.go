package layout

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"congmingpay/internal/escpos"
)

func (r *renderer) table(e *Element) {
	names, pcts := parseThead(e.Thead)
	if len(pcts) == 0 {
		return
	}

	cpl := r.cpl
	w, _ := sizeMag(e.Size)
	if w > 0 {
		cpl = cpl / (w + 1) // 放大后每行字符变少
	}

	// 按百分比分配列字符宽,末列吸收误差补足到 cpl
	cols := make([]int, len(pcts))
	sum := 0
	for i, p := range pcts {
		cols[i] = p * cpl / 100
		if cols[i] < 1 {
			cols[i] = 1
		}
		sum += cols[i]
	}
	cols[len(cols)-1] += cpl - sum
	if cols[len(cols)-1] < 1 {
		cols[len(cols)-1] = 1
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
	for _, row := range e.Tbody {
		r.tableRow(cellsToStrings(row), cols)
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

// parseThead 解析 thead:对象{列名:宽%}(保序)或数组[宽%]。
func parseThead(raw json.RawMessage) (names []string, pcts []int) {
	if len(raw) == 0 {
		return nil, nil
	}
	// 数组:["50%","20%",...]
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		for _, p := range arr {
			pcts = append(pcts, parsePct(p))
		}
		return nil, pcts
	}
	// 对象:保序读取键与值
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, nil
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			break
		}
		key, _ := keyTok.(string)
		valTok, err := dec.Token()
		if err != nil {
			break
		}
		val, _ := valTok.(string)
		names = append(names, key)
		pcts = append(pcts, parsePct(val))
	}
	return names, pcts
}

func parsePct(s string) int {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func cellsToStrings(row []interface{}) []string {
	out := make([]string, len(row))
	for i, v := range row {
		out[i] = cellString(v)
	}
	return out
}

func cellString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return ""
	}
}
