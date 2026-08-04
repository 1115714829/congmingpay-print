package com.congmingpay.android.util

import java.nio.charset.Charset

/**
 * GB18030 字符集。热敏小票打印机的中文通常按 GB18030/GBK 处理，而非 UTF-8。
 */
private val GB18030: Charset = Charset.forName("GB18030")

/**
 * 将 UTF-8 字符串编码为 GB18030 字节。
 */
fun toGB18030(s: String): ByteArray = s.toByteArray(GB18030)
