package api

import (
	"strings"
	"testing"

	"congmingpay/internal/config"
)

func intp(v int) *int { return &v }

// baseReq 返回带齐 5 个必填字段的合法请求。
func baseReq() PrintRequest {
	return PrintRequest{
		Buzzer: intp(0), Cut: intp(1), Reprint: intp(0),
		HeadLines: intp(0), TailLines: intp(0),
	}
}

// 必填范围校验:不合规直接拒绝,无降级。
func TestValidateRanges(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*PrintRequest)
		want string // 空=应通过
	}{
		{"headLines 负", func(r *PrintRequest) { r.HeadLines = intp(-1) }, "headLines"},
		{"headLines 101", func(r *PrintRequest) { r.HeadLines = intp(101) }, "headLines"},
		{"tailLines 负", func(r *PrintRequest) { r.TailLines = intp(-1) }, "tailLines"},
		{"tailLines 101", func(r *PrintRequest) { r.TailLines = intp(101) }, "tailLines"},
		{"pWidth 33", func(r *PrintRequest) { r.PWidth = 33 }, "pWidth"},
		{"边界合法", func(r *PrintRequest) { r.HeadLines = intp(100); r.TailLines = intp(100); r.PWidth = 58 }, ""},
		{"pWidth 不填合法", func(r *PrintRequest) { r.PWidth = 0 }, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := baseReq()
			c.mod(&req)
			err := req.validate()
			if c.want == "" {
				if err != nil {
					t.Fatalf("应通过,got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("应拒且错误含 %q,got: %v", c.want, err)
			}
		})
	}
}

// 自动登记必须能确定纸宽:printer.width 与 pWidth 皆缺 → 拒,不兜底 80。
func TestResolveTargetRequiresWidth(t *testing.T) {
	cfg := config.Default()
	req := baseReq()
	req.Gateway = "192.168.99.99"
	if _, _, err := resolveTarget(cfg, &req); err == nil || !strings.Contains(err.Error(), "无法确定纸宽") {
		t.Fatalf("双缺纸宽应拒,got: %v", err)
	}

	req.PWidth = 80
	p, isNew, err := resolveTarget(cfg, &req)
	if err != nil || !isNew || p.Width != 80 {
		t.Fatalf("pWidth=80 应登记成功(width=80),got p=%+v isNew=%v err=%v", p, isNew, err)
	}
}
