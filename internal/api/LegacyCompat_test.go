package api

import (
	"strings"
	"testing"

	"congmingpay/internal/config"
	"congmingpay/internal/errcode"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// 云盒兼容开:旧后端样例(type=5、无五参数、contents 末尾 cut、gateway+pWidth)。
const legacyCompatOldJSON = `{
  "contents":[
    {"size":"22","bold":true,"type":"text","align":"center","cont":"结账单"},
    {"size":"11","type":"text","align":"center","cont":"【堂食】"},
    {"type":"div_line"},
    {"size":"00","tbody":[["1.青椒肉丝","1份",0.01]],"thead":{"商品名称":"70%","数量":"15%","金额":"15%"}},
    {"type":"text"},
    {"cont":"1","type":"cut"}
  ],
  "id":1615411579,
  "type":5,
  "pWidth":80,
  "gateway":"192.168.68.194",
  "pCopy":1
}`

func TestLegacyCompatOldJSONProcess(t *testing.T) {
	reqs, _, err := ParseRequests([]byte(legacyCompatOldJSON))
	if err != nil || len(reqs) != 1 {
		t.Fatalf("ParseRequests: %v len=%d", err, len(reqs))
	}
	req := &reqs[0]
	if req.Type != 5 {
		t.Fatalf("type=%d", req.Type)
	}
	if req.Buzzer != nil || req.Cut != nil || req.Reprint != nil || req.HeadLines != nil || req.TailLines != nil {
		t.Fatal("五参数应全部省略")
	}

	cfg := config.Default()
	svc := printsvc.New()
	res, changed, err := Process(cfg, svc, req)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if !changed || res == nil {
		t.Fatalf("应自动登记打印机,changed=%v res=%v", changed, res)
	}
	if res.Printer.IP != "192.168.68.194" || res.Printer.Width != 80 {
		t.Fatalf("打印机 %+v", res.Printer)
	}
	if res.Cut != 1 {
		t.Fatalf("contents cut 意图应使本单 Cut=1,got %d", res.Cut)
	}
}

func TestStrictRejectsOldJSON(t *testing.T) {
	reqs, _, err := ParseRequests([]byte(legacyCompatOldJSON))
	if err != nil || len(reqs) != 1 {
		t.Fatalf("ParseRequests: %v", err)
	}
	cfg := config.Default()
	cfg.Settings.YunheCompatDisabled = true
	svc := printsvc.New()
	_, _, err = Process(cfg, svc, &reqs[0])
	if err == nil {
		t.Fatal("兼容关应拒旧 JSON")
	}
	code := errcode.CodeOf(err)
	if code != errcode.BadContentType && code != errcode.MissingField {
		t.Fatalf("期望 2005 或 2001,got code=%d err=%v", code, err)
	}
}

func TestStrictRejectType5(t *testing.T) {
	cfg := config.Default()
	cfg.Settings.YunheCompatDisabled = true
	svc := printsvc.New()
	req := &PrintRequest{
		Gateway:  "10.0.0.1",
		PWidth:   80,
		Type:     5,
		Contents: []byte(`[{"type":"text","cont":"x"}]`),
		Buzzer:   intp(0), Cut: intp(0), Reprint: intp(0),
		HeadLines: intp(0), TailLines: intp(0),
	}
	_, _, err := Process(cfg, svc, req)
	if err == nil || !strings.Contains(err.Error(), "不支持的 type: 5") {
		t.Fatalf("兼容关应拒 type=5,got: %v", err)
	}
	if errcode.CodeOf(err) != errcode.BadContentType {
		t.Fatalf("code=%d", errcode.CodeOf(err))
	}
}

func TestStrictRejectContentsCut(t *testing.T) {
	cfg := config.Default()
	cfg.Settings.YunheCompatDisabled = true
	svc := printsvc.New()
	req := &PrintRequest{
		Gateway:  "10.0.0.2",
		PWidth:   80,
		Type:     0,
		Contents: []byte(`[{"type":"text","cont":"x"},{"type":"cut","cont":"1"}]`),
		Buzzer:   intp(0), Cut: intp(1), Reprint: intp(0),
		HeadLines: intp(0), TailLines: intp(0),
	}
	_, _, err := Process(cfg, svc, req)
	if err == nil || !strings.Contains(err.Error(), "cut") {
		t.Fatalf("兼容关应拒 contents cut,got: %v", err)
	}
}

func TestLegacyCompatType5EqualsJSON(t *testing.T) {
	cfg := config.Default()
	svc := printsvc.New()
	req := &PrintRequest{
		Gateway:  "10.0.0.1",
		PWidth:   80,
		Type:     5,
		Contents: []byte(`[{"type":"text","cont":"x"}]`),
		Buzzer:   intp(0), Cut: intp(0), Reprint: intp(0),
		HeadLines: intp(0), TailLines: intp(0),
	}
	if _, _, err := Process(cfg, svc, req); err != nil {
		t.Fatalf("type=5: %v", err)
	}
}

func TestLegacyCompatOmitParamsNoOverwrite(t *testing.T) {
	cfg := config.Default()
	cfg.Printers = nil
	p, isNew := cfg.UpsertPrinter(&model.Printer{
		Name: "预置机", Width: 80, Conn: model.ConnNetwork, IP: "10.9.9.8", Port: "9100",
		BuzzerEnabled: true, CutDisabled: true, HeadLines: 3, TailLines: 2,
	})
	if !isNew || p == nil {
		t.Fatal("应新建预置机")
	}

	svc := printsvc.New()
	req := &PrintRequest{
		Gateway:  "10.9.9.8",
		PWidth:   80,
		Type:     0,
		Contents: []byte(`[{"type":"text","cont":"hi"}]`),
	}
	res, changed, err := Process(cfg, svc, req)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if changed {
		t.Fatal("省略五参数且已登记不应改配置")
	}
	got := cfg.FindPrinter(p.ID)
	if got == nil || !got.BuzzerEnabled || !got.CutDisabled || got.HeadLines != 3 || got.TailLines != 2 {
		t.Fatalf("属性被覆盖: %+v", got)
	}
	if res.Buzzer != 1 || res.Cut != 0 || res.HeadLines != 3 || res.TailLines != 2 {
		t.Fatalf("生效参数应沿用机台: buzzer=%d cut=%d head=%d tail=%d", res.Buzzer, res.Cut, res.HeadLines, res.TailLines)
	}
}

func TestLegacyCompatTopLevelCutOverridesContent(t *testing.T) {
	cfg := config.Default()
	svc := printsvc.New()
	req := &PrintRequest{
		Gateway:  "10.8.8.8",
		PWidth:   80,
		Type:     0,
		Cut:      intp(0), // 顶层关切
		Contents: []byte(`[{"type":"text","cont":"x"},{"type":"cut","cont":"1"}]`),
	}
	res, _, err := Process(cfg, svc, req)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res.Cut != 0 {
		t.Fatalf("顶层 cut 应优先,got %d", res.Cut)
	}
}
