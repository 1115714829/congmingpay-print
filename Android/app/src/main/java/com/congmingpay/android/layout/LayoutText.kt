package com.congmingpay.android.layout

import com.congmingpay.android.escpos.ALIGN_CENTER
import com.congmingpay.android.escpos.ALIGN_LEFT
import com.congmingpay.android.escpos.ALIGN_RIGHT
import com.congmingpay.android.escpos.EscposBuilder
import com.congmingpay.android.escpos.Layout
import com.congmingpay.android.util.boolOr
import com.congmingpay.android.util.intOr
import com.congmingpay.android.util.strOr
import com.google.gson.JsonObject

// —— 文本/标题/分割线/左右两列渲染 ——

internal fun Renderer.applyAlign(align: String) {
    when (align) {
        "", "left" -> builder.setAlign(ALIGN_LEFT)
        "center" -> builder.setAlign(ALIGN_CENTER)
        "right" -> builder.setAlign(ALIGN_RIGHT)
        else -> throw RuntimeException("align \"$align\" 非法(可选 left/center/right)")
    }
}

/** 解析 size "WH" → (宽放大, 高放大)。缺失/空=不放大；存在则必须恰为两位 0-7 */
internal fun sizeMag(size: String): Pair<Int, Int> {
    if (size.isEmpty()) return 0 to 0
    if (size.length == 2) {
        val w = size[0] - '0'
        val h = size[1] - '0'
        if (w in 0..7 && h in 0..7) return w to h
    }
    throw RuntimeException("size \"$size\" 非法(需两位 0-7 数字,如 \"11\")")
}

internal fun Renderer.title(e: JsonObject) {
    val s = e.contString()
    builder.setAlign(ALIGN_CENTER).setEmphasize(true).setSize(1, 1)
        .line(s)
        .setSize(0, 0).setEmphasize(false).setAlign(ALIGN_LEFT)
}

internal fun Renderer.text(e: JsonObject) {
    applyAlign(e.strOr("align", ""))
    val (w, h) = sizeMag(e.strOr("size", ""))
    val s = e.contString()
    if (e.boolOr("bold", false)) builder.setEmphasize(true)
    if (w > 0 || h > 0) builder.setSize(w, h)
    builder.line(s)
    builder.setSize(0, 0)
    if (e.boolOr("bold", false)) builder.setEmphasize(false)
    builder.setAlign(ALIGN_LEFT)
}

internal fun Renderer.divider(e: JsonObject, ch: String) {
    val s = e.contString()
    builder.setAlign(ALIGN_LEFT).line(buildDivider(s, ch, cpl))
}

internal fun buildDivider(label: String, ch: String, cpl: Int): String {
    if (label.isEmpty()) return ch.repeat(cpl)
    val lw = Layout.displayWidth(label) + 2
    if (lw >= cpl) return label
    val total = cpl - lw
    val left = total / 2
    val right = total - left
    return ch.repeat(left) + " " + label + " " + ch.repeat(right)
}

internal fun Renderer.bothSides(e: JsonObject) {
    // 对齐 Windows Text.go：仅一条 `both_sides 需恰为 2 段文本,实得 %d 段`
    val raw = e.get("both_sides")
    val arr = if (raw != null && raw.isJsonArray) raw.asJsonArray else null
    val n = arr?.size() ?: 0
    if (n != 2) {
        throw RuntimeException("both_sides 需恰为 2 段文本,实得 $n 段")
    }
    val left = bothSideString(arr!![0], n)
    val right = bothSideString(arr[1], n)
    val (w, h) = sizeMag(e.strOr("size", ""))
    var cpl = cpl
    if (w > 0) cpl = cpl / (w + 1)
    builder.setAlign(ALIGN_LEFT)
    if (w > 0 || h > 0) builder.setSize(w, h)
    builder.line(padBetween(left, right, cpl))
    if (w > 0 || h > 0) builder.setSize(0, 0)
}

/** both_sides 元素须为字符串；非字符串时 Windows 在 Element 反序列化失败，此处用同款计数文案收口 */
private fun bothSideString(el: com.google.gson.JsonElement, n: Int): String {
    if (el.isJsonPrimitive && el.asJsonPrimitive.isString) return el.asString
    throw RuntimeException("both_sides 需恰为 2 段文本,实得 $n 段")
}

internal fun padBetween(left: String, right: String, cpl: Int): String {
    var pad = cpl - Layout.displayWidth(left) - Layout.displayWidth(right)
    if (pad < 1) pad = 1
    return left + " ".repeat(pad) + right
}
