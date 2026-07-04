package printsvc

import (
	"congmingpay/internal/model"
	"congmingpay/internal/transport"
)

// Status 查询一台打印机的当前状态。
// USB 走 winspool;网口走 **ICMP ping**(1s 超时,不占用 9100、不与打印争用),只给在线/离线,供高频巡检。
func Status(p *model.Printer) transport.PrinterStatus {
	if p.Conn == model.ConnUSB {
		st, err := transport.QuerySpoolerStatus(p.USBName)
		if err != nil {
			return transport.PrinterStatus{Detail: "查询失败: " + err.Error()}
		}
		return st
	}
	return transport.QueryICMPPing(p.IP)
}
