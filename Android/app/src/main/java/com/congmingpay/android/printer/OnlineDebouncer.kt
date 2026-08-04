package com.congmingpay.android.printer

/**
 * 网口离线防抖（对齐 Windows `internal/ui/OnlineDebounce.go`）。
 *
 * 任一次成功立即在线；连续失败持续满 window 才离线；window=0 时失败即时生效（USB/蓝牙）。
 */
class OnlineDebouncer(
    private val windowMs: Long = OFFLINE_AFTER_MS
) {
    companion object {
        /** 与 Windows `offlineAfter = 30 * time.Second` 一致 */
        var OFFLINE_AFTER_MS: Long = 30_000
    }

    /** -1=未定 / 0=离线 / 1=在线 */
    var effective: Int = -1
        private set

    /** 连败起点（0=未在失败中） */
    private var failSince: Long = 0

    /** 连败首次失败原因（离线 detail / 通知用） */
    var failFirst: String = ""
        private set

    /**
     * 录入一次原始探测，返回**当前生效态**（对齐 Windows `observe`，非边沿）。
     */
    fun observe(ok: Boolean, detail: String, now: Long = System.currentTimeMillis()): Int {
        if (ok) {
            failSince = 0L
            failFirst = ""
            effective = 1
            return effective
        }
        if (failSince == 0L) {
            failSince = now
            failFirst = detail
        }
        if (now - failSince >= windowMs) {
            effective = 0
        }
        return effective
    }

    /** 当前连败已持续（毫秒）；无连败=0。对齐 Windows `failFor`。 */
    fun failForMs(now: Long = System.currentTimeMillis()): Long {
        if (failSince == 0L) return 0L
        return now - failSince
    }

    fun reset() {
        effective = -1
        failSince = 0L
        failFirst = ""
    }
}

/**
 * 整秒时长格式化为 Go `time.Duration.String()` 在 Truncate(Second) 后的形态：
 * `5s` / `30s` / `1m0s` / `1m30s` / `1h0m0s` / `10h1m1s`。
 */
fun formatGoDurationSeconds(ms: Long): String {
    val totalSec = (ms / 1000).coerceAtLeast(0)
    if (totalSec < 60) return "${totalSec}s"
    val s = totalSec % 60
    val totalMin = totalSec / 60
    if (totalMin < 60) return "${totalMin}m${s}s"
    return "${totalMin / 60}h${totalMin % 60}m${s}s"
}
