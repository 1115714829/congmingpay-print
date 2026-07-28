package preview

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"congmingpay/internal/escpos"
	"congmingpay/internal/layout"

	qrcode "github.com/skip2/go-qrcode"
)

func (c *canvas) renderContents(raw []byte) error {
	var elems []layout.Element
	if err := json.Unmarshal(raw, &elems); err != nil {
		return fmt.Errorf("contents 需为数组: %v", err)
	}
	for i := range elems {
		if err := c.element(&elems[i]); err != nil {
			return fmt.Errorf("contents[%d]: %v", i, err)
		}
	}
	return nil
}

func (c *canvas) element(e *layout.Element) error {
	switch {
	case len(bytes.TrimSpace(e.Thead)) > 0 && string(bytes.TrimSpace(e.Thead)) != "null":
		return c.table(e)
	case len(e.Tbody) > 0:
		return fmt.Errorf("有 tbody 但缺 thead")
	case e.BothSides != nil:
		return c.bothSides(e)
	}
	switch e.Type {
	case "", "text":
		return c.text(e)
	case "title":
		return c.title(e)
	case "div_line":
		return c.divider(e, "-")
	case "div_star":
		return c.divider(e, "*")
	case "qrcode":
		return c.qrcode(e)
	case "bc128", "code39":
		return c.barcode(e)
	case "png":
		return c.png(e)
	case "cut":
		return nil // 切纸意图无视觉(页脚已示意)
	default:
		return fmt.Errorf("未知元素 type %q", e.Type)
	}
}

func contString(e *layout.Element) (string, error) {
	t := bytes.TrimSpace(e.Cont)
	if len(t) == 0 || string(t) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(t, &s); err != nil {
		return "", fmt.Errorf("cont 需为字符串")
	}
	return s, nil
}

func contArray(e *layout.Element) ([]string, error) {
	t := bytes.TrimSpace(e.Cont)
	if len(t) == 0 || t[0] != '[' {
		return nil, nil
	}
	var a []string
	if err := json.Unmarshal(t, &a); err != nil {
		return nil, fmt.Errorf("cont 数组元素需为字符串")
	}
	return a, nil
}

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
	return 0, 0, fmt.Errorf("size %q 非法", size)
}

func parseAlign(align string) (int, error) {
	switch align {
	case "", "left":
		return alignLeft, nil
	case "center":
		return alignCenter, nil
	case "right":
		return alignRight, nil
	default:
		return 0, fmt.Errorf("align %q 非法", align)
	}
}

func (c *canvas) title(e *layout.Element) error {
	s, err := contString(e)
	if err != nil {
		return err
	}
	c.textLine(s, alignCenter, true, 1, 1)
	return nil
}

func (c *canvas) text(e *layout.Element) error {
	al, err := parseAlign(e.Align)
	if err != nil {
		return err
	}
	w, h, err := sizeMag(e.Size)
	if err != nil {
		return err
	}
	s, err := contString(e)
	if err != nil {
		return err
	}
	c.textLine(s, al, e.Bold, w, h)
	return nil
}

func (c *canvas) divider(e *layout.Element, ch string) error {
	s, err := contString(e)
	if err != nil {
		return err
	}
	c.textLine(buildDivider(s, ch, c.cpl), alignLeft, false, 0, 0)
	return nil
}

func buildDivider(label, ch string, cpl int) string {
	if label == "" {
		return strings.Repeat(ch, cpl)
	}
	lw := escpos.DisplayWidth(label) + 2
	if lw >= cpl {
		return label
	}
	total := cpl - lw
	left := total / 2
	right := total - left
	return strings.Repeat(ch, left) + " " + label + " " + strings.Repeat(ch, right)
}

func (c *canvas) bothSides(e *layout.Element) error {
	if len(e.BothSides) != 2 {
		return fmt.Errorf("both_sides 需恰为 2 段")
	}
	w, h, err := sizeMag(e.Size)
	if err != nil {
		return err
	}
	cpl := c.cpl
	if w > 0 {
		cpl = cpl / (w + 1)
	}
	line := padBetween(e.BothSides[0], e.BothSides[1], cpl)
	c.textLine(line, alignLeft, false, w, h)
	return nil
}

func padBetween(left, right string, cpl int) string {
	pad := cpl - escpos.DisplayWidth(left) - escpos.DisplayWidth(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

func (c *canvas) qrcode(e *layout.Element) error {
	al, err := parseAlign(e.Align)
	if err != nil {
		return err
	}
	arr, err := contArray(e)
	if err != nil {
		return err
	}
	if arr != nil {
		if len(arr) != 2 {
			return fmt.Errorf("双码需恰 2 项")
		}
		ia, err := genQR(arr[0])
		if err != nil {
			return err
		}
		ib, err := genQR(arr[1])
		if err != nil {
			return err
		}
		c.drawImage(composeSideBySide(ia, ib, 24), al)
		return nil
	}
	s, err := contString(e)
	if err != nil {
		return err
	}
	if s == "" {
		return fmt.Errorf("二维码内容为空")
	}
	img, err := genQR(s)
	if err != nil {
		return err
	}
	c.drawImage(img, al)
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

func (c *canvas) barcode(e *layout.Element) error {
	al, err := parseAlign(e.Align)
	if err != nil {
		return err
	}
	s, err := contString(e)
	if err != nil {
		return err
	}
	if s == "" {
		return fmt.Errorf("条码内容为空")
	}
	h := 60
	switch e.Size {
	case "22":
		h = 90
	case "33":
		h = 120
	}
	img := fakeBarcode(s, c.width*3/4, h)
	c.drawImage(img, al)
	if e.Hri == 1 || e.Hri == 3 {
		// already drawn above in real printers; we put HRI below for simplicity
	}
	if e.Hri == 2 || e.Hri == 3 || e.Hri == 0 {
		if e.Hri != 0 {
			c.textLine(s, al, false, 0, 0)
		}
	}
	return nil
}

// fakeBarcode 用内容哈希画近似条纹(非真实编码,仅预览占位)。
func fakeBarcode(s string, w, h int) image.Image {
	if w < 40 {
		w = 40
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	x := 4
	for i := 0; i < len(s)*4 && x < w-4; i++ {
		bw := 1 + int(s[i%len(s)])%3
		if i%2 == 0 {
			for dy := 2; dy < h-2; dy++ {
				for dx := 0; dx < bw && x+dx < w-2; dx++ {
					img.Set(x+dx, dy, color.Black)
				}
			}
		}
		x += bw + 1
	}
	return img
}

func (c *canvas) png(e *layout.Element) error {
	al, err := parseAlign(e.Align)
	if err != nil {
		return err
	}
	s, err := contString(e)
	if err != nil {
		return err
	}
	img, err := loadImage(s)
	if err != nil {
		return err
	}
	c.drawImage(img, al)
	return nil
}

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
			return nil, fmt.Errorf("png data URI 非法")
		}
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src[i+1:]))
		if err != nil {
			return nil, err
		}
		data = b
	case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
		cli := &http.Client{Timeout: 10 * time.Second}
		resp, err := cli.Get(src)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil, err
		}
		data = b
	default:
		return nil, fmt.Errorf("png 来源非法")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

func (c *canvas) table(e *layout.Element) error {
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
	cpl := c.cpl
	w, _, err := sizeMag(e.Size)
	if err != nil {
		return err
	}
	if w > 0 {
		cpl = cpl / (w + 1)
	}
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
	rows := make([][]string, len(e.Tbody))
	for ri, row := range e.Tbody {
		if len(row) != len(pcts) {
			return fmt.Errorf("tbody 第 %d 行列数不符", ri+1)
		}
		cells := make([]string, len(row))
		for ci, v := range row {
			s, err := cellString(v)
			if err != nil {
				return err
			}
			cells[ci] = s
		}
		rows[ri] = cells
	}
	if names != nil {
		c.tableRow(names, cols, w)
		if e.LineDiv == 1 {
			c.textLine(strings.Repeat("-", cpl), alignLeft, false, w, w)
		}
	}
	for _, cells := range rows {
		c.tableRow(cells, cols, w)
		if e.LineDiv == 1 {
			c.textLine(strings.Repeat("-", cpl), alignLeft, false, w, w)
		}
		for i := 0; i < e.LineSpace; i++ {
			c.feed(1)
		}
	}
	return nil
}

func (c *canvas) tableRow(cells []string, cols []int, mag int) {
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
		c.textLine(strings.TrimRight(sb.String(), " "), alignLeft, false, mag, mag)
	}
}

func padCell(s string, cw int) string {
	pad := cw - escpos.DisplayWidth(s)
	if pad < 0 {
		pad = 0
	}
	return s + strings.Repeat(" ", pad)
}

func parseThead(raw json.RawMessage) (names []string, pcts []int, err error) {
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
			return nil, nil, fmt.Errorf("thead 为空")
		}
		return nil, pcts, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err2 := dec.Token(); err2 != nil || t != json.Delim('{') {
		return nil, nil, fmt.Errorf("thead 需为对象或数组")
	}
	for dec.More() {
		keyTok, err2 := dec.Token()
		if err2 != nil {
			return nil, nil, err2
		}
		key, _ := keyTok.(string)
		valTok, err2 := dec.Token()
		if err2 != nil {
			return nil, nil, err2
		}
		val, ok := valTok.(string)
		if !ok {
			return nil, nil, fmt.Errorf("thead 宽度需为字符串")
		}
		n, err2 := parsePct(val)
		if err2 != nil {
			return nil, nil, err2
		}
		names = append(names, key)
		pcts = append(pcts, n)
	}
	if len(pcts) == 0 {
		return nil, nil, fmt.Errorf("thead 为空")
	}
	return names, pcts, nil
}

func parsePct(s string) (int, error) {
	t := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	n, err := strconv.Atoi(t)
	if err != nil || n < 1 || n > 100 {
		return 0, fmt.Errorf("列宽 %q 非法", s)
	}
	return n, nil
}

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
		return "", fmt.Errorf("单元格值非法")
	}
}
