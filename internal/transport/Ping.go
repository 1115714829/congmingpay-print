//go:build windows

package transport

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
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
func QueryICMPPing(ip string) PrinterStatus {
	ok, detail := icmpEcho(ip, PingTimeout)
	return PrinterStatus{Reachable: ok, Online: ok, Detail: detail}
}

// icmpEcho 向 ip 发一个 ICMP Echo,timeout 内收到成功回复即返回 true。
func icmpEcho(ip string, timeout time.Duration) (bool, string) {
	host := strings.TrimSpace(ip)
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i] // 去掉可能带的端口
	}
	ip4 := net.ParseIP(host).To4()
	if ip4 == nil {
		return false, "非合法 IPv4,无法 ping"
	}

	h, _, _ := procIcmpCreateFile.Call()
	if h == 0 || h == ^uintptr(0) { // 0 或 INVALID_HANDLE_VALUE
		return false, "IcmpCreateFile 失败"
	}
	defer procIcmpCloseHandle.Call(h)

	dest := uintptr(binary.LittleEndian.Uint32(ip4)) // IPAddr(网络字节序 = 地址字节按内存序)
	req := make([]byte, 32)                           // 32 字节负载,与系统 ping 一致
	// 回复缓冲:sizeof(ICMP_ECHO_REPLY)(32位=28) + 请求长度 + 8,留足取 128。
	reply := make([]byte, 128)
	timeoutMs := uint32(timeout / time.Millisecond)

	n, _, _ := procIcmpSendEcho.Call(
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
		return false, "ping 超时/不可达"
	}
	// ICMP_ECHO_REPLY.Status 在偏移 4(4 字节);0 = IP_SUCCESS。
	if status := binary.LittleEndian.Uint32(reply[4:8]); status != 0 {
		return false, fmt.Sprintf("ping 回复状态码 %d", status)
	}
	rtt := binary.LittleEndian.Uint32(reply[8:12]) // RoundTripTime,ms
	return true, fmt.Sprintf("ping %dms", rtt)
}
