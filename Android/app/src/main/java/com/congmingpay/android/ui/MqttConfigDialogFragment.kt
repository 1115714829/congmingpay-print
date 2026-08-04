package com.congmingpay.android.ui

import android.graphics.Color
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.RadioGroup
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatDialogFragment
import androidx.lifecycle.lifecycleScope
import com.congmingpay.android.R
import com.congmingpay.android.model.AliyunMqtt
import com.congmingpay.android.model.IotMqtt
import com.congmingpay.android.model.MqttConfig
import com.congmingpay.android.model.MqttProvider
import com.congmingpay.android.mqtt.Resolve
import com.congmingpay.android.service.PrintService
import com.google.android.material.button.MaterialButton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * MQTT 三 Provider 配置弹窗（校验 + 连接测试 + 保存）。
 * 启用开关在设置页；本弹窗写完整连接参数并保留当前 enabled。
 */
class MqttConfigDialogFragment : AppCompatDialogFragment() {

    override fun onStart() {
        super.onStart()
        dialog?.window?.setLayout(
            WindowManager.LayoutParams.MATCH_PARENT,
            WindowManager.LayoutParams.WRAP_CONTENT
        )
    }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        return inflater.inflate(R.layout.dialog_mqtt_config, container, false)
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        view.findViewById<RadioGroup>(R.id.rg_provider).setOnCheckedChangeListener { _, _ -> showProviderFields(view) }
        view.findViewById<MaterialButton>(R.id.btn_test_conn).setOnClickListener { testConnection(view) }
        view.findViewById<MaterialButton>(R.id.btn_save_mqtt).setOnClickListener { save(view) }
        load(view)
    }

    private fun load(view: View) {
        val mqtt = PrintService.instance?.configManager()?.settings()?.mqtt ?: return
        when (mqtt.effectiveProvider()) {
            MqttProvider.ALIYUN -> view.findViewById<RadioGroup>(R.id.rg_provider).check(R.id.rb_provider_aliyun)
            MqttProvider.IOT -> view.findViewById<RadioGroup>(R.id.rg_provider).check(R.id.rb_provider_iot)
            else -> view.findViewById<RadioGroup>(R.id.rg_provider).check(R.id.rb_provider_generic)
        }
        showProviderFields(view)
        et(view, R.id.et_broker).setText(mqtt.broker)
        et(view, R.id.et_mqtt_port).setText(mqtt.port.toString())
        et(view, R.id.et_username).setText(mqtt.username)
        et(view, R.id.et_password).setText(mqtt.password)
        et(view, R.id.et_topic).setText(mqtt.topic)
        et(view, R.id.et_report_topic).setText(mqtt.reportTopic)
        val a = mqtt.aliyun
        et(view, R.id.et_ali_endpoint).setText(a.endpoint)
        et(view, R.id.et_ali_port).setText(a.port.toString())
        et(view, R.id.et_instance_id).setText(a.instanceId)
        et(view, R.id.et_access_key).setText(a.accessKey)
        et(view, R.id.et_secret_key).setText(a.secretKey)
        et(view, R.id.et_group_id).setText(a.groupId)
        et(view, R.id.et_parent_topic).setText(a.parentTopic)
        et(view, R.id.et_device_id).setText(a.deviceId)
        et(view, R.id.et_down_suffix).setText(a.downSuffix)
        et(view, R.id.et_up_suffix).setText(a.upSuffix)
        val i = mqtt.iot
        et(view, R.id.et_iot_pk).setText(i.productKey)
        et(view, R.id.et_iot_dn).setText(i.deviceName)
        et(view, R.id.et_iot_ds).setText(i.deviceSecret)
        et(view, R.id.et_iot_region).setText(i.regionId)
        et(view, R.id.et_iot_endpoint).setText(i.endpoint)
        et(view, R.id.et_iot_port).setText(i.port.toString())
        et(view, R.id.et_iot_down).setText(i.downSuffix)
        et(view, R.id.et_iot_up).setText(i.upSuffix)
    }

    private fun showProviderFields(view: View) {
        val sel = provider(view)
        view.findViewById<LinearLayout>(R.id.ll_generic).visibility =
            if (sel == MqttProvider.GENERIC) View.VISIBLE else View.GONE
        view.findViewById<LinearLayout>(R.id.ll_aliyun).visibility =
            if (sel == MqttProvider.ALIYUN) View.VISIBLE else View.GONE
        view.findViewById<LinearLayout>(R.id.ll_iot).visibility =
            if (sel == MqttProvider.IOT) View.VISIBLE else View.GONE
    }

    private fun provider(view: View): String = when (view.findViewById<RadioGroup>(R.id.rg_provider).checkedRadioButtonId) {
        R.id.rb_provider_aliyun -> MqttProvider.ALIYUN
        R.id.rb_provider_iot -> MqttProvider.IOT
        else -> MqttProvider.GENERIC
    }

    private fun collect(view: View, enabled: Boolean): MqttConfig {
        val p = provider(view)
        val cfg = MqttConfig(enabled = enabled, provider = p)
        cfg.broker = et(view, R.id.et_broker).text.toString().trim()
        cfg.port = et(view, R.id.et_mqtt_port).text.toString().toIntOrNull() ?: 1883
        cfg.username = et(view, R.id.et_username).text.toString()
        cfg.password = et(view, R.id.et_password).text.toString()
        cfg.topic = et(view, R.id.et_topic).text.toString().trim()
        cfg.reportTopic = et(view, R.id.et_report_topic).text.toString().trim()
        cfg.aliyun = AliyunMqtt(
            endpoint = et(view, R.id.et_ali_endpoint).text.toString().trim(),
            port = et(view, R.id.et_ali_port).text.toString().toIntOrNull() ?: 1883,
            instanceId = et(view, R.id.et_instance_id).text.toString().trim(),
            accessKey = et(view, R.id.et_access_key).text.toString().trim(),
            secretKey = et(view, R.id.et_secret_key).text.toString().trim(),
            groupId = et(view, R.id.et_group_id).text.toString().trim(),
            parentTopic = et(view, R.id.et_parent_topic).text.toString().trim(),
            deviceId = et(view, R.id.et_device_id).text.toString().trim(),
            downSuffix = et(view, R.id.et_down_suffix).text.toString().trim(),
            upSuffix = et(view, R.id.et_up_suffix).text.toString().trim()
        )
        cfg.iot = IotMqtt(
            productKey = et(view, R.id.et_iot_pk).text.toString().trim(),
            deviceName = et(view, R.id.et_iot_dn).text.toString().trim(),
            deviceSecret = et(view, R.id.et_iot_ds).text.toString().trim(),
            regionId = et(view, R.id.et_iot_region).text.toString().trim().ifEmpty { "cn-shanghai" },
            endpoint = et(view, R.id.et_iot_endpoint).text.toString().trim(),
            port = et(view, R.id.et_iot_port).text.toString().toIntOrNull() ?: 1883,
            downSuffix = et(view, R.id.et_iot_down).text.toString().trim(),
            upSuffix = et(view, R.id.et_iot_up).text.toString().trim()
        )
        return cfg
    }

    fun validateMqtt(cfg: MqttConfig): String? {
        if (!cfg.enabled) return null
        val miss = Resolve.missing(cfg)
        if (miss.isNotEmpty()) return "缺少 ${miss.joinToString("/")}"
        val loop = when (cfg.effectiveProvider()) {
            MqttProvider.GENERIC -> cfg.topic.isNotEmpty() && cfg.topic == cfg.reportTopic
            MqttProvider.ALIYUN -> cfg.aliyun.downSuffix.isNotEmpty() && cfg.aliyun.downSuffix == cfg.aliyun.upSuffix
            MqttProvider.IOT -> cfg.iot.downSuffix.isNotEmpty() && cfg.iot.downSuffix == cfg.iot.upSuffix
            else -> false
        }
        if (loop) return "上报主题与订阅主题相同(自订阅回环风险)"
        return null
    }

    private fun save(view: View) {
        val svc = PrintService.instance ?: return
        val enabled = svc.configManager()?.settings()?.mqtt?.enabled ?: false
        val mqtt = collect(view, enabled)
        val err = validateMqtt(mqtt.copy(enabled = true))
        if (err != null) {
            Toast.makeText(requireContext(), "保存失败: $err", Toast.LENGTH_LONG).show()
            return
        }
        svc.saveSettings { s ->
            mqtt.enabled = s.mqtt.enabled
            s.mqtt = mqtt
        }
        Toast.makeText(requireContext(), "MQTT 已保存", Toast.LENGTH_SHORT).show()
        dismiss()
        (parentFragment as? SettingsFragment)?.onMqttConfigSaved()
    }

    private fun testConnection(view: View) {
        val svc = PrintService.instance ?: return
        val mc = svc.mqttClient()
        val status = view.findViewById<TextView>(R.id.tv_mqtt_status)
        if (mc == null) {
            status.text = "服务未就绪"
            status.setTextColor(Color.parseColor("#D32F2F"))
            return
        }
        val mqtt = collect(view, true)
        val err = validateMqtt(mqtt)
        if (err != null) {
            status.text = "配置不完整: $err"
            status.setTextColor(Color.parseColor("#D32F2F"))
            return
        }
        status.text = "测试中…"
        status.setTextColor(Color.parseColor("#666666"))
        viewLifecycleOwner.lifecycleScope.launch {
            val res = withContext(Dispatchers.IO) { mc.testConnect(mqtt) }
            if (!isAdded) return@launch
            if (res == null) {
                status.text = "连接成功"
                status.setTextColor(Color.parseColor("#388E3C"))
            } else {
                status.text = "连接失败: $res"
                status.setTextColor(Color.parseColor("#D32F2F"))
            }
        }
    }

    private fun et(view: View, id: Int): EditText = view.findViewById(id)

    companion object {
        fun newInstance() = MqttConfigDialogFragment()
    }
}
