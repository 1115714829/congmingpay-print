package printsvc

import (
	"congmingpay/internal/model"
	"congmingpay/internal/transport"
)

// Status 是全应用**唯一的公共在线/离线检测入口**(要求6 的「公用代码」),三处共用:
//   ① UI 每台后台巡检 monitorLoop(状态显示 + 上线边沿 NudgeOnline);
//   ② printsvc.dispatch 的「判在线」分支(要求5,仅首次门控);
//   ③ 自动登记后新机的监测 goroutine(syncMonitors→monitorLoop)。
//
// USB 走 winspool;网口走 **ICMP ping**(1s 超时,**不占用 9100、不与打印/第三方争用**,守要求2),
// 只给在线/离线,供高频巡检。9100 只在真正打印那一刻被 dispatch 接触(transport.Print),空闲零接触。
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
