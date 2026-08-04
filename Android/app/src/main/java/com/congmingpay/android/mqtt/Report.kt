package com.congmingpay.android.mqtt

import com.congmingpay.android.api.ProcessResult
import com.congmingpay.android.errcode.ErrCode
import com.congmingpay.android.model.Conn
import com.congmingpay.android.model.Printer
import com.congmingpay.android.printer.JobEvent
import com.congmingpay.android.printer.OnlineInfo
import com.google.gson.JsonArray
import com.google.gson.JsonObject

/**
 * 上行报文构造（report 打印回执 + state 状态快照）。
 *
 * 字段名、取值与空字段省略语义逐项对齐 Windows `internal/mqtt/Report.go` 的
 * `reportMsg`/`reportPrinter`/`reportParams`/`stateMsg`/`statePrinter`：
 * Go 结构体标签上带 `omitempty` 的字段（`jobNo`/`ip`/`port`/`usbName`/`detail`/
 * `lastPrint`/`printers`）在零值时整键不出现。
 */

/** report(accepted)：带打印机身份段与本单生效参数段 */
internal fun reportAcceptedJson(
    merchant: String,
    id: Long,
    result: ProcessResult,
    ts: Long
): JsonObject = JsonObject().apply {
    addProperty("type", "report")
    addProperty("merchant", merchant)
    addProperty("event", "accepted")
    addProperty("id", id)
    addIfNotZero("jobNo", result.jobNo)
    addProperty("code", ErrCode.OK)
    addProperty("message", "已提交")
    add("printer", printerReportJson(result.printer))
    add("params", JsonObject().apply {
        // 开关类参数在协议中为 0/1 整数（对齐 Windows reportParams）
        addProperty("buzzer", if (result.buzzer) 1 else 0)
        addProperty("cut", if (result.cut) 1 else 0)
        addProperty("reprint", if (result.reprint) 1 else 0)
        addProperty("headLines", result.headLines)
        addProperty("tailLines", result.tailLines)
        addProperty("pWidth", result.pWidth)
        addProperty("pCopy", result.pCopy)
        addProperty("contentType", result.contentType)
    })
    addProperty("ts", ts)
}

/** report(failed)：受理阶段失败，无 jobNo、无打印机身份段 */
internal fun reportFailedJson(
    merchant: String,
    id: Long,
    code: Int,
    message: String,
    ts: Long
): JsonObject = JsonObject().apply {
    addProperty("type", "report")
    addProperty("merchant", merchant)
    addProperty("event", "failed")
    addProperty("id", id)
    addProperty("code", code)
    addProperty("message", message)
    addProperty("ts", ts)
}

/** report(done/failed/waiting)：由任务事件驱动 */
internal fun reportJobEventJson(
    merchant: String,
    event: String,
    ev: JobEvent,
    ts: Long
): JsonObject = JsonObject().apply {
    addProperty("type", "report")
    addProperty("merchant", merchant)
    addProperty("event", event)
    addProperty("id", ev.cloudId ?: 0L)
    addIfNotZero("jobNo", ev.jobNo)
    addProperty("code", ev.code)
    addProperty("message", if (ev.event == "done") "打印成功" else ev.err)
    add("printer", printerReportJson(ev.printer))
    addProperty("ts", ts)
}

/** state：服务在线 + 全部打印机全量快照（配置参数 ⊕ 在线注册表） */
internal fun stateJson(
    merchant: String,
    printers: List<Printer>,
    online: Map<String, OnlineInfo>,
    ts: Long
): JsonObject = JsonObject().apply {
    addProperty("type", "state")
    addProperty("merchant", merchant)
    addProperty("online", true)
    val arr = JsonArray()
    for (p in printers) arr.add(statePrinterJson(p, online[p.id]))
    if (arr.size() > 0) add("printers", arr)
    addProperty("ts", ts)
}

/** state 简版离线：LWT 遗嘱与主动收尾共用，无 printers、无 ts */
internal fun stateOfflineJson(merchant: String): JsonObject = JsonObject().apply {
    addProperty("type", "state")
    addProperty("merchant", merchant)
    addProperty("online", false)
}

/** report 的打印机身份段（对齐 Windows `reportPrinterOf`），仅身份字段 */
internal fun printerReportJson(p: Printer): JsonObject = JsonObject().apply {
    addProperty("printerId", p.id)
    addProperty("printer", p.name)
    addProperty("brand", p.brandLabel())
    addProperty("width", p.width)
    addProperty("conn", p.conn)
    addAddress(p)
}

/** state 的单台打印机条目（配置参数 + 在线状态合一） */
internal fun statePrinterJson(p: Printer, oi: OnlineInfo?): JsonObject = JsonObject().apply {
    addProperty("printerId", p.id)
    addProperty("printer", p.name)
    addProperty("brand", p.brandLabel())
    addProperty("width", p.width)
    addProperty("conn", p.conn)
    addAddress(p)
    if (oi != null) {
        addProperty("online", oi.online)
        addIfNotEmpty("detail", oi.detail)
    } else {
        // 启动后首次判定尚未完成（监测未定态不写注册表）：按离线报告，detail 标「检测中」
        addProperty("online", false)
        addProperty("detail", "检测中")
    }
    addProperty("buzzer", p.buzzer)
    addProperty("cut", p.cuts())
    addProperty("headLines", p.headLines)
    addProperty("tailLines", p.tailLines)
    addProperty("source", p.effectiveSource())
    addIfNotEmpty("lastPrint", p.lastPrint)
}

/** 地址字段：USB 出 usbName、蓝牙出 bluetoothMac（Android 独有连接方式），其余出 ip+port */
private fun JsonObject.addAddress(p: Printer) {
    when (p.conn) {
        Conn.USB -> addIfNotEmpty("usbName", p.usbName)
        Conn.BLUETOOTH -> addIfNotEmpty("bluetoothMac", p.bluetoothMac)
        else -> {
            addIfNotEmpty("ip", p.ip)
            addIfNotEmpty("port", p.port)
        }
    }
}

/** 空串不写（对齐 Go `json:"...,omitempty"`） */
internal fun JsonObject.addIfNotEmpty(key: String, value: String) {
    if (value.isNotEmpty()) addProperty(key, value)
}

/** 0 不写（对齐 Go 数值字段的 omitempty） */
internal fun JsonObject.addIfNotZero(key: String, value: Int) {
    if (value != 0) addProperty(key, value)
}
