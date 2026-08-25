package layout

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
)

// DecodeBMP 解码 24/32 位未压缩 BMP(BI_RGB)为 image.Image。
// 支持 BITMAPINFOHEADER(40 字节)及更长信息头;RLE/调色板(16/8/4/1 位)、
// 数据不完整等一律拒绝(6K 限制下可接受该覆盖范围)。
func DecodeBMP(data []byte) (image.Image, error) {
	fail := func(detail string) error {
		return fmt.Errorf("bmp 解码失败: %s", detail)
	}
	rd := bytes.NewReader(data)
	var magic [2]byte
	if _, err := rd.Read(magic[:]); err != nil || magic != [2]byte{'B', 'M'} {
		return nil, fail("非 BMP 数据")
	}
	// 文件头:文件大小(4) 保留(4) 像素数据偏移(4)
	if _, err := rd.Seek(10, io.SeekStart); err != nil {
		return nil, fail("文件头不完整")
	}
	var pixOff uint32
	if err := binary.Read(rd, binary.LittleEndian, &pixOff); err != nil {
		return nil, fail("文件头不完整")
	}
	// 信息头
	var infoLen uint32
	if err := binary.Read(rd, binary.LittleEndian, &infoLen); err != nil {
		return nil, fail("信息头不完整")
	}
	if infoLen < 40 {
		return nil, fail("不支持的信息头(需 BITMAPINFOHEADER)")
	}
	var width, height int32
	var planes, bpp uint16
	var comp uint32
	if err := binary.Read(rd, binary.LittleEndian, &width); err != nil {
		return nil, fail("信息头不完整")
	}
	if err := binary.Read(rd, binary.LittleEndian, &height); err != nil {
		return nil, fail("信息头不完整")
	}
	if err := binary.Read(rd, binary.LittleEndian, &planes); err != nil {
		return nil, fail("信息头不完整")
	}
	if err := binary.Read(rd, binary.LittleEndian, &bpp); err != nil {
		return nil, fail("信息头不完整")
	}
	if err := binary.Read(rd, binary.LittleEndian, &comp); err != nil {
		return nil, fail("信息头不完整")
	}
	if width <= 0 || height == 0 {
		return nil, fail("图像尺寸非法")
	}
	if planes != 1 {
		return nil, fail("颜色平面数异常")
	}
	if bpp != 24 && bpp != 32 {
		return nil, fail("仅支持 24/32 位位图")
	}
	if comp != 0 { // 0 = BI_RGB
		return nil, fail("仅支持未压缩(BI_RGB)位图")
	}

	w := int(width)
	h := int(height)
	if h < 0 {
		h = -h // 负高度 = 自顶向下
	}
	bppN := int(bpp) / 8
	rowSize := (w*bppN + 3) &^ 3 // 行按 4 字节对齐
	if int64(pixOff)+int64(rowSize)*int64(h) > int64(len(data)) {
		return nil, fail("像素数据不完整")
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		srcRow := rowSize * y
		if height > 0 {
			srcRow = rowSize * (h - 1 - y) // 自底向上
		}
		base := int(pixOff) + srcRow
		for x := 0; x < w; x++ {
			o := base + x*bppN
			img.SetRGBA(x, y, color.RGBA{R: data[o+2], G: data[o+1], B: data[o], A: 255})
		}
	}
	return img, nil
}
