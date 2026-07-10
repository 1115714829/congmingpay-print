package transport

// PrinterStatus 表示一次打印机状态检测结果,统一只有在线/离线两态。
// 网口检测走 ICMP ping(Ping.go),USB 检测走 winspool(SpoolerStatus.go)。
type PrinterStatus struct {
	Reachable bool   // 通道是否连通(网口能 ping 通 / USB 后台可查)
	Online    bool   // 打印机是否在线
	RTTms     int    // 往返时延 ms(仅网口 ping 成功时有效;供监测丢包统计,不上报云端)
	Detail    string // 人类可读描述
}
