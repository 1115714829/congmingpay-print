// Package escpos 构建 ESC/POS 指令字节流(58/80mm 热敏票据打印机通用)。
package escpos

import (
	"bytes"

	"congmingpay/internal/util"
)

// Align 表示打印对齐方式。
type Align byte

const (
	AlignLeft   Align = 0
	AlignCenter Align = 1
	AlignRight  Align = 2
)

// Builder 累积 ESC/POS 指令字节;链式调用,首个出现的错误会被记录并在 Bytes 时返回。
type Builder struct {
	buf bytes.Buffer
	err error
}

// NewBuilder 创建 Builder 并写入初始化指令(ESC @)。
func NewBuilder() *Builder {
	b := &Builder{}
	return b.Init()
}

// Init 初始化打印机(ESC @)。
func (b *Builder) Init() *Builder {
	return b.raw(0x1B, 0x40)
}

// SetAlign 设置对齐(ESC a n)。
func (b *Builder) SetAlign(a Align) *Builder {
	return b.raw(0x1B, 0x61, byte(a))
}

// SetEmphasize 设置加粗(ESC E n)。
func (b *Builder) SetEmphasize(on bool) *Builder {
	return b.raw(0x1B, 0x45, boolByte(on))
}

// SetSize 设置字符放大倍数(GS ! n)。wMag/hMag 为 0-7(0=1倍…7=8倍)。
func (b *Builder) SetSize(wMag, hMag int) *Builder {
	return b.raw(0x1D, 0x21, byte(clampByte(wMag, 0, 7)<<4|clampByte(hMag, 0, 7)))
}

// Text 追加一段文本(GB18030 编码),不换行。
func (b *Builder) Text(s string) *Builder {
	if b.err != nil {
		return b
	}
	data, err := util.ToGB18030(s)
	if err != nil {
		b.err = err
		return b
	}
	b.buf.Write(data)
	return b
}

// Line 追加一行文本并换行。
func (b *Builder) Line(s string) *Builder {
	return b.Text(s).Feed(1)
}

// Feed 走纸 n 行(n 个 LF)。
func (b *Builder) Feed(n int) *Builder {
	for i := 0; i < n; i++ {
		b.raw(0x0A)
	}
	return b
}

// Bytes 返回累积的字节;过程中若出错则返回该错误。
func (b *Builder) Bytes() ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.buf.Bytes(), nil
}

// Raw 追加任意原始字节(用于嵌入已生成的位图/条码指令)。
func (b *Builder) Raw(data []byte) *Builder {
	if b.err != nil {
		return b
	}
	b.buf.Write(data)
	return b
}

func (b *Builder) raw(bs ...byte) *Builder {
	if b.err != nil {
		return b
	}
	b.buf.Write(bs)
	return b
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}
