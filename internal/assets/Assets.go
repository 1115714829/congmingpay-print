// Package assets 内嵌应用资源(图标等),保证单 exe 分发。
package assets

import _ "embed"

// AppPNG 是应用/托盘图标的 PNG 字节。
//
//go:embed app.png
var AppPNG []byte
