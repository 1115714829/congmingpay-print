package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"congmingpay/internal/config"
	"congmingpay/internal/errcode"
	"congmingpay/internal/layout"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// PrinterRef 是打印目标的引用,可为**字符串**(打印机名称/ID)或**对象**(携带身份用于自动登记)。
//
//	"printer": "飞蛾1"                                                   // 选已注册的机(网口/USB 通用)
//	"printer": {"name":"飞蛾1","ip":"192.168.68.128","brand":"飞蛾","width":80}  // 带身份;未注册则自动登记入库
type PrinterRef struct {
	Name  string `json:"name"`
	ID    string `json:"id"`
	IP    string `json:"ip"`
	Port  string `json:"port"`
	Brand string `json:"brand"`
	Width int    `json:"width"`
}

// UnmarshalJSON 支持两种写法:字符串 → 当作 Name(名称/ID 选择器);对象 → 读各身份字段。
func (p *PrinterRef) UnmarshalJSON(b []byte) error {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || string(t) == "null" {
		return nil
	}
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err != nil {
			return err
		}
		p.Name = strings.TrimSpace(s)
		return nil
	}
	type alias PrinterRef // 别名无方法,避免递归调用本 UnmarshalJSON
	var a alias
	if err := json.Unmarshal(t, &a); err != nil {
		return err
	}
	*p = PrinterRef(a)
	return nil
}

// Empty 表示未指定任何目标身份。
func (p PrinterRef) Empty() bool {
	return p.Name == "" && p.ID == "" && p.IP == ""
}

// PrintRequest 打印请求体。buzzer/cut/reprint/headLines/tailLines 为**必填字段**(缺一即拒,无降级)。
//
// 目标二选一:printer(名称/ID 字符串,或 {name,ip,brand,width} 对象)或 gateway(网口 IP)。
// USB 打印机以 printer 指定名称或 ID(与状态快照上报的 printer/printerId 对应)。
// 目标未注册且带了 IP(printer.ip 或 gateway)→ **自动登记入库**(随打印建列表)再打印。
// MQTT 打印消息的 payload 可为单个对象(打一台)或对象数组(一次打多台,并发执行)。
//
// 用例(gateway 指定网口 IP,单台):
//
//	{
//	  "gateway": "192.168.0.89",
//	  "id": 1295873547,
//	  "type": 0,
//	  "pWidth": 80,
//	  "pCopy": 1,
//	  "buzzer": 0, "cut": 1, "reprint": 0, "headLines": 0, "tailLines": 0,
//	  "contents": [
//	    {"cont":"收款小票","type":"title"},
//	    {"both_sides":["金额","99.00"]},
//	    {"cont":"https://congmingpay","type":"qrcode","align":"center"}
//	  ]
//	}
//
// 用例(printer 对象,未注册则自动登记):
//
//	{"printer":{"name":"飞蛾1","ip":"192.168.68.128","brand":"飞蛾","width":80},"type":0,"pWidth":80,
//	 "buzzer":0,"cut":1,"reprint":0,"headLines":0,"tailLines":0,"contents":[...]}
//
// 用例(多台,数组):[{"printer":{...},...必填字段...}, {"printer":{...},...}]
type PrintRequest struct {
	// 打印目标(与 gateway 二选一):字符串=打印机名/ID(网口/USB 通用);对象={name,ip,brand,width},未注册则自动登记入库。
	Printer PrinterRef `json:"printer"`
	// 打印目标网关(与 printer 二选一):网口 IP(如 "192.168.0.89",未注册则按 IP 自动登记入库)。USB 打印以 printer 指定名称。
	Gateway string `json:"gateway"`
	// 任务 ID:仅用于回执与日志对应,不用于去重(每条消息都会处理并打印)。
	ID uint32 `json:"id"`
	// 排版类型(默认 0):0=JSON 排版(contents 为排版元素数组,由服务端渲染);1=原始 ESC/POS(contents 为 base64 字符串,解码后直接下发)。
	Type int `json:"type"`
	// 纸宽 mm:58 或 80(可选,默认按打印机设置)。
	PWidth int `json:"pWidth"`
	// 份数(可选,默认 1)。
	PCopy int `json:"pCopy"`
	// 打印内容:type=0 为排版元素数组;type=1 为 base64 字符串(原始 ESC/POS)。切纸/走纸由顶层 cut、tailLines 统一处理,contents 内不接受切纸指令。
	Contents json.RawMessage `json:"contents"`
	// 蜂鸣:0=关 1=开(必填)。
	Buzzer *int `json:"buzzer"`
	// 切刀:0=关 1=开(必填)。
	Cut *int `json:"cut"`
	// 重打:0=普通,1=带醒目「重打」抬头(必填)。
	Reprint *int `json:"reprint"`
	// 首部空行:0=默认(必填)。
	HeadLines *int `json:"headLines"`
	// 尾部空行:0=默认(必填)。
	TailLines *int `json:"tailLines"`
}

// validate 校验必填字段(无降级):buzzer/cut/reprint/headLines/tailLines 均必传且合法。
func (r *PrintRequest) validate() error {
	var miss []string
	if r.Buzzer == nil {
		miss = append(miss, "buzzer")
	}
	if r.Cut == nil {
		miss = append(miss, "cut")
	}
	if r.Reprint == nil {
		miss = append(miss, "reprint")
	}
	if r.HeadLines == nil {
		miss = append(miss, "headLines")
	}
	if r.TailLines == nil {
		miss = append(miss, "tailLines")
	}
	if len(miss) > 0 {
		return errcode.Wrap(errcode.MissingField, fmt.Errorf("缺少必填字段: %s(buzzer/cut/reprint/headLines/tailLines 均必传)", strings.Join(miss, "、")))
	}
	if *r.Buzzer != 0 && *r.Buzzer != 1 {
		return errcode.Wrap(errcode.BadSwitch, fmt.Errorf("buzzer 只能为 0 或 1"))
	}
	if *r.Cut != 0 && *r.Cut != 1 {
		return errcode.Wrap(errcode.BadSwitch, fmt.Errorf("cut 只能为 0 或 1"))
	}
	if *r.Reprint != 0 && *r.Reprint != 1 {
		return errcode.Wrap(errcode.BadSwitch, fmt.Errorf("reprint 只能为 0 或 1"))
	}
	// 范围校验(全部严格,不合规直接拒,无降级):
	if *r.HeadLines < 0 || *r.HeadLines > 100 {
		return errcode.Wrap(errcode.BadLineRange, fmt.Errorf("headLines 需在 0-100 之间"))
	}
	if *r.TailLines < 0 || *r.TailLines > 100 {
		return errcode.Wrap(errcode.BadLineRange, fmt.Errorf("tailLines 需在 0-100 之间"))
	}
	if r.PWidth != 0 && r.PWidth != 58 && r.PWidth != 80 {
		return errcode.Wrap(errcode.BadPWidth, fmt.Errorf("pWidth 需为 58 或 80(或不填)"))
	}
	return nil
}

// ParseRequests 解析打印消息 body:支持单个对象或对象数组。
// 返回请求切片、是否为数组、错误。UI「JSON 测试」亦复用。
func ParseRequests(raw []byte) ([]PrintRequest, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false, fmt.Errorf("空请求体")
	}
	if trimmed[0] == '[' {
		var arr []PrintRequest
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, true, err
		}
		if len(arr) == 0 {
			return nil, true, fmt.Errorf("空数组")
		}
		return arr, true, nil
	}
	var one PrintRequest
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return nil, false, err
	}
	return []PrintRequest{one}, false, nil
}

// Process 处理一个打印请求:解析目标(printer/gateway,未注册自动登记)→ 渲染 →
// **云端参数覆盖并持久化到目标打印机(cloud authoritative)** → 提交打印。
// 返回受理结果(任务号+目标打印机快照)与 changed(注册表有变化=新登记或参数被覆盖,供调用方 save+刷新 UI)。
// MQTT 与 UI「JSON 测试」共用此逻辑。
func Process(cfg *config.Config, svc *printsvc.Service, req *PrintRequest) (*ProcessResult, bool, error) {
	// 必填校验(无降级):缺蜂鸣/切刀/重试/头尾行即拒,不回退打印机默认、不自动登记。
	if err := req.validate(); err != nil {
		return nil, false, err
	}
	p, registered, err := resolveTarget(cfg, req)
	if err != nil {
		return nil, false, err
	}
	width := p.Width
	if req.PWidth == 58 || req.PWidth == 80 {
		width = req.PWidth
	}

	var data []byte
	switch req.Type {
	case 0:
		data, err = layout.Render(req.Contents, width)
	case 1:
		data, err = decodeESC(req.Contents)
	default:
		return nil, registered, errcode.Wrap(errcode.BadContentType, fmt.Errorf("不支持的 type: %d(仅 0=JSON 排版 / 1=ESC base64)", req.Type))
	}
	if err != nil {
		return nil, registered, fmt.Errorf("渲染失败: %w", err) // %w 保留链上的错误码
	}

	// 由必填字段生成生效参数(蜂鸣/切刀 0/1→bool;reprint 0/1→是否带重打抬头;头/尾空行透传)。
	buzzer, cut := *req.Buzzer == 1, *req.Cut == 1
	reprint := *req.Reprint == 1
	head, tail := *req.HeadLines, *req.TailLines

	// cloud authoritative:云端下发的参数 + printer 身份覆盖并持久化到目标打印机(reprint 是每单行为,不持久化)。
	changed := cfg.UpdatePrinterFromCloud(p.ID, config.PrinterUpdate{
		Name:          strings.TrimSpace(req.Printer.Name),
		Brand:         model.Brand(strings.TrimSpace(req.Printer.Brand)),
		Width:         req.Printer.Width,
		BuzzerEnabled: buzzer,
		CutDisabled:   !cut,
		HeadLines:     head,
		TailLines:     tail,
	})

	cloudID := req.ID // 值拷贝,任务终态事件回携此 id 上报打印结果
	opts := &printsvc.Options{
		Cut: &cut, Buzzer: &buzzer, Reprint: &reprint,
		HeadLines: &head, TailLines: &tail, CloudID: &cloudID,
	}
	copies := req.PCopy
	if copies <= 0 {
		copies = 1
	}
	first := 0
	for i := 0; i < copies; i++ {
		no := svc.Submit(p, "打印", data, opts)
		if i == 0 {
			first = no
		}
	}
	return &ProcessResult{JobNo: first, Printer: *p}, registered || changed, nil
}

// ProcessResult 是受理成功的结果:首个任务号 + 目标打印机快照(值拷贝,供回执携带身份)。
type ProcessResult struct {
	JobNo   int
	Printer model.Printer
}

// resolveTarget 解析打印目标并在需要时自动登记(随打印下发建列表)。
// 目标由 printer(名/ID 字符串 或 {name,ip,brand,width} 对象)与 gateway(网口 IP)归一得出:
//   - 有 IP(printer.ip 或 gateway 是 IP):已注册按 IP 命中则直接用、不改;未注册则 UpsertPrinter 新增。
//   - 仅名/ID:FindPrinter(网口/USB 通用),未找到报错(无地址无法登记)。
//   - gateway=="usb" 拒绝:USB 打印以 printer 指定名称(与状态快照上报的 printer/printerId 对应)。
//
// 返回目标打印机与 registered(是否新登记)。
func resolveTarget(cfg *config.Config, req *PrintRequest) (*model.Printer, bool, error) {
	ref := req.Printer
	gw := strings.TrimSpace(req.Gateway)

	// gateway 仅承载网口 IP;"usb" 不是合法目标(USB 机必须按名称/ID 指定)
	if strings.EqualFold(gw, "usb") {
		return nil, false, errcode.Wrap(errcode.BadUSBTarget, fmt.Errorf("USB 打印以 printer 指定打印机名称(gateway 仅支持网口 IP)"))
	}

	// 归一 IP/port:printer.ip 优先,否则 gateway 为 IP(可含 :port)
	ip, port := "", "9100"
	if v := strings.TrimSpace(ref.IP); v != "" {
		ip = v
		if pv := strings.TrimSpace(ref.Port); pv != "" {
			port = pv
		}
	} else if gw != "" {
		ip = gw
		if i := strings.LastIndex(gw, ":"); i >= 0 {
			ip, port = gw[:i], gw[i+1:]
		}
	}

	// 1) 有 IP → 网口:已注册直接用(不动),未注册自动登记
	if ip != "" {
		for _, p := range cfg.PrinterList() {
			if p.Conn == model.ConnNetwork && p.IP == ip {
				return p, false, nil
			}
		}
		// 自动登记必须能确定纸宽(printer.width → pWidth 回退链;两者皆缺/非法即拒,不兜底 80)
		width := ref.Width
		if width != 58 && width != 80 {
			if req.PWidth == 58 || req.PWidth == 80 {
				width = req.PWidth
			} else {
				return nil, false, errcode.Wrap(errcode.WidthUnknown, fmt.Errorf("无法确定纸宽: 自动登记新打印机需 printer.width 或 pWidth 为 58/80(收到 printer.width=%d、pWidth=%d)", ref.Width, req.PWidth))
			}
		}
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			name = ip // 没带名字就用 IP
		}
		p, isNew := cfg.UpsertPrinter(&model.Printer{
			Name: name, Brand: model.Brand(strings.TrimSpace(ref.Brand)),
			Width: width, Conn: model.ConnNetwork, IP: ip, Port: port,
		})
		return p, isNew, nil
	}

	// 2) 仅名/ID 选择器(网口/USB 通用;USB 机唯一的云端寻址方式)
	if key := strings.TrimSpace(ref.Name); key != "" {
		if p := cfg.FindPrinter(key); p != nil {
			return p, false, nil
		}
		return nil, false, errcode.Wrap(errcode.PrinterNotFound, fmt.Errorf("未找到打印机: %s", key))
	}
	if key := strings.TrimSpace(ref.ID); key != "" {
		if p := cfg.FindPrinter(key); p != nil {
			return p, false, nil
		}
		return nil, false, errcode.Wrap(errcode.PrinterNotFound, fmt.Errorf("未找到打印机: %s", key))
	}

	return nil, false, errcode.Wrap(errcode.NoTarget, fmt.Errorf("未指定打印目标(printer 或 gateway 至少填一个)"))
}

// decodeESC 把 type=1 的 contents(base64 字符串)解码为原始 ESC 字节。
func decodeESC(contents json.RawMessage) ([]byte, error) {
	var s string
	if err := json.Unmarshal(contents, &s); err != nil {
		return nil, errcode.Wrap(errcode.ESCDecodeFailed, err)
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, errcode.Wrap(errcode.ESCDecodeFailed, err)
	}
	return data, nil
}
