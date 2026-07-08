package escpos

// Code128 打印 CODE128 条码(GS k 73)。前置 "{B" 选码集 B。
// hri(人眼可读字符位置,GS H n):0=无 1=上方 2=下方 3=上下,超范围夹取。
func (b *Builder) Code128(data string, heightDots, moduleWidth, hri int) *Builder {
	b.barcodePrep(heightDots, moduleWidth, hri)
	payload := "{B" + data
	b.raw(0x1D, 0x6B, 73, byte(len(payload)))
	if b.err == nil {
		b.buf.WriteString(payload)
	}
	return b
}

// Code39 打印 CODE39 条码(GS k 69)。
func (b *Builder) Code39(data string, heightDots, moduleWidth, hri int) *Builder {
	b.barcodePrep(heightDots, moduleWidth, hri)
	b.raw(0x1D, 0x6B, 69, byte(len(data)))
	if b.err == nil {
		b.buf.WriteString(data)
	}
	return b
}

// barcodePrep 设置 HRI 位置/字体、条码高度、模块宽度。
func (b *Builder) barcodePrep(heightDots, moduleWidth, hri int) {
	b.raw(0x1D, 0x48, clampByte(hri, 0, 3))          // GS H:HRI 位置
	b.raw(0x1D, 0x66, 0x00)                          // GS f:HRI 字体 A
	b.raw(0x1D, 0x68, clampByte(heightDots, 1, 255)) // GS h:高度
	b.raw(0x1D, 0x77, clampByte(moduleWidth, 2, 6))  // GS w:模块宽
}
