package com.congmingpay.android.printer

import com.gainscha.sdk2.ConnectType
import com.gainscha.sdk2.PrinterFinder
import com.gainscha.sdk2.model.BluetoothPrinterDevice
import com.gainscha.sdk2.model.SerialPortPrinterDevice
import com.gainscha.sdk2.model.UsbAccessoryPrinterDevice
import com.gainscha.sdk2.model.UsbPrinterDevice
import com.gainscha.sdk2.model.WifiPrinterDevice
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume

/**
 * 搜索到的设备（归一化展示）。
 */
data class FoundDevice(
    val type: String,          // usb / bt / net
    val displayName: String,
    val identifier: String     // usb=deviceName / bt=MAC / net=IP
)

/**
 * 佳博 SDK 设备搜索封装（PrinterFinder 桥接为 suspend）。
 *
 * 搜索在 SDK 内部线程异步执行，5 个回调收集结果，
 * onSearchCompleted 时恢复调用方协程；取消协程即停止搜索。
 *
 * API 依据（demo 源码 + classes.jar 反编译确认）：
 * PrinterFinder.searchPrinters(SearchPrinterResultListener) 全类型搜索；
 * searchPrinters(ConnectType, listener) 按类型搜索；stopSearchDevice() 停止。
 */
class DeviceSearcher {

    private val finder = PrinterFinder()

    /**
     * 搜索设备。
     * @param connectType null=全部类型（含串口，结果仅保留 usb/bt/net）；指定则只搜该类型
     */
    suspend fun search(connectType: ConnectType? = null): List<FoundDevice> =
        suspendCancellableCoroutine { cont ->
            val devices = mutableListOf<FoundDevice>()
            val listener = object : PrinterFinder.SearchPrinterResultListener {
                override fun onSearchBluetoothPrinter(device: BluetoothPrinterDevice?) {
                    device ?: return
                    val bd = device.bluetoothDevice ?: return
                    val mac = bd.address ?: return
                    val name = device.printerName?.ifEmpty { null } ?: bd.name
                    devices += FoundDevice("bt", name ?: mac, mac)
                }

                override fun onSearchUsbPrinter(device: UsbPrinterDevice?) {
                    device ?: return
                    val ud = device.usbDevice ?: return
                    val id = ud.deviceName ?: return
                    val name = device.printerName?.ifEmpty { null } ?: ud.productName
                    devices += FoundDevice("usb", name ?: id, id)
                }

                override fun onSearchUsbPrinter(device: UsbAccessoryPrinterDevice?) {
                    // USB Accessory 模式不支持
                }

                override fun onSearchNetworkPrinter(device: WifiPrinterDevice?) {
                    device ?: return
                    val ip = device.ip ?: return
                    val name = device.printerName?.ifEmpty { null }
                    devices += FoundDevice("net", name ?: ip, ip)
                }

                override fun onSearchSerialPortPrinter(device: SerialPortPrinterDevice?) {
                    // 串口不支持
                }

                override fun onSearchCompleted() {
                    if (cont.isActive) cont.resume(devices)
                }
            }

            cont.invokeOnCancellation { finder.stopSearchDevice() }
            try {
                if (connectType != null) finder.searchPrinters(connectType, listener)
                else finder.searchPrinters(listener)
            } catch (e: Exception) {
                if (cont.isActive) cont.resume(emptyList())
            }
        }
}
