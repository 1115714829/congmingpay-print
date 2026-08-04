package com.congmingpay.android.printer

import org.junit.Assert.assertEquals
import org.junit.Test

/** 锁定防抖语义：对齐 Windows OnlineDebounce_test.go */
class OnlineDebouncerTest {

    @Test
    fun successImmediateOnline() {
        val d = OnlineDebouncer(30_000)
        assertEquals(1, d.observe(true, "ping 1ms", 0))
        assertEquals(1, d.effective)
    }

    @Test
    fun failFullWindowThenOffline() {
        val d = OnlineDebouncer(30_000)
        val t0 = 1_000_000L
        assertEquals(-1, d.observe(false, "ping 失败: 请求超时(11010)", t0))
        assertEquals("ping 失败: 请求超时(11010)", d.failFirst)
        // 未满窗仍未定
        assertEquals(-1, d.observe(false, "ping 失败: x", t0 + 29_000))
        // 满 30s → 离线
        assertEquals(0, d.observe(false, "ping 失败: x", t0 + 30_000))
        assertEquals(0, d.effective)
    }

    @Test
    fun successDuringWindowResets() {
        val d = OnlineDebouncer(30_000)
        val t0 = 1_000_000L
        d.observe(false, "ping 失败: 请求超时(11010)", t0)
        assertEquals(1, d.observe(true, "ping 2ms", t0 + 10_000))
        assertEquals("", d.failFirst)
        // 再失败需重新满窗
        assertEquals(1, d.observe(false, "ping 失败: 请求超时(11010)", t0 + 10_001))
        assertEquals(1, d.effective) // 曾在线，窗内仍保持在线
    }

    @Test
    fun windowZeroInstantOffline() {
        val d = OnlineDebouncer(0)
        assertEquals(0, d.observe(false, "打印后台标记脱机", 100))
        assertEquals(0, d.effective)
        assertEquals(1, d.observe(true, "打印后台正常", 200))
    }

    @Test
    fun formatGoDuration() {
        assertEquals("0s", formatGoDurationSeconds(0))
        assertEquals("5s", formatGoDurationSeconds(5_000))
        assertEquals("30s", formatGoDurationSeconds(30_000))
        assertEquals("1m0s", formatGoDurationSeconds(60_000))
        assertEquals("1m30s", formatGoDurationSeconds(90_000))
        assertEquals("59m59s", formatGoDurationSeconds(3_599_000))
        assertEquals("1h0m0s", formatGoDurationSeconds(3_600_000))
        assertEquals("1h1m1s", formatGoDurationSeconds(3_661_000))
        assertEquals("10h0m0s", formatGoDurationSeconds(36_000_000))
    }
}
