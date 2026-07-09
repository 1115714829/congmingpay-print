// Package model 定义应用核心数据类型:打印机、任务、设置。
package model

import "fmt"

// Conn 是打印机连接方式。
type Conn string

const (
	ConnNetwork Conn = "network" // 网口 IP:port
	ConnUSB     Conn = "usb"     // USB(经 Windows 打印驱动)
)

// Brand 是打印机品牌。佳博与飞蛾走同一套 ESC/POS 协议;其他也按 ESC/POS 处理。
// 品牌当前仅作元数据 + 未来品牌差异适配预留。
type Brand string

const (
	BrandGprinter Brand = "佳博"
	BrandFeie     Brand = "飞蛾"
	BrandOther    Brand = "其他"
)

// Brands 是下拉可选品牌顺序。
var Brands = []Brand{BrandGprinter, BrandFeie, BrandOther}

// BrandForIndex 按下拉索引返回品牌,越界回退佳博。
func BrandForIndex(i int) Brand {
	if i < 0 || i >= len(Brands) {
		return BrandGprinter
	}
	return Brands[i]
}

// IndexOfBrand 返回品牌在下拉中的索引,未匹配回退 0。
func IndexOfBrand(b Brand) int {
	for i, x := range Brands {
		if x == b {
			return i
		}
	}
	return 0
}

// Printer 是一台已注册的打印机。
type Printer struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Brand         Brand  `json:"brand"`
	Width         int    `json:"width"` // 58 或 80
	Conn          Conn   `json:"conn"`
	IP            string `json:"ip"`      // 网口地址
	Port          string `json:"port"`    // 网口端口,默认 9100
	USBName       string `json:"usbName"` // USB:绑定的 Windows 打印机驱动名
	BuzzerEnabled bool   `json:"buzzer"`    // 打印时是否蜂鸣提示
	HeadLines     int    `json:"headLines"`   // 内容前空行数(走纸)
	TailLines     int    `json:"tailLines"`   // 尾部空行相对基数的偏移(可负;实际=escpos.TailFeed)
	CutDisabled   bool   `json:"cutDisabled"` // true=不切纸(该机无切刀);反向字段,默认 false=启用切刀
	LastPrint     string `json:"lastPrint"`
}

// Cuts 返回是否在打印末尾切纸(反向字段:零值即启用切刀,缺省切纸)。
func (p *Printer) Cuts() bool { return !p.CutDisabled }

// BrandLabel 返回品牌展示文本,空值回退「其他」。
func (p *Printer) BrandLabel() string {
	if p.Brand == "" {
		return string(BrandOther)
	}
	return string(p.Brand)
}

// ConnLabel 返回连接方式的中文标签。
func (p *Printer) ConnLabel() string {
	if p.Conn == ConnUSB {
		return "USB"
	}
	return "网口"
}

// Address 返回「地址 / 设备」列的展示文本。
func (p *Printer) Address() string {
	if p.Conn == ConnUSB {
		if p.USBName != "" {
			return p.USBName
		}
		return "USB 直连"
	}
	return p.IP + ":" + p.Port
}

// WidthLabel 返回规格标签,如 "80mm"。
func (p *Printer) WidthLabel() string {
	return fmt.Sprintf("%dmm", p.Width)
}

// Target 返回打印目标的展示文本,用于日志排查。
func (p *Printer) Target() string {
	if p.Conn == ConnUSB {
		return "USB『" + p.USBName + "』"
	}
	port := p.Port
	if port == "" {
		port = "9100"
	}
	return p.IP + ":" + port
}
