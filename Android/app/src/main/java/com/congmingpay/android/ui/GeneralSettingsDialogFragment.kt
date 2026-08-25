package com.congmingpay.android.ui

import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatDialogFragment
import androidx.appcompat.widget.SwitchCompat
import com.congmingpay.android.R
import com.congmingpay.android.model.clampJobHistoryDays
import com.congmingpay.android.service.PrintService
import com.congmingpay.android.util.Permissions
import com.google.android.material.button.MaterialButton

/** 通用设置弹窗：服务名 / 通知 / 云盒兼容 / 开机自启 / 悬浮窗权限 / 历史天数。 */
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
        view.findViewById<MaterialButton>(R.id.btn_overlay_perm).setOnClickListener {
            Permissions.requestOverlayPermission(requireContext())
            Toast.makeText(requireContext(), "请在系统设置中允许「显示在其他应用上层」", Toast.LENGTH_LONG).show()
        }
    }

    override fun onResume() {
        super.onResume()
        view?.let { refreshOverlayStatus(it) }
        // 从系统设置返回后补挂告警球
        PrintService.instance?.refreshAlertOverlay()
    }

    private fun load(view: View) {
        val s = PrintService.instance?.configManager()?.settings() ?: return
        view.findViewById<EditText>(R.id.et_service_name).setText(s.serviceName)
        view.findViewById<SwitchCompat>(R.id.sw_notify).isChecked = !s.notifyDisabled
        view.findViewById<SwitchCompat>(R.id.sw_yunhe).isChecked = !s.yunheCompatDisabled
        view.findViewById<SwitchCompat>(R.id.sw_boot_start).isChecked = s.bootStartEnabled
        view.findViewById<EditText>(R.id.et_history_days).setText(s.jobHistoryDays.toString())
        refreshOverlayStatus(view)
    }

    private fun refreshOverlayStatus(view: View) {
        val ok = Permissions.canDrawOverlays(requireContext())
        view.findViewById<TextView>(R.id.tv_overlay_status).text =
            if (ok) "悬浮窗权限：已授予（告警球可用）"
            else "悬浮窗权限：未授予（告警球不可用，故障时仅系统通知）"
        view.findViewById<MaterialButton>(R.id.btn_overlay_perm).isEnabled = !ok
    }

    private fun save(view: View) {
        val svc = PrintService.instance ?: return
        val name = view.findViewById<EditText>(R.id.et_service_name).text.toString().trim()
        val days = view.findViewById<EditText>(R.id.et_history_days).text.toString().toIntOrNull() ?: 7
        val notifyOn = view.findViewById<SwitchCompat>(R.id.sw_notify).isChecked
        val yunheOn = view.findViewById<SwitchCompat>(R.id.sw_yunhe).isChecked
        val bootOn = view.findViewById<SwitchCompat>(R.id.sw_boot_start).isChecked

        svc.saveSettings { s ->
            s.serviceName = name
            s.notifyDisabled = !notifyOn
            s.yunheCompatDisabled = !yunheOn
            s.bootStartEnabled = bootOn
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
