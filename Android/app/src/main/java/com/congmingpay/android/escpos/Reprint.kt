package com.congmingpay.android.escpos

/**
 * 重印抬头生成。
 */
object Reprint {
    /**
     * 生成醒目的「打印服务器重印」抬头，用于失败自动重印 / 重新打印的小票顶部。
     * 按纸宽自适应星号线宽度。抬头之后接原始小票内容。
     */
    fun reprintBanner(widthMM: Int): ByteArray {
        val cpl = Layout.charsPerLine(widthMM)
        val line = "*".repeat(cpl)
        return EscposBuilder()
            .setAlign(ALIGN_CENTER)
            .line(line)
            .setEmphasize(true).setSize(1, 1)
            .line("重打小票")
            .setSize(0, 0)
            .line("打印服务器 重印")
            .setEmphasize(false)
            .line(line)
            .setAlign(ALIGN_LEFT)
            .feed(1)
            .bytes()
    }
}
