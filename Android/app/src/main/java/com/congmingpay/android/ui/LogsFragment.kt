package com.congmingpay.android.ui

import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.Spinner
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.fragment.app.Fragment
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.congmingpay.android.R
import com.congmingpay.android.logger.LogEntry
import com.congmingpay.android.logger.LogLevel
import com.congmingpay.android.logger.LogListener
import com.congmingpay.android.logger.Logger
import com.google.android.material.button.MaterialButton

/**
 * 运行日志：内存环实时展示；级别过滤；上下滚动；贴底自动滚。
 */
class LogsFragment : Fragment() {

    private lateinit var recycler: RecyclerView
    private lateinit var adapter: LogAdapter
    private lateinit var layoutManager: LinearLayoutManager
    private lateinit var pathView: TextView

    /** 用户离开底部后暂停自动滚，滚到底或滑回底部恢复 */
    private var autoScroll = true
    private var minLevel: LogLevel = LogLevel.INFO

    private val listener = LogListener { entry ->
        if (!isAdded) return@LogListener
        if (!minLevel.passes(entry.level)) return@LogListener
        adapter.append(entry)
        if (autoScroll) {
            recycler.post {
                val last = adapter.itemCount - 1
                if (last >= 0) recycler.smoothScrollToPosition(last)
            }
        }
    }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        val view = inflater.inflate(R.layout.fragment_logs, container, false)
        pathView = view.findViewById(R.id.tv_log_path)
        recycler = view.findViewById(R.id.rv_logs)
        layoutManager = LinearLayoutManager(requireContext())
        recycler.layoutManager = layoutManager
        adapter = LogAdapter()
        recycler.adapter = adapter

        recycler.addOnScrollListener(object : RecyclerView.OnScrollListener() {
            override fun onScrolled(rv: RecyclerView, dx: Int, dy: Int) {
                autoScroll = isNearBottom()
            }
        })

        val spinner = view.findViewById<Spinner>(R.id.spinner_log_level)
        spinner.adapter = ArrayAdapter(
            requireContext(),
            android.R.layout.simple_spinner_dropdown_item,
            listOf("全部", "INFO", "WARN", "ERROR")
        )
        spinner.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, v: View?, position: Int, id: Long) {
                minLevel = when (position) {
                    2 -> LogLevel.WARN
                    3 -> LogLevel.ERROR
                    else -> LogLevel.INFO
                }
                reload()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }

        view.findViewById<MaterialButton>(R.id.btn_scroll_bottom).setOnClickListener {
            autoScroll = true
            scrollToBottom(smooth = true)
        }
        view.findViewById<MaterialButton>(R.id.btn_clear_logs).setOnClickListener {
            Logger.clearRing()
            adapter.replace(emptyList())
        }

        return view
    }

    override fun onResume() {
        super.onResume()
        pathView.text = Logger.currentPath()?.let { "文件: $it" } ?: "文件: —"
        reload()
        Logger.addListener(listener)
        if (autoScroll) scrollToBottom(smooth = false)
    }

    override fun onPause() {
        super.onPause()
        Logger.removeListener(listener)
    }

    private fun reload() {
        adapter.replace(Logger.snapshot(minLevel))
        if (autoScroll) scrollToBottom(smooth = false)
    }

    private fun isNearBottom(): Boolean {
        val lastVisible = layoutManager.findLastVisibleItemPosition()
        val last = adapter.itemCount - 1
        return last < 0 || lastVisible >= last - 2
    }

    private fun scrollToBottom(smooth: Boolean) {
        val last = adapter.itemCount - 1
        if (last < 0) return
        if (smooth) recycler.smoothScrollToPosition(last) else recycler.scrollToPosition(last)
    }

    private class LogAdapter : RecyclerView.Adapter<LogAdapter.VH>() {
        private val items = ArrayList<LogEntry>()

        fun replace(list: List<LogEntry>) {
            items.clear()
            items.addAll(list)
            notifyDataSetChanged()
        }

        fun append(entry: LogEntry) {
            items.add(entry)
            notifyItemInserted(items.size - 1)
        }

        override fun getItemCount(): Int = items.size

        override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): VH {
            val v = LayoutInflater.from(parent.context).inflate(R.layout.item_log, parent, false)
            return VH(v)
        }

        override fun onBindViewHolder(holder: VH, position: Int) {
            holder.bind(items[position])
        }

        class VH(itemView: View) : RecyclerView.ViewHolder(itemView) {
            private val ts: TextView = itemView.findViewById(R.id.tv_log_ts)
            private val level: TextView = itemView.findViewById(R.id.tv_log_level)
            private val msg: TextView = itemView.findViewById(R.id.tv_log_msg)

            fun bind(e: LogEntry) {
                ts.text = e.ts.substringAfter(' ') // 只显示时分秒段以省宽；完整仍在 e.ts
                // 保留完整时间更清晰：用完整 ts
                ts.text = e.ts
                level.text = e.level.name
                msg.text = e.msg
                val color = when (e.level) {
                    LogLevel.ERROR -> ContextCompat.getColor(itemView.context, R.color.log_error)
                    LogLevel.WARN -> ContextCompat.getColor(itemView.context, R.color.log_warn)
                    LogLevel.INFO -> ContextCompat.getColor(itemView.context, R.color.log_info)
                }
                level.setTextColor(color)
                msg.setTextColor(if (e.level == LogLevel.INFO) color else color)
            }
        }
    }
}
