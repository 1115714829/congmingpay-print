package printsvc

import (
	"testing"

	"congmingpay/internal/model"
)

func TestPreviewDataSourceJSON(t *testing.T) {
	svc := New()
	p := &model.Printer{Name: "t", Width: 80, Conn: model.ConnNetwork, IP: "10.0.0.1", Port: "9100"}
	ct := ContentJSON
	src := []byte(`[{"type":"text","cont":"x"}]`)
	no := svc.Submit(p, "打印", []byte("esc"), &Options{ContentType: &ct, SourceJSON: src})
	pd, err := svc.PreviewData(no)
	if err != nil {
		t.Fatal(err)
	}
	if pd.ContentType != ContentJSON || string(pd.SourceJSON) != string(src) {
		t.Fatalf("%+v", pd)
	}
	if pd.WidthMM != 80 {
		t.Fatalf("width %d", pd.WidthMM)
	}
}

func TestPreviewDataMissing(t *testing.T) {
	svc := New()
	_, err := svc.PreviewData(99999)
	if err == nil {
		t.Fatal("应报不存在")
	}
}

func TestPreviewDataUnknownLocal(t *testing.T) {
	svc := New()
	p := &model.Printer{Name: "t", Width: 58, Conn: model.ConnNetwork, IP: "10.0.0.2", Port: "9100"}
	no := svc.Submit(p, "测试页", []byte("x"), nil)
	pd, err := svc.PreviewData(no)
	if err != nil {
		t.Fatal(err)
	}
	if pd.ContentType != ContentUnknown || len(pd.SourceJSON) != 0 {
		t.Fatalf("%+v", pd)
	}
}
