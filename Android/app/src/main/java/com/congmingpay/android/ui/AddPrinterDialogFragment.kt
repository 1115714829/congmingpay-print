package com.congmingpay.android.ui

import android.content.pm.PackageManager
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.ArrayAdapter
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.RadioGroup
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatDialogFragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.congmingpay.android.R
import com.congmingpay.android.model.Brand
import com.congmingpay.android.model.Conn
import com.congmingpay.android.model.Printer
import com.congmingpay.android.model.Source
import com.congmingpay.android.printer.DeviceSearcher
import com.congmingpay.android.printer.FoundDevice
import com.congmingpay.android.service.PrintService
import com.congmingpay.android.util.Permissions
import com.gainscha.sdk2.ConnectType
import com.google.android.material.button.MaterialButton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * 新增打印机三步向导（弹窗）：
 * 步骤1 通道选择；步骤2 网口参数或 USB/蓝牙搜索；步骤3 名称/品牌/纸宽/蜂鸣/切刀。
 */
class AddPrinterDialogFragment : AppCompatDialogFragment() {

    private var step = 0
    /** 0=网口 1=USB 2=蓝牙 */
    private var channel = 0
    private var selectedDevice: FoundDevice? = null
    private var searchJob: Job? = null
    private var networkIp = ""
    private var networkPort = "9100"
    private lateinit var deviceAdapter: DeviceAdapter
    private lateinit var root: View

    override fun onStart() {
        super.onStart()
        dialog?.window?.setLayout(
            WindowManager.LayoutParams.MATCH_PARENT,
            (resources.displayMetrics.heightPixels * 0.9f).toInt()
        )
    }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        return inflater.inflate(R.layout.dialog_add_printer, container, false)
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        root = view

        val brandSpinner = view.findViewById<Spinner>(R.id.spinner_brand)
        brandSpinner.adapter = ArrayAdapter(requireContext(), android.R.layout.simple_spinner_item, Brand.ALL).apply {
            setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item)
        }
        val widthSpinner = view.findViewById<Spinner>(R.id.spinner_width)
        widthSpinner.adapter = ArrayAdapter(requireContext(), android.R.layout.simple_spinner_item, listOf("58mm", "80mm")).apply {
            setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item)
        }
        widthSpinner.setSelection(1)

        deviceAdapter = DeviceAdapter { d -> selectedDevice = d }
        view.findViewById<RecyclerView>(R.id.recycler_devices).apply {
            layoutManager = LinearLayoutManager(requireContext())
            adapter = deviceAdapter
        }

        view.findViewById<RadioGroup>(R.id.rg_channel).setOnCheckedChangeListener { _, checkedId ->
            channel = when (checkedId) {
                R.id.rb_channel_usb -> 1
                R.id.rb_channel_bluetooth -> 2
                else -> 0
            }
            selectedDevice = null
            if (step == 1) showStep2Body()
        }

        view.findViewById<MaterialButton>(R.id.btn_start_search).setOnClickListener { startSearch() }
        view.findViewById<MaterialButton>(R.id.btn_prev).setOnClickListener { prev() }
        view.findViewById<MaterialButton>(R.id.btn_next).setOnClickListener { next() }
        view.findViewById<MaterialButton>(R.id.btn_cancel).setOnClickListener { dismiss() }
        updateStep()
    }

    override fun onDestroyView() {
        searchJob?.cancel()
        super.onDestroyView()
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == Permissions.RC_BLUETOOTH) {
            val granted = grantResults.isNotEmpty() && grantResults.all { it == PackageManager.PERMISSION_GRANTED }
            if (granted) {
                startSearch()
            } else {
                Toast.makeText(requireContext(), "蓝牙权限未授予，无法搜索设备", Toast.LENGTH_LONG).show()
            }
        }
    }

    private fun startSearch() {
        if (channel == 2 && !Permissions.hasBluetooth(requireContext())) {
            Permissions.requestBluetooth(this)
            return
        }
        val container = root.findViewById<LinearLayout>(R.id.ll_search)
        val pb = root.findViewById<ProgressBar>(R.id.pb_search)
        val btn = root.findViewById<MaterialButton>(R.id.btn_start_search)
        btn.isEnabled = false
        pb.visibility = View.VISIBLE
        deviceAdapter.submit(emptyList())
        selectedDevice = null

        val type = if (channel == 1) ConnectType.USB_DEVICE else ConnectType.BLUETOOTH
        searchJob = viewLifecycleOwner.lifecycleScope.launch {
            val devices = withContext(Dispatchers.Default) {
                DeviceSearcher().search(type)
            }
            if (!isActive || !isAdded) return@launch
            btn.isEnabled = true
            pb.visibility = View.GONE
            deviceAdapter.submit(devices)
            container.visibility = View.VISIBLE
            if (devices.isEmpty()) {
                Toast.makeText(requireContext(), "未搜索到设备（请确认已连接/配对）", Toast.LENGTH_LONG).show()
            }
        }
    }

    private fun prev() {
        if (step <= 0) return
        step--
        updateStep()
    }

    private fun next() {
        when (step) {
            0 -> {
                step = 1
                showStep2Body()
                updateStep()
            }
            1 -> {
                if (channel == 0) {
                    val ip = root.findViewById<EditText>(R.id.et_ip).text.toString().trim()
                    if (ip.isEmpty()) {
                        Toast.makeText(requireContext(), "请填写打印机 IP", Toast.LENGTH_SHORT).show()
                        return
                    }
                    networkIp = ip
                    val port = root.findViewById<EditText>(R.id.et_port).text.toString().trim()
                    networkPort = if (port.isEmpty()) "9100" else port
                } else {
                    val d = selectedDevice
                    val expectType = if (channel == 1) "usb" else "bt"
                    if (d == null || d.type != expectType) {
                        Toast.makeText(
                            requireContext(),
                            "请先搜索并选择${if (channel == 1) "USB" else "蓝牙"}设备",
                            Toast.LENGTH_SHORT
                        ).show()
                        return
                    }
                }
                step = 2
                val nameEdit = root.findViewById<EditText>(R.id.et_name)
                if (nameEdit.text.isNullOrBlank()) {
                    nameEdit.setText(defaultName())
                }
                updateStep()
            }
            2 -> finishWizard()
        }
    }

    private fun defaultName(): String = when (channel) {
        1 -> selectedDevice?.displayName ?: "USB 打印机"
        2 -> selectedDevice?.displayName ?: "蓝牙打印机"
        else -> "网口-$networkIp"
    }

    private fun showStep2Body() {
        root.findViewById<LinearLayout>(R.id.ll_network_input).visibility =
            if (channel == 0) View.VISIBLE else View.GONE
        root.findViewById<LinearLayout>(R.id.ll_search).visibility =
            if (channel == 0) View.GONE else View.VISIBLE
    }

    private fun updateStep() {
        root.findViewById<View>(R.id.rg_channel).visibility =
            if (step == 0) View.VISIBLE else View.GONE
        if (step == 1) {
            showStep2Body()
        } else {
            root.findViewById<LinearLayout>(R.id.ll_network_input).visibility = View.GONE
            root.findViewById<LinearLayout>(R.id.ll_search).visibility = View.GONE
        }
        root.findViewById<LinearLayout>(R.id.ll_config).visibility =
            if (step == 2) View.VISIBLE else View.GONE
        root.findViewById<MaterialButton>(R.id.btn_prev).visibility =
            if (step > 0) View.VISIBLE else View.GONE
        root.findViewById<MaterialButton>(R.id.btn_next).text =
            if (step == 2) "完成" else "下一步"
    }

    private fun finishWizard() {
        val svc = PrintService.instance
        if (svc == null) {
            Toast.makeText(requireContext(), "服务未就绪，请稍后重试", Toast.LENGTH_LONG).show()
            return
        }

        val name = root.findViewById<EditText>(R.id.et_name).text.toString().trim()
        if (name.isEmpty()) {
            Toast.makeText(requireContext(), "请填写打印机名称", Toast.LENGTH_SHORT).show()
            return
        }
        val brand = Brand.ALL[root.findViewById<Spinner>(R.id.spinner_brand).selectedItemPosition]
        val width = if (root.findViewById<Spinner>(R.id.spinner_width).selectedItemPosition == 0) 58 else 80
        val buzzer = root.findViewById<androidx.appcompat.widget.SwitchCompat>(R.id.sw_buzzer).isChecked
        val cutEnabled = root.findViewById<androidx.appcompat.widget.SwitchCompat>(R.id.sw_cut).isChecked

        val p = when (channel) {
            1 -> Printer(
                name = name,
                brand = brand,
                width = width,
                conn = Conn.USB,
                usbName = selectedDevice?.identifier ?: "",
                buzzer = buzzer,
                cutDisabled = !cutEnabled,
                source = Source.LOCAL
            )
            2 -> Printer(
                name = name,
                brand = brand,
                width = width,
                conn = Conn.BLUETOOTH,
                bluetoothMac = selectedDevice?.identifier ?: "",
                buzzer = buzzer,
                cutDisabled = !cutEnabled,
                source = Source.LOCAL
            )
            else -> Printer(
                name = name,
                brand = brand,
                width = width,
                conn = Conn.NETWORK,
                ip = networkIp,
                port = networkPort,
                buzzer = buzzer,
                cutDisabled = !cutEnabled,
                source = Source.LOCAL
            )
        }

        svc.addPrinter(p)
        Toast.makeText(requireContext(), "已添加打印机『$name』", Toast.LENGTH_SHORT).show()
        dismiss()
    }

    companion object {
        fun newInstance() = AddPrinterDialogFragment()
    }
}

/** 搜索设备列表适配器（单行选中）。 */
class DeviceAdapter(private val onSelect: (FoundDevice) -> Unit) : RecyclerView.Adapter<DeviceAdapter.Holder>() {

    private val items = mutableListOf<FoundDevice>()
    private var selectedPos = -1

    class Holder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.tv_dev_name)
        val id: TextView = view.findViewById(R.id.tv_dev_id)
    }

    fun submit(list: List<FoundDevice>) {
        items.clear()
        items.addAll(list)
        selectedPos = -1
        notifyDataSetChanged()
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): Holder =
        Holder(LayoutInflater.from(parent.context).inflate(R.layout.item_device, parent, false))

    override fun onBindViewHolder(holder: Holder, position: Int) {
        val d = items[position]
        val prefix = when (d.type) {
            "usb" -> "[USB]"
            "bt" -> "[蓝牙]"
            else -> "[网口]"
        }
        holder.name.text = "$prefix ${d.displayName}"
        holder.id.text = d.identifier

        val selected = position == selectedPos
        holder.itemView.setBackgroundColor(if (selected) SELECT_BG else 0x00000000)
        holder.itemView.setOnClickListener {
            selectedPos = position
            notifyDataSetChanged()
            onSelect(d)
        }
    }

    override fun getItemCount(): Int = items.size

    companion object {
        private val SELECT_BG = android.graphics.Color.argb(0x1A, 0x21, 0x96, 0xF3)
    }
}
