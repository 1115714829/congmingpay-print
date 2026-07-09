//go:build windows

package transport

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ICMP ping 走 Windows IP Helper(iphlpapi.dll)的 IcmpSendEcho —— 与系统 ping 命令同源,
// 普通用户权限即可(不需要管理员/原始套接字),且**不占用打印口 9100**,不与打印争用通道。
var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
)

// PingTimeout 是单次 ICMP ping 的超时。
const PingTimeout = 1 * time.Second

// QueryICMPPing 用 ICMP ping 判在线/离线:ping 通=在线,超时/不可达=离线。
// 只给在线/离线(读不到缺纸/开盖)。供高频后台巡检,且不碰 9100。
// RTTms 仅 ping 成功时有效,供监测丢包统计求均值。
func QueryICMPPing(ip string) PrinterStatus {
	ok, rtt, detail := icmpEcho(ip, PingTimeout)
	return PrinterStatus{Reachable: ok, Online: ok, RTTms: rtt, Detail: detail}
}

// icmpEcho 向 ip 发一个 ICMP Echo,timeout 内收到成功回复即返回 true。
func icmpEcho(ip string, timeout time.Duration) (ok bool, rttMs int, detail string) {
	host := strings.TrimSpace(ip)
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i] // 去掉可能带的端口
	}
	ip4 := net.ParseIP(host).To4()
	if ip4 == nil {
		return false, 0, "非合法 IPv4,无法 ping"
	}

	h, _, _ := procIcmpCreateFile.Call()
	if h == 0 || h == ^uintptr(0) { // 0 或 INVALID_HANDLE_VALUE
		return false, 0, "IcmpCreateFile 失败"
	}
	defer procIcmpCloseHandle.Call(h)

	dest := uintptr(binary.LittleEndian.Uint32(ip4)) // IPAddr(网络字节序 = 地址字节按内存序)
	req := make([]byte, 32)                           // 32 字节负载,与系统 ping 一致
	// 回复缓冲:sizeof(ICMP_ECHO_REPLY)(32位=28) + 请求长度 + 8,留足取 128。
	reply := make([]byte, 128)
	timeoutMs := uint32(timeout / time.Millisecond)

	n, _, callErr := procIcmpSendEcho.Call(
		h,
		dest,
		uintptr(unsafe.Pointer(&req[0])),
		uintptr(uint16(len(req))),
		0, // 无 IP_OPTION_INFORMATION
		uintptr(unsafe.Pointer(&reply[0])),
		uintptr(uint32(len(reply))),
		uintptr(timeoutMs),
	)
	if n == 0 {
		// IcmpSendEcho 失败时 GetLastError 携带 IP_STATUS(11xxx),
		// 可区分「拥塞丢包(11010 超时)」与「ARP/路由不可达(11003/11002)」。
		var code uint32
		if e, isErrno := callErr.(syscall.Errno); isErrno {
			code = uint32(e)
		}
		return false, 0, "ping 失败: " + ipStatusText(code)
	}
	// ICMP_ECHO_REPLY.Status 在偏移 4(4 字节);0 = IP_SUCCESS。
	if status := binary.LittleEndian.Uint32(reply[4:8]); status != 0 {
		return false, 0, "ping 回复异常: " + ipStatusText(status)
	}
	rtt := binary.LittleEndian.Uint32(reply[8:12]) // RoundTripTime,ms
	return true, int(rtt), fmt.Sprintf("ping %dms", rtt)
}

// ipStatusText 译码 IcmpSendEcho 的 IP_STATUS 码(GetLastError 与 ICMP_ECHO_REPLY.Status 两路共用)。
func ipStatusText(code uint32) string {
	switch code {
	case 11002:
		return "目标网络不可达(11002)"
	case 11003:
		return "目标主机不可达(11003)" // ARP 无应答/网关无路由,与拥塞丢包病因不同
	case 11005:
		return "目标端口不可达(11005)"
	case 11010:
		return "请求超时(11010)" // 典型拥塞丢包
	case 11013:
		return "传输中 TTL 耗尽(11013)"
	case 11018:
		return "目标地址无效(11018)"
	case 0:
		return "未知错误"
	default:
		return fmt.Sprintf("错误码 %d", code) // 低于 11000 的是 Win32 错误码,不冒充 IP_STATUS
	}
}
