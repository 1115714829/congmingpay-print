package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"congmingpay/internal/config"
	"congmingpay/internal/layout"
	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// PrinterRef 是打印目标的引用,可为**字符串**(打印机名称/ID)或**对象**(携带身份用于自动登记)。
//
//	"printer": "飞蛾1"                                                   // 选已注册的机
//	"printer": {"name":"飞蛾1","ip":"192.168.68.128","brand":"飞蛾","width":80}  // 带身份;未注册则自动登记入库
type PrinterRef struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	IP      string `json:"ip"`
	Port    string `json:"port"`
	USBName string `json:"usbName"`
	Brand   string `json:"brand"`
	Width   int    `json:"width"`
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
	return p.Name == "" && p.ID == "" && p.IP == "" && p.USBName == ""
}

// PrintRequest 打印请求体。buzzer/cut/retry/headLines/tailLines 为**必填字段**(缺一即拒,无降级)。
//
// 目标二选一:printer(名称/ID 字符串,或 {name,ip,brand,width} 对象)或 gateway(IP 或 "usb")。
// 目标未注册且带了 IP(printer.ip 或 gateway)→ **自动登记入库**(随打印建列表)再打印。
// POST /api/print 的 body 可为单个对象(打一台)或对象数组(一次打多台,并发执行)。
//
// 用例(gateway 指定网口 IP,单台):
//
//	{
//	  "gateway": "192.168.0.89",
//	  "id": 1295873547,
//	  "type": 5,
//	  "pWidth": 80,
//	  "pCopy": 1,
//	  "buzzer": 0, "cut": 1, "retry": 1, "headLines": 0, "tailLines": 0,
//	  "contents": [
//	    {"cont":"收款小票","type":"title"},
//	    {"both_sides":["金额","99.00"]},
//	    {"cont":"https://congmingpay","type":"qrcode","align":"center"},
//	    {"cont":"1","type":"cut"}
//	  ]
//	}
//
// 用例(printer 对象,未注册则自动登记):
//
//	{"printer":{"name":"飞蛾1","ip":"192.168.68.128","brand":"飞蛾","width":80},"type":5,"pWidth":80,
//	 "buzzer":0,"cut":1,"retry":1,"headLines":0,"tailLines":0,"contents":[...]}
//
// 用例(多台,数组):[{"printer":{...},...必填字段...}, {"printer":{...},...}]
type PrintRequest struct {
	// 打印目标(与 gateway 二选一):字符串=打印机名/ID;对象={name,ip,brand,width},未注册则自动登记入库。
	Printer PrinterRef `json:"printer" swaggertype:"string" example:"飞蛾1"`
	// 打印目标网关(与 printer 二选一):网口 IP(如 "192.168.0.89",未注册则按 IP 自动登记入库)或 "usb"(首台 USB 机)。
	Gateway string `json:"gateway" example:"192.168.0.89"`
	// 任务 ID:用于防重打(同 id 只打一次)。建议每单唯一;0 表示不防重。
	ID uint32 `json:"id" example:"1001"`
	// 排版类型:5=JSON 排版(建议),1=ESC 原始指令(base64 字符串)。
	Type int `json:"type" example:"5"`
	// 纸宽 mm:58 或 80(可选,默认按打印机设置)。
	PWidth int `json:"pWidth" example:"80"`
	// 份数(可选,默认 1)。
	PCopy int `json:"pCopy" example:"1"`
	// 打印内容:type=5 为排版元素数组;type=1 为 base64 字符串。
	Contents json.RawMessage `json:"contents" swaggertype:"object"`
	// 蜂鸣:0=关 1=开(必填)。
	Buzzer *int `json:"buzzer" example:"0"`
	// 切刀:0=关 1=开(必填)。
	Cut *int `json:"cut" example:"1"`
	// 重试次数:0=不重试,N=失败重试 N 次(必填)。
	Retry *int `json:"retry" example:"1"`
	// 首部空行:0=默认(必填)。
	HeadLines *int `json:"headLines" example:"0"`
	// 尾部空行:0=默认(必填)。
	TailLines *int `json:"tailLines" example:"0"`
}

// validate 校验必填字段(无降级):buzzer/cut/retry/headLines/tailLines 均必传。
func (r *PrintRequest) validate() error {
	var miss []string
	if r.Buzzer == nil {
		miss = append(miss, "buzzer")
	}
	if r.Cut == nil {
		miss = append(miss, "cut")
	}
	if r.Retry == nil {
		miss = append(miss, "retry")
	}
	if r.HeadLines == nil {
		miss = append(miss, "headLines")
	}
	if r.TailLines == nil {
		miss = append(miss, "tailLines")
	}
	if len(miss) > 0 {
		return fmt.Errorf("缺少必填字段: %s(buzzer/cut/retry/headLines/tailLines 均必传)", strings.Join(miss, "、"))
	}
	if *r.Buzzer != 0 && *r.Buzzer != 1 {
		return fmt.Errorf("buzzer 只能为 0 或 1")
	}
	if *r.Cut != 0 && *r.Cut != 1 {
		return fmt.Errorf("cut 只能为 0 或 1")
	}
	if *r.Retry < 0 {
		return fmt.Errorf("retry(重试次数)不能为负")
	}
	return nil
}

// PrintResponse 打印响应体。
type PrintResponse struct {
	OK      bool   `json:"ok"`
	JobNo   int    `json:"jobNo"`
	Message string `json:"message"`
}

// handlePrint godoc
// @Summary     提交打印任务(支持一次多台)
// @Description body 可为单个对象(打一台)或对象数组(一次打多台,各自目标/contents/参数,并发执行、失败隔离)。
// @Description 目标二选一:printer(字符串=名/ID,或对象 {name,ip,brand,width})或 gateway(网口 IP 或 "usb")。
// @Description 目标未注册且带了 IP(printer.ip 或 gateway)→ 随打印自动登记入库(建列表);没带名字则用 IP 当名字。
// @Description cloud authoritative:下发的 buzzer/cut/retry/headLines/tailLines 与 printer 身份(name/brand/width)会**覆盖并持久化**到目标打印机,属性页同步显示(每次打印都以云端为准)。
// @Description 必填字段(缺一即 400,无降级、不回退打印机默认):buzzer(0关1开)、cut(0关1开,控制打印后是否切纸)、retry(重试次数,0=不重试/N=失败重试N次)、headLines(0=默认)、tailLines(0=默认)。
// @Description 建议值:type=5(JSON 排版),pWidth=58/80,pCopy=1,buzzer=0,cut=1,retry=1,head/tailLines=0。
// @Description contents(type=5)元素:title / text(bold,size"WH",align) / div_line·div_star / 表格(thead+tbody) / both_sides / qrcode / bc128·code39 / png。切纸由顶层 cut 字段统一控制,contents 内无需 cut 元素(写了也会被忽略)。
// @Description 响应:成功 {"ok":true,"jobNo":N,"message":"已提交"};失败 {"ok":false,"message":"原因"}。数组请求返回逐台 PrintResponse 数组(逐台成功/失败)。
// @Description 常见错误码:400=JSON解析失败 / 缺少必填字段(buzzer·cut·retry·headLines·tailLines)/ buzzer·cut 只能为0或1 / retry 为负 / type 不支持(仅5或1)/ 渲染失败 / 未指定打印目标 / gateway=usb 但无已注册USB机;404=printer 名/ID 未找到;405=非 POST。
// @Tags        print
// @Accept      json
// @Produce     json
// @Param       request body PrintRequest true "打印请求(单对象;数组请直接传 JSON 数组)"
// @Success     200 {object} PrintResponse "已提交 {ok:true,jobNo:123};数组返回逐台数组"
// @Failure     400 {object} PrintResponse "参数错误:JSON解析失败 / 缺少必填字段 / buzzer·cut 非0/1 / retry为负 / type不支持 / 渲染失败 / 未指定打印目标"
// @Failure     404 {object} PrintResponse "未找到打印机:printer 名/ID 不存在"
// @Failure     405 {object} PrintResponse "仅支持 POST"
// @Router      /api/print [post]
func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, PrintResponse{Message: "仅支持 POST"})
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, PrintResponse{Message: "读取请求失败: " + err.Error()})
		return
	}
	reqs, wasArray, err := ParseRequests(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, PrintResponse{Message: "JSON 解析失败: " + err.Error()})
		return
	}

	resps := make([]PrintResponse, len(reqs))
	regd := make([]bool, len(reqs)) // 各请求是否新登记了打印机
	var wg sync.WaitGroup
	for i := range reqs {
		// 防重打:同 id 只受理一次(逐个判定)
		if reqs[i].ID != 0 {
			s.mu.Lock()
			dup := s.seen[reqs[i].ID]
			if !dup {
				s.seen[reqs[i].ID] = true
			}
			s.mu.Unlock()
			if dup {
				resps[i] = PrintResponse{OK: true, Message: "任务 id 已受理,跳过重复打印"}
				continue
			}
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			no, reg, err := Process(s.cfg, s.svc, &reqs[i])
			regd[i] = reg
			if err != nil {
				logger.Errorf("打印请求失败: %v", err)
				resps[i] = PrintResponse{Message: err.Error()}
				return
			}
			resps[i] = PrintResponse{OK: true, JobNo: no, Message: "已提交"}
		}(i)
	}
	wg.Wait()

	// 有目标随打印自动登记 → 持久化并通知 UI 刷新列表
	changed := false
	for _, r := range regd {
		if r {
			changed = true
			break
		}
	}
	if changed {
		s.save()
		s.fireChange()
	}

	if wasArray {
		writeJSON(w, http.StatusOK, resps)
		return
	}
	resp := resps[0]
	code := http.StatusOK
	if !resp.OK {
		code = http.StatusBadRequest
		if strings.HasPrefix(resp.Message, "未找到打印机") {
			code = http.StatusNotFound
		}
	}
	writeJSON(w, code, resp)
}

// ParseRequests 解析 POST /api/print 的 body:支持单个对象或对象数组。
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
// 返回首个任务号与 changed(注册表有变化=新登记或参数被覆盖,供调用方 save+刷新 UI)。
// HTTP 接口与 UI「JSON 测试」共用此逻辑(将来 MQTT 亦复用)。
func Process(cfg *config.Config, svc *printsvc.Service, req *PrintRequest) (int, bool, error) {
	// 必填校验(无降级):缺蜂鸣/切刀/重试/头尾行即拒,不回退打印机默认、不自动登记。
	if err := req.validate(); err != nil {
		return 0, false, err
	}
	p, registered, err := resolveTarget(cfg, req)
	if err != nil {
		return 0, false, err
	}
	width := p.Width
	if req.PWidth == 58 || req.PWidth == 80 {
		width = req.PWidth
	}

	var data []byte
	switch req.Type {
	case 0, 5:
		data, err = layout.Render(req.Contents, width)
	case 1:
		data, err = decodeESC(req.Contents)
	default:
		return 0, registered, fmt.Errorf("暂不支持的 type: %d(仅 5=JSON 排版 / 1=ESC base64)", req.Type)
	}
	if err != nil {
		return 0, registered, fmt.Errorf("渲染失败: %v", err)
	}

	// 由必填字段生成生效参数(蜂鸣/切刀 0/1→bool;重试次数 N→开关=N>0、次数=N;头/尾空行透传)。
	buzzer, cut := *req.Buzzer == 1, *req.Cut == 1
	retry, retryMax := *req.Retry > 0, *req.Retry
	head, tail := *req.HeadLines, *req.TailLines

	// cloud authoritative:云端下发的 5 参数 + printer 身份覆盖并持久化到目标打印机(属性页同步)。
	changed := cfg.UpdatePrinterFromCloud(p.ID, config.PrinterUpdate{
		Name:          strings.TrimSpace(req.Printer.Name),
		Brand:         model.Brand(strings.TrimSpace(req.Printer.Brand)),
		Width:         req.Printer.Width,
		BuzzerEnabled: buzzer,
		CutDisabled:   !cut,
		NoRetry:       !retry,
		RetryMax:      retryMax,
		HeadLines:     head,
		TailLines:     tail,
	})

	opts := &printsvc.Options{
		Cut: &cut, Buzzer: &buzzer, Retry: &retry, RetryMax: &retryMax,
		HeadLines: &head, TailLines: &tail,
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
	return first, registered || changed, nil
}

// resolveTarget 解析打印目标并在需要时自动登记(随打印下发建列表)。
// 目标由 printer(名/ID 字符串 或 {name,ip,brand,width} 对象)与 gateway(IP 或 "usb")归一得出:
//   - 有 IP(printer.ip 或 gateway 是 IP):已注册按 IP 命中则直接用、不改;未注册则 UpsertPrinter 新增。
//   - USB(gateway=="usb" 或 printer.usbName):选已注册 USB 机,不自动登记。
//   - 仅名/ID:FindPrinter,未找到报错(无地址无法登记)。
//
// 返回目标打印机与 registered(是否新登记)。
func resolveTarget(cfg *config.Config, req *PrintRequest) (*model.Printer, bool, error) {
	ref := req.Printer
	gw := strings.TrimSpace(req.Gateway)

	// 归一 IP/port:printer.ip 优先,否则 gateway 为 IP(可含 :port)
	ip, port := "", "9100"
	if v := strings.TrimSpace(ref.IP); v != "" {
		ip = v
		if pv := strings.TrimSpace(ref.Port); pv != "" {
			port = pv
		}
	} else if gw != "" && !strings.EqualFold(gw, "usb") {
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
		width := ref.Width
		if width != 58 && width != 80 {
			if req.PWidth == 58 || req.PWidth == 80 {
				width = req.PWidth
			} else {
				width = 80
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

	// 2) USB:选已注册 USB 机(按 usbName 或首台),不自动登记
	if strings.EqualFold(gw, "usb") || strings.TrimSpace(ref.USBName) != "" {
		usbName := strings.TrimSpace(ref.USBName)
		for _, p := range cfg.PrinterList() {
			if p.Conn == model.ConnUSB && (usbName == "" || p.USBName == usbName) {
				return p, false, nil
			}
		}
		return nil, false, fmt.Errorf("没有匹配的已注册 USB 打印机")
	}

	// 3) 仅名/ID 选择器
	if key := strings.TrimSpace(ref.Name); key != "" {
		if p := cfg.FindPrinter(key); p != nil {
			return p, false, nil
		}
		return nil, false, fmt.Errorf("未找到打印机: %s", key)
	}
	if key := strings.TrimSpace(ref.ID); key != "" {
		if p := cfg.FindPrinter(key); p != nil {
			return p, false, nil
		}
		return nil, false, fmt.Errorf("未找到打印机: %s", key)
	}

	return nil, false, fmt.Errorf("未指定打印目标(printer 或 gateway 至少填一个)")
}

// decodeESC 把 type=1 的 contents(base64 字符串)解码为原始 ESC 字节。
func decodeESC(contents json.RawMessage) ([]byte, error) {
	var s string
	if err := json.Unmarshal(contents, &s); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(s)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
