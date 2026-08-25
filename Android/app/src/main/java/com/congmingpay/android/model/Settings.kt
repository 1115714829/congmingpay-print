package com.congmingpay.android.model

/**
 * MQTT 通道提供方常量。
 */
object MqttProvider {
    const val GENERIC = "generic"  // 自建/通用 MQTT（用户名密码）
    const val ALIYUN = "aliyun"    // 阿里云云消息队列 MQTT 版
    const val IOT = "iot"          // 阿里云物联网平台（一机一密）
}

/**
 * 云端 MQTT 连接参数（唯一云端通道；Provider 三选一）。
 */
data class MqttConfig(
    var enabled: Boolean = false,
    var provider: String = MqttProvider.GENERIC,
    // 自建/通用
    var broker: String = "",
    var port: Int = 1883,
    var username: String = "",
    var password: String = "",
    var topic: String = "",         // 短商户号（订阅主题）
    var reportTopic: String = "",   // 上报（发布）主题
    // 阿里云云消息队列 MQTT 版
    var aliyun: AliyunMqtt = AliyunMqtt(),
    // 阿里云物联网平台（一机一密）
    var iot: IotMqtt = IotMqtt()
) {
    /** 返回规范化 Provider（空视为 generic） */
    fun effectiveProvider(): String = when (provider) {
        MqttProvider.ALIYUN -> MqttProvider.ALIYUN
        MqttProvider.IOT -> MqttProvider.IOT
        else -> MqttProvider.GENERIC
    }
}

/** 云消息队列 MQTT 版连接与主题拼接配置 */
data class AliyunMqtt(
    var endpoint: String = "",
    var port: Int = 1883,
    var instanceId: String = "",
    var accessKey: String = "",
    var secretKey: String = "",
    var groupId: String = "",
    var parentTopic: String = "",
    var deviceId: String = "",
    var downSuffix: String = "",
    var upSuffix: String = ""
)

/** 物联网平台一机一密连接配置 */
data class IotMqtt(
    var productKey: String = "",
    var deviceName: String = "",
    var deviceSecret: String = "",
    var regionId: String = "cn-shanghai",
    var endpoint: String = "",
    var port: Int = 1883,
    var downSuffix: String = "",
    var upSuffix: String = ""
)

/** 本地（局域网）只读接口文档服务配置 */
data class DocServerConfig(
    var enabled: Boolean = true,
    var port: Int = 8080
)

// 打印历史保留天数
const val DEFAULT_JOB_HISTORY_DAYS = 7
const val MIN_JOB_HISTORY_DAYS = 1
const val MAX_JOB_HISTORY_DAYS = 365

/**
 * 打印服务的全局设置。
 */
data class Settings(
    var serviceName: String = "票据打印服务",
    var notifyDisabled: Boolean = false,        // 反向字段，零值=启用
    var yunheCompatDisabled: Boolean = false,   // 反向字段，零值=兼容开（C1–C4）
    var bootStartEnabled: Boolean = true,       // 开机启动打印服务（厨房机默认开）
    var jobHistoryDays: Int = DEFAULT_JOB_HISTORY_DAYS,
    var mqtt: MqttConfig = MqttConfig(),
    var docServer: DocServerConfig = DocServerConfig()
) {
    /** 返回是否启用云盒兼容模式（C1–C4）。零值/缺字段为启用。 */
    fun yunheCompat(): Boolean = !yunheCompatDisabled
}

/** 将天数钳制到 1–365；非法回退默认 7 */
fun clampJobHistoryDays(n: Int): Int =
    if (n < MIN_JOB_HISTORY_DAYS || n > MAX_JOB_HISTORY_DAYS) DEFAULT_JOB_HISTORY_DAYS else n
