package escpos

// QRCode 追加一个 QR 码(GS ( K,Model 2)。
// 依据《佳博热敏票据打印机编程手册 v1.0.5》命令 65-68(标准 ESC/POS QR):
//   - 模块大小:GS ( K 03 00 31 43 n     (n=1~15 点)
//   - 纠错等级:GS ( K 03 00 31 45 n     (48=L 49=M 50=Q 51=H)
//   - 存储数据:GS ( K pL pH 31 50 30 d… ((pL+pH*256)=数据长+3)
//   - 打印:    GS ( K 03 00 31 51 30
//
// moduleSize 为模块点大小 1~15;data 建议 ASCII(中文按 UTF-8 存,能否解码取决于扫码软件)。
func (b *Builder) QRCode(data string, moduleSize int) *Builder {
	if b.err != nil {
		return b
	}
	if moduleSize < 1 {
		moduleSize = 1
	} else if moduleSize > 15 {
		moduleSize = 15
	}
	d := []byte(data)

	b.raw(0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x43, byte(moduleSize)) // 模块大小
	b.raw(0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x45, 0x31)            // 纠错等级 M

	n := len(d) + 3 // 存储数据:pL pH = 数据长 + 3
	b.raw(0x1D, 0x28, 0x6B, byte(n%256), byte(n/256), 0x31, 0x50, 0x30)
	b.buf.Write(d)

	b.raw(0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x51, 0x30) // 打印
	return b
}
