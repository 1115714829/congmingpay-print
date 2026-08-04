package com.congmingpay.android.printer

import com.congmingpay.android.errcode.ErrCode
import com.congmingpay.android.escpos.Buzzer
import com.congmingpay.android.escpos.Finish
import com.congmingpay.android.escpos.Layout
import com.congmingpay.android.escpos.Reprint
import com.congmingpay.android.logger.Logger
import com.congmingpay.android.model.Conn
import com.congmingpay.android.model.JobStatus
import com.congmingpay.android.model.Printer
import com.congmingpay.android.store.JobStore
import com.congmingpay.android.store.PersistedJob
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.selects.onTimeout
import kotlinx.coroutines.selects.select
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger

/** 打印任务选项（按需覆盖每台默认） */
data class PrintOptions(
    val cut: Boolean? = null,
    val buzzer: Boolean? = null,
    val headLines: Int? = null,
    val tailLines: Int? = null,
    val reprint: Boolean? = null,
    val cloudId: Long? = null,
    val contentType: Int = -1,
    val sourceJson: ByteArray? = null
)

/** 任务事件 */
data class JobEvent(
    val event: String,         // done/failed/waiting
    val code: Int,             // errcode
    val jobNo: Int,
    val cloudId: Long?,
    val printer: Printer,
    val err: String = ""
)

/** 在线信息 */
data class OnlineInfo(val online: Boolean, val detail: String)

/**
 * 打印调度器。每任务开协程 → 并发打印。
 * dispatch 状态机：判在线 → 真打（串行锁内）→ 错误分类 → 退避/终态。
 */
class PrintDispatcher(
    private val store: JobStore,
    historyDays: Int,
    private val onJobEvent: (JobEvent) -> Unit
) {
    companion object {
        /** 终态条目内存上限（防常驻服务无界增长） */
        private const val MAX_TERMINAL_ENTRIES = 200
        /** 长期等待告警阈值：持续被拒超此时长恰一次发 waiting 事件（code=5101，对照 Windows warnNow） */
        private const val LONG_WAIT_WARN_MS = 20_000L
    }

    /** 历史保留天数（setHistoryDays 可运行时更新） */
    @Volatile private var historyDays: Int = historyDays

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val connectionManager = ConnectionManager()

    // 任务号计数器
    private var nextNo: AtomicInteger = AtomicInteger(1000)

    // 活跃任务列表
    private val entries = ConcurrentHashMap<Int, Entry>()
    // 串行锁（per-printer）
    private val printLocks = ConcurrentHashMap<String, Mutex>()
    // 在线注册表
    private val onlineMap = ConcurrentHashMap<String, OnlineInfo>()

    // SimpleDateFormat 非线程安全：多任务并发 dispatch 各自线程取用
    private val timeFmt = ThreadLocal.withInitial {
        SimpleDateFormat("yyyy-MM-dd HH:mm:ss", Locale.getDefault())
    }

    fun init() {
        nextNo.set(store.getNextNo())
        connectionManager.ensureListener()
    }

    fun close() {
        connectionManager.removeListener()
        scope.cancel()
    }

    /**
     * 提交打印任务。
     * @return 任务号
     */
    fun submit(p: Printer, doc: String, data: ByteArray, options: PrintOptions): Int {
        val no = nextNo.getAndIncrement()
        store.setNextNo(nextNo.get())

        // 解析生效参数
        val cut = options.cut ?: p.cuts()
        val buzzer = options.buzzer ?: p.buzzer
        val headLines = options.headLines ?: p.headLines
        val tailLines = options.tailLines ?: p.tailLines
        val reprint = options.reprint ?: false
        val cloudId = options.cloudId

        // 组装 payload
        val payload = assemblePayload(p, data, buzzer, reprint, headLines, tailLines, cut)

        val entry = Entry(
            no = no,
            doc = doc,
            printer = p.copy(),
            data = payload,
            cut = cut,
            buzzer = buzzer,
            headLines = headLines,
            tailLines = tailLines,
            reprintNext = reprint,
            cloudId = cloudId,
            contentType = options.contentType,
            sourceJson = options.sourceJson
        )
        entries[no] = entry
        persist(entry, JobStatus.QUEUED)

        Logger.info("提交任务 #$no 文档『$doc』打印机『${p.name}』目标 ${p.target()}")

        scope.launch { dispatch(entry) }
        return no
    }

    /** 组装 payload：蜂鸣 → 重印抬头 → 首部空行 → 内容 → 收尾 */
    private fun assemblePayload(
        p: Printer, data: ByteArray, buzzer: Boolean, reprint: Boolean,
        headLines: Int, tailLines: Int, cut: Boolean
    ): ByteArray {
        val out = java.io.ByteArrayOutputStream()
        if (buzzer) out.write(Buzzer.buildBuzzer(3, 3))
        if (reprint) out.write(Reprint.reprintBanner(p.width))
        out.write(Finish.feedBytes(headLines))
        out.write(data)
        out.write(Finish.finish(p.width, tailLines, cut))
        return out.toByteArray()
    }

    /** dispatch 状态机 */
    private suspend fun dispatch(entry: Entry) {
        val myGen = entry.gen.get()
        var firstGate = !entry.gated
        entry.gated = true

        while (true) {
            // 检查取消/代次（不 remove：gen 不匹配说明新代协程拥有该条目，删除会让其失控）
            if (entry.cancelled.get() || entry.gen.get() != myGen) {
                return
            }

            val reprint = entry.reprintNext

            // 首次门控（仅网口）
            if (firstGate && entry.printer.conn == Conn.NETWORK) {
                if (!StatusCheck.isOnline(entry.printer)) {
                    Logger.info("任务 #${entry.no} 判在线: 离线 → 进队等待联通")
                    if (!waitAndContinue(entry, myGen, occupancy = false, "打印机离线")) return
                    firstGate = false
                    continue
                }
            }
            firstGate = false

            // 取串行锁
            val lock = printLocks.computeIfAbsent(entry.printer.id) { Mutex() }

            lock.withLock {
                // 复查取消/代次
                if (entry.cancelled.get() || entry.gen.get() != myGen) {
                    return
                }

                setStatus(entry, JobStatus.PRINTING)

                // 真打
                val tag = if (reprint) "(重印)" else ""
                try {
                    connectionManager.connectAndPrint(entry.printer, entry.data)
                    Logger.info("任务 #${entry.no}$tag 打印成功")
                    setStatus(entry, JobStatus.DONE)
                    fireEvent(entry, "done", ErrCode.OK, "打印成功")
                    return
                } catch (e: ConnectException) {
                    Logger.warn("任务 #${entry.no} 连接失败: ${e.message}")
                    // 连接类错误 → 退避重试（withLock 退出时释放锁）
                } catch (e: PrintException) {
                    Logger.error("任务 #${entry.no} 打印失败: ${e.message}")
                    setStatus(entry, JobStatus.FAILED, e.message ?: "")
                    fireEvent(entry, "failed", ErrCode.NET_WRITE_FAILED, e.message ?: "打印失败")
                    return
                } catch (e: kotlinx.coroutines.CancellationException) {
                    // 作用域取消(服务关闭)：不标终态，留 PRINTING 供重启恢复
                    throw e
                } catch (e: Exception) {
                    Logger.error("任务 #${entry.no} 异常: ${e.message}")
                    setStatus(entry, JobStatus.FAILED, e.message ?: "")
                    fireEvent(entry, "failed", ErrCode.UNKNOWN, e.message ?: "未知错误")
                    return
                }
            }

            // 连接失败 → 退避重试
            if (!waitAndContinue(entry, myGen, occupancy = false, "连接失败")) return
        }
    }

    /**
     * 退避等待 + 唤醒。
     * 持续被拒超 [LONG_WAIT_WARN_MS] 恰一次发 waiting 事件（code=5101，非终态）。
     * @return true=继续重试，false=停止退出
     */
    private suspend fun waitAndContinue(entry: Entry, myGen: Int, occupancy: Boolean, reason: String): Boolean {
        // 退避计算
        val start = if (occupancy) 1000L else 5000L
        val cap = if (occupancy) 3000L else 30_000L
        if (entry.backoff < start) entry.backoff = start
        else {
            entry.backoff = (entry.backoff * 2).coerceAtMost(cap)
        }
        val wait = entry.backoff

        setStatus(entry, JobStatus.WAITING, reason)
        if (!entry.warnedWaiting && entry.waitSince == 0L) {
            entry.waitSince = System.currentTimeMillis()
        }
        val warnRemaining = if (entry.warnedWaiting) -1L
        else (entry.waitSince + LONG_WAIT_WARN_MS - System.currentTimeMillis()).coerceAtLeast(0)

        // select：退避定时器 / 唤醒通道 / 长期等待告警（恰一次）
        val stopped = select {
            entry.wake.onReceive {
                entry.backoff = 0
                Logger.info("任务 #${entry.no} 被唤醒(上线/释放),立即重打")
                false
            }
            onTimeout(wait) {
                true  // 退避到期，自然醒来重试
            }
            if (warnRemaining >= 0) {
                onTimeout(warnRemaining) {
                    entry.warnedWaiting = true
                    fireEvent(entry, "waiting", ErrCode.LONG_WAITING, "长期等待: $reason")
                    false  // 继续等待
                }
            }
        }

        if (stopped) return true

        // 复查取消/代次（不 remove：新代协程拥有该条目）
        if (entry.cancelled.get() || entry.gen.get() != myGen) {
            return false
        }
        return true
    }

    /** 唤醒某台打印机的所有等待任务 */
    fun nudgeOnline(printerId: String) {
        for ((_, entry) in entries) {
            if (entry.printer.id == printerId && entry.jobStatus == JobStatus.WAITING) {
                entry.wake.trySend(Unit)
            }
        }
    }

    /** 取消任务 */
    fun cancel(no: Int) {
        entries[no]?.let { e ->
            e.cancelled.set(true)
            e.wake.trySend(Unit)
        }
        store.deleteJob(no)
        entries.remove(no)
    }

    /** 重新打印（带重印抬头） */
    fun retry(no: Int) {
        val e = entries[no]
        if (e == null) {
            // 条目已被淘汰(终态上限)：从 DB 重载重打
            reloadFromDbAndRetry(no)
            return
        }
        // tryLock 消除 check-then-act 窗口：打印中(锁被持)直接拒绝重打
        val lock = printLocks.computeIfAbsent(e.printer.id) { Mutex() }
        if (!lock.tryLock()) {
            Logger.info("任务 #$no 打印中,拒绝重打")
            return
        }
        try {
            // 正在打印中的任务拒绝重打(避免与在途真打叠出一份重复,对照 Windows Service.go)
            if (e.jobStatus == JobStatus.PRINTING) return
            e.gen.incrementAndGet()
            // 立即离开终态：终态淘汰过滤与 clearDone 对该条目不再可见
            e.jobStatus = JobStatus.QUEUED
            e.finishTime = 0
            e.gated = false
            e.backoff = 0
            e.reprintNext = true
            e.cancelled.set(false)
            persist(e, JobStatus.QUEUED)
            e.wake.trySend(Unit) // 唤醒旧代协程退出 select，避免吞掉后续 nudge
            scope.launch { dispatch(e) }
        } finally {
            lock.unlock()
        }
    }

    /** 终态条目被淘汰后的重打：从 DB 按号重载重建 Entry */
    private fun reloadFromDbAndRetry(no: Int) {
        val pj = store.loadByNo(no) ?: return
        if (pj.status != JobStatus.DONE && pj.status != JobStatus.FAILED) return
        val entry = Entry(
            no = pj.no,
            doc = pj.doc,
            printer = pj.printer,
            data = pj.data,
            cut = pj.cut,
            buzzer = pj.buzzer,
            headLines = pj.headLines,
            tailLines = pj.tailLines,
            reprintNext = true,
            cloudId = pj.cloudId,
            contentType = pj.contentType,
            sourceJson = pj.sourceJson
        )
        entries[entry.no] = entry
        persist(entry, JobStatus.QUEUED)
        Logger.info("重打任务 #$no 从 DB 重载(原状态 ${pj.status})")
        scope.launch { dispatch(entry) }
    }

    /** 清除已完成 */
    fun clearDone() {
        store.deleteDone()
        entries.entries.removeAll { it.value.jobStatus == JobStatus.DONE || it.value.jobStatus == JobStatus.FAILED }
    }

    /**
     * 启动恢复未完成任务（对照 Windows RestoreAndResume）。
     * JobPrinting → JobQueued 重建 Entry 再 dispatch。
     */
    fun restoreAndResume(active: List<PersistedJob>) {
        for (pj in active) {
            val entry = Entry(
                no = pj.no,
                doc = pj.doc,
                printer = pj.printer,
                data = pj.data,
                cut = pj.cut,
                buzzer = pj.buzzer,
                headLines = pj.headLines,
                tailLines = pj.tailLines,
                reprintNext = pj.reprintNext,
                cloudId = pj.cloudId,
                contentType = pj.contentType,
                sourceJson = pj.sourceJson
            )
            entries[entry.no] = entry
            setStatus(entry, JobStatus.QUEUED)
            Logger.info("恢复任务 #${entry.no} 文档『${entry.doc}』打印机『${entry.printer.name}』(原状态 ${pj.status}→queued)")
            scope.launch { dispatch(entry) }
        }
    }

    /** 清理超期 done/failed 历史 */
    fun pruneHistory() {
        try {
            val n = store.pruneTerminal(historyDays)
            if (n > 0) Logger.info("清理超期历史任务 $n 条")
        } catch (e: Exception) {
            Logger.error("清理历史任务失败: ${e.message}")
        }
    }

    /** 运行时更新历史保留天数并立即清理 */
    fun setHistoryDays(days: Int) {
        historyDays = days
        pruneHistory()
    }

    /** 进行中任务数（排队/打印中/等待重试；状态栏用） */
    fun activeJobCount(): Int =
        entries.values.count { JobStatus.active(it.jobStatus) }

    /** 设置在线状态 */
    fun setPrinterOnline(id: String, online: Boolean, detail: String) {
        onlineMap[id] = OnlineInfo(online, detail)
    }

    /** 在线快照 */
    fun onlineSnapshot(): Map<String, OnlineInfo> = onlineMap.toMap()

    private fun setStatus(entry: Entry, status: String, err: String = "") {
        entry.jobStatus = status
        entry.errMsg = err
        if (status == JobStatus.DONE || status == JobStatus.FAILED) {
            entry.finishTime = System.currentTimeMillis()
            evictOldTerminal()
        }
        val timeLabel = timeFmt.get().format(Date())
        persist(entry, status)
    }

    /** 终态条目内存上限：超过则淘汰最旧（重打已淘汰任务从 DB 重载） */
    private fun evictOldTerminal() {
        val terminal = entries.entries
            .filter { it.value.jobStatus == JobStatus.DONE || it.value.jobStatus == JobStatus.FAILED }
            .sortedBy { it.value.finishTime }
        if (terminal.size > MAX_TERMINAL_ENTRIES) {
            terminal.take(terminal.size - MAX_TERMINAL_ENTRIES)
                .forEach { (no, _) -> entries.remove(no) }
        }
    }

    private fun persist(entry: Entry, status: String) {
        val pj = PersistedJob(
            no = entry.no,
            doc = entry.doc,
            status = status,
            timeLabel = timeFmt.get().format(Date()),
            err = entry.errMsg,
            printer = entry.printer,
            data = entry.data,
            cut = entry.cut,
            buzzer = entry.buzzer,
            headLines = entry.headLines,
            tailLines = entry.tailLines,
            reprintNext = entry.reprintNext,
            cloudId = entry.cloudId,
            contentType = entry.contentType,
            sourceJson = entry.sourceJson
        )
        try {
            store.upsertJob(pj)
        } catch (e: Exception) {
            Logger.error("持久化任务 #${entry.no} 失败: ${e.message}")
        }
    }

    private fun fireEvent(entry: Entry, event: String, code: Int, err: String) {
        onJobEvent(JobEvent(event, code, entry.no, entry.cloudId, entry.printer.copy(), err))
    }
}

/** 打印任务内部状态 */
class Entry(
    val no: Int,
    val doc: String,
    val printer: Printer,
    val data: ByteArray,
    val cut: Boolean,
    val buzzer: Boolean,
    val headLines: Int,
    val tailLines: Int,
    var reprintNext: Boolean,
    val cloudId: Long?,
    val contentType: Int,
    val sourceJson: ByteArray?
) {
    @Volatile var jobStatus: String = JobStatus.QUEUED
    var errMsg: String = ""
    var gated: Boolean = false
    var backoff: Long = 0
    var finishTime: Long = 0
    var waitSince: Long = 0
    var warnedWaiting: Boolean = false
    val wake: kotlinx.coroutines.channels.Channel<Unit> = kotlinx.coroutines.channels.Channel(1)
    val gen: AtomicInteger = AtomicInteger(0)
    val cancelled: AtomicBoolean = AtomicBoolean(false)
}
