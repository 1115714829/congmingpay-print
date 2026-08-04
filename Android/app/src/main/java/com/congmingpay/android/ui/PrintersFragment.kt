package com.congmingpay.android.ui

import android.graphics.Color
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.congmingpay.android.R
import com.congmingpay.android.escpos.Receipt
import com.congmingpay.android.layout.LayoutRenderer
import com.congmingpay.android.layout.Sample
import com.congmingpay.android.model.Printer
import com.congmingpay.android.printer.PrintOptions
import com.congmingpay.android.service.PrintService
import com.congmingpay.android.service.UiListener
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.Date

/** 打印机列表合并项：配置 + 在线状态 */
data class PrinterViewItem(
    val printer: Printer,
    val online: Boolean?,   // null=未定
    val detail: String
)

/**
 * 打印机视图：工具栏 + 筛选/搜索 + RecyclerView 彩色状态。
 */
class PrintersFragment : Fragment(), UiListener {

    private lateinit var adapter: PrinterAdapter
    private var allItems: List<PrinterViewItem> = emptyList()
    private var filterIndex = 0
    private var searchText = ""
    private val attachHandler = Handler(Looper.getMainLooper())
    private val attachRetry = Runnable { if (isResumed) attachAndRefresh() }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        val view = inflater.inflate(R.layout.fragment_printers, container, false)

        adapter = PrinterAdapter(
            onSelectionChanged = { refreshActionButtons() },
            onLongPress = { openProperties(it) }
        )
        val recycler = view.findViewById<RecyclerView>(R.id.recycler_printers)
        recycler.layoutManager = LinearLayoutManager(requireContext())
        recycler.adapter = adapter

        // 筛选
        val spinner = view.findViewById<Spinner>(R.id.spinner_filter)
        spinner.adapter = ArrayAdapter(
            requireContext(),
            android.R.layout.simple_spinner_item,
            listOf("全部", "58mm", "80mm", "在线")
        ).apply {
            setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item)
        }
        spinner.setOnItemSelectedListener(object : android.widget.AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: android.widget.AdapterView<*>?, v: View?, pos: Int, id: Long) {
                filterIndex = pos
                applyFilter()
            }

            override fun onNothingSelected(parent: android.widget.AdapterView<*>?) {}
        })

        // 搜索
        val search = view.findViewById<EditText>(R.id.et_search)
        search.addTextChangedListener(object : android.text.TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
            override fun afterTextChanged(s: android.text.Editable?) {
                searchText = s?.toString() ?: ""
                applyFilter()
            }
        })

        view.findViewById<Button>(R.id.btn_add).setOnClickListener {
            AddPrinterDialogFragment.newInstance().show(parentFragmentManager, "add_printer")
        }
        view.findViewById<Button>(R.id.btn_refresh).setOnClickListener { refresh() }
        view.findViewById<Button>(R.id.btn_test_print).setOnClickListener { testPrint() }
        view.findViewById<Button>(R.id.btn_sample).setOnClickListener { samplePrint() }
        view.findViewById<Button>(R.id.btn_json_test).setOnClickListener {
            JsonTestDialogFragment.newInstance().show(parentFragmentManager, "json_test")
        }
        view.findViewById<Button>(R.id.btn_props).setOnClickListener {
            val p = selectedPrinter() ?: return@setOnClickListener
            openProperties(p)
        }
        view.findViewById<Button>(R.id.btn_delete).setOnClickListener { confirmDelete() }
        return view
    }

    override fun onResume() {
        super.onResume()
        attachAndRefresh()
    }

    override fun onPause() {
        super.onPause()
        attachHandler.removeCallbacks(attachRetry)
        PrintService.instance?.removeUiListener(this)
    }

    /** 服务尚未 onCreate 时短重试挂 listener；cfg 未就绪由 PrintService init/回放补齐 */
    private fun attachAndRefresh() {
        val svc = PrintService.instance
        if (svc == null) {
            attachHandler.postDelayed(attachRetry, 150)
            return
        }
        svc.addUiListener(this)
        refresh()
    }

    override fun onPrintersChanged() {
        if (isAdded) refresh()
    }

    private fun refresh() {
        if (!isAdded) return
        val svc = PrintService.instance ?: return
        val cfg = svc.configManager() ?: return
        val online = svc.printDispatcher()?.onlineSnapshot() ?: emptyMap()
        allItems = cfg.printers().map { p ->
            val oi = online[p.id]
            PrinterViewItem(p, oi?.online, oi?.detail ?: "")
        }
        applyFilter()
        refreshActionButtons()
    }

    private fun applyFilter() {
        var list = allItems
        list = when (filterIndex) {
            1 -> list.filter { it.printer.width == 58 }
            2 -> list.filter { it.printer.width == 80 }
            3 -> list.filter { it.online == true }
            else -> list
        }
        if (searchText.isNotBlank()) {
            val q = searchText.trim()
            list = list.filter {
                it.printer.name.contains(q) || it.printer.ip.contains(q) || it.printer.usbName.contains(q) || it.printer.bluetoothMac.contains(q)
            }
        }
        adapter.submit(list)

        val empty = view?.findViewById<TextView>(R.id.tv_empty)
        val recycler = view?.findViewById<RecyclerView>(R.id.recycler_printers)
        empty?.visibility = if (list.isEmpty()) View.VISIBLE else View.GONE
        recycler?.visibility = if (list.isEmpty()) View.GONE else View.VISIBLE
    }

    private fun selectedPrinter(): Printer? {
        val id = adapter.selectedId() ?: return null
        return allItems.firstOrNull { it.printer.id == id }?.printer
    }

    private fun refreshActionButtons() {
        if (!isAdded) return
        val has = selectedPrinter() != null
        view?.findViewById<Button>(R.id.btn_test_print)?.isEnabled = has
        view?.findViewById<Button>(R.id.btn_sample)?.isEnabled = has
        view?.findViewById<Button>(R.id.btn_props)?.isEnabled = has
        view?.findViewById<Button>(R.id.btn_delete)?.isEnabled = has
    }

    private fun testPrint() {
        val p = selectedPrinter() ?: return
        val svc = PrintService.instance ?: return
        val ctx = context ?: return
        val data = Receipt.buildTestReceipt(Date(), p.width)
        svc.submitPrint(p, "测试打印", data, PrintOptions()) { no ->
            if (isAdded) Toast.makeText(ctx, "已提交任务 #$no", Toast.LENGTH_SHORT).show()
        }
    }

    private fun samplePrint() {
        val p = selectedPrinter() ?: return
        val svc = PrintService.instance ?: return
        val ctx = context ?: return
        val compat = svc.configManager()?.settings()?.yunheCompat() ?: true
        lifecycleScope.launch {
            try {
                // 渲染在后台线程（含二维码/位图生成）
                val data = withContext(Dispatchers.IO) {
                    LayoutRenderer.render(Sample.contents, p.width, compat).data
                }
                if (!isAdded) return@launch
                svc.submitPrint(p, "打印样票", data, PrintOptions()) { no ->
                    if (isAdded) Toast.makeText(ctx, "已提交任务 #$no", Toast.LENGTH_SHORT).show()
                }
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (e: Exception) {
                if (isAdded) Toast.makeText(ctx, "样票渲染失败: ${e.message}", Toast.LENGTH_LONG).show()
            }
        }
    }

    private fun openProperties(p: Printer) {
        PropertiesSheetFragment.newInstance(p.id).show(parentFragmentManager, "props")
    }

    private fun confirmDelete() {
        val p = selectedPrinter() ?: return
        com.google.android.material.dialog.MaterialAlertDialogBuilder(requireContext())
            .setTitle("删除打印机")
            .setMessage("确定删除『${p.name}』？")
            .setPositiveButton("删除") { _, _ ->
                PrintService.instance?.removePrinter(p.id)
            }
            .setNegativeButton("取消", null)
            .show()
    }
}

/**
 * 打印机列表适配器：彩色状态点 + 单行选中；长按打开属性。
 */
class PrinterAdapter(
    private val onSelectionChanged: () -> Unit,
    private val onLongPress: (Printer) -> Unit
) : RecyclerView.Adapter<PrinterAdapter.Holder>() {

    private val items = mutableListOf<PrinterViewItem>()
    private var selectedId: String? = null

    class Holder(view: View) : RecyclerView.ViewHolder(view) {
        val dot: View = view.findViewById(R.id.status_dot)
        val name: TextView = view.findViewById(R.id.tv_name)
        val source: TextView = view.findViewById(R.id.tv_source)
        val detail: TextView = view.findViewById(R.id.tv_detail)
        val lastPrint: TextView = view.findViewById(R.id.tv_last_print)
        val status: TextView = view.findViewById(R.id.tv_status)
    }

    fun selectedId(): String? = selectedId

    fun submit(list: List<PrinterViewItem>) {
        items.clear()
        items.addAll(list)
        notifyDataSetChanged()
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): Holder =
        Holder(LayoutInflater.from(parent.context).inflate(R.layout.item_printer, parent, false))

    override fun onBindViewHolder(holder: Holder, position: Int) {
        val item = items[position]
        val p = item.printer
        holder.name.text = p.name
        holder.source.text = p.sourceLabel()
        holder.detail.text = "${p.brandLabel()} ${p.widthLabel()} ${p.connLabel()} ${p.address()}"
        holder.lastPrint.text = if (p.lastPrint.isNotEmpty()) "上次打印: ${p.lastPrint}" else "未打印"

        // 状态：在线绿 / 离线红 / 未定灰
        val online = item.online
        val statusColor = when {
            online == null -> COLOR_UNKNOWN
            online -> COLOR_ONLINE
            else -> COLOR_OFFLINE
        }
        holder.dot.background?.mutate()?.setTint(statusColor)
            ?: holder.dot.setBackgroundColor(statusColor)
        // 列表短标签对齐 Windows「就绪/离线/—」；完整 detail 在注册表供 MQTT state
        holder.status.text = when {
            online == null -> "—"
            online -> "就绪"
            else -> "离线"
        }
        holder.status.setTextColor(statusColor)

        val selected = p.id == selectedId
        val card = (holder.itemView as? android.view.ViewGroup)?.getChildAt(0)
        card?.setBackgroundResource(if (selected) R.drawable.bg_card_selected else R.drawable.bg_card)
        holder.itemView.setOnClickListener {
            selectedId = if (selectedId == p.id) null else p.id
            notifyDataSetChanged()
            onSelectionChanged()
        }
        holder.itemView.setOnLongClickListener {
            onLongPress(p)
            true
        }
    }

    override fun getItemCount(): Int = items.size

    companion object {
        private val COLOR_ONLINE = Color.parseColor("#2E7D32")
        private val COLOR_OFFLINE = Color.parseColor("#C62828")
        private val COLOR_UNKNOWN = Color.parseColor("#9E9E9E")
    }
}
