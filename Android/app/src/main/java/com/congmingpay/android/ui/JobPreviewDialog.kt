package com.congmingpay.android.ui

import android.content.Context
import android.view.LayoutInflater
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.appcompat.app.AlertDialog
import com.congmingpay.android.R
import com.congmingpay.android.model.JobStatus
import com.congmingpay.android.service.PrintService
import com.congmingpay.android.store.JobStore
import com.google.gson.GsonBuilder
import com.google.gson.JsonParser
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * 任务预览对话框：任务详情 + JSON 原文（source_json）。
 * 数据从 DB 按号重载（不依赖内存条目，终态条目被淘汰后仍可预览）。
 */
object JobPreviewDialog {

    fun show(context: Context, no: Int) {
        val view = LayoutInflater.from(context).inflate(R.layout.dialog_job_preview, null)
        val dialog = AlertDialog.Builder(context)
            .setView(view)
            .create()

        val title = view.findViewById<TextView>(R.id.tv_job_title)
        val info = view.findViewById<TextView>(R.id.tv_job_info)
        val jsonLabel = view.findViewById<TextView>(R.id.tv_json_label)
        val jsonView = view.findViewById<EditText>(R.id.tv_json)

        view.findViewById<Button>(R.id.btn_preview_close).setOnClickListener { dialog.dismiss() }
        view.findViewById<Button>(R.id.btn_preview_retry).setOnClickListener {
            PrintService.instance?.retryJob(no)
            dialog.dismiss()
        }

        val store = JobStore.open(context)
        CoroutineScope(Dispatchers.Main).launch {
            val pj = withContext(Dispatchers.IO) { store.loadByNo(no) }
            if (pj == null) {
                title.text = "任务 #$no"
                info.text = "任务不存在（可能已被清除）"
                return@launch
            }
            title.text = "任务 #${pj.no}"
            info.text = buildString {
                append("文档: ${pj.doc.ifEmpty { "打印" }}\n")
                append("打印机: ${pj.printer.name.ifEmpty { "未知" }}\n")
                append("状态: ${JobStatus.label(pj.status)}")
                if (pj.err.isNotBlank()) append("\n错误: ${pj.err}")
                append("\n时间: ${pj.timeLabel}")
                if (pj.cloudId != null) append("\n云端ID: ${pj.cloudId}")
                append("\n切纸: ${if (pj.cut) "开" else "关"}  蜂鸣: ${if (pj.buzzer) "开" else "关"}")
                append("\n首部空行: ${pj.headLines}  尾部偏移: ${pj.tailLines}")
                if (pj.reprintNext) append("\n重印: 是")
            }

            val src = pj.sourceJson
            if (src != null && src.isNotEmpty()) {
                jsonLabel.visibility = View.VISIBLE
                jsonView.visibility = View.VISIBLE
                jsonView.setText(prettyJson(String(src, Charsets.UTF_8)))
            }
        }

        dialog.show()
    }

    private fun prettyJson(raw: String): String {
        return try {
            GsonBuilder().setPrettyPrinting().create().toJson(JsonParser.parseString(raw))
        } catch (e: Exception) {
            raw
        }
    }
}
