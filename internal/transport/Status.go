package transport

import (
	"io"
	"net"
	"strings"
	"time"
)

// PrinterStatus 表示一次打印机状态检测结果。
type PrinterStatus struct {
	Reachable    bool   // 通道是否连通(网口能连上 / USB 后台可查)
	Online       bool   // 打印机是否在线
	CoverOpen    bool   // 上盖打开
	PaperOut     bool   // 缺纸
	PaperNearEnd bool   // 纸将尽
	Error        bool   // 存在错误(切刀/不可恢复/可恢复)
	Detail       string // 人类可读描述
	Raw          []byte // 原始状态字节(网口 ESC v 返回 4 字节)
}

// StatusPort 是佳博网口打印机的状态查询端口:ESC v 走此端口,数据打印走 9100。
// 依据《佳博热敏票据打印机编程手册 v1.0.5》命令 64。
const StatusPort = "4000"

// OnlineProbeTimeout 是快速在线探测(TCP 连 9100)的超时。
const OnlineProbeTimeout = 1 * time.Second

// QueryTCPOnline 快速判在线/离线:1 秒内能连上数据端口 9100 即在线,否则离线。
// 只给在线/离线(不读缺纸/开盖/错误——那需 QueryTCPStatus 的 ESC v)。供高频后台巡检用。
func QueryTCPOnline(ip string) PrinterStatus {
	conn, err := net.DialTimeout("tcp", hostWithPort(ip, RawPrintPort), OnlineProbeTimeout)
	if err != nil {
		return PrinterStatus{Reachable: false, Online: false, Detail: "离线(1s 内无法连接 9100)"}
	}
	_ = conn.Close()
	return PrinterStatus{Reachable: true, Online: true, Detail: "在线(9100 可连)"}
}

// QueryTCPStatus 查询网口打印机状态。
//   - 首选:向端口 4000 发 ESC v(1B 76),读回 4 字节详细状态(缺纸/开盖/错误…)。
//   - 回退:若 4000 不可用,测数据端口 9100 是否可连通,以判断在线与否。
func QueryTCPStatus(ip string) (PrinterStatus, error) {
	if conn, err := net.DialTimeout("tcp", hostWithPort(ip, StatusPort), 3*time.Second); err == nil {
		defer conn.Close()
		if _, err := conn.Write([]byte{0x1B, 0x76}); err == nil {
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			buf := make([]byte, 4)
			if n, err := io.ReadFull(conn, buf); err == nil {
				return parseGprinterStatus(buf[:n]), nil
			}
		}
		return PrinterStatus{Reachable: true, Online: true, Detail: "在线(状态端口已连,但未返回状态字节)"}, nil
	}

	// 回退:数据端口 9100 可连通即视为在线,但无法读缺纸/开盖等细节。
	if conn, err := net.DialTimeout("tcp", hostWithPort(ip, RawPrintPort), 3*time.Second); err == nil {
		_ = conn.Close()
		return PrinterStatus{Reachable: true, Online: true, Detail: "在线(可连通 9100;该机型/网络未开放 4000 状态查询,无法读缺纸/开盖)"}, nil
	}

	return PrinterStatus{Reachable: false, Online: false, Detail: "离线/不可达(4000 与 9100 均无法连接)"}, nil
}

// parseGprinterStatus 解析 ESC v 返回的状态字节(佳博票据手册命令 64 的位定义)。
func parseGprinterStatus(b []byte) PrinterStatus {
	s := PrinterStatus{Reachable: true, Raw: append([]byte(nil), b...)}
	if len(b) < 3 {
		s.Detail = "状态字节不足"
		return s
	}
	b0, b1, b2 := b[0], b[1], b[2]
	s.Online = b0&0x08 == 0    // 字节1 bit3:0=在线,1=不在线
	s.CoverOpen = b0&0x20 != 0 // 字节1 bit5:上盖打开
	s.Error = b1&0x08 != 0 ||  // 字节2 bit3:切刀错误
		b1&0x20 != 0 || // 字节2 bit5:不可恢复错误
		b1&0x40 != 0 // 字节2 bit6:可自动恢复错误
	s.PaperNearEnd = b2&0x03 != 0 // 字节3 bit0,1:纸将尽
	s.PaperOut = b2&0x0C != 0     // 字节3 bit2,3:缺纸
	s.Detail = summarizeStatus(s)
	return s
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
	if s.PaperNearEnd {
		issues = append(issues, "纸将尽")
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

// hostWithPort 取 ip 的主机部分并拼上指定端口(去掉用户可能带的端口)。
func hostWithPort(ip, port string) string {
	ip = strings.TrimSpace(ip)
	if i := strings.LastIndex(ip, ":"); i >= 0 {
		ip = ip[:i]
	}
	return ip + ":" + port
}
