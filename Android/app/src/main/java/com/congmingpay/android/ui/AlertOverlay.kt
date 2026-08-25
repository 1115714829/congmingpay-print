package com.congmingpay.android.ui

import android.annotation.SuppressLint
import android.content.Context
import android.content.Intent
import android.graphics.PixelFormat
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.view.ContextThemeWrapper
import android.view.Gravity
import android.view.LayoutInflater
import android.view.MotionEvent
import android.view.View
import android.view.WindowManager
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.congmingpay.android.R
import com.congmingpay.android.logger.Logger
import com.congmingpay.android.util.Permissions
import com.google.android.material.button.MaterialButton
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * 悬浮告警球：平时小圆点，故障自动展开面板。
 *
 * 对齐 Windows 常驻告警窗语义；由 [PrintService] 持有生命周期。
 * 需 SYSTEM_ALERT_WINDOW；无权限时静默不显示。
 *
 * 注意：Service/Application Context 无 Activity 主题，inflate Material 组件前
 * 必须包一层 [ContextThemeWrapper]（Theme.Congmingpay），否则点击展开会 InflateException 崩进程。
 */
class AlertOverlay(private val appContext: Context) {

    private val wm = appContext.getSystemService(Context.WINDOW_SERVICE) as WindowManager
    private val mainHandler = Handler(Looper.getMainLooper())
    private val model = AlertModel()
    private val timeFmt = SimpleDateFormat("HH:mm:ss", Locale.getDefault())

    /** 带 Material 主题的 inflater，专供悬浮窗布局 */
    private val themedInflater: LayoutInflater by lazy {
        val themed = ContextThemeWrapper(appContext, R.style.Theme_Congmingpay)
        LayoutInflater.from(themed)
    }

    private var dotView: View? = null
    private var panelView: View? = null
    private var expanded = false
    private var attached = false

    private var dotParams: WindowManager.LayoutParams? = null
    private var panelParams: WindowManager.LayoutParams? = null

    private var adapter: AlertAdapter? = null

    private val tickRunnable = object : Runnable {
        override fun run() {
            if (!attached) return
            if (expanded && !model.isEmpty()) {
                adapter?.notifyDataSetChanged()
            }
            updateDotBadge()
            mainHandler.postDelayed(this, 1000L)
        }
    }

    init {
        model.setOnChanged {
            mainHandler.post { onModelChanged() }
        }
    }

    /** 服务启动后调用：有悬浮窗权限则挂出圆点 */
    fun attach() {
        mainHandler.post {
            if (attached) return@post
            if (!Permissions.canDrawOverlays(appContext)) {
                return@post
            }
            showDot()
            attached = true
            mainHandler.post(tickRunnable)
        }
    }

    /** 服务销毁时调用 */
    fun detach() {
        mainHandler.post {
            attached = false
            mainHandler.removeCallbacks(tickRunnable)
            hidePanel()
            hideDot()
            model.dismissAll()
        }
    }

    /** 权限刚授予后可再调，补挂圆点 */
    fun refreshPermission() {
        mainHandler.post {
            if (!Permissions.canDrawOverlays(appContext)) {
                if (attached) {
                    attached = false
                    mainHandler.removeCallbacks(tickRunnable)
                    hidePanel()
                    hideDot()
                }
                return@post
            }
            if (!attached) {
                showDot()
                attached = true
                mainHandler.post(tickRunnable)
            }
        }
    }

    fun raise(kind: String, key: String, sev: Int, text: String, whenMs: Long = System.currentTimeMillis()) {
        mainHandler.post {
            model.raise(kind, key, sev, text, whenMs)
            // 新告警自动展开
            if (attached) {
                try {
                    expand()
                } catch (e: Exception) {
                    Logger.error("悬浮告警球自动展开失败: ${e.message}")
                }
            }
        }
    }

    fun resolve(kind: String, key: String) {
        mainHandler.post {
            model.resolve(kind, key)
            if (model.isEmpty() && expanded) collapse()
        }
    }

    fun dismissAll() {
        mainHandler.post {
            model.dismissAll()
            collapse()
        }
    }

    private fun onModelChanged() {
        updateDotBadge()
        if (expanded) {
            bindPanel()
            if (model.isEmpty()) {
                // 列表被 resolve 清空时收起
                collapse()
            }
        }
    }

    private fun overlayType(): Int =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY
        } else {
            @Suppress("DEPRECATION")
            WindowManager.LayoutParams.TYPE_PHONE
        }

    @SuppressLint("ClickableViewAccessibility", "InflateParams")
    private fun showDot() {
        if (dotView != null) return
        val view = themedInflater.inflate(R.layout.overlay_alert_dot, null)
        val density = appContext.resources.displayMetrics.density
        val size = (48 * density).toInt()
        val params = WindowManager.LayoutParams(
            size, size,
            overlayType(),
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
                WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN or
                WindowManager.LayoutParams.FLAG_LAYOUT_NO_LIMITS,
            PixelFormat.TRANSLUCENT
        ).apply {
            gravity = Gravity.TOP or Gravity.START
            val dm = appContext.resources.displayMetrics
            x = dm.widthPixels - size - (16 * density).toInt()
            y = (dm.heightPixels * 0.35).toInt()
        }

        var downX = 0f
        var downY = 0f
        var startX = 0
        var startY = 0
        var moved = false
        view.setOnTouchListener { _, ev ->
            when (ev.action) {
                MotionEvent.ACTION_DOWN -> {
                    downX = ev.rawX
                    downY = ev.rawY
                    startX = params.x
                    startY = params.y
                    moved = false
                    true
                }
                MotionEvent.ACTION_MOVE -> {
                    val dx = (ev.rawX - downX).toInt()
                    val dy = (ev.rawY - downY).toInt()
                    if (Math.abs(dx) > 8 || Math.abs(dy) > 8) moved = true
                    params.x = startX + dx
                    params.y = startY + dy
                    try {
                        wm.updateViewLayout(view, params)
                    } catch (_: Exception) {}
                    true
                }
                MotionEvent.ACTION_UP -> {
                    if (!moved) {
                        try {
                            if (expanded) collapse() else expand()
                        } catch (e: Exception) {
                            Logger.error("悬浮告警球展开失败: ${e.message}")
                        }
                    }
                    true
                }
                else -> false
            }
        }

        try {
            wm.addView(view, params)
            dotView = view
            dotParams = params
            updateDotBadge()
        } catch (e: Exception) {
            // 权限被拒或 ROM 限制
            dotView = null
            dotParams = null
        }
    }

    private fun hideDot() {
        val v = dotView ?: return
        try {
            wm.removeView(v)
        } catch (_: Exception) {}
        dotView = null
        dotParams = null
    }

    @SuppressLint("InflateParams")
    private fun expand() {
        if (!attached) return
        if (expanded && panelView != null) {
            bindPanel()
            return
        }
        val view = try {
            themedInflater.inflate(R.layout.overlay_alert_panel, null)
        } catch (e: Exception) {
            Logger.error("悬浮告警面板 inflate 失败: ${e.message}")
            return
        }
        val density = appContext.resources.displayMetrics.density
        val width = (320 * density).toInt()
        val params = WindowManager.LayoutParams(
            width,
            WindowManager.LayoutParams.WRAP_CONTENT,
            overlayType(),
            WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL or
                WindowManager.LayoutParams.FLAG_WATCH_OUTSIDE_TOUCH or
                WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN,
            PixelFormat.TRANSLUCENT
        ).apply {
            gravity = Gravity.TOP or Gravity.START
            val dp = dotParams
            if (dp != null) {
                x = (dp.x - width + (48 * density).toInt()).coerceAtLeast(8)
                y = dp.y
            } else {
                val dm = appContext.resources.displayMetrics
                x = dm.widthPixels - width - 16
                y = (dm.heightPixels * 0.3).toInt()
            }
        }

        val rv = view.findViewById<RecyclerView>(R.id.rv_alerts)
        rv.layoutManager = LinearLayoutManager(appContext)
        adapter = AlertAdapter(model) { openApp() }
        rv.adapter = adapter

        view.findViewById<MaterialButton>(R.id.btn_open_app).setOnClickListener { openApp() }
        view.findViewById<MaterialButton>(R.id.btn_dismiss_all).setOnClickListener { dismissAll() }
        view.findViewById<MaterialButton>(R.id.btn_collapse).setOnClickListener { collapse() }

        // 点面板外收起
        view.setOnTouchListener { _, ev ->
            if (ev.action == MotionEvent.ACTION_OUTSIDE) {
                collapse()
                true
            } else false
        }

        try {
            // 展开时隐藏圆点，避免叠层
            hideDot()
            wm.addView(view, params)
            panelView = view
            panelParams = params
            expanded = true
            bindPanel()
        } catch (e: Exception) {
            Logger.error("悬浮告警面板 addView 失败: ${e.message}")
            panelView = null
            panelParams = null
            expanded = false
            showDot()
        }
    }

    private fun collapse() {
        hidePanel()
        expanded = false
        if (attached && Permissions.canDrawOverlays(appContext)) {
            showDot()
        }
    }

    private fun hidePanel() {
        val v = panelView ?: return
        try {
            wm.removeView(v)
        } catch (_: Exception) {}
        panelView = null
        panelParams = null
        adapter = null
        expanded = false
    }

    private fun bindPanel() {
        val view = panelView ?: return
        val empty = view.findViewById<TextView>(R.id.tv_empty)
        val rv = view.findViewById<RecyclerView>(R.id.rv_alerts)
        if (model.isEmpty()) {
            empty.visibility = View.VISIBLE
            rv.visibility = View.GONE
        } else {
            empty.visibility = View.GONE
            rv.visibility = View.VISIBLE
        }
        adapter?.notifyDataSetChanged()
    }

    private fun updateDotBadge() {
        val badge = dotView?.findViewById<TextView>(R.id.tv_dot_badge) ?: return
        val n = model.size()
        badge.text = when {
            n <= 0 -> ""
            n > 99 -> "99+"
            else -> n.toString()
        }
    }

    private fun openApp() {
        try {
            val intent = Intent(appContext, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or
                    Intent.FLAG_ACTIVITY_SINGLE_TOP or
                    Intent.FLAG_ACTIVITY_REORDER_TO_FRONT
            }
            appContext.startActivity(intent)
        } catch (_: Exception) {}
    }

    private inner class AlertAdapter(
        private val model: AlertModel,
        private val onRowClick: () -> Unit
    ) : RecyclerView.Adapter<AlertAdapter.VH>() {

        inner class VH(v: View) : RecyclerView.ViewHolder(v) {
            val whenTv: TextView = v.findViewById(R.id.tv_when)
            val durTv: TextView = v.findViewById(R.id.tv_dur)
            val textTv: TextView = v.findViewById(R.id.tv_text)
        }

        override fun onCreateViewHolder(parent: android.view.ViewGroup, viewType: Int): VH {
            val v = themedInflater.inflate(R.layout.item_alert, parent, false)
            return VH(v)
        }

        override fun getItemCount(): Int = model.size()

        override fun onBindViewHolder(holder: VH, position: Int) {
            val items = model.snapshot()
            if (position < 0 || position >= items.size) return
            val it = items[position]
            holder.whenTv.text = timeFmt.format(Date(it.whenMs))
            holder.durTv.text = alertDurText(System.currentTimeMillis() - it.whenMs)
            holder.textTv.text = it.text
            val colorRes = if (it.sev == AlertModel.SEV_ERROR) {
                R.color.status_failed
            } else {
                R.color.status_waiting
            }
            holder.textTv.setTextColor(ContextCompat.getColor(appContext, colorRes))
            holder.itemView.setOnClickListener { onRowClick() }
        }
    }
}
