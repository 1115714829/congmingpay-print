package com.congmingpay.android.escpos

/**
 * 蜂鸣指令。依据《佳博热敏票据打印机编程手册 v1.0.5》命令 62（ESC B）。
 * 蜂鸣属厂商扩展指令，不同机型可能有差异；当前以佳博为准，多品牌接入时再适配。
 */
object Buzzer {
    /**
     * 生成一条独立蜂鸣指令（不含小票内容）。
     * @param times 鸣叫次数（1~9）
     * @param durationUnits 每次时长单位（1~9，每单位 50ms）
     */
    fun buildBuzzer(times: Int, durationUnits: Int): ByteArray =
        byteArrayOf(0x1B, 0x42, clampByte(times, 1, 9), clampByte(durationUnits, 1, 9))
}
