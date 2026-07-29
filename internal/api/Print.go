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

// PrintRequest 打印请求体。
//
// 云盒兼容模式(Settings.YunheCompat)开启时:type=5≡0;五参数可省略;contents 可含 cut。
// 关闭时:仅 type 0/1;五参数必填;拒 contents cut。
//
// 目标二选一:printer 或 gateway。MQTT payload 可为单对象或数组。
type PrintRequest struct {
	Printer PrinterRef `json:"printer"`
	Gateway string     `json:"gateway"`
	ID      uint32     `json:"id"`
	// 0=JSON 排版;1=ESC base64;云盒兼容下 5≡0。
	Type      int             `json:"type"`
	PWidth    int             `json:"pWidth"`
	PCopy     int             `json:"pCopy"`
	Contents  json.RawMessage `json:"contents"`
	Buzzer    *int            `json:"buzzer"`
	Cut       *int            `json:"cut"`
	Reprint   *int            `json:"reprint"`
	HeadLines *int            `json:"headLines"`
	TailLines *int            `json:"tailLines"`
}

// validate 校验字段。compat=false 时五参数必填。
func (r *PrintRequest) validate(compat bool) error {
	if !compat {
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
	}
	if r.Buzzer != nil && *r.Buzzer != 0 && *r.Buzzer != 1 {
		return errcode.Wrap(errcode.BadSwitch, fmt.Errorf("buzzer 只能为 0 或 1"))
	}
	if r.Cut != nil && *r.Cut != 0 && *r.Cut != 1 {
		return errcode.Wrap(errcode.BadSwitch, fmt.Errorf("cut 只能为 0 或 1"))
	}
	if r.Reprint != nil && *r.Reprint != 0 && *r.Reprint != 1 {
		return errcode.Wrap(errcode.BadSwitch, fmt.Errorf("reprint 只能为 0 或 1"))
	}
	if r.HeadLines != nil && (*r.HeadLines < 0 || *r.HeadLines > 100) {
		return errcode.Wrap(errcode.BadLineRange, fmt.Errorf("headLines 需在 0-100 之间"))
	}
	if r.TailLines != nil && (*r.TailLines < 0 || *r.TailLines > 100) {
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
// 云端已传参数覆盖并持久化 → 提交打印。
// 返回受理结果(任务号+目标打印机快照+本单生效参数)与 changed。
// MQTT 与 UI「JSON 测试」共用此逻辑。
func Process(cfg *config.Config, svc *printsvc.Service, req *PrintRequest) (*ProcessResult, bool, error) {
	compat := cfg.Settings.YunheCompat()
	if err := req.validate(compat); err != nil {
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
	var contentCut *bool
	switch req.Type {
	case 0:
		data, contentCut, err = layout.Render(req.Contents, width, compat)
	case 5: // 云盒兼容 C1
		if !compat {
			return nil, registered, errcode.Wrap(errcode.BadContentType, fmt.Errorf("不支持的 type: 5(仅 0=JSON 排版 / 1=ESC base64)"))
		}
		data, contentCut, err = layout.Render(req.Contents, width, true)
	case 1:
		data, err = decodeESC(req.Contents)
	default:
		hint := "仅 0=JSON 排版 / 1=ESC base64"
		if compat {
			hint = "仅 0/5=JSON 排版 / 1=ESC base64"
		}
		return nil, registered, errcode.Wrap(errcode.BadContentType, fmt.Errorf("不支持的 type: %d(%s)", req.Type, hint))
	}
	if err != nil {
		return nil, registered, fmt.Errorf("渲染失败: %w", err)
	}

	upd := config.PrinterUpdate{
		Name:  strings.TrimSpace(req.Printer.Name),
		Brand: model.Brand(strings.TrimSpace(req.Printer.Brand)),
		Width: req.Printer.Width,
	}
	opts := &printsvc.Options{}
	var effBuzzer, effCut, effReprint, effHead, effTail int
	if p.BuzzerEnabled {
		effBuzzer = 1
	}
	if p.Cuts() {
		effCut = 1
	}
	effHead, effTail = p.HeadLines, p.TailLines

	if req.Buzzer != nil {
		b := *req.Buzzer == 1
		opts.Buzzer = &b
		upd.BuzzerEnabled = &b
		effBuzzer = *req.Buzzer
	}
	if req.Cut != nil {
		c := *req.Cut == 1
		opts.Cut = &c
		cd := !c
		upd.CutDisabled = &cd
		effCut = *req.Cut
	} else if compat && contentCut != nil {
		// 云盒兼容 C3: 顶层未传 cut 时用 contents cut;只影响本单,不写回。
		opts.Cut = contentCut
		if *contentCut {
			effCut = 1
		} else {
			effCut = 0
		}
	}
	if req.Reprint != nil {
		r := *req.Reprint == 1
		opts.Reprint = &r
		effReprint = *req.Reprint
	}
	if req.HeadLines != nil {
		h := *req.HeadLines
		opts.HeadLines = &h
		upd.HeadLines = &h
		effHead = h
	}
	if req.TailLines != nil {
		tl := *req.TailLines
		opts.TailLines = &tl
		upd.TailLines = &tl
		effTail = tl
	}

	changed := cfg.UpdatePrinterFromCloud(p.ID, upd)
	if p2 := cfg.FindPrinter(p.ID); p2 != nil {
		p = p2
	}

	cloudID := req.ID
	opts.CloudID = &cloudID
	ct := req.Type
	if ct == 5 {
		ct = 0 // 入库/预览归一为 JSON
	}
	opts.ContentType = &ct
	if ct == 0 {
		opts.SourceJSON = append([]byte(nil), req.Contents...)
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
	return &ProcessResult{
		JobNo: first, Printer: *p,
		Buzzer: effBuzzer, Cut: effCut, Reprint: effReprint,
		HeadLines: effHead, TailLines: effTail,
		PWidth: width, PCopy: copies, ContentType: ct,
	}, registered || changed, nil
}

// ProcessResult 是受理成功的结果:首个任务号 + 目标打印机快照 + 本单生效参数(供 accepted 回执)。
type ProcessResult struct {
	JobNo                int
	Printer              model.Printer
	Buzzer, Cut, Reprint int
	HeadLines, TailLines int
	PWidth, PCopy        int // 实际渲染纸宽 / 份数
	ContentType          int // 0=JSON / 1=ESC(type=5 归一为 0)
}

// resolveTarget 解析打印目标并在需要时自动登记(随打印下发建列表)。
// 优先级:printer.ip > gateway:"usb" > 普通 gateway IP > printer 名/ID。
//   - 有 IP(printer.ip 或 gateway 为网口地址):已注册按 IP 命中则直接用;未注册则 UpsertPrinter 新增。
//   - gateway=="usb":按配置数组序取第一台 Conn=usb;无则 BadUSBTarget;不自动登记、不排序。
//   - 仅名/ID:FindPrinter(网口/USB 通用),未找到报错。
//
// 返回目标打印机与 registered(是否新登记)。
func resolveTarget(cfg *config.Config, req *PrintRequest) (*model.Printer, bool, error) {
	ref := req.Printer
	gw := strings.TrimSpace(req.Gateway)

	// 归一 IP/port:仅 printer.ip;gateway 为 usb 时不得当作 IP。
	ip, port := "", "9100"
	if v := strings.TrimSpace(ref.IP); v != "" {
		ip = v
		if pv := strings.TrimSpace(ref.Port); pv != "" {
			port = pv
		}
	}

	// 1) printer.ip → 网口
	if ip != "" {
		return resolveNetworkTarget(cfg, req, ref, ip, port)
	}

	// 2) gateway:"usb" → 配置序第一台 USB(须在把 gateway 当 IP 之前处理)
	if strings.EqualFold(gw, "usb") {
		for _, p := range cfg.PrinterList() {
			if p.Conn == model.ConnUSB {
				return p, false, nil
			}
		}
		return nil, false, errcode.Wrap(errcode.BadUSBTarget, fmt.Errorf("未找到 USB 打印机，须先在本机添加 USB 打印机"))
	}

	// 3) gateway 为网口地址
	if gw != "" {
		ip = gw
		if i := strings.LastIndex(gw, ":"); i >= 0 {
			ip, port = gw[:i], gw[i+1:]
		}
		return resolveNetworkTarget(cfg, req, ref, ip, port)
	}

	// 4) 仅名/ID 选择器(网口/USB 通用)
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

// resolveNetworkTarget 按网口 IP 命中已登记机或自动登记。
func resolveNetworkTarget(cfg *config.Config, req *PrintRequest, ref PrinterRef, ip, port string) (*model.Printer, bool, error) {
	for _, p := range cfg.PrinterList() {
		if p.Conn == model.ConnNetwork && p.IP == ip {
			return p, false, nil
		}
	}
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
		name = ip
	}
	p, isNew := cfg.UpsertPrinter(&model.Printer{
		Name: name, Brand: model.Brand(strings.TrimSpace(ref.Brand)),
		Width: width, Conn: model.ConnNetwork, IP: ip, Port: port,
	})
	return p, isNew, nil
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
