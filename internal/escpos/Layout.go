package escpos

// CharsPerLine 返回给定纸宽(mm)每行可打印的标准字体(Font A,12 点宽)字符数。
//
// 依据佳博 C# demo 与票据编程手册:58mm 可打印 48mm = 384 点 = 32 字符;
// 80mm = 576 点 = 48 字符。其它宽度按 80mm 处理。
func CharsPerLine(widthMM int) int {
	if widthMM == 58 {
		return 32
	}
	return 48
}

// BaseTailLinesFor 返回该纸宽尾部空行的内置基数(不显示在界面):
// 58mm=5;80mm=3(保持历史默认)。
func BaseTailLinesFor(widthMM int) int {
	if widthMM == 58 {
		return 5
	}
	return 3
}

// TailFeed 返回实际尾部空行数 = 基数(按纸宽) + 用户偏移,下限 0(不能负走纸)。
// 偏移可为负:如 58mm 偏移 -2 → 3、-10 → 0。
func TailFeed(widthMM, offset int) int {
	n := BaseTailLinesFor(widthMM) + offset
	if n < 0 {
		n = 0
	}
	return n
}

// DisplayWidth 返回字符串的显示列宽:ASCII/半角=1,中文/全角(非 ASCII)=2。用于列对齐。
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r > 0x7F {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// WidthDots 返回该纸宽的可打印点宽:58mm=384、80mm=576(标准 203dpi)。
func WidthDots(widthMM int) int {
	if widthMM == 58 {
		return 384
	}
	return 576
}
