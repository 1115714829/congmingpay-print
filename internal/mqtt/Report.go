package mqtt

import (
	"time"

	pmqtt "github.com/eclipse/paho.mqtt.golang"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// 本文件是上行「打印结果 / 打印机在线离线」的发布口。
// 消息 type 与字段为本 APP 约定,待与云端最终对齐;全部发往配置的上报主题(best-effort,失败原因落日志)。

// resultMsg 打印结果回执(任务终态:成功/失败各发一条;手动重打再次到终态会再发一条,同 jobNo)。
type resultMsg struct {
	Type     string `json:"type"`     // "result"
	Merchant string `json:"merchant"` // 短商户号(来源身份)
	ID       uint32 `json:"id"`       // 对应打印消息的 id(恒回显)
	JobNo    int    `json:"jobNo"`
	OK       bool   `json:"ok"`      // true=打印成功 false=打印失败
	Printer  string `json:"printer"` // 打印机名称
	Message  string `json:"message"` // 成功="打印成功";失败=原因
	TS       int64  `json:"ts"`
}

// printerState 是单台打印机的在线状态条目(printerStatusMsg.Printers 元素)。
type printerState struct {
	PrinterID string `json:"printerId"`
	Printer   string `json:"printer"`           // 名称
	Conn      string `json:"conn"`              // "network" / "usb"
	IP        string `json:"ip,omitempty"`      // 网口
	Port      string `json:"port,omitempty"`    // 网口(空则为默认 9100)
	USBName   string `json:"usbName,omitempty"` // USB 驱动名
	Online    bool   `json:"online"`
	Detail    string `json:"detail"` // 人类可读描述,如 "就绪(ping 3ms)" / "离线(ping 超时/不可达)"
}

// printerStatusMsg 打印机在线/离线上报:一条消息内用数组承载,支持多设备状态。
// 有状态变动立即上报(当前每次含变动的那台;数组格式为将来批量预留)。
type printerStatusMsg struct {
	Type     string         `json:"type"`     // "printerStatus"
	Merchant string         `json:"merchant"` // 短商户号(来源身份)
	Printers []printerState `json:"printers"`
	TS       int64          `json:"ts"`
}

// printerEntry 是打印机列表上报的单台条目(含每台配置参数;不含在线态——在线以 printerStatus 流为准)。
type printerEntry struct {
	PrinterID string `json:"printerId"`
	Printer   string `json:"printer"` // 名称
	Brand     string `json:"brand"`
	Width     int    `json:"width"`             // 58 / 80
	Conn      string `json:"conn"`              // "network" / "usb"
	IP        string `json:"ip,omitempty"`      // 网口
	Port      string `json:"port,omitempty"`    // 网口(空则为默认 9100)
	USBName   string `json:"usbName,omitempty"` // USB 驱动名
	Buzzer    bool   `json:"buzzer"`            // 每台配置参数(cloud authoritative,随打印消息覆盖)
	Cut       bool   `json:"cut"`
	HeadLines int    `json:"headLines"`
	TailLines int    `json:"tailLines"`
	LastPrint string `json:"lastPrint,omitempty"`
}

// printerListMsg 打印机列表上报:全量快照,服务器以最新一条为准(幂等)。
// 触发:连接成功基线 / 云端登记或参数覆盖 / UI 新增、删除、属性保存 / JSON 测试造成变更。
type printerListMsg struct {
	Type     string         `json:"type"`     // "printerList"
	Merchant string         `json:"merchant"` // 短商户号(来源身份)
	Printers []printerEntry `json:"printers"`
	TS       int64          `json:"ts"`
}

// PublishJobResult 上报一个打印任务的最终结果(成功/失败)。MQTT 未启用时静默跳过。
func (c *Client) PublishJobResult(ev printsvc.JobFinalEvent) {
	cli, merchant, enabled := c.snapshot()
	if !enabled {
		return
	}
	msg := resultMsg{
		Type: "result", Merchant: merchant, JobNo: ev.JobNo, OK: ev.OK,
		Printer: ev.Printer, TS: time.Now().UnixMilli(),
	}
	if ev.CloudID != nil {
		msg.ID = *ev.CloudID
	}
	if ev.OK {
		msg.Message = "打印成功"
	} else {
		msg.Message = ev.Err
	}
	logger.Infof("MQTT 上报打印结果 id=%d 任务#%d ok=%v(%s %s)%s", msg.ID, ev.JobNo, ev.OK, ev.Printer, ev.Target, failSuffix(ev))
	c.publish(cli, msg)
}

// snapshot 取当前客户端、短商户号与启用状态(锁内快照)。
func (c *Client) snapshot() (pmqtt.Client, string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cli, c.merchant, c.enabled
}

func failSuffix(ev printsvc.JobFinalEvent) string {
	if ev.OK {
		return ""
	}
	return ": " + ev.Err
}

// PublishPrinterStatus 上报一台打印机的在线/离线状态(在线布尔边沿触发,含启动首查基线)。
// MQTT 未启用时静默跳过。
func (c *Client) PublishPrinterStatus(p *model.Printer, online bool, detail string) {
	cli, merchant, enabled := c.snapshot()
	if !enabled {
		return
	}
	st := printerState{
		PrinterID: p.ID, Printer: p.Name, Conn: string(p.Conn),
		Online: online, Detail: detail,
	}
	if p.Conn == model.ConnUSB {
		st.USBName = p.USBName
	} else {
		st.IP = p.IP
		st.Port = p.Port
	}
	msg := printerStatusMsg{
		Type: "printerStatus", Merchant: merchant, Printers: []printerState{st}, TS: time.Now().UnixMilli(),
	}
	logger.Infof("MQTT 上报打印机状态 [id=%s] 名称『%s』%s online=%v(%s)", p.ID, p.Name, p.Address(), online, detail)
	c.publish(cli, msg)
}

// PublishPrinterList 上报当前打印机列表全量快照(含每台配置参数)。MQTT 未启用时静默跳过。
// 触发点:连接成功基线 / 云端登记或参数覆盖 / UI 新增、删除、属性保存 / JSON 测试造成变更。
func (c *Client) PublishPrinterList() {
	cli, merchant, enabled := c.snapshot()
	if !enabled {
		return
	}
	// 值拷贝快照:锁内深拷贝,读侧不碰共享指针(与 UI 属性写、云端参数覆盖并发安全)。
	list := c.cfg.PrinterSnapshots()
	entries := make([]printerEntry, 0, len(list))
	for i := range list {
		p := &list[i] // 指向本地副本
		e := printerEntry{
			PrinterID: p.ID, Printer: p.Name, Brand: p.BrandLabel(), Width: p.Width,
			Conn: string(p.Conn), Buzzer: p.BuzzerEnabled, Cut: p.Cuts(),
			HeadLines: p.HeadLines, TailLines: p.TailLines, LastPrint: p.LastPrint,
		}
		if p.Conn == model.ConnUSB {
			e.USBName = p.USBName
		} else {
			e.IP = p.IP
			e.Port = p.Port
		}
		entries = append(entries, e)
	}
	msg := printerListMsg{
		Type: "printerList", Merchant: merchant, Printers: entries, TS: time.Now().UnixMilli(),
	}
	logger.Infof("MQTT 上报打印机列表(%d 台)", len(entries))
	c.publish(cli, msg)
}
