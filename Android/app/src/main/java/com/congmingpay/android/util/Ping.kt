package com.congmingpay.android.util

import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.InetAddress

/**
 * 系统 ping 封装。成功/失败 detail 对齐 Windows `internal/transport/Ping.go` 文案：
 * - 成功：`ping %dms`
 * - 失败：`ping 失败: ` + ipStatusText 同款
 */
object Ping {

    data class Result(val reachable: Boolean, val rttMs: Int = 0, val detail: String = "")

    /**
     * @param ip 目标 IP
     * @param timeoutSec 超时秒数（传给 ping -W）
     */
    fun ping(ip: String, timeoutSec: Int = 1): Result {
        if (ip.isBlank()) {
            return Result(false, detail = "ping 失败: " + ipStatusText(11018))
        }

        try {
            val proc = Runtime.getRuntime().exec(arrayOf("ping", "-c", "1", "-W", "$timeoutSec", ip))
            val out = BufferedReader(InputStreamReader(proc.inputStream)).readText()
            val err = BufferedReader(InputStreamReader(proc.errorStream)).readText()
            val exitCode = proc.waitFor()
            val combined = out + err
            if (exitCode == 0) {
                val rtt = parseRttMs(combined)
                return Result(true, rtt, "ping ${rtt}ms")
            }
            return Result(false, detail = "ping 失败: " + mapPingFailure(combined, exitCode))
        } catch (_: Exception) {
            // ping 二进制不可用，回退 isReachable
        }

        return try {
            val reachable = InetAddress.getByName(ip).isReachable(timeoutSec * 1000)
            if (reachable) {
                Result(true, 0, "ping 0ms")
            } else {
                Result(false, detail = "ping 失败: " + ipStatusText(11010))
            }
        } catch (_: Exception) {
            Result(false, detail = "ping 失败: " + ipStatusText(11003))
        }
    }

    /** 对齐 Windows `ipStatusText` */
    fun ipStatusText(code: Int): String = when (code) {
        11002 -> "目标网络不可达(11002)"
        11003 -> "目标主机不可达(11003)"
        11005 -> "目标端口不可达(11005)"
        11010 -> "请求超时(11010)"
        11013 -> "传输中 TTL 耗尽(11013)"
        11018 -> "目标地址无效(11018)"
        0 -> "未知错误"
        else -> "错误码 $code"
    }

    private fun parseRttMs(output: String): Int {
        // Linux: time=12.3 ms 或 rtt min/avg/max/mdev = a/b/c/d ms
        Regex("""time[=<]([\d.]+)\s*ms""", RegexOption.IGNORE_CASE).find(output)?.groupValues?.get(1)
            ?.toFloatOrNull()?.toInt()?.let { return it }
        val idx = output.indexOf("min/avg/max")
        if (idx >= 0) {
            val after = output.substring(idx)
            val parts = Regex("""=\s*([\d.]+)/([\d.]+)/([\d.]+)""").find(after)
            if (parts != null) {
                return parts.groupValues[2].toFloatOrNull()?.toInt() ?: 0
            }
        }
        return 0
    }

    /**
     * 将系统 ping 失败输出映射到 Windows IP_STATUS 同款文案。
     * 无可靠码时：超时类→11010，不可达类→11003，其余→错误码 exit。
     */
    private fun mapPingFailure(output: String, exitCode: Int): String {
        val lower = output.lowercase()
        return when {
            "network is unreachable" in lower || "network unreachable" in lower ->
                ipStatusText(11002)
            "no route to host" in lower || "host unreachable" in lower ||
                "destination host unreachable" in lower ->
                ipStatusText(11003)
            "ttl exceeded" in lower || "time to live exceeded" in lower ->
                ipStatusText(11013)
            "unknown host" in lower || "name or service not known" in lower ||
                "bad address" in lower || "invalid" in lower && "argument" in lower ->
                ipStatusText(11018)
            "100%" in lower && "packet loss" in lower ||
                "0 received" in lower ||
                "timed out" in lower || "timeout" in lower ||
                exitCode == 1 || exitCode == 2 ->
                ipStatusText(11010)
            else -> ipStatusText(if (exitCode != 0) exitCode else 0)
        }
    }
}
