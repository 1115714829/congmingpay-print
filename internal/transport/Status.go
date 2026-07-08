package transport

import "strings"

// PrinterStatus 表示一次打印机状态检测结果。
// 网口检测走 ICMP ping(Ping.go),只给在线/离线;
// USB 检测走 winspool(SpoolerStatus.go),可附带缺纸/开盖/错误位(普通 RAW 驱动多不上报)。
type PrinterStatus struct {
	Reachable bool   // 通道是否连通(网口能 ping 通 / USB 后台可查)
	Online    bool   // 打印机是否在线
	CoverOpen bool   // 上盖打开(仅 USB winspool 可能给出)
	PaperOut  bool   // 缺纸(仅 USB winspool 可能给出)
	Error     bool   // 存在错误(仅 USB winspool 可能给出)
	Detail    string // 人类可读描述
}

// summarizeStatus 生成人类可读的状态描述。
func summarizeStatus(s PrinterStatus) string {
	if !s.Online {
		return "离线(打印机不在线)"
	}
	var issues []string
	if s.PaperOut {
		issues = append(issues, "缺纸")
	}
	if s.CoverOpen {
		issues = append(issues, "上盖打开")
	}
	if s.Error {
		issues = append(issues, "打印机错误")
	}
	if len(issues) == 0 {
		return "在线,就绪"
	}
	return "在线,但:" + strings.Join(issues, "、")
}
