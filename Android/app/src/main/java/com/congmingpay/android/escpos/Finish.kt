package com.congmingpay.android.escpos

/**
 * 打印收尾字节生成。
 */
object Finish {
    /** 返回 n 个换行（走纸 n 行）；n<=0 返回空 */
    fun feedBytes(n: Int): ByteArray {
        if (n <= 0) return ByteArray(0)
        return ByteArray(n) { 0x0A }
    }

    /**
     * 生成打印收尾字节：尾部走纸（TailFeed=按纸宽的基数+偏移）+ 可选半切（GS V 1）。
     * 不含初始化；由 PrintDispatcher 统一追加到内容之后。
     */
    fun finish(widthMM: Int, tailOffset: Int, cut: Boolean): ByteArray {
        val out = java.io.ByteArrayOutputStream()
        out.write(feedBytes(Layout.tailFeed(widthMM, tailOffset)))
        if (cut) {
            out.write(0x1D)
            out.write(0x56)
            out.write(0x01)
        }
        return out.toByteArray()
    }
}
