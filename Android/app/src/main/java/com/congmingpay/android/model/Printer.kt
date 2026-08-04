package com.congmingpay.android.model

/**
 * 打印机连接方式。
 */
object Conn {
    const val NETWORK = "network"
    const val USB = "usb"
    const val BLUETOOTH = "bluetooth"
}

/**
 * 打印机品牌。佳博与飞蛾走同一套 ESC/POS 协议；其他也按 ESC/POS 处理。
 * 品牌当前仅作元数据 + 未来品牌差异适配预留。
 */
object Brand {
    const val GPRINTER = "佳博"
    const val FEIE = "飞蛾"
    const val OTHER = "其他"

    /** 下拉可选品牌顺序 */
    val ALL = listOf(GPRINTER, FEIE, OTHER)

    fun forIndex(i: Int): String =
        if (i in ALL.indices) ALL[i] else GPRINTER

    fun indexOf(brand: String): Int {
        val idx = ALL.indexOf(brand)
        return if (idx >= 0) idx else 0
    }
}

/**
 * 打印机登记来源（仅两种）。
 */
object Source {
    const val LOCAL = "local"
    const val CLOUD = "cloud"
}

/** 「上次打印时间」的固定墙钟格式 */
const val LAST_PRINT_TIME_LAYOUT = "yyyy-MM-dd HH:mm:ss"

/**
 * 打印机数据模型（可 Gson 序列化到 config.json）。
 */
data class Printer(
    var id: String = "",
    var name: String = "",
    var brand: String = Brand.OTHER,
    var width: Int = 80,
    var conn: String = Conn.NETWORK,
    var ip: String = "",
    var port: String = "",
    var usbName: String = "",
    var bluetoothMac: String = "",
    var buzzer: Boolean = true,
    var headLines: Int = 0,
    var tailLines: Int = 0,
    var cutDisabled: Boolean = false,
    var source: String = Source.LOCAL,
    var lastPrint: String = ""
) {
    /** 规范化来源（空=local） */
    fun effectiveSource(): String =
        if (source == Source.CLOUD) Source.CLOUD else Source.LOCAL

    fun sourceLabel(): String =
        if (effectiveSource() == Source.CLOUD) "云端下发" else "本地添加"

    /** 返回是否在打印末尾切纸（反向字段：默认 false=启用切刀） */
    fun cuts(): Boolean = !cutDisabled

    fun brandLabel(): String = if (brand.isEmpty()) Brand.OTHER else brand

    fun connLabel(): String = when (conn) {
        Conn.USB -> "USB"
        Conn.BLUETOOTH -> "蓝牙"
        else -> "网口"
    }

    fun address(): String = when (conn) {
        Conn.USB -> if (usbName.isNotEmpty()) usbName else "USB 直连"
        Conn.BLUETOOTH -> if (bluetoothMac.isNotEmpty()) bluetoothMac else "蓝牙"
        else -> "$ip:${if (port.isNotEmpty()) port else "9100"}"
    }

    fun widthLabel(): String = "${width}mm"

    fun target(): String = when (conn) {
        Conn.USB -> "USB『$usbName』"
        Conn.BLUETOOTH -> "蓝牙[$bluetoothMac]"
        else -> "$ip:${if (port.isNotEmpty()) port else "9100"}"
    }
}
