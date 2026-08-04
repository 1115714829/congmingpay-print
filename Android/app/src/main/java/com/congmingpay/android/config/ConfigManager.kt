package com.congmingpay.android.config

import com.congmingpay.android.model.Brand
import com.congmingpay.android.model.Conn
import com.congmingpay.android.model.Printer
import com.congmingpay.android.model.Settings
import com.congmingpay.android.model.Source
import com.congmingpay.android.model.clampJobHistoryDays
import com.congmingpay.android.logger.Logger
import com.google.gson.Gson
import com.google.gson.GsonBuilder
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.atomic.AtomicLong

/**
 * 应用配置（config.json）。持久化 Settings + Printers 列表。
 */
data class AppConfig(
    var settings: Settings = Settings(),
    var printers: MutableList<Printer> = mutableListOf()
)

/**
 * 配置管理器。线程安全（synchronized）。Gson 序列化到 config.json。
 */
class ConfigManager(private val configPath: File) {

    private val gson: Gson = GsonBuilder().setPrettyPrinting().create()
    private val lock = Any()
    private var cfg: AppConfig = AppConfig()

    /** 打印机 ID 生成：高精度时间戳 + 原子递增序号 */
    private val idSeq = AtomicLong(0)

    fun newPrinterId(): String = "p${System.nanoTime()}-${idSeq.incrementAndGet()}"

    /** 加载配置；文件不存在用默认，损坏则备份再用默认 */
    fun load() {
        synchronized(lock) {
            if (!configPath.exists()) {
                cfg = AppConfig()
                Logger.info("config.json 不存在,使用默认配置")
                return
            }
            try {
                val text = configPath.readText()
                cfg = gson.fromJson(text, AppConfig::class.java) ?: AppConfig()
                normalize()
                Logger.info("config.json 加载成功: ${cfg.printers.size} 台打印机")
            } catch (e: Exception) {
                val badName = "config.json.bad-" + SimpleDateFormat("yyyyMMdd-HHmmss", Locale.getDefault()).format(Date())
                val badPath = File(configPath.parentFile, badName)
                try {
                    configPath.renameTo(badPath)
                    Logger.error("加载配置失败(${configPath}): ${e.message};损坏文件已备份到 $badName,改用默认配置")
                } catch (e2: Exception) {
                    Logger.error("加载配置失败(${configPath}): ${e.message}(备份失败: ${e2.message}),改用默认配置")
                }
                cfg = AppConfig()
            }
        }
    }

    /** 保存到磁盘 */
    fun save() {
        synchronized(lock) {
            try {
                configPath.parentFile?.mkdirs()
                configPath.writeText(gson.toJson(cfg))
            } catch (e: Exception) {
                Logger.error("保存配置失败: ${e.message}")
            }
        }
    }

    fun settings(): Settings = synchronized(lock) { cfg.settings }
    fun printers(): List<Printer> = synchronized(lock) { cfg.printers.toList() }

    /** 更新设置（不落盘，调用方负责 save） */
    fun updateSettings(apply: (Settings) -> Unit) {
        synchronized(lock) { apply(cfg.settings) }
    }

    fun printerSnapshots(): List<Printer> = synchronized(lock) {
        cfg.printers.map { it.copy() }
    }

    fun findPrinter(idOrName: String): Printer? = synchronized(lock) {
        cfg.printers.firstOrNull { it.id == idOrName || it.name == idOrName }
    }

    fun findPrinterByIp(ip: String): Printer? = synchronized(lock) {
        cfg.printers.firstOrNull { it.conn == Conn.NETWORK && it.ip == ip }
    }

    /** 本地添加打印机 */
    fun addPrinter(p: Printer) {
        synchronized(lock) {
            if (p.id.isEmpty()) p.id = newPrinterId()
            if (p.brand.isEmpty()) p.brand = Brand.OTHER
            if (p.source.isEmpty()) p.source = Source.LOCAL
            if (p.conn == Conn.NETWORK && p.port.isEmpty()) p.port = "9100"
            cfg.printers.add(p)
        }
        save()
    }

    /** 移除打印机 */
    fun removePrinter(id: String) {
        synchronized(lock) {
            cfg.printers.removeAll { it.id == id }
        }
        save()
    }

    /**
     * 云端目标自动登记/更新（仅网口机按 IP 匹配）。
     * @return (Printer, registered) registered=true 表示新增
     */
    fun upsertPrinter(input: Printer): Pair<Printer, Boolean> {
        synchronized(lock) {
            if (input.conn == Conn.NETWORK && input.ip.isNotEmpty()) {
                val existing = cfg.printers.firstOrNull { it.conn == Conn.NETWORK && it.ip == input.ip }
                if (existing != null) {
                    applyPrinterUpdate(existing, input)
                    return existing to false
                }
            }
            // 新增
            if (input.id.isEmpty()) input.id = newPrinterId()
            if (input.brand.isEmpty()) input.brand = Brand.OTHER
            if (input.conn == Conn.NETWORK && input.port.isEmpty()) input.port = "9100"
            input.source = Source.CLOUD
            input.lastPrint = ""
            cfg.printers.add(input)
            return input to true
        }
    }

    /** 更新打印机字段 */
    fun updatePrinterFields(id: String, apply: (Printer) -> Unit) {
        synchronized(lock) {
            cfg.printers.firstOrNull { it.id == id }?.let { apply(it) }
        }
        save()
    }

    /** 写入上次打印时间 */
    fun updateLastPrint(id: String, when_: String) {
        synchronized(lock) {
            cfg.printers.firstOrNull { it.id == id }?.lastPrint = when_
        }
        save()
    }

    private fun applyPrinterUpdate(existing: Printer, input: Printer) {
        if (input.name.isNotEmpty()) existing.name = input.name
        if (input.brand.isNotEmpty()) existing.brand = input.brand
        if (input.width == 58 || input.width == 80) existing.width = input.width
        if (input.port.isNotEmpty()) existing.port = input.port
    }

    private fun normalize() {
        cfg.settings.jobHistoryDays = clampJobHistoryDays(cfg.settings.jobHistoryDays)
        for (p in cfg.printers) {
            if (p.source != Source.CLOUD) p.source = Source.LOCAL
        }
    }
}
