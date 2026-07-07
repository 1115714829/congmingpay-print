// Package transport 提供打印通道抽象:USB(经已装 Windows 驱动)与网口(IP:9100)。
//
// 品牌差异收敛在本层之下:任何 ESC/POS 打印机都通过同一接口发送字节。
package transport

import "errors"

// ConnError 表示"连不上打印机"(网口拨号失败/超时等)。用它区分于渲染/数据类错误:
// 连接类错误由 printsvc 转为"等待重试",堆在任务列表,网络恢复后自动打出。
//
// Refused=true 特指"连接被拒绝"(RST):主机在但 9100 未开放/打印机未就绪/被别的客户端占用。
// 这不同于"超时/不可达"——由 printsvc 走"占用短退避"排队等待(占用释放/端口就绪即打),
// 而超时/不可达走"离线退避"(联通即打);两者都不失败。
type ConnError struct {
	Err     error
	Refused bool // 端口拒绝(RST):主机可达但 9100 关闭/未就绪/被占用
}

func (e *ConnError) Error() string { return e.Err.Error() }
func (e *ConnError) Unwrap() error { return e.Err }

// IsConnError 判断 err 是否为连接类错误(可等待重试)。
func IsConnError(err error) bool {
	var c *ConnError
	return errors.As(err, &c)
}

// IsRefused 判断 err 是否为"连接被拒绝"(端口关/未就绪),供调用方与超时/不可达区分处理。
func IsRefused(err error) bool {
	var c *ConnError
	return errors.As(err, &c) && c.Refused
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
