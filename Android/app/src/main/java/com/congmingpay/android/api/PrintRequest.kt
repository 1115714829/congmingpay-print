package com.congmingpay.android.api

import com.congmingpay.android.util.intOr
import com.congmingpay.android.util.nullableInt
import com.congmingpay.android.util.strOr
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser

/**
 * 打印机引用（JSON 中 printer 字段可以是字符串或对象）。
 */
data class PrinterRef(
    val name: String = "",
    val id: String = "",
    val ip: String = "",
    val port: String = "",
    val brand: String = "",
    val width: Int = 0
) {
    fun isEmpty(): Boolean = name.isEmpty() && id.isEmpty() && ip.isEmpty()

    companion object {
        /**
         * 从 JSON 元素解析 PrinterRef。
         * 字符串 → name；对象 → 各字段。
         */
        fun fromJson(elem: JsonElement?): PrinterRef {
            if (elem == null || elem.isJsonNull) return PrinterRef()
            if (elem.isJsonPrimitive && elem.asJsonPrimitive.isString) {
                val s = elem.asString.trim()
                return PrinterRef(name = s)
            }
            if (elem.isJsonObject) {
                val o = elem.asJsonObject
                return PrinterRef(
                    name = o.strOr("name", ""),
                    id = o.strOr("id", ""),
                    ip = o.strOr("ip", ""),
                    port = o.strOr("port", ""),
                    brand = o.strOr("brand", ""),
                    width = o.intOr("width", 0)
                )
            }
            return PrinterRef()
        }
    }
}

/**
 * 打印请求（云端 MQTT 下发或 JSON 测试）。
 */
data class PrintRequest(
    val printer: PrinterRef = PrinterRef(),
    val gateway: String = "",
    val id: Long = 0,
    val type: Int = 0,
    val pWidth: Int = 0,
    val pCopy: Int = 0,
    val contents: JsonElement? = null,
    val buzzer: Int? = null,
    val cut: Int? = null,
    val reprint: Int? = null,
    val headLines: Int? = null,
    val tailLines: Int? = null
)

/**
 * 处理结果。
 */
data class ProcessResult(
    val jobNo: Int,
    val printer: com.congmingpay.android.model.Printer,
    val buzzer: Boolean,
    val cut: Boolean,
    val reprint: Boolean,
    val headLines: Int,
    val tailLines: Int,
    val pWidth: Int,
    val pCopy: Int,
    val contentType: Int
)

object PrintRequestParser {
    private val gson = com.google.gson.Gson()

    /**
     * 解析原始 JSON 为 PrintRequest 列表。
     * 支持单对象或数组。
     * @return (requests, wasArray, error)
     */
    fun parse(raw: ByteArray): Triple<List<PrintRequest>, Boolean, Exception?> {
        val text = raw.toString(Charsets.UTF_8).trim()
        // 文案对齐 Windows ParseRequests
        if (text.isEmpty()) return Triple(emptyList(), false, Exception("空请求体"))

        val parsed = try {
            JsonParser.parseString(text)
        } catch (e: Exception) {
            return Triple(emptyList(), false, Exception("JSON 解析失败: ${e.message}"))
        }

        val isArray = parsed.isJsonArray
        val arr = if (isArray) parsed.asJsonArray else com.google.gson.JsonArray().apply { add(parsed) }
        if (isArray && arr.size() == 0) return Triple(emptyList(), true, Exception("空数组"))

        val requests = mutableListOf<PrintRequest>()
        for (elem in arr) {
            if (!elem.isJsonObject) {
                return Triple(emptyList(), isArray, Exception("payload 元素非对象"))
            }
            requests.add(parseOne(elem.asJsonObject))
        }
        return Triple(requests, isArray, null)
    }

    fun parseOne(o: JsonObject): PrintRequest {
        return PrintRequest(
            printer = PrinterRef.fromJson(o.get("printer")),
            gateway = o.strOr("gateway", ""),
            id = o.get("id")?.takeIf { !it.isJsonNull }?.asLong ?: 0L,
            type = o.intOr("type", 0),
            pWidth = o.intOr("pWidth", 0),
            pCopy = o.intOr("pCopy", 0),
            contents = o.get("contents")?.takeIf { !it.isJsonNull },
            buzzer = o.nullableInt("buzzer"),
            cut = o.nullableInt("cut"),
            reprint = o.nullableInt("reprint"),
            headLines = o.nullableInt("headLines"),
            tailLines = o.nullableInt("tailLines")
        )
    }
}
