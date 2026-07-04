// Package transport 提供打印通道抽象:USB(经已装 Windows 驱动)与网口(IP:9100)。
//
// 品牌差异收敛在本层之下:任何 ESC/POS 打印机都通过同一接口发送字节。
package transport

import "errors"

// ConnError 表示"连不上打印机"(网口拨号失败/超时等)。用它区分于渲染/数据类错误:
// 连接类错误由 printsvc 转为"等待重试",堆在任务列表,网络恢复后自动打出。
type ConnError struct{ Err error }

func (e *ConnError) Error() string { return e.Err.Error() }
func (e *ConnError) Unwrap() error { return e.Err }

// IsConnError 判断 err 是否为连接类错误(可等待重试)。
func IsConnError(err error) bool {
	var c *ConnError
	return errors.As(err, &c)
}

// Printer 是一条打印通道的统一接口。
type Printer interface {
	Open() error
	Write(data []byte) error
	Close() error
}

// Print 打开通道、写入数据、关闭通道,是一次性打印的便捷封装。
func Print(p Printer, data []byte) error {
	if err := p.Open(); err != nil {
		return err
	}
	defer p.Close()
	return p.Write(data)
}
