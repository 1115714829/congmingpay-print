package com.congmingpay.android.layout

import com.congmingpay.android.escpos.ALIGN_LEFT
import com.congmingpay.android.escpos.Layout
import com.congmingpay.android.util.intOr
import com.congmingpay.android.util.strOr
import com.google.gson.JsonObject

// —— 表格渲染（thead + tbody）——

internal fun Renderer.table(e: JsonObject) {
    val (names, pcts) = parseThead(e)
    val sum = pcts.sum()
    if (sum != 100) {
        throw RuntimeException("thead 列宽总和 ${sum}% ≠ 100%")
    }

    val cpl = this.cpl
    val (w, _) = sizeMag(e.strOr("size", ""))

    // 按纸宽满行分配列宽；末列吸收取整误差
    val cols = IntArray(pcts.size)
    var used = 0
    for (i in pcts.indices) {
        cols[i] = pcts[i] * cpl / 100
        if (cols[i] < 1) cols[i] = 1
        used += cols[i]
    }
    cols[cols.size - 1] += cpl - used
    if (cols[cols.size - 1] < 1) cols[cols.size - 1] = 1

    // 校验 tbody 行。tbody 缺失/非数组 → 只渲染表头(对齐 Windows：Tbody 为空即跳过行循环)
    val tbody = if (e.has("tbody") && e.get("tbody").isJsonArray) {
        e.getAsJsonArray("tbody")
    } else {
        com.google.gson.JsonArray()
    }
    // 非数组行跳过（Windows 在 Element 反序列化阶段整单失败；Android 按行解析，跳过以对齐「不单独报 需为数组」）
    val rows = ArrayList<List<String>>(tbody.size())
    for (ri in 0 until tbody.size()) {
        val row = tbody[ri]
        if (!row.isJsonArray) continue
        val rowArr = row.asJsonArray
        if (rowArr.size() != pcts.size) {
            throw RuntimeException("tbody 第 ${ri + 1} 行有 ${rowArr.size()} 个单元格,与列数 ${pcts.size} 不符")
        }
        val cells = ArrayList<String>(rowArr.size())
        for (ci in 0 until rowArr.size()) {
            cells.add(cellString(rowArr[ci], ri + 1, ci + 1))
        }
        rows.add(cells)
    }

    if (names != null) {
        tableRow(names, cols, 1)
        if (e.intOr("line_div", 0) == 1) builder.line("-".repeat(cpl))
    }

    val scale = if (w > 0) { builder.setSize(w, w); w + 1 } else 1
    for (cells in rows) {
        tableRow(cells, cols, scale)
        if (e.intOr("line_div", 0) == 1) builder.line("-".repeat(cpl))
        val lineSpace = e.intOr("line_space", 0)
        repeat(lineSpace) { builder.feed(1) }
    }
    if (w > 0) builder.setSize(0, 0)
}

private fun Renderer.tableRow(cells: List<String>, cols: IntArray, scale: Int) {
    val wrapped = ArrayList<List<String>>(cols.size)
    var maxLines = 1
    for (i in cols.indices) {
        val cell = if (i < cells.size) cells[i] else ""
        val lines = wrapByWidth(cell, cols[i])
        wrapped.add(lines)
        if (lines.size > maxLines) maxLines = lines.size
    }
    for (ln in 0 until maxLines) {
        val sb = StringBuilder()
        for (i in cols.indices) {
            val seg = if (ln < wrapped[i].size) wrapped[i][ln] else ""
            sb.append(padCell(seg, cols[i], scale))
        }
        builder.setAlign(ALIGN_LEFT).line(sb.toString().trimEnd())
    }
}

private fun padCell(s: String, cw: Int, scale: Int): String {
    var pad = (cw - Layout.displayWidth(s)) * scale
    if (pad < 0) pad = 0
    return s + " ".repeat(pad)
}

internal fun wrapByWidth(s: String, cw: Int): List<String> {
    if (s.isEmpty()) return listOf("")
    val lines = mutableListOf<String>()
    val cur = StringBuilder()
    var curw = 0
    for (ru in s) {
        val rw = if (ru.code > 0x7F) 2 else 1
        if (curw + rw > cw && curw > 0) {
            lines.add(cur.toString())
            cur.clear()
            curw = 0
        }
        cur.append(ru)
        curw += rw
    }
    lines.add(cur.toString())
    return lines
}

internal fun parseThead(e: JsonObject): Pair<List<String>?, List<Int>> {
    val raw = e.get("thead")
    // hasJson("thead") 已挡 null；此处兜底对齐 Windows parseThead 对非法形态的文案
    if (raw == null || raw.isJsonNull) {
        throw RuntimeException("thead 需为对象{列名:\"宽%\"}或字符串数组[\"宽%\"]")
    }

    // 数组形态：["50%","20%",...]
    if (raw.isJsonArray) {
        val arr = raw.asJsonArray
        val pcts = ArrayList<Int>(arr.size())
        for (el in arr) {
            if (!el.isJsonPrimitive || !el.asJsonPrimitive.isString) {
                throw RuntimeException("thead 需为对象{列名:\"宽%\"}或字符串数组[\"宽%\"]")
            }
            pcts.add(parsePct(el.asString))
        }
        if (pcts.isEmpty()) throw RuntimeException("thead 为空(至少 1 列)")
        return null to pcts
    }

    // 对象形态：保序读取键值
    if (raw.isJsonObject) {
        val obj = raw.asJsonObject
        val names = mutableListOf<String>()
        val pcts = mutableListOf<Int>()
        for ((key, value) in obj.entrySet()) {
            if (!value.isJsonPrimitive || !value.asJsonPrimitive.isString) {
                throw RuntimeException("thead 第 ${pcts.size + 1} 列宽度需为字符串(如 \"50%\")")
            }
            names.add(key)
            pcts.add(parsePct(value.asString))
        }
        if (pcts.isEmpty()) throw RuntimeException("thead 为空(至少 1 列)")
        return names to pcts
    }

    throw RuntimeException("thead 需为对象{列名:\"宽%\"}或字符串数组[\"宽%\"]")
}

internal fun parsePct(s: String): Int {
    val t = s.trim().removeSuffix("%").trim()
    val n = t.toIntOrNull()
        ?: throw RuntimeException("thead 列宽 \"$s\" 非法(需 1-100 的整数百分比)")
    if (n < 1 || n > 100) throw RuntimeException("thead 列宽 ${n}% 超出 1-100")
    return n
}

internal fun cellString(v: com.google.gson.JsonElement, row: Int, col: Int): String {
    if (!v.isJsonPrimitive) {
        throw RuntimeException("tbody 第 $row 行第 $col 列值非法(需字符串/数字/布尔)")
    }
    val p = v.asJsonPrimitive
    if (p.isString) return p.asString
    if (p.isNumber) {
        // 统一定点格式,禁用科学计数法(对齐 Go FormatFloat('f',-1,64));
        // 不单独走整数快速路径:避免 2^63 边界 toLong 饱和 off-by-one
        return java.math.BigDecimal.valueOf(p.asDouble).stripTrailingZeros().toPlainString()
    }
    if (p.isBoolean) return p.asBoolean.toString()
    throw RuntimeException("tbody 第 $row 行第 $col 列值非法(需字符串/数字/布尔)")
}
