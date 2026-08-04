package com.congmingpay.android.escpos

import android.graphics.Bitmap
import java.io.ByteArrayOutputStream

/**
 * 图像光栅化（GS v 0 指令）。
 *
 * 把 Bitmap 转为单色位图并生成 `1D 76 30 m ...` 打印指令。
 * 采用 Floyd-Steinberg 误差扩散抖动，适合 logo/照片。
 */
object Raster {
    /**
     * @param src 源图像
     * @param maxWidthDots 纸宽点数（58=384, 80=576）
     * @return GS v 0 指令字节；图像为空返回空数组
     */
    fun raster(src: Bitmap, maxWidthDots: Int): ByteArray {
        var sw = src.width
        var sh = src.height
        if (sw <= 0 || sh <= 0) return ByteArray(0)

        // 大图先近邻降采样到 ~2 倍打印宽，避免全分辨率 IntArray 内存峰值
        // （th 取偶数使双 floor 组合与单步近邻取样严格等价，输出不变）
        var work = src
        var scaled = false
        if (sw.toLong() * sh > MAX_SRC_PIXELS) {
            val tw = (maxWidthDots * 2).coerceAtMost(sw)
            if (tw < sw) {
                var th = (sh.toLong() * tw / sw).toInt().coerceAtLeast(1)
                if (th % 2 == 1) th -= 1
                if (th < 1) th = 1
                val s = Bitmap.createScaledBitmap(src, tw, th, false)
                if (s !== src) {
                    work = s
                    scaled = true
                    sw = tw
                    sh = th
                }
            }
        }
        try {
            return rasterScaled(work, sw, sh, maxWidthDots)
        } finally {
            if (scaled) work.recycle()
        }
    }

    /** 源像素上限：超过先降采样（约 100 万像素，2MB 图数据的最坏情况） */
    private const val MAX_SRC_PIXELS = 1_000_000L

    private fun rasterScaled(src: Bitmap, sw: Int, sh: Int, maxWidthDots: Int): ByteArray {
        var w = if (sw > maxWidthDots) maxWidthDots else sw
        w = w / 8 * 8
        if (w < 8) w = 8
        var h = sh * w / sw
        if (h < 1) h = 1

        // 一次性取出全部像素（避免逐像素 JNI 往返）
        val pixels = IntArray(sw * sh)
        src.getPixels(pixels, 0, sw, 0, 0, sw, sh)

        // 缩放取样 + 灰度
        val gray = DoubleArray(w * h)
        for (y in 0 until h) {
            val sy = y * sh / h
            for (x in 0 until w) {
                val sx = x * sw / w
                val pixel = pixels[sy * sw + sx]
                val r8 = ((pixel shr 16) and 0xFF).toDouble()
                val g8 = ((pixel shr 8) and 0xFF).toDouble()
                val b8 = (pixel and 0xFF).toDouble()
                val a = ((pixel ushr 24) and 0xFF).toDouble() / 255.0
                val lum = 0.299 * r8 + 0.587 * g8 + 0.114 * b8
                gray[y * w + x] = lum * a + 255.0 * (1.0 - a) // 透明视为白
            }
        }

        // Floyd-Steinberg 二值化
        val rowBytes = w / 8
        val out = ByteArray(rowBytes * h)
        for (y in 0 until h) {
            for (x in 0 until w) {
                val old = gray[y * w + x]
                val nv = if (old < 128) {
                    out[y * rowBytes + x / 8] = (out[y * rowBytes + x / 8].toInt() or (0x80 shr (x % 8))).toByte()
                    0.0
                } else {
                    255.0
                }
                val e = old - nv
                if (x + 1 < w) gray[y * w + x + 1] += e * 7 / 16
                if (y + 1 < h) {
                    if (x > 0) gray[(y + 1) * w + x - 1] += e * 3 / 16
                    gray[(y + 1) * w + x] += e * 5 / 16
                    if (x + 1 < w) gray[(y + 1) * w + x + 1] += e * 1 / 16
                }
            }
        }

        val cmd = byteArrayOf(
            0x1D, 0x76, 0x30, 0x00,
            (rowBytes and 0xFF).toByte(), (rowBytes shr 8 and 0xFF).toByte(),
            (h and 0xFF).toByte(), (h shr 8 and 0xFF).toByte()
        )
        val bos = ByteArrayOutputStream()
        bos.write(cmd)
        bos.write(out)
        return bos.toByteArray()
    }
}
