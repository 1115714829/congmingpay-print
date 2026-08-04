package com.congmingpay.android.logger

import android.os.Handler
import android.os.Looper
import java.io.BufferedWriter
import java.io.File
import java.io.FileWriter
import java.text.SimpleDateFormat
import java.util.ArrayDeque
import java.util.Date
import java.util.Locale
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/** 日志等级（与文件标记 INFO/WARN/ERROR 对应） */
enum class LogLevel {
    INFO, WARN, ERROR;

    /** 过滤下限：本级及以上保留（INFO=全部，WARN=WARN+ERROR，ERROR=仅 ERROR） */
    fun passes(entry: LogLevel): Boolean = entry.ordinal >= this.ordinal
}

/** 内存环中的一条日志（供 UI） */
data class LogEntry(
    val ts: String,
    val level: LogLevel,
    val msg: String
)

fun interface LogListener {
    fun onLog(entry: LogEntry)
}

/**
 * 全局文件日志 + 内存环旁路（供运行日志页）。
 *
 * 日志按天分文件：log/<yyyy-MM-dd>.log；仅保留最近 7 天（含当天）。
 * 内存环容量 [RING_CAPACITY]，超出丢最旧；监听器在主线程回调。
 * 使用方式：App 启动时 [init]，其余代码统一调用 [info]/[warn]/[error]。
 */
object Logger {

    private const val DAY_LAYOUT = "yyyy-MM-dd"
    private const val TS_LAYOUT = "yyyy-MM-dd HH:mm:ss"
    private const val RETAIN_DAYS = 7
    private const val FLUSH_INTERVAL_MS = 2000L
    private const val RING_CAPACITY = 1000

    private val executor = Executors.newSingleThreadExecutor { r ->
        Thread(r, "logger").apply { isDaemon = true }
    }
    private val flusher = Executors.newSingleThreadScheduledExecutor { r ->
        Thread(r, "logger-flush").apply { isDaemon = true }
    }
    private val mainHandler = Handler(Looper.getMainLooper())

    private val lock = Any()
    private var logDir: File? = null
    private var writer: BufferedWriter? = null
    private var currentDate: String = ""
    private val tsFormat = SimpleDateFormat(TS_LAYOUT, Locale.getDefault())
    private val dayFormat = SimpleDateFormat(DAY_LAYOUT, Locale.getDefault())

    private val ring = ArrayDeque<LogEntry>(RING_CAPACITY)
    private val listeners = CopyOnWriteArrayList<LogListener>()

    fun init(dir: File) {
        synchronized(lock) {
            if (!dir.exists()) dir.mkdirs()
            logDir = dir
            currentDate = dayFormat.format(Date())
            openWriterLocked()
            pruneOldLogs()
        }
        flusher.scheduleAtFixedRate({ flush() }, FLUSH_INTERVAL_MS, FLUSH_INTERVAL_MS, TimeUnit.MILLISECONDS)
    }

    /** 立即将缓冲写入磁盘（退出时调用） */
    fun flush() {
        synchronized(lock) {
            try {
                writer?.flush()
            } catch (_: Exception) {}
        }
    }

    fun currentPath(): String? {
        synchronized(lock) {
            val d = logDir ?: return null
            return File(d, "$currentDate.log").absolutePath
        }
    }

    fun addListener(listener: LogListener) {
        listeners.addIfAbsent(listener)
    }

    fun removeListener(listener: LogListener) {
        listeners.remove(listener)
    }

    /**
     * 内存环快照（旧→新）。
     * @param minLevel INFO=全部；WARN=WARN+ERROR；ERROR=仅 ERROR
     */
    fun snapshot(minLevel: LogLevel = LogLevel.INFO): List<LogEntry> {
        synchronized(lock) {
            return ring.filter { minLevel.passes(it.level) }
        }
    }

    /** 清空内存环（不影响已写文件） */
    fun clearRing() {
        synchronized(lock) {
            ring.clear()
        }
    }

    fun info(msg: String) = write(LogLevel.INFO, msg)
    fun warn(msg: String) = write(LogLevel.WARN, msg)
    fun error(msg: String) = write(LogLevel.ERROR, msg)

    fun info(fmt: String, vararg args: Any?) = write(LogLevel.INFO, fmt.format(*args))
    fun warn(fmt: String, vararg args: Any?) = write(LogLevel.WARN, fmt.format(*args))
    fun error(fmt: String, vararg args: Any?) = write(LogLevel.ERROR, fmt.format(*args))

    private fun write(level: LogLevel, msg: String) {
        executor.execute {
            val entry: LogEntry
            synchronized(lock) {
                val ts = tsFormat.format(Date())
                entry = LogEntry(ts, level, msg)
                val line = "$ts [${level.name}] $msg"
                val date = dayFormat.format(Date())
                if (date != currentDate) {
                    currentDate = date
                    openWriterLocked()
                    pruneOldLogs()
                }
                try {
                    writer?.write(line)
                    writer?.newLine()
                } catch (_: Exception) {
                    // 日志失败不影响主流程
                }
                while (ring.size >= RING_CAPACITY) {
                    ring.removeFirst()
                }
                ring.addLast(entry)
            }
            if (listeners.isNotEmpty()) {
                mainHandler.post {
                    for (l in listeners) {
                        try {
                            l.onLog(entry)
                        } catch (_: Exception) {}
                    }
                }
            }
        }
    }

    private fun openWriterLocked() {
        val dir = logDir ?: return
        try {
            writer?.close()
        } catch (_: Exception) {}
        writer = try {
            BufferedWriter(FileWriter(File(dir, "$currentDate.log"), true))
        } catch (_: Exception) {
            null
        }
    }

    private fun pruneOldLogs() {
        val dir = logDir ?: return
        val cutoff = System.currentTimeMillis() - (RETAIN_DAYS - 1) * 24L * 60 * 60 * 1000
        dir.listFiles()?.forEach { f ->
            if (f.isFile && f.name.endsWith(".log")) {
                if (f.lastModified() < cutoff) f.delete()
            }
        }
    }
}
