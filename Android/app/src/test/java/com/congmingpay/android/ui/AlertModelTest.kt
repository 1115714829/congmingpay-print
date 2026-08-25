package com.congmingpay.android.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AlertModelTest {

    @Test
    fun raiseInsertsNewestFirstAndUpdatesInPlace() {
        val m = AlertModel()
        m.raise(AlertModel.KIND_JOB_FAILED, "1", AlertModel.SEV_ERROR, "a", 1000)
        m.raise(AlertModel.KIND_JOB_WAITING, "2", AlertModel.SEV_WARN, "b", 2000)
        assertEquals(2, m.size())
        assertEquals("2", m.snapshot()[0].key)
        assertEquals("1", m.snapshot()[1].key)

        m.raise(AlertModel.KIND_JOB_FAILED, "1", AlertModel.SEV_ERROR, "a2", 3000)
        assertEquals(2, m.size())
        val updated = m.snapshot().first { it.key == "1" }
        assertEquals("a2", updated.text)
        assertEquals(3000L, updated.whenMs)
    }

    @Test
    fun resolveRemovesAndDismissAllClears() {
        val m = AlertModel()
        m.raise(AlertModel.KIND_MQTT_DOWN, "", AlertModel.SEV_ERROR, "x", 1)
        m.raise(AlertModel.KIND_PRINTER_OFFLINE, "p1", AlertModel.SEV_ERROR, "y", 2)
        assertTrue(m.resolve(AlertModel.KIND_MQTT_DOWN, ""))
        assertEquals(1, m.size())
        assertFalse(m.resolve(AlertModel.KIND_MQTT_DOWN, ""))
        m.dismissAll()
        assertTrue(m.isEmpty())
    }

    @Test
    fun capsAtMaxItems() {
        val m = AlertModel()
        for (i in 0 until AlertModel.MAX_ITEMS + 50) {
            m.raise(AlertModel.KIND_ACCEPT_FAILED, i.toString(), AlertModel.SEV_ERROR, "t$i", i.toLong())
        }
        assertEquals(AlertModel.MAX_ITEMS, m.size())
        // 最新在最上
        assertEquals((AlertModel.MAX_ITEMS + 49).toString(), m.snapshot()[0].key)
    }

    @Test
    fun durTextWindowsStyle() {
        assertEquals("0秒", alertDurText(0))
        assertEquals("5秒", alertDurText(5_000))
        assertEquals("59秒", alertDurText(59_000))
        assertEquals("1分00秒", alertDurText(60_000))
        assertEquals("1分05秒", alertDurText(65_000))
        assertEquals("1时0分", alertDurText(3_600_000))
        assertEquals("1时1分", alertDurText(3_661_000))
        assertEquals("10时0分", alertDurText(36_000_000))
    }
}
