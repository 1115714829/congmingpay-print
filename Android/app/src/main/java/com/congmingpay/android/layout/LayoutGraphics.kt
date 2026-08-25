package com.congmingpay.android.layout

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.util.Base64
import com.congmingpay.android.escpos.Layout
import com.congmingpay.android.escpos.Raster
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

private const val QR_MAX_BYTES = 512
private const val QR_DEFAULT = 4

/** qrcode size："01"-"09" → 模块大小；缺省＝"04"（厂商默认） */
private fun qrSize(size: String): Int {
    if (size.isEmpty()) return QR_DEFAULT
    if (size.length == 2 && size[0] == '0' && size[1] in '1'..'9') {
        return size[1] - '0'
    }
    throw RuntimeException("size \"$size\" 非法(qrcode 需两位 01-09 数字,如 \"04\")")
}

internal fun Renderer.qrcode(e: JsonObject) {
    applyAlign(e.strOr("align", ""))
    val m = qrSize(e.strOr("size", ""))
    val arr = e.contArray()
    if (arr != null) {
        if (arr.size != 2) {
            throw RuntimeException("cont 为数组时需恰含 2 项(双码),实得 ${arr.size} 项")
        }
        for (s in arr) {
            val n = s.toByteArray(Charsets.UTF_8).size
            if (n > QR_MAX_BYTES) throw RuntimeException("二维码内容过长($n 字节,上限 $QR_MAX_BYTES)")
        }
        doubleQR(arr[0], arr[1], m)
    } else {
        val s = e.contString()
        if (s.isEmpty()) throw RuntimeException("二维码内容为空")
        val n = s.toByteArray(Charsets.UTF_8).size
        if (n > QR_MAX_BYTES) throw RuntimeException("二维码内容过长($n 字节,上限 $QR_MAX_BYTES)")
        builder.qrCode(s, m) // 单码走 native
    }
    builder.setAlign(com.congmingpay.android.escpos.ALIGN_LEFT)
}

/** 并排双码 → 合成位图光栅；moduleSize 决定位图边长(30×模块,模块 6=180 与旧默认一致) */
private fun Renderer.doubleQR(a: String, b: String, moduleSize: Int) {
    val ia = try { genQR(a, moduleSize) } catch (e: Exception) {
        throw RuntimeException("第 1 个二维码生成失败: ${e.message}")
    }
    val ib = try { genQR(b, moduleSize) } catch (e: Exception) {
        throw RuntimeException("第 2 个二维码生成失败: ${e.message}")
    }
    val composed = composeSideBySide(ia, ib, 24)
    builder.raw(Raster.raster(composed, Layout.widthDots(width)))
}

private fun genQR(data: String, moduleSize: Int): Bitmap {
    val writer = QRCodeWriter()
    val size = 30 * moduleSize
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

internal fun Renderer.barcode(e: JsonObject, typ: String) {
    applyAlign(e.strOr("align", ""))
    val (h, mw) = barcodeSize(e.strOr("size", ""))
    val hri = e.intOr("hri", 0)
    if (hri < 0 || hri > 3) {
        throw RuntimeException("hri $hri 非法(0=无 1=上 2=下 3=上下)")
    }
    val s = e.contString()
    if (s.isEmpty()) throw RuntimeException("条码内容为空")
    if (s.toByteArray(Charsets.US_ASCII).size > 250) throw RuntimeException("条码内容过长(${s.length} 字节,上限 250)")
    when (typ) {
        "code39" -> builder.code39(s, h, mw, hri)
        "bc128c" -> {
            if (!s.all { it in '0'..'9' }) throw RuntimeException("bc128c 内容仅支持数字(0-9)")
            if (s.length > 28) throw RuntimeException("bc128c 内容过长(${s.length} 位,上限 28)")
            builder.code128(s, 'C', h, mw, hri)
        }
        else -> { // bc128 / bc128a：Code128 码集 A
            if (!s.all { it in 'A'..'Z' || it in '0'..'9' }) {
                throw RuntimeException("$typ 内容仅支持大写字母与数字(0-9/A-Z)")
            }
            if (s.length > 14) throw RuntimeException("$typ 内容过长(${s.length} 位,上限 14)")
            builder.code128(s, 'A', h, mw, hri)
        }
    }
    builder.setAlign(com.congmingpay.android.escpos.ALIGN_LEFT)
}

/** 条码规格 "AB"：A=模块宽 1-6(默认2,GS w 下限按 2 夹取)、B=高度档 0-9(高度=(B+1)×24 点,默认2)。缺省 = A2 B2 → (72,2) */
private fun barcodeSize(size: String): Pair<Int, Int> {
    if (size.isEmpty()) return 72 to 2
    if (size.length == 2 && size[0] in '1'..'6' && size[1] in '0'..'9') {
        val mw = maxOf(size[0] - '0', 2)
        val h = (size[1] - '0' + 1) * 24
        return h to mw
    }
    throw RuntimeException("size \"$size\" 非法(条码需两位:模块宽 1-6 + 高度档 0-9,如 \"22\")")
}

internal fun Renderer.png(e: JsonObject) {
    applyAlign(e.strOr("align", ""))
    val s = e.contString()
    val data = fetchImageBytes(s, "png")
    if (data.size > 60 * 1024) throw RuntimeException("png 内容过大(${data.size} 字节,上限 60K)")
    val img = decodeImageBytes(data, "png")
    builder.raw(Raster.raster(img, Layout.widthDots(width)))
    builder.setAlign(com.congmingpay.android.escpos.ALIGN_LEFT)
}

/** bmp 元素：24/32 位未压缩 BMP（BitmapFactory 原生支持），来源与 png 相同；字节上限 6K */
internal fun Renderer.bmp(e: JsonObject) {
    applyAlign(e.strOr("align", ""))
    val s = e.contString()
    val data = fetchImageBytes(s, "bmp")
    if (data.size > 6 * 1024) throw RuntimeException("bmp 内容过大(${data.size} 字节,上限 6K)")
    if (data.size < 2 || data[0] != 'B'.code.toByte() || data[1] != 'M'.code.toByte()) {
        throw RuntimeException("bmp 解码失败: 非 BMP 数据")
    }
    val img = decodeImageBytes(data, "bmp")
    builder.raw(Raster.raster(img, Layout.widthDots(width)))
    builder.setAlign(com.congmingpay.android.escpos.ALIGN_LEFT)
}

/**
 * 按来源取图片原始字节（data URI 与 http(s) URL）。
 * 取源范围与文案对齐 Windows `layout.FetchImageBytes`：局域网/私网地址与 http 明文一律放行。
 * kind 用于错误文案（如 "png"/"bmp"）。
 */
private fun fetchImageBytes(src: String, kind: String): ByteArray {
    val trimmed = src.trim()
    if (trimmed.isEmpty()) throw RuntimeException("$kind 内容为空")

    return when {
        trimmed.startsWith("data:") -> {
            val i = trimmed.indexOf(",")
            if (i < 0) throw RuntimeException("$kind 来源非法(data: 需为 data:image/...;base64,<数据>)")
            try {
                Base64.decode(trimmed.substring(i + 1).trim(), Base64.DEFAULT)
            } catch (e: Exception) {
                throw RuntimeException("$kind base64 解码失败: ${e.message}")
            }
        }
        trimmed.startsWith("http://") || trimmed.startsWith("https://") -> {
            try {
                val resp = httpClient.newCall(Request.Builder().url(trimmed).build()).execute()
                resp.use { r ->
                    if (r.code != 200) throw RuntimeException("$kind 下载失败: HTTP ${r.code}")
                    readLimited(r.body?.byteStream())
                        ?: throw RuntimeException("$kind 下载读取失败: body 为空")
                }
            } catch (e: RuntimeException) {
                throw e
            } catch (e: Exception) {
                throw RuntimeException("$kind 下载失败: ${e.message}")
            }
        }
        else -> throw RuntimeException("$kind 来源非法(需 http(s) URL 或 data:image/...;base64,)")
    }
}

/** 解码图片字节（先读尺寸再按需降采样：移动端 OOM 兜底，不拒绝大图） */
private fun decodeImageBytes(data: ByteArray, kind: String): Bitmap {
    val bounds = android.graphics.BitmapFactory.Options().apply { inJustDecodeBounds = true }
    android.graphics.BitmapFactory.decodeStream(ByteArrayInputStream(data), null, bounds)
    if (bounds.outWidth <= 0 || bounds.outHeight <= 0) {
        throw RuntimeException("$kind 解码失败: 无法识别图片数据")
    }
    val opts = android.graphics.BitmapFactory.Options().apply {
        inSampleSize = sampleSizeFor(bounds.outWidth, bounds.outHeight)
    }
    return android.graphics.BitmapFactory.decodeStream(ByteArrayInputStream(data), null, opts)
        ?: throw RuntimeException("$kind 解码失败: 无法识别图片数据")
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



