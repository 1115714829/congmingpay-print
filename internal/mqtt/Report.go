package mqtt

import (
	"time"

	pmqtt "github.com/eclipse/paho.mqtt.golang"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// 本文件是两类上行消息(report/state)的结构与发布口。
// 消息 type 与字段为本服务与服务端的约定;全部发往配置的上报主题(best-effort,失败原因落日志)。

// eventAccepted 是受理成功事件(在 mqtt 层产生;done/failed/waiting 由 printsvc 事件产生)。
const eventAccepted = "accepted"

// reportPrinter 是 report 消息中的打印机身份段。
type reportPrinter struct {
	PrinterID string `json:"printerId"`
	Printer   string `json:"printer"` // 名称
	Brand     string `json:"brand"`
	Width     int    `json:"width"`
	Conn      string `json:"conn"`              // "network" / "usb"
	IP        string `json:"ip,omitempty"`      // 网口
	Port      string `json:"port,omitempty"`    // 网口
	USBName   string `json:"usbName,omitempty"` // USB 驱动名
}

// reportParams 是 report(accepted)消息携带的本单生效参数。
type reportParams struct {
	Buzzer      int `json:"buzzer"`
	Cut         int `json:"cut"`
	Reprint     int `json:"reprint"`
	HeadLines   int `json:"headLines"`
	TailLines   int `json:"tailLines"`
	PWidth      int `json:"pWidth"` // 实际渲染纸宽(58/80)
	PCopy       int `json:"pCopy"`  // 实际打印份数(≥1)
	ContentType int `json:"contentType"`
}

// reportMsg 打印回执:event=accepted/waiting/done/failed,统一结构。
type reportMsg struct {
	Type     string         `json:"type"` // "report"
	Merchant string         `json:"merchant"`
	Event    string         `json:"event"`
	ID       uint32         `json:"id"` // 请求 id,恒回显(整条解析失败时为 0)
	JobNo    int            `json:"jobNo,omitempty"`
	Code     int            `json:"code"` // 0=成功;非 0 见接口文档错误码表
	Message  string         `json:"message"`
	Printer  *reportPrinter `json:"printer,omitempty"` // 目标已解析时携带
	Params   *reportParams  `json:"params,omitempty"`  // 仅 accepted 携带
	TS       int64          `json:"ts"`
}

// statePrinter 是 state 消息中的单台打印机条目(配置参数 + 在线状态合一)。
type statePrinter struct {
	PrinterID string `json:"printerId"`
	Printer   string `json:"printer"`
	Brand     string `json:"brand"`
	Width     int    `json:"width"`
	Conn      string `json:"conn"`
	IP        string `json:"ip,omitempty"`
	Port      string `json:"port,omitempty"`
	USBName   string `json:"usbName,omitempty"`
	Online    bool   `json:"online"`
	Detail    string `json:"detail,omitempty"`
	Buzzer    bool   `json:"buzzer"`
	Cut       bool   `json:"cut"`
	HeadLines int    `json:"headLines"`
	TailLines int    `json:"tailLines"` // 尾部走纸偏移量(本地属性页允许负值)
	LastPrint string `json:"lastPrint,omitempty"`
}

// stateMsg 状态快照:服务在线 + 全部打印机全量;离线为简版(printers 省略,遗嘱同)。
type stateMsg struct {
	Type     string         `json:"type"` // "state"
	Merchant string         `json:"merchant"`
	Online   bool           `json:"online"`
	Printers []statePrinter `json:"printers,omitempty"`
	TS       int64          `json:"ts,omitempty"`
}

// reportPrinterOf 由打印机快照构造 report 身份段。
func reportPrinterOf(p *model.Printer) *reportPrinter {
	rp := &reportPrinter{
		PrinterID: p.ID, Printer: p.Name, Brand: p.BrandLabel(), Width: p.Width, Conn: string(p.Conn),
	}
	if p.Conn == model.ConnUSB {
		rp.USBName = p.USBName
	} else {
		rp.IP, rp.Port = p.IP, p.Port
	}
	return rp
}

// snapshot 取当前客户端、短商户号与启用状态(锁内快照)。
func (c *Client) snapshot() (pmqtt.Client, string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cli, c.merchant, c.enabled
}

// PublishJobEvent 上报任务事件(done/failed/waiting → report)。MQTT 未启用时静默跳过。
func (c *Client) PublishJobEvent(ev printsvc.JobEvent) {
	cli, merchant, enabled := c.snapshot()
	if !enabled {
		return
	}
	msg := reportMsg{
		Type: "report", Merchant: merchant, Event: ev.Event,
		JobNo: ev.JobNo, Code: ev.Code, TS: time.Now().UnixMilli(),
		Printer: reportPrinterOf(&ev.Printer),
	}
	if ev.CloudID != nil {
		msg.ID = *ev.CloudID
	}
	if ev.Event == printsvc.EventDone {
		msg.Message = "打印成功"
	} else {
		msg.Message = ev.Err
	}
	logger.Infof("MQTT 上报 report event=%s id=%d 任务#%d code=%d(%s %s)", ev.Event, msg.ID, ev.JobNo, ev.Code, ev.Printer.Name, ev.Printer.Target())
	c.publish(cli, msg)
}

// PublishState 上报状态快照(服务在线 + 全部打印机配置与在线态,全量幂等)。MQTT 未启用时静默跳过。
func (c *Client) PublishState() {
	cli, merchant, enabled := c.snapshot()
	if !enabled {
		return
	}
	list := c.cfg.PrinterSnapshots() // 锁内值拷贝,与 UI/云端写并发安全
	online := c.svc.OnlineSnapshot()
	printers := make([]statePrinter, 0, len(list))
	for i := range list {
		p := &list[i]
		sp := statePrinter{
			PrinterID: p.ID, Printer: p.Name, Brand: p.BrandLabel(), Width: p.Width, Conn: string(p.Conn),
			Buzzer: p.BuzzerEnabled, Cut: p.Cuts(), HeadLines: p.HeadLines, TailLines: p.TailLines, LastPrint: p.LastPrint,
		}
		if p.Conn == model.ConnUSB {
			sp.USBName = p.USBName
		} else {
			sp.IP, sp.Port = p.IP, p.Port
		}
		if oi, ok := online[p.ID]; ok {
			sp.Online, sp.Detail = oi.Online, oi.Detail
		} else {
			// 启动后首次判定尚未完成(监测未定态不写注册表):按离线报告,detail 标「检测中」
			sp.Detail = "检测中"
		}
		printers = append(printers, sp)
	}
	msg := stateMsg{Type: "state", Merchant: merchant, Online: true, Printers: printers, TS: time.Now().UnixMilli()}
	logger.Infof("MQTT 上报 state(%d 台打印机)", len(printers))
	c.publish(cli, msg)
}
