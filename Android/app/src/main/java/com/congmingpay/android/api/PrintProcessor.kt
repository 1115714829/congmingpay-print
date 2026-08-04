package com.congmingpay.android.api

import android.util.Base64
import com.congmingpay.android.config.ConfigManager
import com.congmingpay.android.errcode.CodedException
import com.congmingpay.android.errcode.ErrCode
import com.congmingpay.android.layout.LayoutRenderer
import com.congmingpay.android.model.Brand
import com.congmingpay.android.model.Conn
import com.congmingpay.android.model.Printer
import com.congmingpay.android.printer.PrintDispatcher
import com.congmingpay.android.printer.PrintOptions

/**
 * 消息处理核心（翻译自 Windows 端 api/Print.go）。
 *
 * process() 是入口：校验 → 解析目标 → 渲染 → 参数覆盖 → pCopy 展开 → submit。
 */
object PrintProcessor {

    /**
     * 处理单条打印请求。
     *
     * @return (ProcessResult, changed) changed=true 表示配置有变更（新增打印机或参数覆盖），需 save
     * @throws CodedException 处理失败
     */
    fun process(cfg: ConfigManager, svc: PrintDispatcher, req: PrintRequest): Pair<ProcessResult, Boolean> {
        val compat = cfg.settings().yunheCompat()

        // 1. 校验
        validate(req, compat)

        // 2. 解析目标 + 自动登记
        val (printer, registered) = resolveTarget(cfg, req)

        // 3. 确定纸宽
        var width = printer.width
        if (req.pWidth == 58 || req.pWidth == 80) width = req.pWidth

        // 4. 渲染
        var contentType = 0
        val (data, contentCut) = try {
            when {
                req.type == 0 || (compat && req.type == 5) -> {
                    val contentsStr = req.contents?.toString() ?: "[]"
                    val r = LayoutRenderer.render(contentsStr, width, compat)
                    contentType = 0
                    r.data to r.contentCut
                }
                req.type == 1 -> {
                    val escStr = req.contents?.let { if (it.isJsonPrimitive) it.asString else null }
                        ?: throw CodedException(ErrCode.ESC_DECODE_FAILED, "type=1 的 contents 非 base64 字符串")
                    val bytes = try { Base64.decode(escStr, Base64.DEFAULT) }
                        catch (e: Exception) { throw CodedException(ErrCode.ESC_DECODE_FAILED, "base64 解码失败: ${e.message}") }
                    contentType = 1
                    bytes to null
                }
                else -> {
                    // 对齐 Windows Print.go：type 非法直接拒绝，不加「渲染失败」前缀
                    val hint = if (compat) "仅 0/5=JSON 排版 / 1=ESC base64" else "仅 0=JSON 排版 / 1=ESC base64"
                    throw CodedException(ErrCode.BAD_CONTENT_TYPE, "不支持的 type: ${req.type}($hint)")
                }
            }
        } catch (e: CodedException) {
            // 对齐 Windows Print.go 第 186 行 `fmt.Errorf("渲染失败: %w", err)`：保码、加前缀
            if (e.code == ErrCode.BAD_CONTENT_TYPE) throw e
            throw CodedException(e.code, "渲染失败: ${e.message}", e)
        }

        // 5. 构造 Options + 参数覆盖
        val effBuzzer = req.buzzer?.let { it != 0 } ?: printer.buzzer
        val effCut = req.cut?.let { it != 0 } ?: printer.cuts()
        val effHead = req.headLines ?: printer.headLines
        val effTail = req.tailLines ?: printer.tailLines
        val effReprint = req.reprint?.let { it != 0 } ?: false

        // C3: 云盒兼容下 contents cut 覆盖（只影响本单、不写回配置）
        var finalCut = effCut
        if (compat && contentCut != null && req.cut == null) {
            finalCut = contentCut
        }

        // 参数覆盖写回配置
        var changed = registered
        if (!registered && (req.buzzer != null || req.cut != null || req.headLines != null || req.tailLines != null)) {
            changed = true
        }

        // 6. pCopy 展开
        val copies = if (req.pCopy > 0) req.pCopy else 1
        var firstNo = 0
        for (i in 0 until copies) {
            val no = svc.submit(printer, "打印", data, PrintOptions(
                cut = finalCut,
                buzzer = effBuzzer,
                headLines = effHead,
                tailLines = effTail,
                reprint = effReprint,
                cloudId = if (req.id != 0L) req.id else null,
                contentType = contentType,
                sourceJson = if (contentType == 0) req.contents?.toString()?.toByteArray() else null
            ))
            if (i == 0) firstNo = no
        }

        return ProcessResult(
            jobNo = firstNo,
            printer = printer,
            buzzer = effBuzzer,
            cut = finalCut,
            reprint = effReprint,
            headLines = effHead,
            tailLines = effTail,
            pWidth = width,
            pCopy = copies,
            contentType = contentType
        ) to changed
    }

    /** 校验请求字段（文案与顺序对齐 Windows `PrintRequest.validate`） */
    private fun validate(req: PrintRequest, compat: Boolean) {
        // C2: 兼容关时五参数必填，缺失字段一次汇总上报
        if (!compat) {
            val miss = mutableListOf<String>()
            if (req.buzzer == null) miss.add("buzzer")
            if (req.cut == null) miss.add("cut")
            if (req.reprint == null) miss.add("reprint")
            if (req.headLines == null) miss.add("headLines")
            if (req.tailLines == null) miss.add("tailLines")
            if (miss.isNotEmpty()) {
                throw CodedException(
                    ErrCode.MISSING_FIELD,
                    "缺少必填字段: ${miss.joinToString("、")}(buzzer/cut/reprint/headLines/tailLines 均必传)"
                )
            }
        }

        // 开关类字段 0/1
        for ((v, name) in listOf(req.buzzer to "buzzer", req.cut to "cut", req.reprint to "reprint")) {
            if (v != null && v != 0 && v != 1) {
                throw CodedException(ErrCode.BAD_SWITCH, "$name 只能为 0 或 1")
            }
        }
        // 行数范围 0-100
        for ((v, name) in listOf(req.headLines to "headLines", req.tailLines to "tailLines")) {
            if (v != null && (v < 0 || v > 100)) {
                throw CodedException(ErrCode.BAD_LINE_RANGE, "$name 需在 0-100 之间")
            }
        }
        // pWidth
        if (req.pWidth != 0 && req.pWidth != 58 && req.pWidth != 80) {
            throw CodedException(ErrCode.BAD_P_WIDTH, "pWidth 需为 58 或 80(或不填)")
        }
        // type 的合法性在渲染分支判定（与 Windows 同序：先解析目标，再判 type）
    }

    /**
     * 解析打印目标。优先级：printer.ip → gateway=usb → gateway=IP → printer.name/id → 无目标。
     * 仅网口 IP 目标自动登记；USB/蓝牙/仅名/ID 不自动登记。
     */
    private fun resolveTarget(cfg: ConfigManager, req: PrintRequest): Pair<Printer, Boolean> {
        // printer.ip 非空 → 网口目标
        if (req.printer.ip.isNotEmpty()) {
            return resolveNetworkTarget(cfg, req.printer.ip, req.printer.port, req.printer, req.pWidth)
        }

        // gateway = "usb" → 取第一台 USB 打印机
        if (req.gateway.equals("usb", ignoreCase = true)) {
            val usbPrinter = cfg.printers().firstOrNull { it.conn == Conn.USB }
                ?: throw CodedException(ErrCode.BAD_USB_TARGET, "未找到 USB 打印机，须先在本机添加 USB 打印机")
            return usbPrinter to false
        }

        // gateway = IP[:port] → 网口目标
        if (req.gateway.isNotEmpty() && !req.gateway.equals("usb", ignoreCase = true)) {
            val (ip, port) = parseGateway(req.gateway)
            return resolveNetworkTarget(cfg, ip, port, req.printer, req.pWidth)
        }

        // printer.name 或 printer.id → 查已注册
        if (req.printer.name.isNotEmpty() || req.printer.id.isNotEmpty()) {
            val key = if (req.printer.id.isNotEmpty()) req.printer.id else req.printer.name
            val p = cfg.findPrinter(key)
                ?: throw CodedException(ErrCode.PRINTER_NOT_FOUND, "未找到打印机: $key")
            return p to false
        }

        throw CodedException(ErrCode.NO_TARGET, "未指定打印目标(printer 或 gateway 至少填一个)")
    }

    private fun resolveNetworkTarget(
        cfg: ConfigManager, ip: String, port: String,
        ref: PrinterRef, pWidth: Int
    ): Pair<Printer, Boolean> {
        // 已注册
        val existing = cfg.findPrinterByIp(ip)
        if (existing != null) {
            // 参数覆盖
            return existing to false
        }
        // 自动登记：必须能确定纸宽
        val width = when {
            ref.width == 58 || ref.width == 80 -> ref.width
            pWidth == 58 || pWidth == 80 -> pWidth
            else -> throw CodedException(
                ErrCode.WIDTH_UNKNOWN,
                "无法确定纸宽: 自动登记新打印机需 printer.width 或 pWidth 为 58/80" +
                    "(收到 printer.width=${ref.width}、pWidth=$pWidth)"
            )
        }
        val p = Printer(
            name = if (ref.name.isNotEmpty()) ref.name else "云端-$ip",
            brand = if (ref.brand.isNotEmpty()) ref.brand else Brand.OTHER,
            width = width,
            conn = Conn.NETWORK,
            ip = ip,
            port = if (port.isNotEmpty()) port else "9100"
        )
        return cfg.upsertPrinter(p)
    }

    private fun parseGateway(gw: String): Pair<String, String> {
        val colon = gw.lastIndexOf(':')
        return if (colon > 0) {
            gw.substring(0, colon) to gw.substring(colon + 1)
        } else {
            gw to ""
        }
    }
}
