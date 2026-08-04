package com.congmingpay.android.ui

import android.graphics.Color
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.TextView
import android.widget.Toast
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.congmingpay.android.R
import com.congmingpay.android.model.JobStatus
import com.congmingpay.android.service.PrintService
import com.congmingpay.android.service.UiListener
import com.congmingpay.android.store.JobListRow
import com.congmingpay.android.store.JobStore
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** 任务状态配色 */
fun jobStatusColor(status: String): Int = when (status) {
    JobStatus.QUEUED -> Color.parseColor("#1976D2")    // 排队蓝
    JobStatus.PRINTING -> Color.parseColor("#388E3C")  // 打印中绿
    JobStatus.WAITING -> Color.parseColor("#F57C00")   // 等待重试橙
    JobStatus.DONE -> Color.parseColor("#9E9E9E")      // 完成灰
    JobStatus.FAILED -> Color.parseColor("#D32F2F")    // 失败红
    else -> Color.parseColor("#9E9E9E")
}

/**
 * 打印队列视图：任务列表（预览/重新打印/取消/清除已完成），状态彩色。
 * 数据源 JobStore.loadAllList()（SQLite 投影查询，后台线程）+ 3s 轻量轮询 + 事件回调。
 */
class JobsFragment : Fragment(), UiListener {

    private lateinit var adapter: JobAdapter
    private var store: JobStore? = null
    private var pollJob: Job? = null
    private val attachHandler = Handler(Looper.getMainLooper())
    private val attachRetry = Runnable { if (isResumed) attachAndRefresh() }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        val view = inflater.inflate(R.layout.fragment_jobs, container, false)

        adapter = JobAdapter { refreshActionButtons() }
        val recycler = view.findViewById<RecyclerView>(R.id.recycler_jobs)
        recycler.layoutManager = LinearLayoutManager(requireContext())
        recycler.adapter = adapter

        view.findViewById<Button>(R.id.btn_preview).setOnClickListener { preview() }
        view.findViewById<Button>(R.id.btn_retry).setOnClickListener { retry() }
        view.findViewById<Button>(R.id.btn_cancel).setOnClickListener { confirmCancel() }
        view.findViewById<Button>(R.id.btn_clear_done).setOnClickListener { confirmClearDone() }
        return view
    }

    override fun onResume() {
        super.onResume()
        attachAndRefresh()
        startPolling()
    }

    override fun onPause() {
        super.onPause()
        attachHandler.removeCallbacks(attachRetry)
        PrintService.instance?.removeUiListener(this)
        pollJob?.cancel()
        pollJob = null
    }

    private fun attachAndRefresh() {
        val svc = PrintService.instance
        if (svc == null) {
            attachHandler.postDelayed(attachRetry, 150)
            return
        }
        svc.addUiListener(this)
        refresh()
    }

    override fun onJobsChanged() {
        if (isAdded) refresh()
    }

    private fun startPolling() {
        pollJob?.cancel()
        pollJob = lifecycleScope.launch {
            while (isActive) {
                delay(3000)
                refresh()
            }
        }
    }

    private fun refresh() {
        if (!isAdded) return
        lifecycleScope.launch {
            if (!isAdded) return@launch
            val s = store ?: run {
                store = JobStore.open(requireContext())
                store!!
            }
            val jobs = withContext(Dispatchers.IO) { s.loadAllList() }
            if (!isAdded) return@launch
            adapter.submit(jobs)
            val empty = view?.findViewById<TextView>(R.id.tv_empty)
            val recycler = view?.findViewById<RecyclerView>(R.id.recycler_jobs)
            empty?.visibility = if (jobs.isEmpty()) View.VISIBLE else View.GONE
            recycler?.visibility = if (jobs.isEmpty()) View.GONE else View.VISIBLE
            refreshActionButtons()
        }
    }

    private fun selectedNo(): Int? = adapter.selectedNo()

    private fun refreshActionButtons() {
        if (!isAdded) return
        val has = selectedNo() != null
        view?.findViewById<Button>(R.id.btn_preview)?.isEnabled = has
        view?.findViewById<Button>(R.id.btn_retry)?.isEnabled = has
        view?.findViewById<Button>(R.id.btn_cancel)?.isEnabled = has
    }

    private fun preview() {
        val no = selectedNo() ?: return
        JobPreviewDialog.show(requireContext(), no)
    }

    private fun retry() {
        val no = selectedNo() ?: return
        PrintService.instance?.retryJob(no)
        Toast.makeText(requireContext(), "任务 #$no 重新打印（带重印抬头）", Toast.LENGTH_SHORT).show()
    }

    private fun confirmCancel() {
        val no = selectedNo() ?: return
        MaterialAlertDialogBuilder(requireContext())
            .setTitle("取消任务")
            .setMessage("确定取消任务 #$no ？（会从队列删除）")
            .setPositiveButton("取消任务") { _, _ ->
                PrintService.instance?.cancelJob(no)
            }
            .setNegativeButton("不取消", null)
            .show()
    }

    private fun confirmClearDone() {
        MaterialAlertDialogBuilder(requireContext())
            .setTitle("清除已完成")
            .setMessage("确定清除所有已完成/失败的任务？")
            .setPositiveButton("清除") { _, _ ->
                PrintService.instance?.clearDoneJobs()
            }
            .setNegativeButton("取消", null)
            .show()
    }
}

/**
 * 任务列表适配器：任务号/文档/打印机/状态彩色/时间/错误详情。
 */
class JobAdapter(private val onSelectionChanged: () -> Unit) : RecyclerView.Adapter<JobAdapter.Holder>() {

    private val items = mutableListOf<JobListRow>()
    private var selectedNo: Int? = null

    class Holder(view: View) : RecyclerView.ViewHolder(view) {
        val jobNo: TextView = view.findViewById(R.id.tv_jobno)
        val doc: TextView = view.findViewById(R.id.tv_doc)
        val status: TextView = view.findViewById(R.id.tv_status)
        val printer: TextView = view.findViewById(R.id.tv_printer)
        val time: TextView = view.findViewById(R.id.tv_time)
        val err: TextView = view.findViewById(R.id.tv_err)
    }

    fun selectedNo(): Int? = selectedNo

    fun submit(list: List<JobListRow>) {
        items.clear()
        items.addAll(list)
        notifyDataSetChanged()
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): Holder =
        Holder(LayoutInflater.from(parent.context).inflate(R.layout.item_job, parent, false))

    override fun onBindViewHolder(holder: Holder, position: Int) {
        val j = items[position]
        holder.jobNo.text = "#${j.no}"
        holder.doc.text = j.doc.ifEmpty { "打印" }
        holder.status.text = JobStatus.label(j.status)
        holder.status.setTextColor(jobStatusColor(j.status))
        holder.printer.text = "打印机: ${j.printerName.ifEmpty { "未知" }}"
        holder.time.text = j.timeLabel
        if (j.err.isNotBlank()) {
            holder.err.visibility = View.VISIBLE
            holder.err.text = j.err
        } else {
            holder.err.visibility = View.GONE
        }

        val selected = j.no == selectedNo
        val card = (holder.itemView as? ViewGroup)?.getChildAt(0)
        card?.setBackgroundResource(if (selected) R.drawable.bg_card_selected else R.drawable.bg_card)
        holder.itemView.setOnClickListener {
            selectedNo = if (selectedNo == j.no) null else j.no
            notifyDataSetChanged()
            onSelectionChanged()
        }
    }

    override fun getItemCount(): Int = items.size
}
