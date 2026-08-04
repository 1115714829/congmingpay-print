package com.congmingpay.android.escpos

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * 测试小票生成。
 */
object Receipt {
    private val timeFmt = SimpleDateFormat("yyyy-MM-dd HH:mm:ss", Locale.getDefault())

    /**
     * 生成一张中文测试小票的正文（ESC/POS），按纸宽自适应排版。
     * 收尾（首/尾走纸、切纸）由 PrintDispatcher 按打印机设置统一追加，这里不含。
     */
    fun buildTestReceipt(now: Date, widthMM: Int): ByteArray {
        val cpl = Layout.charsPerLine(widthMM)
        val divider = "-".repeat(cpl)
        return EscposBuilder()
            .setAlign(ALIGN_CENTER)
            .setSize(1, 1).setEmphasize(true)
            .line("聪明付 打印测试")
            .setSize(0, 0).setEmphasize(false)
            .line("congmingpay Print Test")
            .setAlign(ALIGN_LEFT)
            .line(divider)
            .line("时间: ${timeFmt.format(now)}")
            .line("纸宽: ${widthMM}mm  ($cpl 字符/行)")
            .line("指令: ESC/POS   编码: GB18030")
            .line(divider)
            .setAlign(ALIGN_CENTER)
            .line("能清晰打印本票并正确切纸")
            .line("即表示该打印通道正常")
            .feed(1)
            .line("扫码验证二维码")
            .qrCode("congmingpay QR ${SimpleDateFormat("HH:mm:ss", Locale.getDefault()).format(now)}", 6)
            .bytes()
    }
}
