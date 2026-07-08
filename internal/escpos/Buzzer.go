package escpos

// 蜂鸣相关指令。依据《佳博热敏票据打印机编程手册 v1.0.5》命令 62(ESC B)。
// 注:蜂鸣属厂商扩展指令,不同机型可能有差异;当前以佳博为准,多品牌接入时再适配。
// 报警灯扩展(ESC C,手册命令 63)未接入、代码已清理,需要时按手册实现。

// BuildBuzzer 生成一条独立蜂鸣指令(不含小票内容),用于「蜂鸣测试」或来单提示。
// times = 鸣叫次数(1~9);durationUnits = 每次时长单位(1~9,每单位 50ms)。
func BuildBuzzer(times, durationUnits int) []byte {
	return []byte{0x1B, 0x42, clampByte(times, 1, 9), clampByte(durationUnits, 1, 9)}
}

func clampByte(v, lo, hi int) byte {
	if v < lo {
		v = lo
	}
	if v > hi {
		v = hi
	}
	return byte(v)
}
