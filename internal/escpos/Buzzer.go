package escpos

// 蜂鸣相关指令。依据《佳博热敏票据打印机编程手册 v1.0.5》命令 62(ESC B)、63(ESC C)。
// 注:蜂鸣属厂商扩展指令,不同机型可能有差异;当前以佳博为准,多品牌接入时再适配。

// Buzzer 让打印机蜂鸣(ESC B n t)。
// times = 鸣叫次数(1~9);durationUnits = 每次时长单位(1~9,每单位 50ms)。
func (b *Builder) Buzzer(times, durationUnits int) *Builder {
	return b.raw(0x1B, 0x42, clampByte(times, 1, 9), clampByte(durationUnits, 1, 9))
}

// BuildBuzzer 生成一条独立蜂鸣指令(不含小票内容),用于「蜂鸣测试」或来单提示。
func BuildBuzzer(times, durationUnits int) []byte {
	return []byte{0x1B, 0x42, clampByte(times, 1, 9), clampByte(durationUnits, 1, 9)}
}

// 蜂鸣/报警模式(ESC C 的 n 参数)。
const (
	AlarmNone   = 0 // 不鸣叫、不闪灯
	AlarmBuzzer = 1 // 仅蜂鸣
	AlarmLight  = 2 // 仅报警灯闪烁
	AlarmBoth   = 3 // 蜂鸣 + 报警灯闪烁
)

// BuildBuzzerAndAlarm 生成蜂鸣并/或报警灯闪烁指令(ESC C m t n),适用于带报警灯的厨房打印机。
// count = 次数(1~20);intervalUnits = 间隔(1~20,每单位 50ms);mode = AlarmNone/AlarmBuzzer/AlarmLight/AlarmBoth。
func BuildBuzzerAndAlarm(count, intervalUnits, mode int) []byte {
	return []byte{0x1B, 0x43, clampByte(count, 1, 20), clampByte(intervalUnits, 1, 20), clampByte(mode, 0, 3)}
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
