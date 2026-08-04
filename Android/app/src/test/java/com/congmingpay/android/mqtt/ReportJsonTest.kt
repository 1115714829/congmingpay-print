package com.congmingpay.android.mqtt

import com.congmingpay.android.api.ProcessResult
import com.congmingpay.android.model.Conn
import com.congmingpay.android.model.Printer
import com.congmingpay.android.model.Source
import com.congmingpay.android.printer.JobEvent
import com.congmingpay.android.printer.OnlineInfo
import com.google.gson.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * 上行报文键集与取值断言：锁定与 Windows `internal/mqtt/Report.go` 的逐字段一致，
 * 含 omitempty 语义（空值字段整键不出现）。
 */
class ReportJsonTest {

    private fun net() = Printer(
        id = "p-net", name = "厨房1", brand = "佳博", width = 80,
        conn = Conn.NETWORK, ip = "192.168.1.20", port = "9100",
        buzzer = true, headLines = 1, tailLines = 2,
        source = Source.CLOUD, lastPrint = "2026-08-04 10:00:00"
    )

    private fun usb() = Printer(
        id = "p-usb", name = "前台", brand = "飞蛾", width = 58,
        conn = Conn.USB, usbName = "USB001", cutDisabled = true
    )

    private fun keys(o: JsonObject): List<String> = o.keySet().toList()

    @Test
    fun acceptedKeysAndParams() {
        val result = ProcessResult(
            jobNo = 1001, printer = net(),
            buzzer = true, cut = false, reprint = true,
            headLines = 1, tailLines = 2, pWidth = 80, pCopy = 2, contentType = 0
        )
        val o = reportAcceptedJson("m1", 4_000_000_000L, result, 1_700_000_000_000L)

        assertEquals(
            listOf("type", "merchant", "event", "id", "jobNo", "code", "message", "printer", "params", "ts"),
            keys(o)
        )
        assertEquals("report", o.get("type").asString)
        assertEquals("accepted", o.get("event").asString)
        // uint32 大值不得溢出为负
        assertEquals(4_000_000_000L, o.get("id").asLong)
        assertEquals(0, o.get("code").asInt)
        assertEquals("已提交", o.get("message").asString)

        val params = o.getAsJsonObject("params")
        assertEquals(
            listOf("buzzer", "cut", "reprint", "headLines", "tailLines", "pWidth", "pCopy", "contentType"),
            keys(params)
        )
        // 开关类参数为 0/1 整数
        assertEquals(1, params.get("buzzer").asInt)
        assertEquals(0, params.get("cut").asInt)
        assertEquals(1, params.get("reprint").asInt)
        assertEquals(80, params.get("pWidth").asInt)
        assertEquals(2, params.get("pCopy").asInt)
    }

    @Test
    fun reportPrinterIsIdentityOnly() {
        val o = reportAcceptedJson(
            "m1", 1L,
            ProcessResult(1, net(), true, true, false, 0, 0, 80, 1, 0),
            1L
        ).getAsJsonObject("printer")
        assertEquals(
            listOf("printerId", "printer", "brand", "width", "conn", "ip", "port"),
            keys(o)
        )
        // 身份段不含在线态与每台参数
        assertFalse(o.has("online"))
        assertFalse(o.has("detail"))
        assertFalse(o.has("buzzer"))
        assertEquals("network", o.get("conn").asString)
    }

    @Test
    fun reportPrinterUsbUsesUsbName() {
        val o = printerReportJson(usb())
        assertEquals(listOf("printerId", "printer", "brand", "width", "conn", "usbName"), keys(o))
        assertEquals("USB001", o.get("usbName").asString)
        assertFalse(o.has("ip"))
        assertFalse(o.has("port"))
    }

    @Test
    fun failedHasNoJobNoAndNoPrinter() {
        val o = reportFailedJson("m1", 7L, 3002, "未找到打印机: 厨房9", 1L)
        assertEquals(listOf("type", "merchant", "event", "id", "code", "message", "ts"), keys(o))
        assertEquals("failed", o.get("event").asString)
        assertEquals(3002, o.get("code").asInt)
    }

    @Test
    fun jobEventDoneAndWaiting() {
        val done = reportJobEventJson(
            "m1", "done",
            JobEvent("done", 0, 1002, 9L, net()),
            1L
        )
        assertEquals(
            listOf("type", "merchant", "event", "id", "jobNo", "code", "message", "printer", "ts"),
            keys(done)
        )
        assertEquals("打印成功", done.get("message").asString)

        val waiting = reportJobEventJson(
            "m1", "waiting",
            JobEvent("waiting", 5101, 1003, 9L, net(), "长期等待: 打印机离线"),
            1L
        )
        assertEquals(5101, waiting.get("code").asInt)
        assertEquals("长期等待: 打印机离线", waiting.get("message").asString)
    }

    @Test
    fun stateSnapshotKeysAndOmitEmpty() {
        val online = mapOf("p-net" to OnlineInfo(true, "就绪(ping 5ms)"))
        val o = stateJson("m1", listOf(net(), usb()), online, 1_700_000_000_000L)

        assertEquals(listOf("type", "merchant", "online", "printers", "ts"), keys(o))
        assertTrue(o.get("online").asBoolean)

        val arr = o.getAsJsonArray("printers")
        assertEquals(2, arr.size())

        val netItem = arr[0].asJsonObject
        assertEquals(
            listOf(
                "printerId", "printer", "brand", "width", "conn", "ip", "port",
                "online", "detail", "buzzer", "cut", "headLines", "tailLines", "source", "lastPrint"
            ),
            keys(netItem)
        )
        assertTrue(netItem.get("online").asBoolean)
        assertEquals("就绪(ping 5ms)", netItem.get("detail").asString)
        assertEquals("cloud", netItem.get("source").asString)

        // 未进在线注册表(监测未定)→ online=false + detail「检测中」；空 lastPrint 整键省略
        val usbItem = arr[1].asJsonObject
        assertFalse(usbItem.get("online").asBoolean)
        assertEquals("检测中", usbItem.get("detail").asString)
        assertFalse(usbItem.has("lastPrint"))
        assertFalse(usbItem.get("cut").asBoolean) // cutDisabled=true → cut=false
        assertEquals("local", usbItem.get("source").asString)
    }

    @Test
    fun stateWithoutPrintersOmitsKey() {
        val o = stateJson("m1", emptyList(), emptyMap(), 1L)
        assertFalse(o.has("printers"))
        assertEquals(listOf("type", "merchant", "online", "ts"), keys(o))
    }

    @Test
    fun stateOfflineIsMinimal() {
        val o = stateOfflineJson("m1")
        assertEquals(listOf("type", "merchant", "online"), keys(o))
        assertFalse(o.get("online").asBoolean)
    }
}
