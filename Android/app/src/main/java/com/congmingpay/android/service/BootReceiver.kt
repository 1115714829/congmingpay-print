package com.congmingpay.android.service

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import com.congmingpay.android.config.ConfigManager
import com.congmingpay.android.logger.Logger
import java.io.File

/**
 * 开机拉起打印前台服务（受 Settings.bootStartEnabled 控制，默认开）。
 */
class BootReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent?) {
        val action = intent?.action ?: return
        if (action != Intent.ACTION_BOOT_COMPLETED &&
            action != "android.intent.action.QUICKBOOT_POWERON"
        ) {
            return
        }

        val cfg = try {
            ConfigManager(File(context.filesDir, "config.json")).also { it.load() }
        } catch (e: Exception) {
            Logger.error("开机自启读配置失败: ${e.message}")
            return
        }
        if (!cfg.settings().bootStartEnabled) {
            Logger.info("开机自启已关闭，跳过启动 PrintService")
            return
        }

        try {
            val svc = Intent(context, PrintService::class.java)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                context.startForegroundService(svc)
            } else {
                context.startService(svc)
            }
            Logger.info("开机自启已启动 PrintService")
        } catch (e: Exception) {
            Logger.error("开机自启启动失败: ${e.message}")
        }
    }
}
