package com.congmingpay.android.escpos

/**
 * 纸宽自适应布局。
 */
object Layout {
    /**
     * 返回给定纸宽(mm)每行可打印的标准字体字符数。
     * 58mm = 384 点 = 32 字符；80mm = 576 点 = 48 字符。其它宽度按 80mm 处理。
     */
    fun charsPerLine(widthMM: Int): Int = if (widthMM == 58) 32 else 48

    /** 返回该纸宽尾部空行的标准基数：58mm=5；80mm=3 */
    fun baseTailLinesFor(widthMM: Int): Int = if (widthMM == 58) 5 else 3

    /** 实际尾部空行数 = 基数(按纸宽) + 用户偏移，下限 0 */
    fun tailFeed(widthMM: Int, offset: Int): Int =
        (baseTailLinesFor(widthMM) + offset).coerceAtLeast(0)

    /** 字符串显示列宽：ASCII/半角=1，中文/全角(非 ASCII)=2 */
    fun displayWidth(s: String): Int {
        var w = 0
        for (c in s) {
            w += if (c.code > 0x7F) 2 else 1
        }
        return w
    }

    /** 返回该纸宽的可打印点宽：58mm=384、80mm=576（标准 203dpi） */
    fun widthDots(widthMM: Int): Int = if (widthMM == 58) 384 else 576
}
