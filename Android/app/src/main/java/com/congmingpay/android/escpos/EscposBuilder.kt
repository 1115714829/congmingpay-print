package com.congmingpay.android.escpos

import com.congmingpay.android.util.toGB18030
import java.io.ByteArrayOutputStream

/** 对齐方式 */
const val ALIGN_LEFT: Byte = 0
const val ALIGN_CENTER: Byte = 1
const val ALIGN_RIGHT: Byte = 2

/**
 * ESC/POS 指令构建器（链式调用）。
 * 累积字节到内部缓冲区，首个错误记录后在 [bytes] 时抛出。
 */
class EscposBuilder {
    private val buf = ByteArrayOutputStream()
    private var error: Throwable? = null

    init {
        init()
    }

    /** 初始化打印机（ESC @） */
    fun init(): EscposBuilder = raw(0x1B, 0x40)

    /** 设置对齐（ESC a n） */
    fun setAlign(align: Byte): EscposBuilder = raw(0x1B, 0x61, align.toInt())

    /** 设置加粗（ESC E n） */
    fun setEmphasize(on: Boolean): EscposBuilder = raw(0x1B, 0x45, if (on) 1 else 0)

    /** 设置字符放大倍数（GS ! n）。wMag/hMag 为 0-7（0=1倍…7=8倍） */
    fun setSize(wMag: Int, hMag: Int): EscposBuilder =
        raw(0x1D, 0x21, ((clampByte(wMag, 0, 7).toInt() shl 4) or clampByte(hMag, 0, 7).toInt()))

    /** 追加一段文本（GB18030 编码），不换行 */
    fun text(s: String): EscposBuilder {
        if (error != null) return this
        buf.write(toGB18030(s))
        return this
    }

    /** 追加一行文本并换行 */
    fun line(s: String): EscposBuilder = text(s).feed(1)

    /** 走纸 n 行（n 个 LF） */
    fun feed(n: Int): EscposBuilder {
        repeat(n) { raw(0x0A) }
        return this
    }

    /** 追加任意原始字节（用于嵌入已生成的位图/条码指令） */
    fun raw(vararg bs: Int): EscposBuilder {
        if (error != null) return this
        for (b in bs) buf.write(b and 0xFF)
        return this
    }

    /** 追加原始字节数组 */
    fun raw(data: ByteArray): EscposBuilder {
        if (error != null) return this
        buf.write(data)
        return this
    }

    /** 返回累积的字节；过程中若出错则抛出该错误 */
    fun bytes(): ByteArray {
        error?.let { throw it }
        return buf.toByteArray()
    }

    // —— QR 码（GS ( K, Model 2）——
    // 依据《佳博热敏票据打印机编程手册 v1.0.5》命令 65-68
    fun qrCode(data: String, moduleSize: Int): EscposBuilder {
        if (error != null) return this
        val ms = clampByte(moduleSize, 1, 15)
        val d = data.toByteArray(Charsets.UTF_8)

        raw(0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x43, ms.toInt())  // 模块大小
        raw(0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x45, 0x31)         // 纠错等级 M

        val n = d.size + 3  // 存储数据：pL pH = 数据长 + 3
        raw(0x1D, 0x28, 0x6B, n and 0xFF, (n shr 8) and 0xFF, 0x31, 0x50, 0x30)
        buf.write(d)

        raw(0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x51, 0x30)  // 打印
        return this
    }

    // —— CODE128 条码（GS k 73）——
    // 前置 "{B" 选码集 B。hri：0=无 1=上方 2=下方 3=上下
    fun code128(data: String, heightDots: Int, moduleWidth: Int, hri: Int): EscposBuilder {
        barcodePrep(heightDots, moduleWidth, hri)
        val payload = "{B$data"
        val pb = payload.toByteArray(Charsets.US_ASCII)
        raw(0x1D, 0x6B, 73, pb.size)
        buf.write(pb)
        return this
    }

    // —— CODE39 条码（GS k 69）——
    fun code39(data: String, heightDots: Int, moduleWidth: Int, hri: Int): EscposBuilder {
        barcodePrep(heightDots, moduleWidth, hri)
        val db = data.toByteArray(Charsets.US_ASCII)
        raw(0x1D, 0x6B, 69, db.size)
        buf.write(db)
        return this
    }

    private fun barcodePrep(heightDots: Int, moduleWidth: Int, hri: Int) {
        raw(0x1D, 0x48, clampByte(hri, 0, 3).toInt())           // GS H：HRI 位置
        raw(0x1D, 0x66, 0x00)                                    // GS f：HRI 字体 A
        raw(0x1D, 0x68, clampByte(heightDots, 1, 255).toInt())   // GS h：高度
        raw(0x1D, 0x77, clampByte(moduleWidth, 2, 6).toInt())    // GS w：模块宽
    }
}

/** 将 int 钳制到 [lo, hi] 范围并转为 Byte */
internal fun clampByte(v: Int, lo: Int, hi: Int): Byte =
    v.coerceIn(lo, hi).toByte()
