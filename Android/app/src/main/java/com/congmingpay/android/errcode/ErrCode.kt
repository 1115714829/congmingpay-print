package com.congmingpay.android.errcode

/**
 * 全局错误码。码表与《接口文档.md》第七章一致：0=成功；首位=阶段
 * (1 报文解析 / 2 字段校验 / 3 打印目标 / 4 内容渲染 / 5 打印执行 / 9 兜底)。
 * 赋码原则：在各层边界按阶段赋码，不匹配错误文本；message 保留中文人读文本。
 */
object ErrCode {
    const val OK = 0
    const val PARSE_FAILED = 1001
    const val UNSUPPORTED_QUERY = 1002
    const val MISSING_FIELD = 2001
    const val BAD_SWITCH = 2002
    const val BAD_LINE_RANGE = 2003
    const val BAD_P_WIDTH = 2004
    const val BAD_CONTENT_TYPE = 2005
    const val NO_TARGET = 3001
    const val PRINTER_NOT_FOUND = 3002
    const val BAD_USB_TARGET = 3003
    const val WIDTH_UNKNOWN = 3004
    const val RENDER_INVALID = 4001
    const val RENDER_NOT_ARRAY = 4002
    const val ENCODE_FAILED = 4003
    const val ESC_DECODE_FAILED = 4004
    const val NET_WRITE_FAILED = 5001
    const val USB_SUBMIT_FAILED = 5002
    const val LONG_WAITING = 5101
    const val UNKNOWN = 9001
}

/**
 * 给 Exception 附加全局错误码。
 */
class CodedException(
    val code: Int,
    message: String,
    cause: Throwable? = null
) : Exception(message, cause)

/** 提取异常携带的错误码；未携带返回 UNKNOWN */
fun errorCode(e: Throwable?): Int {
    if (e is CodedException) return e.code
    return ErrCode.UNKNOWN
}

/** 包装异常为 CodedException（null 安全） */
fun codedWrap(code: Int, e: Throwable?): CodedException =
    CodedException(code, e?.message ?: "", e)
