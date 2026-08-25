package com.congmingpay.android.layout

import com.congmingpay.android.errcode.CodedException
import com.congmingpay.android.errcode.ErrCode
import com.congmingpay.android.escpos.EscposBuilder
import com.congmingpay.android.escpos.Layout
import com.congmingpay.android.logger.Logger
import com.congmingpay.android.util.boolOr
import com.congmingpay.android.util.hasJson
import com.congmingpay.android.util.intOr
import com.congmingpay.android.util.strOr
import com.google.gson.Gson
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser

/**
 * JSON 小票排版渲染器（严格模式）。
 *
 * 把云端 MQTT type=0 的 contents 数组渲染为 ESC/POS 字节。
 * 字段缺失=文档声明默认（合法）；字段存在但非法 → 整单拒绝。
 */
object LayoutRenderer {

    private val gson = Gson()

    /**
     * 渲染 contents JSON 为 ESC/POS 字节。
     *
     * @param contents JSON 排版数组字符串
     * @param widthMM 纸宽（58/80）
     * @param compat 云盒兼容模式（true 时接受 type=cut 并返回切纸意图）
     * @return RenderResult（data=字节, contentCut=切纸意图或 null）
     * @throws CodedException 渲染失败（code=RenderInvalid/RenderNotArray/EncodeFailed）
     */
    fun render(contents: String, widthMM: Int, compat: Boolean): RenderResult {
        val elems: List<JsonElement> = try {
            val parsed = JsonParser.parseString(contents)
            if (!parsed.isJsonArray) {
                throw CodedException(ErrCode.RENDER_NOT_ARRAY, "contents 非排版元素数组")
            }
            parsed.asJsonArray.toList()
        } catch (e: CodedException) {
            throw e
        } catch (e: Exception) {
            throw CodedException(ErrCode.RENDER_NOT_ARRAY, "contents 解析失败: ${e.message}")
        }

        val r = Renderer(widthMM, compat)
        for (i in elems.indices) {
            try {
                r.element(elems[i].asJsonObject)
            } catch (e: CodedException) {
                throw e
            } catch (e: Exception) {
                throw CodedException(
                    ErrCode.RENDER_INVALID,
                    "contents[$i](${kindOf(elems[i].asJsonObject)}): ${e.message}"
                )
            }
        }

        val data = try {
            r.builder.bytes()
        } catch (e: Exception) {
            throw CodedException(ErrCode.ENCODE_FAILED, e.message ?: "")
        }
        return RenderResult(data, r.contentCut)
    }

    /** 渲染结果 */
    data class RenderResult(val data: ByteArray, val contentCut: Boolean?)
}

/**
 * 内部渲染状态。
 */
internal class Renderer(widthMM: Int, val compat: Boolean) {
    val builder = EscposBuilder()
    val width = widthMM
    val cpl = Layout.charsPerLine(widthMM)
    var contentCut: Boolean? = null

    fun element(e: JsonObject) {
        // 表格优先检测
        if (e.hasJson("thead")) {
            table(e)
            return
        }
        if (e.has("tbody") && e.get("tbody").isJsonArray && e.getAsJsonArray("tbody").size() > 0) {
            throw RuntimeException("有 tbody 但缺 thead")
        }
        if (e.has("both_sides") && e.get("both_sides").isJsonArray) {
            bothSides(e)
            return
        }

        when (val type = e.strOr("type", "")) {
            "", "text" -> text(e)
            "title" -> title(e)
            "div_line" -> divider(e, "-")
            "div_star" -> divider(e, "*")
            "qrcode" -> qrcode(e)
            "bc128" -> barcode(e, "bc128")
            "bc128a" -> barcode(e, "bc128a")
            "bc128c" -> barcode(e, "bc128c")
            "code39" -> barcode(e, "code39")
            "png" -> png(e)
            "bmp" -> bmp(e)
            "plugin" -> plugin(e)
            "cut" -> {
                if (!compat) {
                    throw RuntimeException("未知元素 type \"$type\"(支持 text/title/div_line/div_star/qrcode/bc128/bc128a/bc128c/code39/png/bmp/plugin)")
                }
                contentCut = parseCutCont(e)
            }
            else -> {
                val sup = if (compat)
                    "text/title/div_line/div_star/qrcode/bc128/bc128a/bc128c/code39/png/bmp/plugin/cut"
                else
                    "text/title/div_line/div_star/qrcode/bc128/bc128a/bc128c/code39/png/bmp/plugin"
                throw RuntimeException("未知元素 type \"$type\"(支持 $sup)")
            }
        }
    }
}

// —— JSON 字段辅助函数（cont 相关为 layout 专用，其余在 util/Json.kt 共享）—— //

/** cont 当字符串返回；缺失/null→空串；存在但非字符串→报错 */
internal fun JsonObject.contString(): String {
    if (!has("cont")) return ""
    val v = get("cont")
    if (v.isJsonNull) return ""
    if (v.isJsonPrimitive && v.asJsonPrimitive.isString) return v.asString
    throw RuntimeException("cont 需为字符串")
}

/** cont 当字符串数组返回；非数组→null（交调用方按字符串处理）；是数组但元素非字符串→报错 */
internal fun JsonObject.contArray(): List<String>? {
    if (!has("cont")) return null
    val v = get("cont")
    if (v.isJsonNull || !v.isJsonArray) return null
    return v.asJsonArray.map { el ->
        if (el.isJsonPrimitive && el.asJsonPrimitive.isString) el.asString
        else throw RuntimeException("cont 数组元素需为字符串")
    }
}

internal fun kindOf(e: JsonObject): String {
    if (e.hasJson("thead") || (e.has("tbody") && e.get("tbody").isJsonArray)) return "table"
    if (e.has("both_sides")) return "both_sides"
    val t = e.strOr("type", "")
    return if (t.isEmpty()) "text" else t
}

/** C3: cut 元素 cont "0"=全切、"1"=半切,均触发切纸;0/1 与 true → 切;false/"off" → 不切 */
internal fun parseCutCont(e: JsonObject): Boolean {
    if (!e.has("cont")) return true
    val v = e.get("cont")
    if (v.isJsonNull) return true
    if (v.isJsonPrimitive) {
        val p = v.asJsonPrimitive
        if (p.isBoolean) return p.asBoolean
        if (p.isNumber) {
            val n = p.asInt
            if (n == 0 || n == 1) return true
            throw RuntimeException("cut.cont 需为 0 或 1")
        }
        if (p.isString) {
            return when (p.asString.trim()) {
                "1", "0", "true", "on" -> true
                "false", "off" -> false
                else -> throw RuntimeException("cut.cont 需为 0 或 1")
            }
        }
    }
    throw RuntimeException("cut.cont 需为 0 或 1")
}

/** plugin 开钱箱:cont "0"/"1" 选引脚(0=引脚 2、1=引脚 5);缺失 → 引脚 2 */
internal fun parsePluginPin(e: JsonObject): Int {
    if (!e.has("cont")) return 0
    val v = e.get("cont")
    if (v.isJsonNull) return 0
    if (v.isJsonPrimitive) {
        val p = v.asJsonPrimitive
        if (p.isNumber) {
            val n = p.asInt
            if (n == 0 || n == 1) return n
            throw RuntimeException("plugin.cont 需为 0 或 1")
        }
        if (p.isString) {
            return when (p.asString.trim()) {
                "0" -> 0
                "1" -> 1
                else -> throw RuntimeException("plugin.cont 需为 0 或 1")
            }
        }
    }
    throw RuntimeException("plugin.cont 需为 0 或 1")
}

/** plugin 开钱箱:发出 ESC p 脉冲指令 */
internal fun Renderer.plugin(e: JsonObject) {
    builder.cashDrawer(parsePluginPin(e))
}
