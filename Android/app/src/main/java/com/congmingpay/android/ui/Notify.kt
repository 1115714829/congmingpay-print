package com.congmingpay.android.ui

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.core.app.NotificationCompat
import com.congmingpay.android.service.PrintService
import java.util.concurrent.atomic.AtomicInteger

/**
 * 系统通知统一出口（托盘气泡）。
 *
 * 触发：打印失败/卡单 waiting/打印机离线·恢复/MQTT 断线·恢复。
 * 受 Settings.notifyDisabled 控制（反向字段，false=启用）。
 * 任意线程可调（NotificationManager 线程安全）。
 */
object Notify {

    private const val CHANNEL_ID = "print_events"
    private val seq = AtomicInteger(10)

    /** 创建通知渠道（App 内多次调用幂等） */
    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        val channel = NotificationChannel(
            CHANNEL_ID,
            "打印事件",
            NotificationManager.IMPORTANCE_DEFAULT
        ).apply {
            description = "打印失败/卡单/打印机离线/云端断线"
        }
        manager.createNotificationChannel(channel)
    }

    /**
     * 发送通知。
     * @param level info / warn / error
     */
    fun notify(context: Context, level: String, title: String, msg: String) {
        // 通知开关：反向字段（false=启用）
        val disabled = PrintService.instance?.configManager()?.settings()?.notifyDisabled ?: false
        if (disabled) return

        ensureChannel(context)

        val content = if (msg.isBlank()) title else msg
        val icon = when (level) {
            "error" -> android.R.drawable.stat_notify_error
            "warn" -> android.R.drawable.ic_dialog_alert
            else -> android.R.drawable.stat_notify_sync
        }

        val intent = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val pi = PendingIntent.getActivity(
            context,
            0,
            intent,
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) PendingIntent.FLAG_IMMUTABLE else PendingIntent.FLAG_UPDATE_CURRENT
        )

        val notification = NotificationCompat.Builder(context, CHANNEL_ID)
            .setContentTitle(title)
            .setContentText(content)
            .setSmallIcon(icon)
            .setAutoCancel(true)
            .setContentIntent(pi)
            .build()

        try {
            val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            manager.notify(seq.getAndIncrement(), notification)
        } catch (_: Exception) {}
    }

    fun info(context: Context, title: String, msg: String) = notify(context, "info", title, msg)
    fun warn(context: Context, title: String, msg: String) = notify(context, "warn", title, msg)
    fun error(context: Context, title: String, msg: String) = notify(context, "error", title, msg)
}
