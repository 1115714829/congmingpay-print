//go:build windows

package transport

import (
	"fmt"
	"strings"

	"github.com/alexbrainman/printer"
)

// ListPrinters 返回本机已安装的打印机名称(USB 打印机需先由 Windows 驱动接管才会出现)。
func ListPrinters() ([]string, error) {
	return printer.ReadNames()
}

// IsVirtualPrinter 判断是否为 Windows 虚拟打印机(OneNote/PDF/XPS/Fax),它们不接受 RAW ESC/POS。
func IsVirtualPrinter(name string) bool {
	n := strings.ToLower(name)
	for _, v := range []string{"onenote", "print to pdf", "xps document writer", "fax"} {
		if strings.Contains(n, v) {
			return true
		}
	}
	return false
}

// ListRealPrinters 返回已装打印机中过滤掉虚拟打印机后的名称,供选择真实 USB 打印机。
func ListRealPrinters() ([]string, error) {
	names, err := printer.ReadNames()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range names {
		if !IsVirtualPrinter(n) {
			out = append(out, n)
		}
	}
	return out, nil
}

// SpoolerPrinter 通过 Windows 打印后台以 RAW 方式向已装驱动的打印机发送字节。
// 用于 USB 打印:用户在系统里装好该打印机驱动后,这里按驱动名直发 ESC/POS。
type SpoolerPrinter struct {
	name string
	p    *printer.Printer
}

// NewSpoolerPrinter 按打印机驱动名创建 USB 打印通道。
func NewSpoolerPrinter(name string) *SpoolerPrinter {
	return &SpoolerPrinter{name: name}
}

// Open 打开打印机并开始一个 RAW 打印作业。
func (s *SpoolerPrinter) Open() error {
	if s.name == "" {
		return fmt.Errorf("未选择打印机")
	}
	p, err := printer.Open(s.name)
	if err != nil {
		return fmt.Errorf("打开打印机 %q 失败: %w", s.name, err)
	}
	if err := p.StartRawDocument("congmingpay 测试小票"); err != nil {
		p.Close()
		return fmt.Errorf("开始打印作业失败: %w", err)
	}
	s.p = p
	return nil
}

// Write 向打印作业写入字节。
func (s *SpoolerPrinter) Write(data []byte) error {
	if s.p == nil {
		return fmt.Errorf("打印作业未开始")
	}
	if len(data) == 0 {
		return nil
	}
	_, err := s.p.Write(data)
	return err
}

// Close 结束打印作业并关闭打印机句柄。
func (s *SpoolerPrinter) Close() error {
	if s.p == nil {
		return nil
	}
	endErr := s.p.EndDocument()
	closeErr := s.p.Close()
	s.p = nil
	if endErr != nil {
		return endErr
	}
	return closeErr
}
