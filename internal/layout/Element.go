// Package layout 把云端 JSON 小票排版(MQTT type=5 的 contents 数组)渲染为 ESC/POS 字节。
//
// 渲染为「全部严格」模式:字段缺失用文档声明的默认(合法);字段存在但非法 → 整单拒绝,
// Render 返回带 contents[i] 定位的 error(调用方回执 ok:false 并落日志)。
package layout

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Element 是一个小票排版元素;多态字段用 RawMessage/interface{} 延后解析。
type Element struct {
	Type      string          `json:"type"`
	Cont      json.RawMessage `json:"cont"` // string 或 []string
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

// contString 把 cont 当字符串返回。缺失/null → 空串(文本类元素的空行语义,合法);
// 存在但非字符串(数组走 contArray,对象/数字/布尔在此拒)→ 报错。
func (e *Element) contString() (string, error) {
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

// contArray 把 cont 当字符串数组返回;非数组 → (nil, nil)(交调用方按字符串处理);
// 是数组但元素非字符串 → 报错。
func (e *Element) contArray() ([]string, error) {
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
