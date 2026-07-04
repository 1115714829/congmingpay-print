package escpos

import "strings"

// ReprintBanner 生成醒目的「打印服务器重印」抬头,用于失败自动重印 / 重新打印的小票顶部,
// 按纸宽自适应星号线宽度。抬头之后接原始小票内容(原内容自带初始化与切纸)。
func ReprintBanner(widthMM int) []byte {
	line := strings.Repeat("*", CharsPerLine(widthMM))
	data, _ := NewBuilder().
		SetAlign(AlignCenter).
		Line(line).
		SetEmphasize(true).SetDoubleSize(true).
		Line("重打小票").
		SetDoubleSize(false).
		Line("打印服务器 重印").
		SetEmphasize(false).
		Line(line).
		SetAlign(AlignLeft).
		Feed(1).
		Bytes()
	return data
}
