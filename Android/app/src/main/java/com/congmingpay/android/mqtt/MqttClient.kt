package com.congmingpay.android.mqtt

import com.congmingpay.android.api.PrintRequestParser
import com.congmingpay.android.api.PrintProcessor
import com.congmingpay.android.config.ConfigManager
import com.congmingpay.android.errcode.ErrCode
import com.congmingpay.android.errcode.errorCode
import com.congmingpay.android.logger.Logger
import com.congmingpay.android.printer.JobEvent
import com.congmingpay.android.printer.PrintDispatcher
import com.google.gson.Gson
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import org.eclipse.paho.client.mqttv3.IMqttDeliveryToken
import org.eclipse.paho.client.mqttv3.MqttAsyncClient
import org.eclipse.paho.client.mqttv3.MqttCallbackExtended
import org.eclipse.paho.client.mqttv3.MqttConnectOptions
import org.eclipse.paho.client.mqttv3.MqttMessage
import org.eclipse.paho.client.mqttv3.persist.MemoryPersistence
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/**
 * MQTT 客户端（paho Java，MQTT 3.1.1）。
 *
 * 翻译自 Windows 端 mqtt/Client.go + Report.go。
 *
 * 订阅短商户号收打印 → parseRequests → process → 回执。
 * 发布到上报主题：report（打印回执）+ state（定时全量快照 + 查询 + LWT 离线）。
 */
class MqttClient(
    private val cfg: ConfigManager,
    private val svc: PrintDispatcher,
    private val onChange: () -> Unit,
    private val onStatus: (connected: Boolean, error: String?) -> Unit
) {
    private val gson = Gson()
    private val executor = Executors.newSingleThreadExecutor()

    @Volatile private var client: MqttAsyncClient? = null
    @Volatile private var enabled = false
    @Volatile private var resolved: Resolve.Resolved? = null
    @Volatile private var connected = false

    private var stateScheduler: java.util.concurrent.ScheduledExecutorService? = null

    /** 启动/重载 */
    fun start() {
        reload(cfg.settings().mqtt)
    }

    /** 未启用/已停用 中性态判断（UI 状态标签，对照 Windows 端 Active()） */
    fun active(): Boolean = enabled && client != null

    /** 当前是否已连接（仅 active 时有意义） */
    fun isConnected(): Boolean = connected

    /**
     * 连接测试（阻塞，须在后台线程调用）。
     * 解析 + 同步连接 + 断开；成功返回 null，失败返回错误文案。
     * 注意：测试与主连接同 ClientID，测试时 broker 会替换主连接、靠自动重连恢复（与 Windows 端一致）。
     */
    fun testConnect(cfg2: com.congmingpay.android.model.MqttConfig): String? {
        if (!cfg2.enabled) return "未启用"
        val r = try {
            Resolve.resolve(cfg2)
        } catch (t: Throwable) {
            return t.message ?: "解析配置失败"
        } ?: return "配置不完整（当前 Provider 缺必填项）"
        if (r.publishTopic == r.subscribeTopic) return "上报主题与订阅主题相同(自订阅回环风险)"
        return try {
            val testCli = org.eclipse.paho.client.mqttv3.MqttClient(r.brokerUrl, r.clientId, MemoryPersistence())
            val opts = MqttConnectOptions().apply {
                isCleanSession = true
                connectionTimeout = 10
                userName = r.username
                password = r.password.toCharArray()
            }
            testCli.connect(opts)
            testCli.disconnect(1000)
            testCli.close()
            null
        } catch (e: Exception) {
            e.message ?: "连接失败"
        }
    }

    fun reload(cfg2: com.congmingpay.android.model.MqttConfig) {
        close()
        enabled = cfg2.enabled
        if (!enabled) return

        val r = try {
            Resolve.resolve(cfg2)
        } catch (t: Throwable) {
            Logger.error("MQTT 解析配置失败: ${t.message}")
            return
        }
        if (r == null) {
            Logger.error("MQTT 启用但配置不完整(缺 broker/短商户号/上报主题)，只停不连")
            return
        }
        resolved = r

        // 自订阅回环守卫
        if (r.publishTopic == r.subscribeTopic) {
            Logger.error("MQTT 上报主题 == 订阅主题(自订阅回环风险)，拒连")
            return
        }

        executor.execute { connect(r) }
    }

    fun close() {
        enabled = false
        sendOffline()
        stateScheduler?.shutdownNow()
        stateScheduler = null
        try {
            client?.disconnect(1000)
        } catch (_: Exception) {}
        client = null
        connected = false
    }

    private fun connect(r: Resolve.Resolved) {
        // 代际守卫：close()/reload() 后排队中的旧连接任务直接放弃（防僵尸连接/顶号）
        if (!enabled) return
        if (client != null) return
        try {
            val mqtt = MqttAsyncClient(r.brokerUrl, r.clientId, MemoryPersistence())
            client = mqtt

            val opts = MqttConnectOptions().apply {
                isAutomaticReconnect = true
                isCleanSession = true
                connectionTimeout = 10
                keepAliveInterval = 30
                userName = r.username
                password = r.password.toCharArray()
                // LWT
                setWill(r.publishTopic, buildStateOffline(r.merchant), 1, false)
            }

            mqtt.setCallback(object : MqttCallbackExtended {
                override fun connectComplete(reconnect: Boolean, serverURI: String?) {
                    // 代际守卫：close()/reload() 后迟到的连接完成直接放弃（防僵尸连接/顶号/泄漏 stateLoop）
                    if (!enabled) return
                    Logger.info("MQTT 已连接: $serverURI (reconnect=$reconnect)")
                    if (!reconnect) {
                        try {
                            mqtt.subscribe(r.subscribeTopic, 1)
                            Logger.info("MQTT 已订阅: ${r.subscribeTopic}")
                        } catch (e: Exception) {
                            Logger.error("MQTT 订阅失败: ${e.message}")
                        }
                    }
                    connected = true
                    onStatus(true, null)
                    // 上线首拍
                    publishState(r)
                    startStateLoop(r)
                }

                override fun connectionLost(cause: Throwable?) {
                    Logger.warn("MQTT 连接断开: ${cause?.message}")
                    connected = false
                    onStatus(false, cause?.message)
                }

                override fun messageArrived(topic: String?, message: MqttMessage?) {
                    handleIncoming(topic, message?.payload ?: ByteArray(0), r)
                }

                override fun deliveryComplete(token: IMqttDeliveryToken?) {}
            })

            mqtt.connect(opts)
        } catch (e: Exception) {
            Logger.error("MQTT 连接失败: ${e.message}")
            onStatus(false, e.message)
        }
    }

    /** 处理下行消息 */
    private fun handleIncoming(topic: String?, payload: ByteArray, r: Resolve.Resolved) {
        try {
            val text = payload.toString(Charsets.UTF_8)

            // 查询消息
            val parsed = JsonParser.parseString(text)
            if (parsed.isJsonObject) {
                val obj = parsed.asJsonObject
                if (obj.has("query") && obj.get("query").isJsonPrimitive) {
                    val q = obj.get("query").asString
                    if (q == "printers") {
                        publishState(r)
                        Logger.info("MQTT 收到查询 printers → 回发全量 state")
                    } else {
                        Logger.warn("MQTT 未知 query: $q")
                        // 对齐 Windows Client.go
                        publishReportFailed(
                            r, 0L, ErrCode.UNSUPPORTED_QUERY,
                            """不支持的 query: $q(支持 "printers")"""
                        )
                    }
                    return
                }
            }

            // 打印消息
            val (requests, _, err) = PrintRequestParser.parse(payload)
            if (err != null) {
                Logger.error("MQTT 解析打印请求失败: ${err.message}")
                // 对齐 Windows: Message: "解析失败: " + err.Error()
                publishReportFailed(
                    r, 0L, ErrCode.PARSE_FAILED,
                    "解析失败: " + (err.message ?: "")
                )
                return
            }

            for (req in requests) {
                try {
                    val (result, changed) = PrintProcessor.process(cfg, svc, req)
                    if (changed) {
                        cfg.save()
                        onChange()
                    }
                    // accepted 回执
                    publishReportAccepted(r, req.id, result)
                    Logger.info("MQTT 打印请求已受理: id=${req.id} 任务#${result.jobNo}")
                } catch (e: Exception) {
                    val code = errorCode(e)
                    Logger.error("MQTT 处理打印请求失败: id=${req.id} code=$code ${e.message}")
                    publishReportFailed(r, req.id, code, e.message ?: "")
                }
            }
        } catch (e: Exception) {
            Logger.error("MQTT 消息处理异常: ${e.message}")
        }
    }

    // —— 状态上报 ——

    private fun startStateLoop(r: Resolve.Resolved) {
        stateScheduler?.shutdownNow()
        stateScheduler = Executors.newSingleThreadScheduledExecutor()
        stateScheduler?.scheduleAtFixedRate({
            if (enabled && connected) publishState(r)
        }, 60, 60, TimeUnit.SECONDS)
    }

    fun publishState(r: Resolve.Resolved? = resolved) {
        if (r == null || !connected) return
        if (r !== resolved) return  // 代际校验：reload 已换配置，在途发布放弃
        val cli = client ?: return
        try {
            val msg = buildStateMessage(r.merchant)
            cli.publish(r.publishTopic, msg, 1, false, null, null)
        } catch (e: Exception) {
            Logger.error("MQTT 发布 state 失败: ${e.message}")
        }
    }

    private fun buildStateMessage(merchant: String): ByteArray {
        val obj = stateJson(
            merchant,
            cfg.printerSnapshots(),
            svc.onlineSnapshot(),
            System.currentTimeMillis()
        )
        return gson.toJson(obj).toByteArray(Charsets.UTF_8)
    }

    /** 简版离线：无 printers、无 ts（对齐 Windows LWT / sendOffline） */
    private fun buildStateOffline(merchant: String): ByteArray =
        gson.toJson(stateOfflineJson(merchant)).toByteArray(Charsets.UTF_8)

    private fun sendOffline() {
        val r = resolved ?: return
        if (!connected) return
        try {
            val msg = buildStateOffline(r.merchant)
            client?.publish(r.publishTopic, MqttMessage(msg), null, null)
        } catch (_: Exception) {}
    }

    // —— 打印回执 ——

    private fun publishReportAccepted(r: Resolve.Resolved, id: Long, result: com.congmingpay.android.api.ProcessResult) {
        val compat = cfg.settings().yunheCompat()
        if (compat) return  // C4: 兼容开时不发 accepted
        publishReport(r, reportAcceptedJson(r.merchant, id, result, System.currentTimeMillis()))
    }

    private fun publishReportFailed(r: Resolve.Resolved, id: Long, code: Int, message: String) {
        publishReport(r, reportFailedJson(r.merchant, id, code, message, System.currentTimeMillis()))
    }

    /** 由 PrintDispatcher 任务事件驱动 */
    fun publishJobEvent(ev: JobEvent) {
        val r = resolved ?: return
        if (ev.cloudId == null) return  // 本地任务不上报
        val compat = cfg.settings().yunheCompat()

        val event = when (ev.event) {
            "done" -> "done"
            "failed" -> "failed"
            "waiting" -> if (!compat) "waiting" else return  // C4: 兼容开时不发 waiting
            else -> return
        }
        publishReport(r, reportJobEventJson(r.merchant, event, ev, System.currentTimeMillis()))
    }

    private fun publishReport(r: Resolve.Resolved, obj: JsonObject) {
        if (!connected) return
        if (r !== resolved) return  // 代际校验：reload 已换配置，在途发布放弃
        try {
            val msg = gson.toJson(obj).toByteArray(Charsets.UTF_8)
            client?.publish(r.publishTopic, MqttMessage(msg), null, null)
        } catch (e: Exception) {
            Logger.error("MQTT 发布 report 失败: ${e.message}")
        }
    }
}