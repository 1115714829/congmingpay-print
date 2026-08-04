package com.congmingpay.android.printer

import android.bluetooth.BluetoothAdapter
import android.content.Context
import android.hardware.usb.UsbManager
import com.congmingpay.android.logger.Logger
import com.congmingpay.android.model.Conn
import com.congmingpay.android.model.Printer
import com.congmingpay.android.util.Ping
import com.gainscha.sdk2.ConnectionListener
import com.gainscha.sdk2.Printer as SdkPrinter
import com.gainscha.sdk2.model.BluetoothPrinterDevice
import com.gainscha.sdk2.model.PrinterDevice
import com.gainscha.sdk2.model.UsbPrinterDevice
import com.gainscha.sdk2.model.WifiPrinterDevice
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.withTimeout
import java.util.concurrent.ConcurrentHashMap

/**
 * 佳博 SDK 连接管理器。
 *
 * 短连接模型：每次打印 connect → print → disconnect。
 * 封装 SDK 异步回调为 suspend 函数（suspendCancellableCoroutine 桥接）。
 *
 * ConnectionListener 是全局的（Printer.addConnectionListener 静态），
 * 通过 printer.getPrinterDevice() 标识符路由回调到正确的 pending 连接。
 */
class ConnectionManager {

    /** 设备标识符 → 待完成连接 */
    private val pending = ConcurrentHashMap<String, CompletableDeferred<SdkPrinter>>()
    private var listenerRegistered = false
    private val globalListener = object : ConnectionListener {
        override fun onPrinterConnected(printer: SdkPrinter) {
            val key = keyOf(printer)
            pending[key]?.complete(printer)
            Logger.info("SDK 连接成功: $key")
        }

        override fun onPrinterConnectFail(printer: SdkPrinter) {
            val key = keyOf(printer)
            pending[key]?.completeExceptionally(ConnectException("连接失败: $key"))
            Logger.warn("SDK 连接失败: $key")
        }

        override fun onPrinterDisconnect(printer: SdkPrinter) {
            val key = keyOf(printer)
            Logger.info("SDK 连接断开: $key")
        }
    }

    fun ensureListener() {
        if (!listenerRegistered) {
            SdkPrinter.addConnectionListener(globalListener)
            listenerRegistered = true
        }
    }

    fun removeListener() {
        if (listenerRegistered) {
            SdkPrinter.removeConnectionListener(globalListener)
            listenerRegistered = false
        }
    }

    /**
     * 将 Printer 模型解析为 SDK PrinterDevice。
     */
    fun resolveDevice(p: Printer): PrinterDevice {
        return when (p.conn) {
            Conn.NETWORK -> WifiPrinterDevice().apply {
                ip = p.ip
                port = p.port.ifEmpty { "9100" }.toIntOrNull() ?: 9100
            }
            Conn.BLUETOOTH -> {
                // 需要从已配对设备查找（bluetoothMac 匹配）
                BluetoothDeviceResolver.resolve(p.bluetoothMac) ?: throw ConnectException("蓝牙设备未找到: ${p.bluetoothMac}")
            }
            Conn.USB -> {
                // 需要从 UsbManager 查找（usbName 匹配）
                UsbDeviceResolver.resolve(p.usbName) ?: throw ConnectException("USB 设备未找到: ${p.usbName}")
            }
            else -> throw ConnectException("未知连接类型: ${p.conn}")
        }
    }

    /**
     * 连接打印机（suspend，桥接 SDK 异步回调）。
     * @param device SDK 设备对象
     * @param timeoutMs 连接超时
     */
    suspend fun connect(device: PrinterDevice, timeoutMs: Long = 5000): SdkPrinter {
        ensureListener()
        val key = keyOf(device)
        val deferred = CompletableDeferred<SdkPrinter>()
        pending[key] = deferred
        try {
            SdkPrinter.connect(device)
            return withTimeout(timeoutMs) { deferred.await() }
        } catch (e: Exception) {
            pending.remove(key)
            throw if (e is ConnectException) e else ConnectException("连接超时或失败: ${e.message}")
        } finally {
            pending.remove(key)
        }
    }

    /**
     * 发送数据打印（同步重载）。
     * 必须在已连接的 SdkPrinter 上调用。
     */
    fun print(printer: SdkPrinter, data: ByteArray) {
        printer.print(data)  // 同步重载，throws IOException
    }

    /** 断开连接 */
    fun disconnect(printer: SdkPrinter) {
        try {
            printer.disconnect()
        } catch (e: Exception) {
            Logger.warn("断开连接异常: ${e.message}")
        }
    }

    /**
     * 完整短连接打印：connect → print → disconnect。
     * @throws ConnectException 连接失败
     * @throws PrintException 打印失败
     */
    suspend fun connectAndPrint(p: Printer, data: ByteArray) {
        val device = resolveDevice(p)
        val printer = connect(device)
        try {
            print(printer, data)
        } catch (e: Exception) {
            throw PrintException("打印失败: ${e.message}")
        } finally {
            disconnect(printer)
        }
    }

    /** 设备标识符（用于回调路由） */
    private fun keyOf(device: PrinterDevice): String = when (device) {
        is WifiPrinterDevice -> "net:${device.ip}"
        is BluetoothPrinterDevice -> "bt:${device.bluetoothDevice?.address}"
        is UsbPrinterDevice -> "usb:${device.usbDevice?.deviceName}"
        else -> device.toString()
    }

    private fun keyOf(printer: SdkPrinter): String {
        val device = printer.printerDevice ?: return ""
        return keyOf(device)
    }
}

class ConnectException(msg: String) : Exception(msg)
class PrintException(msg: String) : Exception(msg)

/**
 * 蓝牙设备解析器：按 MAC 在已配对设备中查找。
 * 需 [init] 注入 Context（实际仅 BluetoothAdapter 静态获取，保留注入入口与 USB 对称）。
 * 未授权（BLUETOOTH_CONNECT / BLUETOOTH）时返回 null，打印走「连接失败」退避。
 */
object BluetoothDeviceResolver {
    @Volatile private var contextRef: Context? = null

    fun init(context: Context) {
        contextRef = context.applicationContext
    }

    fun resolve(mac: String): BluetoothPrinterDevice? {
        if (mac.isBlank()) return null
        val ctx = contextRef ?: return null
        val adapter = BluetoothAdapter.getDefaultAdapter() ?: return null
        val device = try {
            adapter.bondedDevices.firstOrNull { it.address.equals(mac, ignoreCase = true) }
        } catch (e: SecurityException) {
            Logger.warn("蓝牙权限缺失,无法查找设备 $mac")
            null
        } ?: return null
        return BluetoothPrinterDevice(device, 0)
    }
}

/**
 * USB 设备解析器：按 UsbManager 设备在场列表（deviceName）查找。
 * 需 [init] 注入 Context。getDeviceList 枚举不需要 USB 授权（授权在 SDK connect 时弹窗）。
 */
object UsbDeviceResolver {
    @Volatile private var usbManager: UsbManager? = null

    fun init(context: Context) {
        usbManager = context.applicationContext.getSystemService(Context.USB_SERVICE) as? UsbManager
    }

    fun resolve(name: String): UsbPrinterDevice? {
        if (name.isBlank()) return null
        val um = usbManager ?: return null
        val device = um.deviceList.values.firstOrNull { it.deviceName == name } ?: return null
        return UsbPrinterDevice(device)
    }
}

/**
 * 一次探测结果（对齐 Windows `transport.PrinterStatus` 的 Online + Detail）。
 */
data class ProbeResult(val online: Boolean, val detail: String)

/**
 * 在线状态检测（不碰打印机数据端口）。
 * 网口：系统 ping（detail 对齐 Windows Ping.go）；
 * USB：设备在场（detail 对齐 SpoolerStatus「打印后台正常/标记脱机」）；
 * 蓝牙：配对状态（无 Windows 对照，在场用同款 USB 文案形态）。
 */
object StatusCheck {
    @Volatile private var usbManager: UsbManager? = null
    @Volatile private var btAdapter: BluetoothAdapter? = null

    fun init(context: Context) {
        val ctx = context.applicationContext
        usbManager = ctx.getSystemService(Context.USB_SERVICE) as? UsbManager
        btAdapter = BluetoothAdapter.getDefaultAdapter()
    }

    fun probe(p: Printer): ProbeResult = when (p.conn) {
        Conn.NETWORK -> {
            val r = Ping.ping(p.ip)
            ProbeResult(r.reachable, r.detail)
        }
        Conn.USB -> {
            val name = p.usbName
            val present = name.isNotBlank() &&
                (usbManager?.deviceList?.values?.any { it.deviceName == name } == true)
            // 对齐 Windows SpoolerStatus.go
            ProbeResult(
                present,
                if (present) "打印后台正常" else "打印后台标记脱机"
            )
        }
        Conn.BLUETOOTH -> {
            val mac = p.bluetoothMac
            val bonded = if (mac.isBlank()) {
                false
            } else try {
                btAdapter?.bondedDevices?.any { it.address.equals(mac, ignoreCase = true) } == true
            } catch (_: SecurityException) {
                false
            }
            ProbeResult(
                bonded,
                if (bonded) "打印后台正常" else "打印后台标记脱机"
            )
        }
        else -> ProbeResult(false, "ping 失败: " + Ping.ipStatusText(0))
    }

    fun isOnline(p: Printer): Boolean = probe(p).online
}

/**
 * 一次性注入设备解析器所需 Context（PrintService 初始化时调用）。
 */
fun injectDeviceResolvers(context: Context) {
    BluetoothDeviceResolver.init(context)
    UsbDeviceResolver.init(context)
    StatusCheck.init(context)
}
