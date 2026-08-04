package com.congmingpay.android

import android.app.Application
import com.congmingpay.android.logger.Logger
import java.io.File

/**
 * Application 入口。初始化全局日志。
 */
class App : Application() {
    override fun onCreate() {
        super.onCreate()
        Logger.init(File(filesDir, "log"))
        Logger.info("=== congmingpay Android 启动 ===")
    }
}
