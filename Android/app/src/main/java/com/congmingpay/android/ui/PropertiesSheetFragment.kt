package com.congmingpay.android.ui

import android.app.Dialog
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ArrayAdapter
import android.widget.EditText
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.widget.SwitchCompat
import com.congmingpay.android.R
import com.congmingpay.android.model.Brand
import com.congmingpay.android.service.PrintService
import com.google.android.material.bottomsheet.BottomSheetDialog
import com.google.android.material.bottomsheet.BottomSheetDialogFragment
import com.google.android.material.button.MaterialButton

/**
 * 打印机属性 BottomSheet：名称/品牌/纸宽可编辑；连接信息只读；蜂鸣/首尾空行/切刀。
 */
class PropertiesSheetFragment : BottomSheetDialogFragment() {

    private var printerId: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        printerId = arguments?.getString(ARG_ID)
    }

    override fun onCreateDialog(savedInstanceState: Bundle?): Dialog {
        return BottomSheetDialog(requireContext(), theme)
    }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        return inflater.inflate(R.layout.dialog_properties, container, false)
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        val brandSpinner = view.findViewById<Spinner>(R.id.spinner_brand)
        brandSpinner.adapter = ArrayAdapter(requireContext(), android.R.layout.simple_spinner_item, Brand.ALL).apply {
            setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item)
        }
        val widthSpinner = view.findViewById<Spinner>(R.id.spinner_width)
        widthSpinner.adapter = ArrayAdapter(requireContext(), android.R.layout.simple_spinner_item, listOf("58mm", "80mm")).apply {
            setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item)
        }
        view.findViewById<MaterialButton>(R.id.btn_save).setOnClickListener { save(view) }
        load(view)
    }

    private fun load(view: View) {
        val id = printerId ?: return
        val p = PrintService.instance?.configManager()?.findPrinter(id) ?: return
        view.findViewById<TextView>(R.id.tv_props_id).text = "ID: ${p.id}"
        view.findViewById<EditText>(R.id.et_name).setText(p.name)
        view.findViewById<Spinner>(R.id.spinner_brand).setSelection(Brand.indexOf(p.brand))
        view.findViewById<Spinner>(R.id.spinner_width).setSelection(if (p.width == 58) 0 else 1)
        view.findViewById<TextView>(R.id.tv_conn_info).text = buildString {
            append("连接: ${p.connLabel()}\n")
            append("地址: ${p.address()}\n")
            append("来源: ${p.sourceLabel()}\n")
            append("上次打印: ${if (p.lastPrint.isNotEmpty()) p.lastPrint else "未打印"}")
        }
        view.findViewById<SwitchCompat>(R.id.sw_buzzer).isChecked = p.buzzer
        view.findViewById<EditText>(R.id.et_head).setText(p.headLines.toString())
        view.findViewById<EditText>(R.id.et_tail).setText(p.tailLines.toString())
        view.findViewById<SwitchCompat>(R.id.sw_cut).isChecked = p.cuts()
    }

    private fun save(view: View) {
        val id = printerId ?: return
        val svc = PrintService.instance ?: return
        val name = view.findViewById<EditText>(R.id.et_name).text.toString().trim()
        if (name.isEmpty()) {
            Toast.makeText(requireContext(), "名称不能为空", Toast.LENGTH_SHORT).show()
            return
        }
        val head = view.findViewById<EditText>(R.id.et_head).text.toString().toIntOrNull() ?: 0
        if (head < 0 || head > 100) {
            Toast.makeText(requireContext(), "首部空行需在 0-100 之间", Toast.LENGTH_SHORT).show()
            return
        }
        val tail = view.findViewById<EditText>(R.id.et_tail).text.toString().toIntOrNull() ?: 0
        if (tail < -50 || tail > 100) {
            Toast.makeText(requireContext(), "尾部偏移需在 -50 到 100 之间", Toast.LENGTH_SHORT).show()
            return
        }
        val brand = Brand.ALL[view.findViewById<Spinner>(R.id.spinner_brand).selectedItemPosition]
        val width = if (view.findViewById<Spinner>(R.id.spinner_width).selectedItemPosition == 0) 58 else 80
        val buzzer = view.findViewById<SwitchCompat>(R.id.sw_buzzer).isChecked
        val cut = view.findViewById<SwitchCompat>(R.id.sw_cut).isChecked
        svc.updatePrinter(id) { p ->
            p.name = name
            p.brand = brand
            p.width = width
            p.buzzer = buzzer
            p.headLines = head
            p.tailLines = tail
            p.cutDisabled = !cut
        }
        Toast.makeText(requireContext(), "已保存", Toast.LENGTH_SHORT).show()
        dismiss()
    }

    companion object {
        private const val ARG_ID = "printerId"
        fun newInstance(printerId: String) = PropertiesSheetFragment().apply {
            arguments = Bundle().apply { putString(ARG_ID, printerId) }
        }
    }
}
