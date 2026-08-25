package com.congmingpay.android.util

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.PowerManager
import android.provider.Settings
import androidx.core.content.ContextCompat
import androidx.fragment.app.Fragment

/**
 * 运行时权限工具。
 *
 * 蓝牙：Android 12+ BLUETOOTH_SCAN/CONNECT（+定位，与佳博 demo 一致）；
 *       Android 11- 仅 ACCESS_FINE_LOCATION（BLUETOOTH 为安装时权限）。
 * USB：无需运行时权限（getDeviceList 可枚举；授权弹窗由 SDK connect 触发）。
 * 通知：Android 13+ POST_NOTIFICATIONS。
 * 电量优化白名单：REQUEST_IGNORE_BATTERY_OPTIMIZATIONS 引导。
 */
object Permissions {

    const val RC_BLUETOOTH = 100
    const val RC_NOTIFICATION = 101

    /** 蓝牙搜索所需权限是否已全部授予 */
    fun hasBluetooth(context: Context): Boolean {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            return ContextCompat.checkSelfPermission(context, Manifest.permission.BLUETOOTH_SCAN) == PackageManager.PERMISSION_GRANTED &&
                ContextCompat.checkSelfPermission(context, Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED &&
                ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            return ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
        }
        return true
    }

    private fun bluetoothPerms(): Array<String> = when {
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> arrayOf(
            Manifest.permission.BLUETOOTH_SCAN,
            Manifest.permission.BLUETOOTH_CONNECT,
            Manifest.permission.ACCESS_FINE_LOCATION
        )
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.M -> arrayOf(Manifest.permission.ACCESS_FINE_LOCATION)
        else -> emptyArray()
    }

    /** 请求蓝牙运行时权限（结果经 onRequestPermissionsResult 返回，requestCode=RC_BLUETOOTH） */
    fun requestBluetooth(activity: Activity) {
        val perms = bluetoothPerms()
        if (perms.isNotEmpty()) {
            activity.requestPermissions(perms, RC_BLUETOOTH)
        }
    }

    /** 弹窗/Fragment 内请求蓝牙权限 */
    fun requestBluetooth(fragment: Fragment) {
        val perms = bluetoothPerms()
        if (perms.isNotEmpty()) {
            fragment.requestPermissions(perms, RC_BLUETOOTH)
        }
    }

    /** 通知权限是否已授予（API 32- 恒为已授予） */
    fun hasNotification(context: Context): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return true
        return ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
    }

    /** 请求通知权限（API 33+，结果经 onRequestPermissionsResult，requestCode=RC_NOTIFICATION） */
    fun requestNotification(activity: Activity) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            activity.requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), RC_NOTIFICATION)
        }
    }

    /** 是否已在电量优化白名单（API 22- 恒为已豁免） */
    fun isIgnoringBatteryOptimizations(context: Context): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) return true
        val pm = context.getSystemService(Context.POWER_SERVICE) as? PowerManager ?: return true
        return pm.isIgnoringBatteryOptimizations(context.packageName)
    }

    /** 引导加入电量优化白名单（部分 ROM 不支持则静默忽略） */
    fun requestBatteryExemption(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) return
        if (isIgnoringBatteryOptimizations(context)) return
        try {
            val intent = Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS).apply {
                data = Uri.parse("package:${context.packageName}")
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            context.startActivity(intent)
        } catch (_: Exception) {}
    }

    /** 是否已授予「显示在其他应用上层」（悬浮告警球） */
    fun canDrawOverlays(context: Context): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) return true
        return Settings.canDrawOverlays(context)
    }

    /** 跳转系统页申请悬浮窗权限 */
    fun requestOverlayPermission(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) return
        if (canDrawOverlays(context)) return
        try {
            val intent = Intent(
                Settings.ACTION_MANAGE_OVERLAY_PERMISSION,
                Uri.parse("package:${context.packageName}")
            ).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            context.startActivity(intent)
        } catch (_: Exception) {}
    }
}
