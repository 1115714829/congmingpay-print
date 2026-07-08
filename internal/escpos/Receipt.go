package escpos

import (
	"fmt"
	"strings"
	"time"
)

// BuildTestReceipt 生成一张中文测试小票的正文(ESC/POS),按纸宽自适应排版。
// 收尾(首/尾走纸、切纸)由 printsvc 按打印机设置统一追加,这里不含。
func BuildTestReceipt(now time.Time, widthMM int) ([]byte, error) {
	cpl := CharsPerLine(widthMM)
	divider := strings.Repeat("-", cpl)
	return NewBuilder().
		SetAlign(AlignCenter).
		SetSize(1, 1).SetEmphasize(true).
		Line("聪明付 打印测试").
		SetSize(0, 0).SetEmphasize(false).
		Line("congmingpay Print Test").
		SetAlign(AlignLeft).
		Line(divider).
		Line("时间: " + now.Format("2006-01-02 15:04:05")).
		Line(fmt.Sprintf("纸宽: %dmm  (%d 字符/行)", widthMM, cpl)).
		Line("指令: ESC/POS   编码: GB18030").
		Line(divider).
		SetAlign(AlignCenter).
		Line("能清晰打印本票并正确切纸").
		Line("即表示该打印通道正常").
		Feed(1).
		Line("扫码验证二维码").
		QRCode("congmingpay QR "+now.Format("15:04:05"), 6).
		Bytes()
}
