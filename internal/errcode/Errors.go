// Package errcode 定义全局错误码与携带错误码的 error 包装。
//
// 码表与《接口文档.md》第七章一致:0=成功;首位=阶段
// (1 报文解析 / 2 字段校验 / 3 打印目标 / 4 内容渲染 / 5 打印执行 / 9 兜底)。
// 赋码原则:在各层边界按阶段赋码,不匹配错误文本;message 保留中文人读文本。
package errcode

import "errors"

const (
	OK               = 0    // 成功
	ParseFailed      = 1001 // 报文解析失败(payload 非法,id 不可得)
	UnsupportedQuery = 1002 // 不支持的 query 取值(查询消息仅支持 "printers")
	MissingField     = 2001 // 缺少必填字段
	BadSwitch        = 2002 // buzzer/cut/reprint 取值非法
	BadLineRange     = 2003 // headLines/tailLines 超出 0-100
	BadPWidth        = 2004 // pWidth 非法
	BadContentType   = 2005 // 内容类型 type 非 0/1
	NoTarget         = 3001 // 未指定打印目标
	PrinterNotFound  = 3002 // 打印机名称/ID 未命中
	BadUSBTarget     = 3003 // gateway:"usb" 时本机无已登记 USB 打印机
	WidthUnknown     = 3004 // 自动登记无法确定纸宽
	RenderInvalid    = 4001 // 排版内容非法(元素级,message 带 contents[i] 定位)
	RenderNotArray   = 4002 // contents 整体非排版元素数组
	EncodeFailed     = 4003 // GB18030 文本编码失败
	ESCDecodeFailed  = 4004 // type=1 的 ESC 指令解码失败
	NetWriteFailed   = 5001 // 网口写入失败(终态)
	USBSubmitFailed  = 5002 // USB 提交失败(终态)
	LongWaiting      = 5101 // 长期等待告警(waiting,非终态)
	Unknown          = 9001 // 未知错误(兜底)
)

// CodedError 给 error 附加全局错误码;Error() 返回原文本,错误码经 CodeOf 提取。
type CodedError struct {
	Code int
	Err  error
}

func (e *CodedError) Error() string { return e.Err.Error() }
func (e *CodedError) Unwrap() error { return e.Err }

// Wrap 给 err 附加错误码(err 为 nil 时返回 nil)。
func Wrap(code int, err error) error {
	if err == nil {
		return nil
	}
	return &CodedError{Code: code, Err: err}
}

// CodeOf 提取 err 链上携带的错误码;未携带返回 Unknown。
func CodeOf(err error) int {
	var ce *CodedError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return Unknown
}
