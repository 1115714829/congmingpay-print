package ui

import (
	"bytes"
	"image/png"

	"congmingpay/internal/assets"

	"github.com/lxn/walk"
)

// loadAppIcon 从内嵌 PNG 解码并创建 walk 图标;失败时返回 nil(界面仍可运行)。
func loadAppIcon() *walk.Icon {
	img, err := png.Decode(bytes.NewReader(assets.AppPNG))
	if err != nil {
		return nil
	}
	icon, err := walk.NewIconFromImage(img)
	if err != nil {
		return nil
	}
	return icon
}
