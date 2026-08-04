package com.congmingpay.android.ui

import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.EditText
import android.widget.Toast
import androidx.appcompat.app.AppCompatDialogFragment
import androidx.appcompat.widget.SwitchCompat
import com.congmingpay.android.R
import com.congmingpay.android.model.clampJobHistoryDays
import com.congmingpay.android.service.PrintService
import com.google.android.material.button.MaterialButton

/** 通用设置弹窗：服务名 / 通知 / 云盒兼容 / 历史天数。 */
class GeneralSettingsDialogFragment : AppCompatDialogFragment() {

    override fun onStart() {
        super.onStart()
        dialog?.window?.setLayout(
            WindowManager.LayoutParams.MATCH_PARENT,
            WindowManager.LayoutParams.WRAP_CONTENT
        )
    }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        return inflater.inflate(R.layout.dialog_general_settings, container, false)
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        load(view)
        view.findViewById<MaterialButton>(R.id.btn_cancel).setOnClickListener { dismiss() }
        view.findViewById<MaterialButton>(R.id.btn_save).setOnClickListener { save(view) }
    }

    private fun load(view: View) {
        val s = PrintService.instance?.configManager()?.settings() ?: return
        view.findViewById<EditText>(R.id.et_service_name).setText(s.serviceName)
        view.findViewById<SwitchCompat>(R.id.sw_notify).isChecked = !s.notifyDisabled
        view.findViewById<SwitchCompat>(R.id.sw_yunhe).isChecked = !s.yunheCompatDisabled
        view.findViewById<EditText>(R.id.et_history_days).setText(s.jobHistoryDays.toString())
    }

    private fun save(view: View) {
        val svc = PrintService.instance ?: return
        val name = view.findViewById<EditText>(R.id.et_service_name).text.toString().trim()
        val days = view.findViewById<EditText>(R.id.et_history_days).text.toString().toIntOrNull() ?: 7
        val notifyOn = view.findViewById<SwitchCompat>(R.id.sw_notify).isChecked
        val yunheOn = view.findViewById<SwitchCompat>(R.id.sw_yunhe).isChecked

        svc.saveSettings { s ->
            s.serviceName = name
            s.notifyDisabled = !notifyOn
            s.yunheCompatDisabled = !yunheOn
            s.jobHistoryDays = clampJobHistoryDays(days)
        }
        Toast.makeText(requireContext(), "已保存", Toast.LENGTH_SHORT).show()
        (parentFragment as? SettingsFragment)?.onGeneralSettingsSaved()
        dismiss()
    }

    companion object {
        fun newInstance() = GeneralSettingsDialogFragment()
    }
}
