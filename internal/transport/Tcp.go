package transport

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"
)

// RawPrintPort 是热敏/票据打印机的标准 RAW 打印端口(JetDirect/AppSocket)。
const RawPrintPort = "9100"

// wsaeConnRefused 是 Windows WSAECONNREFUSED 的数值(10061)。
// 【实测依据】Windows 上 net.DialTimeout 拨到关闭端口,底层 syscall.Errno == 10061,
// 但 syscall.ECONNREFUSED 在 Windows 是另一个 sentinel(≠10061),errors.Is 命不中——故直接比数值。
const wsaeConnRefused syscall.Errno = 10061

// isConnRefused 判断拨号错误是否为"连接被拒绝"(RST:主机在但端口关)。
// 跨平台:Linux/macOS 命中 syscall.ECONNREFUSED(111);Windows 命中 WSAECONNREFUSED(10061)。
func isConnRefused(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var se syscall.Errno
	return errors.As(err, &se) && se == wsaeConnRefused
}

// TCPPrinter 通过 TCP 向网口打印机直发字节(默认端口 9100)。
type TCPPrinter struct {
	addr string
	conn net.Conn
}

// NewTCPPrinter 创建网口打印通道。ip 可为 "192.168.1.50" 或含端口的 "host:port"。
func NewTCPPrinter(ip string) *TCPPrinter {
	addr := strings.TrimSpace(ip)
	if addr != "" && !strings.Contains(addr, ":") {
		addr += ":" + RawPrintPort
	}
	return &TCPPrinter{addr: addr}
}

// Open 建立到打印机的 TCP 连接。
func (t *TCPPrinter) Open() error {
	if t.addr == "" {
		return fmt.Errorf("未填写打印机 IP")
	}
	conn, err := net.DialTimeout("tcp", t.addr, 5*time.Second)
	if err != nil {
		// 连接被拒绝(RST):主机在但 9100 关闭/未就绪/被别的客户端占用 → 标记 Refused,
		// printsvc 据此走"占用短退避"排队等待(不失败),占用释放/端口就绪即打。
		if isConnRefused(err) {
			return &ConnError{Refused: true, Err: fmt.Errorf("连接打印机 %s 被拒绝(打印口未开放/打印机未就绪): %w", t.addr, err)}
		}
		// 其余(超时/不可达/网络中断)= 连接类错误 → printsvc 转为"等待重试",而非直接失败。
		return &ConnError{Err: fmt.Errorf("连接打印机 %s 失败: %w", t.addr, err)}
	}
	t.conn = conn
	return nil
}

// Write 向打印机发送字节。
func (t *TCPPrinter) Write(data []byte) error {
	if t.conn == nil {
		return fmt.Errorf("连接未建立")
	}
	_, err := t.conn.Write(data)
	return err
}

// Close 关闭连接。
func (t *TCPPrinter) Close() error {
	if t.conn == nil {
		return nil
	}
	err := t.conn.Close()
	t.conn = nil
	return err
}
