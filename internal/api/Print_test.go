package api

import (
	"encoding/json"
	"strings"
	"testing"

	"congmingpay/internal/config"
	"congmingpay/internal/errcode"
	"congmingpay/internal/model"
)

func intp(v int) *int { return &v }

// baseReq 返回带齐 5 个必填字段的合法请求。
func baseReq() PrintRequest {
	return PrintRequest{
		Buzzer: intp(0), Cut: intp(1), Reprint: intp(0),
		HeadLines: intp(0), TailLines: intp(0),
	}
}

// 字段校验:不合规直接拒绝;兼容开时五参数可省略,关时必填。
func TestValidateRanges(t *testing.T) {
	cases := []struct {
		name   string
		mod    func(*PrintRequest)
		want   string // 空=应通过
		code   int    // 期望错误码(want 非空时校验)
		compat bool
	}{
		{"headLines 负", func(r *PrintRequest) { r.HeadLines = intp(-1) }, "headLines", errcode.BadLineRange, true},
		{"headLines 101", func(r *PrintRequest) { r.HeadLines = intp(101) }, "headLines", errcode.BadLineRange, true},
		{"tailLines 负", func(r *PrintRequest) { r.TailLines = intp(-1) }, "tailLines", errcode.BadLineRange, true},
		{"tailLines 101", func(r *PrintRequest) { r.TailLines = intp(101) }, "tailLines", errcode.BadLineRange, true},
		{"pWidth 33", func(r *PrintRequest) { r.PWidth = 33 }, "pWidth", errcode.BadPWidth, true},
		{"省略五参数合法(兼容开)", func(r *PrintRequest) {
			r.Buzzer, r.Cut, r.Reprint, r.HeadLines, r.TailLines = nil, nil, nil, nil, nil
		}, "", 0, true},
		{"省略五参数拒(兼容关)", func(r *PrintRequest) {
			r.Buzzer, r.Cut, r.Reprint, r.HeadLines, r.TailLines = nil, nil, nil, nil, nil
		}, "缺少必填字段", errcode.MissingField, false},
		{"buzzer 取值", func(r *PrintRequest) { r.Buzzer = intp(2) }, "buzzer", errcode.BadSwitch, true},
		{"边界合法", func(r *PrintRequest) { r.HeadLines = intp(100); r.TailLines = intp(100); r.PWidth = 58 }, "", 0, true},
		{"pWidth 不填合法", func(r *PrintRequest) { r.PWidth = 0 }, "", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := baseReq()
			c.mod(&req)
			err := req.validate(c.compat)
			if c.want == "" {
				if err != nil {
					t.Fatalf("应通过,got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("应拒且错误含 %q,got: %v", c.want, err)
			}
			if got := errcode.CodeOf(err); got != c.code {
				t.Fatalf("错误码应为 %d,got %d", c.code, got)
			}
		})
	}
}

// 自动登记必须能确定纸宽:printer.width 与 pWidth 皆缺 → 拒(code=WidthUnknown),不兜底 80。
func TestResolveTargetRequiresWidth(t *testing.T) {
	cfg := config.Default()
	req := baseReq()
	req.Gateway = "192.168.99.99"
	_, _, err := resolveTarget(cfg, &req)
	if err == nil || !strings.Contains(err.Error(), "无法确定纸宽") {
		t.Fatalf("双缺纸宽应拒,got: %v", err)
	}
	if got := errcode.CodeOf(err); got != errcode.WidthUnknown {
		t.Fatalf("错误码应为 %d,got %d", errcode.WidthUnknown, got)
	}

	req.PWidth = 80
	p, isNew, err := resolveTarget(cfg, &req)
	if err != nil || !isNew || p.Width != 80 {
		t.Fatalf("pWidth=80 应登记成功(width=80),got p=%+v isNew=%v err=%v", p, isNew, err)
	}
}

// USB 目标:gateway:"usb" 取配置序第一台 USB;无则 3003;名称/ID 选机仍可用。
func TestResolveTargetGatewayUSB(t *testing.T) {
	cfg := config.Default()
	cfg.Printers = []*model.Printer{
		{ID: "n1", Name: "网口1", Conn: model.ConnNetwork, IP: "192.168.1.1", Width: 80},
		{ID: "u1", Name: "打印1", Conn: model.ConnUSB, USBName: "GP-58 Series", Width: 58},
		{ID: "u2", Name: "打印2", Conn: model.ConnUSB, USBName: "GP-80 Series", Width: 80},
	}

	// gateway:"usb" → 跳过网口,命中第一台 USB(u1),不新登记
	req := baseReq()
	req.Gateway = "usb"
	p, isNew, err := resolveTarget(cfg, &req)
	if err != nil || isNew || p == nil || p.ID != "u1" {
		t.Fatalf("gateway=usb 应命中 u1,got p=%+v isNew=%v err=%v", p, isNew, err)
	}
	if p.Conn != model.ConnUSB {
		t.Fatalf("应为 USB,got %s", p.Conn)
	}

	// 大小写不敏感
	req = baseReq()
	req.Gateway = "USB"
	p, isNew, err = resolveTarget(cfg, &req)
	if err != nil || isNew || p.ID != "u1" {
		t.Fatalf("Gateway=USB 应命中 u1,got p=%+v isNew=%v err=%v", p, isNew, err)
	}

	// 无 USB → 3003,且不得登记 IP=usb 的脏网口机
	cfgEmpty := config.Default()
	cfgEmpty.Printers = []*model.Printer{
		{ID: "n1", Name: "网口1", Conn: model.ConnNetwork, IP: "192.168.1.1", Width: 80},
	}
	req = baseReq()
	req.Gateway = "usb"
	_, _, err = resolveTarget(cfgEmpty, &req)
	if err == nil || errcode.CodeOf(err) != errcode.BadUSBTarget {
		t.Fatalf("无 USB 应为 BadUSBTarget,got: %v", err)
	}
	if !strings.Contains(err.Error(), "未找到 USB 打印机") {
		t.Fatalf("文案应含未找到 USB 打印机,got: %v", err)
	}
	for _, x := range cfgEmpty.PrinterList() {
		if x.IP == "usb" || strings.EqualFold(x.IP, "usb") {
			t.Fatalf("不得产生 IP=usb 的脏登记: %+v", x)
		}
	}
	if n := len(cfgEmpty.PrinterList()); n != 1 {
		t.Fatalf("打印机数量应仍为 1,got %d", n)
	}

	// printer.ip 优先于 gateway:"usb"
	req = baseReq()
	req.Printer = PrinterRef{Name: "网口1", IP: "192.168.1.1", Width: 80}
	req.Gateway = "usb"
	p, isNew, err = resolveTarget(cfg, &req)
	if err != nil || isNew || p.ID != "n1" {
		t.Fatalf("printer.ip 应优先命中网口 n1,got p=%+v isNew=%v err=%v", p, isNew, err)
	}

	// printer 名称选 USB 仍可用
	req = baseReq()
	req.Printer = PrinterRef{Name: "打印2"}
	p, isNew, err = resolveTarget(cfg, &req)
	if err != nil || isNew || p.ID != "u2" {
		t.Fatalf("按名称应命中 u2,got p=%+v isNew=%v err=%v", p, isNew, err)
	}

	// 对象只带 name 同样命中
	req = baseReq()
	req.Printer = PrinterRef{Name: "打印1"}
	p, isNew, err = resolveTarget(cfg, &req)
	if err != nil || isNew || p.ID != "u1" {
		t.Fatalf("对象 name 应命中 u1,got p=%+v isNew=%v err=%v", p, isNew, err)
	}

	// 名称未登记 → 3002
	req = baseReq()
	req.Printer = PrinterRef{Name: "打印9"}
	_, _, err = resolveTarget(cfg, &req)
	if err == nil || errcode.CodeOf(err) != errcode.PrinterNotFound {
		t.Fatalf("未知名称应报 PrinterNotFound,got: %v", err)
	}

	// payload 携带 usbName 键 → 字段不识别,无其他目标即 3001
	var raw PrintRequest
	if err := json.Unmarshal([]byte(`{"printer":{"usbName":"GP-58 Series"},"buzzer":0,"cut":1,"reprint":0,"headLines":0,"tailLines":0}`), &raw); err != nil {
		t.Fatalf("解析不应失败: %v", err)
	}
	_, _, err = resolveTarget(cfg, &raw)
	if err == nil || errcode.CodeOf(err) != errcode.NoTarget {
		t.Fatalf("usbName 寻址应不识别(NoTarget),got: %v", err)
	}
}

// 渲染类错误经「渲染失败: %w」包装后错误码仍可提取(RenderInvalid)。
func TestProcessRenderErrorCode(t *testing.T) {
	cfg := config.Default()
	req := baseReq()
	req.Gateway = "192.168.99.98"
	req.PWidth = 80
	req.Contents = []byte(`[{"type":"foo","cont":"x"}]`)
	_, _, err := Process(cfg, nil, &req) // 渲染在 Submit 之前失败,svc 不会被使用
	if err == nil || !strings.Contains(err.Error(), "渲染失败") {
		t.Fatalf("应渲染拒绝,got: %v", err)
	}
	if got := errcode.CodeOf(err); got != errcode.RenderInvalid {
		t.Fatalf("错误码应为 %d,got %d", errcode.RenderInvalid, got)
	}
}
