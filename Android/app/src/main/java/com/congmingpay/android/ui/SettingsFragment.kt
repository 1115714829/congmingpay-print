package com.congmingpay.android.ui

import android.graphics.Color
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.widget.SwitchCompat
import androidx.fragment.app.Fragment
import com.congmingpay.android.R
import com.congmingpay.android.model.MqttProvider
import com.congmingpay.android.mqtt.Resolve
import com.congmingpay.android.service.PrintService
import com.congmingpay.android.service.UiListener
import com.google.android.material.button.MaterialButton

/**
 * 系统设置摘要页：只读摘要 + 弹窗编辑入口（通用 / MQTT）。
 */
class SettingsFragment : Fragment(), UiListener {

    private val attachHandler = Handler(Looper.getMainLooper())
    private val attachRetry = Runnable { if (isResumed) attachAndRefresh() }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        val view = inflater.inflate(R.layout.fragment_settings, container, false)
        view.findViewById<MaterialButton>(R.id.btn_edit_general).setOnClickListener {
            GeneralSettingsDialogFragment.newInstance().show(childFragmentManager, "general_settings")
        }
        view.findViewById<MaterialButton>(R.id.btn_edit_mqtt).setOnClickListener {
            MqttConfigDialogFragment.newInstance().show(childFragmentManager, "mqtt_config")
        }
        view.findViewById<SwitchCompat>(R.id.sw_mqtt_enabled).setOnCheckedChangeListener { _, checked ->
            if (!view.isAttachedToWindow) return@setOnCheckedChangeListener
            toggleMqtt(checked)
        }
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

    override fun onMqttStatus(connected: Boolean, error: String?) {
        if (isAdded) updateMqttStatus(connected, error)
    }

    fun onMqttConfigSaved() {
        if (isAdded) loadSummary()
    }

    fun onGeneralSettingsSaved() {
        if (isAdded) loadSummary()
    }

    private fun attachAndRefresh() {
        val svc = PrintService.instance
        if (svc == null) {
            attachHandler.postDelayed(attachRetry, 150)
            return
        }
        svc.addUiListener(this)
        loadSummary()
    }

    private fun loadSummary() {
        if (!isAdded) return
        val svc = PrintService.instance ?: return
        val s = svc.configManager()?.settings() ?: return
        val mqtt = s.mqtt

        view?.findViewById<TextView>(R.id.tv_general_summary)?.text = buildString {
            append("服务名：${s.serviceName.ifEmpty { "票据打印服务" }}\n")
            append("系统通知：${if (!s.notifyDisabled) "开" else "关"} · ")
            append("云盒兼容：${if (!s.yunheCompatDisabled) "开" else "关"}\n")
            append("打印历史保留：${s.jobHistoryDays} 天")
        }

        val sw = view?.findViewById<SwitchCompat>(R.id.sw_mqtt_enabled) ?: return
        sw.setOnCheckedChangeListener(null)
        sw.isChecked = mqtt.enabled
        sw.setOnCheckedChangeListener { _, checked ->
            if (view?.isAttachedToWindow != true) return@setOnCheckedChangeListener
            toggleMqtt(checked)
        }

        view?.findViewById<TextView>(R.id.tv_mqtt_summary)?.text =
            mqttSummary(mqtt.effectiveProvider(), mqtt)
        updateMqttStatus(svc.mqttClient()?.isConnected() ?: false, null)
    }

    private fun mqttSummary(provider: String, mqtt: com.congmingpay.android.model.MqttConfig): String {
        return when (provider) {
            MqttProvider.ALIYUN -> "云消息队列 · ${mqtt.aliyun.deviceId.ifEmpty { "未填自定义ID" }}"
            MqttProvider.IOT -> "物联网 · ${mqtt.iot.deviceName.ifEmpty { "未填 DeviceName" }}"
            else -> "自建 · ${mqtt.broker.ifEmpty { "未填 broker" }} / ${mqtt.topic.ifEmpty { "未填商户号" }}"
        }
    }

    private fun toggleMqtt(enabled: Boolean) {
        val svc = PrintService.instance ?: return
        if (enabled) {
            val cur = svc.configManager()?.settings()?.mqtt
            if (cur != null) {
                val miss = Resolve.missing(cur.copy(enabled = true))
                if (miss.isNotEmpty()) {
                    Toast.makeText(requireContext(), "请先完善 MQTT：缺少 ${miss.joinToString("/")}", Toast.LENGTH_LONG).show()
                    view?.findViewById<SwitchCompat>(R.id.sw_mqtt_enabled)?.let { sw ->
                        sw.setOnCheckedChangeListener(null)
                        sw.isChecked = false
                        sw.setOnCheckedChangeListener { _, checked ->
                            if (view?.isAttachedToWindow != true) return@setOnCheckedChangeListener
                            toggleMqtt(checked)
                        }
                    }
                    MqttConfigDialogFragment.newInstance().show(childFragmentManager, "mqtt_config")
                    return
                }
            }
        }
        svc.saveSettings { s -> s.mqtt.enabled = enabled }
        loadSummary()
    }

    private fun updateMqttStatus(connected: Boolean, error: String?) {
        if (!isAdded) return
        val status = view?.findViewById<TextView>(R.id.tv_mqtt_status) ?: return
        val mqtt = PrintService.instance?.mqttClient()
        if (mqtt == null || !mqtt.active()) {
            status.text = "状态: 未启用/已停用"
            status.setTextColor(Color.parseColor("#666666"))
        } else if (connected) {
            status.text = "状态: 已连接"
            status.setTextColor(Color.parseColor("#388E3C"))
        } else {
            status.text = "状态: 连接断开 (${error ?: "未知原因"})"
            status.setTextColor(Color.parseColor("#D32F2F"))
        }
    }
}
