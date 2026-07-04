// Package layout 把云端 JSON 小票排版(MQTT type=5 的 contents 数组)渲染为 ESC/POS 字节。
package layout

import "encoding/json"

// Element 是一个小票排版元素;多态字段用 RawMessage/interface{} 延后解析。
type Element struct {
	Type      string          `json:"type"`
	Cont      json.RawMessage `json:"cont"`       // string 或 []string
	Bold      bool            `json:"bold"`
	Size      string          `json:"size"`
	Align     string          `json:"align"`
	Hri       int             `json:"hri"`
	Thead     json.RawMessage `json:"thead"` // object{列名:宽%} 或 []string(宽%)
	Tbody     [][]interface{} `json:"tbody"`
	BothSides []string        `json:"both_sides"`
	LineDiv   int             `json:"line_div"`
	LineSpace int             `json:"line_space"`
}

// contString 把 cont 当字符串返回(若为数组则返回空串)。
func (e *Element) contString() string {
	var s string
	if json.Unmarshal(e.Cont, &s) == nil {
		return s
	}
	return ""
}

// contArray 把 cont 当字符串数组返回(若为字符串则返回 nil)。
func (e *Element) contArray() []string {
	var a []string
	if json.Unmarshal(e.Cont, &a) == nil {
		return a
	}
	return nil
}
