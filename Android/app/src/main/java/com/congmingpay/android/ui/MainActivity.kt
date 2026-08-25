package com.congmingpay.android.ui

import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.WindowCompat
import androidx.fragment.app.Fragment
import com.congmingpay.android.R
import com.congmingpay.android.service.PrintService
import com.congmingpay.android.service.UiListener
import com.congmingpay.android.util.Permissions
import com.congmingpay.android.util.UiInsets
import com.google.android.material.navigation.NavigationBarView

/**
 * 主窗口：竖屏底栏 / 横屏侧栏（layout-land）+ 内容区 + 状态条。
 * targetSdk 35 边缘到边缘时经 [UiInsets] 垫系统栏，避免底栏/按钮盖住系统导航栏。
 */
class MainActivity : AppCompatActivity(), UiListener {

    private lateinit var mainNav: NavigationBarView
    private lateinit var statusBar: TextView

    /** 按导航项持有 Fragment 实例（切换不丢输入/状态） */
    private val fragments = HashMap<Int, Fragment>()
    private val attachHandler = Handler(Looper.getMainLooper())
    private val attachRetry = Runnable { if (!isFinishing) attachAndRefresh() }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // target 35 默认边到边：自行消费 systemBars，布局不钻进手势条/导航栏
        WindowCompat.setDecorFitsSystemWindows(window, false)
        setContentView(R.layout.activity_main)

        val root = findViewById<android.view.View>(R.id.root)
        UiInsets.applySystemBars(root)

        mainNav = findViewById(R.id.main_nav)
        statusBar = findViewById(R.id.status_bar)

        // 启动前台服务
        val intent = Intent(this, PrintService::class.java)
        if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O) {
            startForegroundService(intent)
        } else {
            startService(intent)
        }

        if (!Permissions.hasNotification(this)) Permissions.requestNotification(this)
        Permissions.requestBatteryExemption(this)
        // 悬浮告警球：缺权限时引导一次
        if (!Permissions.canDrawOverlays(this)) {
            Permissions.requestOverlayPermission(this)
        }

        mainNav.setOnItemSelectedListener { item ->
            when (item.itemId) {
                R.id.nav_printers -> { showFragment(R.id.nav_printers) { PrintersFragment() }; true }
                R.id.nav_jobs -> { showFragment(R.id.nav_jobs) { JobsFragment() }; true }
                R.id.nav_settings -> { showFragment(R.id.nav_settings) { SettingsFragment() }; true }
                R.id.nav_logs -> { showFragment(R.id.nav_logs) { LogsFragment() }; true }
                else -> false
            }
        }
        mainNav.selectedItemId = R.id.nav_printers
    }

    private fun showFragment(key: Int, creator: () -> Fragment) {
        val f = fragments.getOrPut(key, creator)
        if (f.isAdded) return
        supportFragmentManager.beginTransaction()
            .replace(R.id.content_frame, f)
            .commit()
    }

    override fun onResume() {
        super.onResume()
        attachAndRefresh()
        // 从悬浮窗权限页返回后补挂告警球
        PrintService.instance?.refreshAlertOverlay()
    }

    override fun onPause() {
        super.onPause()
        attachHandler.removeCallbacks(attachRetry)
        PrintService.instance?.removeUiListener(this)
    }

    private fun attachAndRefresh() {
        val svc = PrintService.instance
        if (svc == null) {
            attachHandler.postDelayed(attachRetry, 150)
            return
        }
        svc.addUiListener(this)
        refreshStatusBar()
    }

    override fun onPrintersChanged() {
        if (!isFinishing) refreshStatusBar()
    }

    override fun onJobsChanged() {
        if (!isFinishing) refreshStatusBar()
    }

    private fun refreshStatusBar() {
        val svc = PrintService.instance
        val total = svc?.configManager()?.printers()?.size ?: 0
        val online = svc?.printDispatcher()?.onlineSnapshot()?.values?.count { it.online } ?: 0
        val active = svc?.printDispatcher()?.activeJobCount() ?: 0
        statusBar.text = "共 $total 台 · $online 在线 · $active 任务进行中"
    }
}
