package com.congmingpay.android.ui

import android.app.Dialog
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.EditText
import android.widget.TextView
import androidx.appcompat.app.AppCompatDialogFragment
import androidx.lifecycle.lifecycleScope
import com.congmingpay.android.R
import com.congmingpay.android.api.PrintProcessor
import com.congmingpay.android.api.PrintRequestParser
import com.congmingpay.android.config.ConfigManager
import com.congmingpay.android.errcode.CodedException
import com.congmingpay.android.errcode.errorCode
import com.congmingpay.android.printer.PrintDispatcher
import com.congmingpay.android.service.PrintService
import com.google.android.material.button.MaterialButton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** JSON 测试弹窗：填 JSON 单对象或数组，本地直通 PrintProcessor。 */
class JsonTestDialogFragment : AppCompatDialogFragment() {

    override fun onCreateDialog(savedInstanceState: Bundle?): Dialog {
        return super.onCreateDialog(savedInstanceState).apply {
            setCanceledOnTouchOutside(true)
        }
    }

    override fun onStart() {
        super.onStart()
        dialog?.window?.setLayout(
            WindowManager.LayoutParams.MATCH_PARENT,
            WindowManager.LayoutParams.WRAP_CONTENT
        )
    }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        return inflater.inflate(R.layout.dialog_json_test, container, false)
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        val et = view.findViewById<EditText>(R.id.et_json)
        val result = view.findViewById<TextView>(R.id.tv_result)
        if (et.text.isNullOrBlank()) {
            et.setText(
                "{\"printer\":{\"ip\":\"192.168.1.100\"},\"type\":0,\"contents\":[" +
                    "{\"cont\":\"云端测试\",\"type\":\"title\"}," +
                    "{\"cont\":\"来自 JSON 测试\",\"type\":\"text\"}]}"
            )
        }
        view.findViewById<MaterialButton>(R.id.btn_close).setOnClickListener { dismiss() }
        view.findViewById<MaterialButton>(R.id.btn_submit).setOnClickListener {
            val raw = et.text.toString()
            val svc = PrintService.instance
            val cfg = svc?.configManager()
            val dispatcher = svc?.printDispatcher()
            if (cfg == null || dispatcher == null) {
                result.text = "服务未就绪，请稍后重试"
                return@setOnClickListener
            }
            result.text = "处理中…"
            viewLifecycleOwner.lifecycleScope.launch {
                val lines = withContext(Dispatchers.IO) { runJsonTest(cfg, dispatcher, raw) }
                result.text = lines.joinToString("\n")
            }
        }
    }

    private fun runJsonTest(cfg: ConfigManager, dispatcher: PrintDispatcher, raw: String): List<String> {
        val out = mutableListOf<String>()
        val (requests, wasArray, err) = PrintRequestParser.parse(raw.toByteArray(Charsets.UTF_8))
        if (err != null) {
            out += "解析失败: ${err.message}"
            return out
        }
        out += if (wasArray) "数组 ${requests.size} 条" else "单对象"
        for ((i, req) in requests.withIndex()) {
            try {
                val (procResult, changed) = PrintProcessor.process(cfg, dispatcher, req)
                if (changed) {
                    cfg.save()
                    PrintService.instance?.notifyPrintersChanged()
                }
                out += "[${i + 1}] 已受理 任务#${procResult.jobNo} 打印机『${procResult.printer.name}』(${procResult.pWidth}mm 共${procResult.pCopy}份)"
            } catch (e: CodedException) {
                out += "[${i + 1}] 拒绝 code=${e.code}: ${e.message}"
            } catch (e: Exception) {
                out += "[${i + 1}] 失败 code=${errorCode(e)}: ${e.message}"
            }
        }
        return out
    }

    companion object {
        fun newInstance() = JsonTestDialogFragment()
    }
}
