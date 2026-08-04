package com.congmingpay.android.layout

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.util.Base64
import com.congmingpay.android.escpos.Layout
import com.congmingpay.android.escpos.Raster
import com.congmingpay.android.logger.Logger
import com.congmingpay.android.util.intOr
import com.congmingpay.android.util.strOr
import com.google.gson.JsonObject
import com.google.zxing.BarcodeFormat
import com.google.zxing.qrcode.QRCodeWriter
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.ByteArrayInputStream
import java.util.concurrent.TimeUnit

// —— 二维码/条码/图片渲染 ——

/**
 * 图片下载客户端。超时与重定向策略对齐 Windows `&http.Client{Timeout: 10 * time.Second}`：
 * 10 秒整体超时、跟随重定向、不限制目标地址（局域网图床可用）。
 */
private val httpClient by lazy {
    OkHttpClient.Builder()
        .callTimeout(10, TimeUnit.SECONDS)
        .build()
}

internal fun Renderer.qrcode(e: JsonObject) {
    applyAlign(e.strOr("align", ""))
    // size 暂不支持：不拒，每次渲染至多警告一次
    if (e.strOr("size", "").isNotEmpty() && !qrSizeWarned) {
        qrSizeWarned = true
        Logger.warn("排版警告: qrcode 的 size \"${e.strOr("size", "")}\" 暂不支持,按默认模块大小 6 渲染")
    }

    val arr = e.contArray()
    if (arr != null) {
        if (arr.size != 2) {
            throw RuntimeException("cont 为数组时需恰含 2 项(双码),实得 ${arr.size} 项")
        }
        doubleQR(arr[0], arr[1])
    } else {
        val s = e.contString()
        if (s.isEmpty()) throw RuntimeException("二维码内容为空")
        if (s.toByteArray(Charsets.UTF_8).size > 2953) {
            throw RuntimeException("二维码内容过长(${s.toByteArray(Charsets.UTF_8).size} 字节,上限 2953)")
        }
        builder.qrCode(s, 6) // 单码走 native
    }
    builder.setAlign(com.congmingpay.android.escpos.ALIGN_LEFT)
}

/** 并排双码 → 合成位图光栅 */
private fun Renderer.doubleQR(a: String, b: String) {
    val ia = try { genQR(a) } catch (e: Exception) {
        throw RuntimeException("第 1 个二维码生成失败: ${e.message}")
    }
    val ib = try { genQR(b) } catch (e: Exception) {
        throw RuntimeException("第 2 个二维码生成失败: ${e.message}")
    }
    val composed = composeSideBySide(ia, ib, 24)
    builder.raw(Raster.raster(composed, Layout.widthDots(width)))
}

private fun genQR(data: String): Bitmap {
    val writer = QRCodeWriter()
    val size = 180
    val bitMatrix = writer.encode(data, BarcodeFormat.QR_CODE, size, size)
    val pixels = IntArray(size * size)
    for (x in 0 until size) {
        for (y in 0 until size) {
            pixels[y * size + x] = if (bitMatrix.get(x, y)) Color.BLACK else Color.WHITE
        }
    }
    val bitmap = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
    bitmap.setPixels(pixels, 0, size, 0, 0, size, size)
    return bitmap
}

private fun composeSideBySide(a: Bitmap, b: Bitmap, gap: Int): Bitmap {
    val w = a.width + gap + b.width
    val h = maxOf(a.height, b.height)
    val dst = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888)
    val canvas = Canvas(dst)
    val paint = Paint().apply { color = Color.WHITE }
    canvas.drawRect(0f, 0f, w.toFloat(), h.toFloat(), paint)
    canvas.drawBitmap(a, 0f, 0f, null)
    canvas.drawBitmap(b, (a.width + gap).toFloat(), 0f, null)
    return dst
}

internal fun Renderer.barcode(e: JsonObject, kind: String) {
    applyAlign(e.strOr("align", ""))
    val (h, mw) = barcodeSize(e.strOr("size", ""))
    val hri = e.intOr("hri", 0)
    if (hri < 0 || hri > 3) {
        throw RuntimeException("hri $hri 非法(0=无 1=上 2=下 3=上下)")
    }
    val s = e.contString()
    if (s.isEmpty()) throw RuntimeException("条码内容为空")
    if (s.length > 250) throw RuntimeException("条码内容过长(${s.length} 字节,上限 250)")
    if (kind == "39") builder.code39(s, h, mw, hri)
    else builder.code128(s, h, mw, hri)
    builder.setAlign(com.congmingpay.android.escpos.ALIGN_LEFT)
}

/** 条码规格：缺失=默认(高60,模块宽2)；"22"/"33" 放大；其余非法即拒 */
private fun barcodeSize(size: String): Pair<Int, Int> = when (size) {
    "" -> 60 to 2
    "22" -> 90 to 3
    "33" -> 120 to 4
    else -> throw RuntimeException("size \"$size\" 非法(条码仅支持 \"22\"/\"33\" 或缺省)")
}

internal fun Renderer.png(e: JsonObject) {
    applyAlign(e.strOr("align", ""))
    val s = e.contString()
    val img = loadImage(s)
    builder.raw(Raster.raster(img, Layout.widthDots(width)))
    builder.setAlign(com.congmingpay.android.escpos.ALIGN_LEFT)
}

/**
 * 支持 http(s) URL 与 data:image/...;base64, 两种来源；任何失败即报错（整单拒）。
 * 取源范围与文案对齐 Windows `layout.loadImage`：局域网/私网地址与 http 明文一律放行。
 */
private fun loadImage(src: String): Bitmap {
    val trimmed = src.trim()
    if (trimmed.isEmpty()) throw RuntimeException("png 内容为空")

    val data: ByteArray = when {
        trimmed.startsWith("data:") -> {
            val i = trimmed.indexOf(",")
            if (i < 0) throw RuntimeException("png 来源非法(data: 需为 data:image/...;base64,<数据>)")
            try {
                Base64.decode(trimmed.substring(i + 1).trim(), Base64.DEFAULT)
            } catch (e: Exception) {
                throw RuntimeException("png base64 解码失败: ${e.message}")
            }
        }
        trimmed.startsWith("http://") || trimmed.startsWith("https://") -> {
            try {
                val resp = httpClient.newCall(Request.Builder().url(trimmed).build()).execute()
                resp.use { r ->
                    if (r.code != 200) throw RuntimeException("png 下载失败: HTTP ${r.code}")
                    readLimited(r.body?.byteStream())
                        ?: throw RuntimeException("png 下载读取失败: body 为空")
                }
            } catch (e: RuntimeException) {
                throw e
            } catch (e: Exception) {
                throw RuntimeException("png 下载失败: ${e.message}")
            }
        }
        else -> throw RuntimeException("png 来源非法(需 http(s) URL 或 data:image/...;base64,)")
    }

    // 先读尺寸再按需降采样解码：移动端 OOM 兜底，不拒绝大图
    // （Raster 最终按纸宽点数缩放，降采样对出纸结果无可见影响）
    val bounds = android.graphics.BitmapFactory.Options().apply { inJustDecodeBounds = true }
    android.graphics.BitmapFactory.decodeStream(ByteArrayInputStream(data), null, bounds)
    if (bounds.outWidth <= 0 || bounds.outHeight <= 0) {
        throw RuntimeException("png 解码失败: 无法识别图片数据")
    }
    val opts = android.graphics.BitmapFactory.Options().apply {
        inSampleSize = sampleSizeFor(bounds.outWidth, bounds.outHeight)
    }
    return android.graphics.BitmapFactory.decodeStream(ByteArrayInputStream(data), null, opts)
        ?: throw RuntimeException("png 解码失败: 无法识别图片数据")
}

/** 下载体积上限：与 Windows `io.LimitReader(resp.Body, 8<<20)` 同为 8MB，超出即截断 */
private const val MAX_IMAGE_BYTES = 8 * 1024 * 1024

/** 解码后单边像素上限，超出按 2 的幂降采样（仅内存兜底，不改变接受/拒绝行为） */
private const val MAX_DECODE_PIXELS = 2048

private fun sampleSizeFor(w: Int, h: Int): Int {
    val maxSide = maxOf(w, h)
    var sample = 1
    while (maxSide / sample > MAX_DECODE_PIXELS) sample *= 2
    return sample
}

/** 限流读取输入流：达到上限即截断（对齐 Windows LimitReader 语义，不报错） */
private fun readLimited(input: java.io.InputStream?): ByteArray? {
    if (input == null) return null
    val out = java.io.ByteArrayOutputStream()
    val buf = ByteArray(8192)
    var total = 0
    while (total < MAX_IMAGE_BYTES) {
        val n = input.read(buf, 0, minOf(buf.size, MAX_IMAGE_BYTES - total))
        if (n < 0) break
        total += n
        out.write(buf, 0, n)
    }
    return out.toByteArray()
}
