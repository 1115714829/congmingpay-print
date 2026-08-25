package com.congmingpay.android.ui

/**
 * 悬浮告警列表模型（对齐 Windows `internal/ui/AlertWindow.go`）。
 *
 * kind+key 幂等；新告警插最上；上限 [MAX_ITEMS]；不受系统通知开关控制。
 */
class AlertModel {

    companion object {
        const val MAX_ITEMS = 200

        const val KIND_JOB_FAILED = "job-failed"
        const val KIND_JOB_WAITING = "job-waiting"
        const val KIND_PRINTER_OFFLINE = "printer-offline"
        const val KIND_MQTT_DOWN = "mqtt-down"
        const val KIND_ACCEPT_FAILED = "accept-failed"

        const val SEV_WARN = 0
        const val SEV_ERROR = 1
    }

    data class Item(
        val kind: String,
        val key: String,
        var sev: Int,
        var text: String,
        /** 事件起始时刻（毫秒）；「已持续」据此动态计时 */
        var whenMs: Long
    )

    private val items = ArrayList<Item>()
    private var listener: (() -> Unit)? = null

    fun setOnChanged(l: (() -> Unit)?) {
        listener = l
    }

    fun snapshot(): List<Item> = ArrayList(items)

    fun size(): Int = items.size

    fun isEmpty(): Boolean = items.isEmpty()

    /**
     * 挂起/更新一条告警。同 kind+key 原位更新；新告警插最上。
     * @return true=新增或更新成功
     */
    @Synchronized
    fun raise(kind: String, key: String, sev: Int, text: String, whenMs: Long): Boolean {
        for (it in items) {
            if (it.kind == kind && it.key == key) {
                it.sev = sev
                it.text = text
                it.whenMs = whenMs
                notifyChanged()
                return true
            }
        }
        items.add(0, Item(kind, key, sev, text, whenMs))
        if (items.size > MAX_ITEMS) {
            items.subList(MAX_ITEMS, items.size).clear()
        }
        notifyChanged()
        return true
    }

    /** 按 kind+key 移除；不存在则忽略 */
    @Synchronized
    fun resolve(kind: String, key: String): Boolean {
        val removed = items.removeAll { it.kind == kind && it.key == key }
        if (removed) notifyChanged()
        return removed
    }

    /** 清空全部 */
    @Synchronized
    fun dismissAll() {
        if (items.isEmpty()) return
        items.clear()
        notifyChanged()
    }

    private fun notifyChanged() {
        listener?.invoke()
    }
}

/**
 * 「已持续」列中文短文本（对齐 Windows `durText`）。
 * `N秒` / `N分SS秒` / `N时M分`。
 */
fun alertDurText(elapsedMs: Long): String {
    var s = (elapsedMs / 1000).toInt()
    if (s < 0) s = 0
    return when {
        s < 60 -> "${s}秒"
        s < 3600 -> {
            val m = s / 60
            val sec = s % 60
            "${m}分${sec.toString().padStart(2, '0')}秒"
        }
        else -> {
            val h = s / 3600
            val m = (s % 3600) / 60
            "${h}时${m}分"
        }
    }
}
