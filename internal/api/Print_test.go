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

// USB 寻址收紧:gateway:"usb" 不是合法目标 → 拒(code=BadUSBTarget);
// USB 机唯一的云端寻址方式是 printer 名称/ID 选择器。
func TestResolveTargetUSBByNameOnly(t *testing.T) {
	cfg := config.Default()
	cfg.Printers = []*model.Printer{
		{ID: "u1", Name: "打印1", Conn: model.ConnUSB, USBName: "GP-58 Series", Width: 58},
		{ID: "u2", Name: "打印2", Conn: model.ConnUSB, USBName: "GP-80 Series", Width: 80},
	}

	// gateway:"usb" → 拒收
	req := baseReq()
	req.Gateway = "usb"
	_, _, err := resolveTarget(cfg, &req)
	if err == nil || !strings.Contains(err.Error(), "printer 指定打印机名称") {
		t.Fatalf("gateway=usb 应拒,got: %v", err)
	}
	if got := errcode.CodeOf(err); got != errcode.BadUSBTarget {
		t.Fatalf("错误码应为 %d,got %d", errcode.BadUSBTarget, got)
	}

	// printer 字符串=名称 → 命中对应 USB 机,不新登记
	req = baseReq()
	req.Printer = PrinterRef{Name: "打印2"}
	p, isNew, err := resolveTarget(cfg, &req)
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

	// 名称未上报过(未登记)→ 3002 不识别
	req = baseReq()
	req.Printer = PrinterRef{Name: "打印9"}
	_, _, err = resolveTarget(cfg, &req)
	if err == nil || errcode.CodeOf(err) != errcode.PrinterNotFound {
		t.Fatalf("未知名称应报 PrinterNotFound,got: %v", err)
	}

	// payload 携带 usbName 键 → 字段不识别(解析忽略),无其他目标即 3001
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
