package transport

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// RawPrintPort 是热敏/票据打印机的标准 RAW 打印端口(JetDirect/AppSocket)。
const RawPrintPort = "9100"

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
		// 拨号失败 = 连接类错误 → printsvc 转为"等待重试",而非直接失败。
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
