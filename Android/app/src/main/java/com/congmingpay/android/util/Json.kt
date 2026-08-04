package com.congmingpay.android.util

import com.google.gson.JsonObject

/**
 * JsonObject 字段读取共享扩展（api/ 与 layout/ 包共用，避免重复定义漂移）。
 */

/** 字段是否实质存在（非缺失、非 JSON null） */
fun JsonObject.hasJson(key: String): Boolean {
    if (!has(key)) return false
    return !get(key).isJsonNull
}

/** 字符串字段；缺失/null/非字符串 返回默认值 */
fun JsonObject.strOr(key: String, default: String): String {
    if (!has(key)) return default
    val v = get(key)
    if (v.isJsonNull) return default
    if (v.isJsonPrimitive && v.asJsonPrimitive.isString) return v.asString
    return default
}

/** int 字段；缺失/null/非数字 返回默认值 */
fun JsonObject.intOr(key: String, default: Int): Int {
    if (!has(key)) return default
    val v = get(key)
    if (v.isJsonNull) return default
    if (v.isJsonPrimitive && v.asJsonPrimitive.isNumber) return v.asInt
    return default
}

/** int? 字段；缺失/null 返回 null，非数字也返回 null */
fun JsonObject.nullableInt(key: String): Int? {
    if (!has(key)) return null
    val v = get(key)
    if (v.isJsonNull) return null
    if (v.isJsonPrimitive && v.asJsonPrimitive.isNumber) return v.asInt
    return null
}

/** boolean 字段；缺失/null/非布尔 返回默认值 */
fun JsonObject.boolOr(key: String, default: Boolean): Boolean {
    if (!has(key)) return default
    val v = get(key)
    if (v.isJsonNull) return default
    if (v.isJsonPrimitive && v.asJsonPrimitive.isBoolean) return v.asBoolean
    return default
}
