package com.congmingpay.android.mqtt

import android.util.Base64
import com.congmingpay.android.model.MqttConfig
import com.congmingpay.android.model.MqttProvider
import java.nio.charset.StandardCharsets
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/**
 * 三 Provider 参数解析（对齐 Windows 端 mqtt/Resolve.go + Iot.go）。
 *
 * 产出统一的连接参数：broker URL、clientId、username、password、订阅主题、发布主题。
 * Base64 用 android.util.Base64（minSdk 21 可用；勿用 java.util.Base64=API26+）。
 */
object Resolve {

    data class Resolved(
        val brokerUrl: String,
        val clientId: String,
        val username: String,
        val password: String,
        val subscribeTopic: String,
        val publishTopic: String,
        val merchant: String
    )

    fun resolve(cfg: MqttConfig): Resolved? {
        if (!cfg.enabled) return null
        return when (cfg.effectiveProvider()) {
            MqttProvider.GENERIC -> resolveGeneric(cfg)
            MqttProvider.ALIYUN -> resolveAliyun(cfg)
            MqttProvider.IOT -> resolveIot(cfg)
            else -> null
        }
    }

    /**
     * 返回当前 Provider 缺失的必填字段名（空=齐全）。
     * 与 [resolve] 的判空要求完全一致，是设置页校验的单一事实源。
     */
    fun missing(cfg: MqttConfig): List<String> {
        if (!cfg.enabled) return emptyList()
        return when (cfg.effectiveProvider()) {
            MqttProvider.GENERIC -> listOfNotNull(
                "broker 地址".takeIf { cfg.broker.isEmpty() },
                "短商户号（订阅主题）".takeIf { cfg.topic.isEmpty() },
                "上报主题".takeIf { cfg.reportTopic.isEmpty() }
            )
            MqttProvider.ALIYUN -> {
                val a = cfg.aliyun
                listOfNotNull(
                    "endpoint".takeIf { a.endpoint.isEmpty() },
                    "AccessKey".takeIf { a.accessKey.isEmpty() },
                    "SecretKey".takeIf { a.secretKey.isEmpty() },
                    "GroupId".takeIf { a.groupId.isEmpty() },
                    "父主题".takeIf { a.parentTopic.isEmpty() },
                    "自定义ID".takeIf { a.deviceId.isEmpty() },
                    "下行后缀".takeIf { a.downSuffix.isEmpty() },
                    "上行后缀".takeIf { a.upSuffix.isEmpty() }
                )
            }
            MqttProvider.IOT -> {
                val i = cfg.iot
                listOfNotNull(
                    "ProductKey".takeIf { i.productKey.isEmpty() },
                    "DeviceName".takeIf { i.deviceName.isEmpty() },
                    "DeviceSecret".takeIf { i.deviceSecret.isEmpty() },
                    "下行后缀".takeIf { i.downSuffix.isEmpty() },
                    "上行后缀".takeIf { i.upSuffix.isEmpty() },
                    "地域 RegionId".takeIf { i.endpoint.isEmpty() && i.regionId.isEmpty() }
                )
            }
            else -> emptyList()
        }
    }

    private fun resolveGeneric(cfg: MqttConfig): Resolved? {
        if (cfg.broker.isEmpty() || cfg.topic.isEmpty() || cfg.reportTopic.isEmpty()) return null
        return Resolved(
            brokerUrl = buildBrokerUrl(cfg.broker, cfg.port),
            clientId = cfg.topic,
            username = cfg.username,
            password = cfg.password,
            subscribeTopic = cfg.topic,
            publishTopic = cfg.reportTopic,
            merchant = cfg.topic
        )
    }

    private fun resolveAliyun(cfg: MqttConfig): Resolved? {
        val a = cfg.aliyun
        if (a.endpoint.isEmpty() || a.accessKey.isEmpty() || a.secretKey.isEmpty() ||
            a.groupId.isEmpty() || a.parentTopic.isEmpty() || a.deviceId.isEmpty() ||
            a.downSuffix.isEmpty() || a.upSuffix.isEmpty()
        ) return null

        val clientId = "${a.groupId}@@@${a.deviceId}"
        val username = "|${a.instanceId}|${a.deviceId}"
        val password = macSignature(a.secretKey, clientId)

        return Resolved(
            brokerUrl = buildBrokerUrl(a.endpoint, a.port),
            clientId = clientId,
            username = username,
            password = password,
            subscribeTopic = "${a.parentTopic}/${a.deviceId}/${a.downSuffix}",
            publishTopic = "${a.parentTopic}/${a.deviceId}/${a.upSuffix}",
            merchant = a.deviceId
        )
    }

    /**
     * 阿里云物联网平台一机一密（对齐 Windows BuildIotConn）：
     * ClientID=`pk.dn|securemode=3,signmethod=hmacsha1,timestamp=…|`
     * Username=`DeviceName&ProductKey`
     * Password=hex(HMAC-SHA1(DeviceSecret, 字典序 key+value 拼接))
     */
    private fun resolveIot(cfg: MqttConfig): Resolved? {
        val i = cfg.iot
        val pk = i.productKey.trim()
        val dn = i.deviceName.trim()
        val secret = i.deviceSecret.trim()
        val down = i.downSuffix.trim().trim('/')
        val up = i.upSuffix.trim().trim('/')
        if (pk.isEmpty() || dn.isEmpty() || secret.isEmpty() || down.isEmpty() || up.isEmpty()) return null

        val endpoint = i.endpoint.trim().ifEmpty {
            val region = i.regionId.trim()
            if (region.isEmpty()) return null
            "$pk.iot-as-mqtt.$region.aliyuncs.com"
        }
        val port = if (i.port <= 0) 1883 else i.port

        val sub = "/$pk/$dn/user/$down"
        val rep = "/$pk/$dn/user/$up"
        if (sub == rep) return null

        var brief = "$pk.$dn"
        if (brief.length > 64) {
            brief = if (dn.length > 64) dn.substring(0, 64) else dn
        }
        val ts = System.currentTimeMillis().toString()
        val mqttClientId = "$brief|securemode=3,signmethod=hmacsha1,timestamp=$ts|"
        val params = linkedMapOf(
            "clientId" to brief,
            "deviceName" to dn,
            "productKey" to pk,
            "timestamp" to ts
        )
        val password = iotSignPassword(secret, params)

        return Resolved(
            brokerUrl = buildBrokerUrl(endpoint, port),
            clientId = mqttClientId,
            username = "$dn&$pk",
            password = password,
            subscribeTopic = sub,
            publishTopic = rep,
            merchant = dn
        )
    }

    private fun buildBrokerUrl(host: String, port: Int): String {
        var h = host.trim()
        if (!h.startsWith("tcp://") && !h.startsWith("ssl://")) h = "tcp://$h"
        // 已带端口则不再追加（简单判断：最后一个 : 在 // 之后）
        val afterScheme = h.substringAfter("://", h)
        if (!afterScheme.contains(':')) {
            h = "$h:$port"
        }
        return h
    }

    /** 云消息队列：Base64(HMAC-SHA1(secret, clientID)) */
    fun macSignature(secretKey: String, content: String): String {
        val mac = Mac.getInstance("HmacSHA1")
        mac.init(SecretKeySpec(secretKey.toByteArray(StandardCharsets.UTF_8), "HmacSHA1"))
        val raw = mac.doFinal(content.toByteArray(StandardCharsets.UTF_8))
        return Base64.encodeToString(raw, Base64.NO_WRAP)
    }

    /** 物联网一机一密：十六进制小写 HMAC-SHA1（参数名按字典序拼接 key+value） */
    fun iotSignPassword(deviceSecret: String, params: Map<String, String>): String {
        val content = params.keys.sorted().joinToString("") { k -> k + (params[k] ?: "") }
        val mac = Mac.getInstance("HmacSHA1")
        mac.init(SecretKeySpec(deviceSecret.toByteArray(StandardCharsets.UTF_8), "HmacSHA1"))
        val raw = mac.doFinal(content.toByteArray(StandardCharsets.UTF_8))
        return raw.joinToString("") { b -> "%02x".format(b) }
    }
}
