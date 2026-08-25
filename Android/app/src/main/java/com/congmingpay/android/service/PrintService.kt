package com.congmingpay.android.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import androidx.core.app.NotificationCompat
import com.congmingpay.android.config.ConfigManager
import com.congmingpay.android.logger.Logger
import com.congmingpay.android.model.Printer
import com.congmingpay.android.model.Settings
import com.congmingpay.android.mqtt.MqttClient
import com.congmingpay.android.printer.PrintDispatcher
import com.congmingpay.android.printer.PrintOptions
import com.congmingpay.android.printer.StatusMonitor
import com.congmingpay.android.printer.injectDeviceResolvers
import com.congmingpay.android.store.JobStore
import com.congmingpay.android.ui.AlertModel
import com.congmingpay.android.ui.AlertOverlay
import com.congmingpay.android.ui.Notify
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import java.io.File
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CopyOnWriteArrayList

/**
 * UI 监听接口：服务事件统一经主线程转发给已注册监听者（Fragment/Activity）。
 * 空实现为默认，监听者按需覆写。
 */
interface UiListener {
    /** 打印机列表/状态变化（新增删除/属性保存/云端登记/在线标签） */
    fun onPrintersChanged() {}

    /** 任务列表变化（提交/状态变更/取消/清除） */
    fun onJobsChanged() {}

    /** MQTT 连接状态变化 */
    fun onMqttStatus(connected: Boolean, error: String?) {}
}

/**
 * 前台服务：常驻运行 MQTT + 打印队列 + 状态监测。
 *
 * 启动顺序：先 startForeground（5 秒限制），再后台协程初始化
 * 配置/任务恢复/历史清理/MQTT/监测，完成后刷新通知标题为服务名。
 *
 * UI 操作入口（全部走后台协程，避免 SQLite/文件 IO 上主线程）：
 * submitPrint / retryJob / cancelJob / clearDoneJobs / addPrinter / removePrinter /
 * updatePrinter / saveSettings / reloadSettings / notifyPrintersChanged。
 */
class PrintService : Service() {

    companion object {
        const val CHANNEL_ID = "print_service"
        const val NOTIFICATION_ID = 1

        @Volatile var instance: PrintService? = null
    }

    private val initScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val mainHandler = Handler(Looper.getMainLooper())

    /** 服务已销毁标志：initAll 各阶段检查，防止销毁后继续启动僵尸 MQTT/监测 */
    @Volatile private var destroyed = false

    /** initAll 是否已完成关键组件初始化（saveSettings 据此决定是否即时重载） */
    @Volatile private var initReady = false

    private lateinit var cfg: ConfigManager
    private lateinit var dispatcher: PrintDispatcher
    private lateinit var mqtt: MqttClient
    private lateinit var statusMonitor: StatusMonitor
    private var alertOverlay: AlertOverlay? = null

    // —— UI 监听与通知状态 ——
    private val uiListeners = CopyOnWriteArrayList<UiListener>()
    /** 每台打印机上次生效在线态（基线静默判定；删除时清除） */
    private val lastOnline = ConcurrentHashMap<String, Boolean>()
    /** MQTT 通知状态机：0=中性(未启用/停用) 1=连 2=断 */
    @Volatile private var mqttNotifyState = 0
    @Volatile private var mqttBaselineDone = false

    override fun onCreate() {
        super.onCreate()
        instance = this
        Logger.info("PrintService onCreate")
        Notify.ensureChannel(this)

        createNotificationChannel()
        // 先出前台通知（默认标题），初始化完成后再刷新为服务名
        startForeground(NOTIFICATION_ID, buildNotification("票据打印服务"))

        initScope.launch {
            try {
                initAll()
            } catch (e: Exception) {
                Logger.error("PrintService 初始化失败: ${e.message}")
            }
        }
    }

    private fun initAll() {
        // 初始化配置
        cfg = ConfigManager(File(filesDir, "config.json"))
        cfg.load()
        if (destroyed) return
        // 设备解析器注入（USB/蓝牙在线检测与连接解析需要 Context）
        injectDeviceResolvers(this)
        for (p in cfg.printers()) {
            Logger.info("已加载打印机: [id=${p.id}] 名称『${p.name}』品牌『${p.brandLabel()}』规格 ${p.widthLabel()} 目标 ${p.target()}")
        }

        // 初始化打印队列
        val store = JobStore.open(this)
        val historyDays = cfg.settings().jobHistoryDays
        dispatcher = PrintDispatcher(store, historyDays) { ev ->
            onJobEvent(ev)
        }
        dispatcher.init()
        if (destroyed) { dispatcher.close(); return }

        // 先建 MQTT（恢复任务完成时 onJobEvent 需上报）
        mqtt = MqttClient(
            cfg = cfg,
            svc = dispatcher,
            onChange = { notifyPrintersChanged() },
            onStatus = { connected, error -> onMqttStatusInternal(connected, error) },
            onAcceptFailed = { id, code, message -> onAcceptFailedInternal(id, code, message) }
        )

        // 重启恢复未完成任务 + 清理超期历史
        dispatcher.restoreAndResume(store.loadActive())
        dispatcher.pruneHistory()
        if (destroyed) { mqtt.close(); dispatcher.close(); return }

        // 初始化状态监测
        statusMonitor = StatusMonitor(
            cfg = cfg,
            dispatcher = dispatcher,
            onLabelChange = { _, _, _ -> notifyUi { it.onPrintersChanged() } },
            onBoolEdge = { id, online, detail, failFirst, sinceMs, baseline ->
                onPrinterEdgeInternal(id, online, detail, failFirst, sinceMs, baseline)
            }
        )

        // 启动（destroyed 复查先于 mqtt.start：start 入队的异步 connect 由 close 的 enabled 守卫拦截）
        statusMonitor.sync()
        if (destroyed) { statusMonitor.stopAll(); mqtt.close(); dispatcher.close(); return }
        mqtt.start()
        initReady = true

        // 悬浮告警球（需悬浮窗权限；无权限则静默）
        mainHandler.post {
            if (destroyed) return@post
            val overlay = AlertOverlay(applicationContext)
            alertOverlay = overlay
            overlay.attach()
        }

        // 刷新通知标题为服务名
        refreshNotificationTitle()
        Logger.info("PrintService 初始化完成")
        // 冷启动时 UI 可能已 refresh 过空列表；通知一次让列表/状态条立刻补齐
        notifyUi {
            it.onPrintersChanged()
            it.onJobsChanged()
        }
    }

    // —— 组件访问（UI 用；未初始化返回 null） ——

    fun configManager(): ConfigManager? = if (::cfg.isInitialized) cfg else null
    fun printDispatcher(): PrintDispatcher? = if (::dispatcher.isInitialized) dispatcher else null
    fun mqttClient(): MqttClient? = if (::mqtt.isInitialized) mqtt else null

    // —— UI 监听注册 ——

    fun addUiListener(l: UiListener) {
        if (!uiListeners.contains(l)) uiListeners.add(l)
        // 晚注册：init 已完成则回放，避免错过 init 末尾那次 notify
        if (initReady) {
            mainHandler.post {
                try {
                    l.onPrintersChanged()
                    l.onJobsChanged()
                } catch (e: Exception) {
                    Logger.error("UI 回调异常: ${e.message}")
                }
            }
        }
    }

    fun removeUiListener(l: UiListener) {
        uiListeners.remove(l)
    }

    /** 向所有 UI 监听者投递事件（主线程执行，快速返回不阻塞调用方） */
    private fun notifyUi(block: (UiListener) -> Unit) {
        if (destroyed) return
        mainHandler.post {
            for (l in uiListeners) {
                try {
                    block(l)
                } catch (e: Exception) {
                    Logger.error("UI 回调异常: ${e.message}")
                }
            }
        }
    }

    // —— UI 操作入口（后台协程，SQLite/文件 IO 不上主线程） ——

    /** 提交打印任务（本地任务 cloudId=null，不上报云端） */
    fun submitPrint(p: Printer, doc: String, data: ByteArray, options: PrintOptions, onSubmitted: (Int) -> Unit = {}) {
        if (destroyed) return
        initScope.launch {
            val d = printDispatcher() ?: return@launch
            try {
                val no = d.submit(p, doc, data, options)
                mainHandler.post { onSubmitted(no) }
            } catch (e: Exception) {
                Logger.error("提交打印失败: ${e.message}")
            }
        }
    }

    fun retryJob(no: Int) {
        initScope.launch { printDispatcher()?.retry(no) }
    }

    fun cancelJob(no: Int) {
        initScope.launch { printDispatcher()?.cancel(no) }
    }

    fun clearDoneJobs() {
        initScope.launch { printDispatcher()?.clearDone() }
    }

    fun addPrinter(p: Printer) {
        if (destroyed) return
        initScope.launch {
            if (::cfg.isInitialized) cfg.addPrinter(p)
            notifyPrintersChanged()
        }
    }

    fun removePrinter(id: String) {
        initScope.launch {
            if (::cfg.isInitialized) cfg.removePrinter(id)
            lastOnline.remove(id)
            alertOverlay?.resolve(AlertModel.KIND_PRINTER_OFFLINE, id)
            notifyPrintersChanged()
        }
    }

    /** 悬浮窗权限刚授予后由 UI 调用，补挂告警球 */
    fun refreshAlertOverlay() {
        mainHandler.post {
            val overlay = alertOverlay ?: AlertOverlay(applicationContext).also { alertOverlay = it }
            overlay.refreshPermission()
        }
    }

    fun updatePrinter(id: String, apply: (Printer) -> Unit) {
        initScope.launch {
            if (::cfg.isInitialized) cfg.updatePrinterFields(id, apply)
            notifyPrintersChanged()
        }
    }

    /** 保存设置并即时生效（通知标题/历史天数/MQTT 重载/监测同步） */
    fun saveSettings(apply: (Settings) -> Unit) {
        if (destroyed) return
        initScope.launch {
            if (!::cfg.isInitialized) return@launch
            cfg.updateSettings(apply)
            cfg.save()
            if (initReady) {
                reloadSettingsInternal()
            } else {
                // 初始化尚未完成：仅落盘，initAll 自身的后续读取会带上新值
                Logger.info("设置已保存(初始化未完成,待初始化时生效)")
            }
        }
    }

    private fun reloadSettingsInternal() {
        if (!::statusMonitor.isInitialized) return
        val s = cfg.settings()
        refreshNotificationTitle()
        dispatcher.setHistoryDays(s.jobHistoryDays)
        mqtt.reload(s.mqtt)
        statusMonitor.sync()
        notifyUi { it.onPrintersChanged() }
        Logger.info("设置已保存并即时生效")
    }

    /** 打印机列表变更（增删/属性保存/云端登记）后调用：监测重建 + UI 刷新 */
    fun notifyPrintersChanged() {
        if (::statusMonitor.isInitialized) statusMonitor.sync()
        notifyUi { it.onPrintersChanged() }
    }

    // —— 事件处理 ——

    private fun onJobEvent(ev: com.congmingpay.android.printer.JobEvent) {
        val key = ev.jobNo.toString()
        when (ev.event) {
            "done" -> {
                // 写入上次打印时间
                val time = java.text.SimpleDateFormat("yyyy-MM-dd HH:mm:ss", java.util.Locale.getDefault())
                    .format(java.util.Date())
                cfg.updateLastPrint(ev.printer.id, time)
                alertOverlay?.resolve(AlertModel.KIND_JOB_WAITING, key)
                notifyUi { it.onJobsChanged() }
            }
            "failed" -> {
                Logger.error("任务 #${ev.jobNo} 失败: ${ev.err}")
                val txt = "任务#${ev.jobNo} 打印机『${ev.printer.name}』:${ev.err}"
                Notify.error(this, "打印失败", txt)
                alertOverlay?.resolve(AlertModel.KIND_JOB_WAITING, key)
                alertOverlay?.raise(
                    AlertModel.KIND_JOB_FAILED, key, AlertModel.SEV_ERROR,
                    "打印失败 $txt"
                )
                notifyUi { it.onJobsChanged() }
            }
            "waiting" -> {
                Logger.warn("任务 #${ev.jobNo} 长期等待: ${ev.err}")
                val txt = "任务#${ev.jobNo} 打印机『${ev.printer.name}』:${ev.err}"
                Notify.warn(this, "打印卡单", "任务 #${ev.jobNo} 等待重试: ${ev.err.ifBlank { "持续连接失败" }}")
                alertOverlay?.raise(
                    AlertModel.KIND_JOB_WAITING, key, AlertModel.SEV_WARN,
                    "长时间等待 $txt"
                )
                notifyUi { it.onJobsChanged() }
            }
        }
        // 云端上报（初始化完成前可能已有恢复任务完成，未初始化则跳过）
        if (::mqtt.isInitialized) {
            mqtt.publishJobEvent(ev)
        }
    }

    /**
     * 打印机生效布尔边沿。
     * 基线静默只免系统通知：基线离线仍进告警窗，基线在线仍 resolve。
     */
    private fun onPrinterEdgeInternal(
        id: String,
        online: Boolean,
        detail: String,
        failFirst: String,
        sinceMs: Long,
        baseline: Boolean
    ) {
        lastOnline[id] = online
        notifyUi { it.onPrintersChanged() }

        val p = cfg.findPrinter(id)
        if (online) {
            alertOverlay?.resolve(AlertModel.KIND_PRINTER_OFFLINE, id)
            if (!baseline && p != null) {
                Logger.info("打印机『${p.name}』上线")
                Notify.info(this, "打印机上线", "『${p.name}』已恢复连接")
            } else if (baseline) {
                Logger.info("打印机 $id 基线: 在线 $detail")
            }
            return
        }

        // 离线
        val name = p?.name ?: id
        val addr = p?.address() ?: ""
        val first = failFirst.ifBlank { detail }
        alertOverlay?.raise(
            AlertModel.KIND_PRINTER_OFFLINE, id, AlertModel.SEV_ERROR,
            "打印机离线 『$name』$addr($first)",
            sinceMs
        )
        if (baseline) {
            Logger.info("打印机 $id 基线: 离线 $detail")
        } else {
            Logger.warn("打印机『$name』离线: $detail")
            Notify.warn(this, "打印机离线", "『$name』${if (detail.isBlank()) "已离线" else detail}")
        }
    }

    /** MQTT 受理失败 → 告警窗（无自动消除） */
    private fun onAcceptFailedInternal(id: Long, code: Int, message: String) {
        val key = if (id == 0L) "t${System.nanoTime()}" else id.toString()
        val txt = "云端打印受理失败 id=$id code=$code: $message"
        alertOverlay?.raise(AlertModel.KIND_ACCEPT_FAILED, key, AlertModel.SEV_ERROR, txt)
    }

    /** MQTT 状态回调：状态机去重 + 基线静默 + 断/连通知 */
    private fun onMqttStatusInternal(connected: Boolean, error: String?) {
        notifyUi { it.onMqttStatus(connected, error) }
        val active = mqttClient()?.active() ?: false
        if (!active) {
            // 中性态（未启用/已停用）：Close/reload 不触发 onStatus，靠此重置
            mqttNotifyState = 0
            mqttBaselineDone = false
            alertOverlay?.resolve(AlertModel.KIND_MQTT_DOWN, "")
            return
        }
        val newState = if (connected) 1 else 2
        if (!mqttBaselineDone) {
            mqttBaselineDone = true
            mqttNotifyState = newState
            Logger.info("MQTT 状态基线: ${if (connected) "已连接" else "断开"}")
            return
        }
        if (newState == mqttNotifyState) return
        mqttNotifyState = newState
        if (connected) {
            Logger.info("MQTT 连接恢复")
            Notify.info(this, "云端连接恢复", "MQTT 已连接")
            alertOverlay?.resolve(AlertModel.KIND_MQTT_DOWN, "")
        } else {
            Logger.warn("MQTT 连接断开: $error")
            val detail = error ?: "MQTT 连接断开"
            Notify.warn(this, "云端连接断开", detail)
            alertOverlay?.raise(
                AlertModel.KIND_MQTT_DOWN, "", AlertModel.SEV_ERROR,
                "云端连接断开:$detail"
            )
        }
    }

    // —— 生命周期 ——

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        Logger.info("PrintService onStartCommand")
        return START_STICKY
    }

    override fun onDestroy() {
        Logger.info("PrintService onDestroy")
        destroyed = true
        initScope.cancel()
        uiListeners.clear()
        mainHandler.post {
            alertOverlay?.detach()
            alertOverlay = null
        }
        if (::statusMonitor.isInitialized) statusMonitor.stopAll()
        if (::mqtt.isInitialized) mqtt.close()
        if (::dispatcher.isInitialized) dispatcher.close()
        Logger.flush()
        instance = null
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun refreshNotificationTitle() {
        if (::cfg.isInitialized) {
            val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            nm.notify(NOTIFICATION_ID, buildNotification(cfg.settings().serviceName.ifEmpty { Settings().serviceName }))
        }
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "打印服务",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "打印机服务运行状态"
                setShowBadge(false)
            }
            val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            manager.createNotificationChannel(channel)
        }
    }

    private fun buildNotification(title: String): Notification {
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(title)
            .setContentText("打印服务运行中 · 点悬浮球可看告警")
            .setSmallIcon(android.R.drawable.ic_menu_manage)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }
}
