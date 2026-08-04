package com.congmingpay.android.printer

import com.congmingpay.android.config.ConfigManager
import com.congmingpay.android.logger.Logger
import com.congmingpay.android.model.Conn
import com.congmingpay.android.model.Printer
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * 状态监测：每台打印机一条独立协程 1s 巡检。
 * 对齐 Windows `internal/ui/StatusMonitor.go`：防抖、detail 公式、生效态每轮写注册表。
 */
class StatusMonitor(
    private val cfg: ConfigManager,
    private val dispatcher: PrintDispatcher,
    private val onLabelChange: (printerId: String, online: Boolean, detail: String) -> Unit,
    private val onBoolEdge: (printerId: String, online: Boolean, detail: String) -> Unit
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val monitors = mutableMapOf<String, Job>()
    private val debouncers = mutableMapOf<String, OnlineDebouncer>()
    private val sigs = mutableMapOf<String, String>()

    @Synchronized
    fun sync() {
        val printers = cfg.printers()
        val activeIds = printers.map { it.id }.toSet()

        monitors.keys.toList().filter { it !in activeIds }.forEach { id ->
            monitors.remove(id)?.cancel()
            debouncers.remove(id)
            sigs.remove(id)
        }

        for (p in printers) {
            val sig = monitorSig(p)
            val existing = monitors[p.id]
            if (existing == null || debouncers[p.id] == null || sigs[p.id] != sig) {
                existing?.cancel()
                sigs[p.id] = sig
                startMonitor(p)
            }
        }
    }

    @Synchronized
    fun stopAll() {
        scope.cancel()
        monitors.clear()
        debouncers.clear()
        sigs.clear()
    }

    private fun monitorSig(p: Printer): String =
        "${p.conn}|${p.ip}|${p.port}|${p.usbName}|${p.bluetoothMac}|${p.name}"

    private data class StatusInfo(val label: String, val detail: String)

    private fun startMonitor(p: Printer) {
        val snapshot = p.copy()
        // Windows: USB window=0；其余（网口）30s。安卓蓝牙同 USB 即时。
        val windowMs = if (p.conn == Conn.NETWORK) OnlineDebouncer.OFFLINE_AFTER_MS else 0L
        val debouncer = OnlineDebouncer(windowMs)
        debouncers[p.id] = debouncer

        var lastLabel = ""
        var lastEff = -1
        var lastRaw = -1
        var lastGood = StatusInfo("就绪", "")

        val job = scope.launch {
            while (true) {
                val t0 = System.currentTimeMillis()
                val st = StatusCheck.probe(snapshot)
                val raw = st.online
                val eff = debouncer.observe(raw, st.detail, t0)

                // ① 原始成功边沿 → NudgeOnline
                if (raw && lastRaw != 1) {
                    dispatcher.nudgeOnline(snapshot.id)
                }
                lastRaw = if (raw) 1 else 0

                // ② 折算生效显示信息（对齐 StatusMonitor.go switch）
                val failDur = formatGoDurationSeconds(debouncer.failForMs(t0))
                val info: StatusInfo = when {
                    eff == 1 && raw -> {
                        // statusInfoFor: 在线 → 就绪 + st.Detail
                        StatusInfo("就绪", st.detail).also { lastGood = it }
                    }
                    eff == 1 -> {
                        // 容错窗口
                        StatusInfo(
                            lastGood.label,
                            "网络抖动: 连续失败 $failDur(${st.detail})"
                        )
                    }
                    eff == 0 -> {
                        StatusInfo(
                            "离线",
                            "${debouncer.failFirst};已持续 $failDur"
                        )
                    }
                    else -> {
                        // eff == -1
                        StatusInfo("—", "检测中(${st.detail})")
                    }
                }

                // ③ 生效标签边沿 → 日志 + UI
                if (info.label != lastLabel) {
                    lastLabel = info.label
                    Logger.info(
                        "打印机状态: [id=${snapshot.id}] 名称『${snapshot.name}』" +
                            "${snapshot.address()} → ${info.label}(${info.detail})"
                    )
                    val onlineForUi = when (info.label) {
                        "就绪" -> true
                        "离线" -> false
                        else -> false
                    }
                    onLabelChange(
                        snapshot.id,
                        onlineForUi,
                        "${info.label}(${info.detail})"
                    )
                }

                // ④ 生效布尔边沿 → 通知/告警（基线静默由回调侧处理也可；此处仍回调）
                when {
                    eff < 0 -> { /* 未定 */ }
                    lastEff < 0 -> {
                        lastEff = eff
                        onBoolEdge(
                            snapshot.id,
                            eff == 1,
                            "${info.label}(${info.detail})"
                        )
                    }
                    eff != lastEff -> {
                        lastEff = eff
                        onBoolEdge(
                            snapshot.id,
                            eff == 1,
                            "${info.label}(${info.detail})"
                        )
                    }
                }

                // ⑤ 生效态每轮写注册表；未定不写
                if (eff >= 0) {
                    val regDetail = "${info.label}(${info.detail})"
                    dispatcher.setPrinterOnline(snapshot.id, eff == 1, regDetail)
                }

                val elapsed = System.currentTimeMillis() - t0
                delay((1000L - elapsed).coerceAtLeast(1L))
            }
        }
        monitors[p.id] = job
    }
}
