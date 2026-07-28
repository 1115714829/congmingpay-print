package preview

import (
	"image"
	"testing"

	"congmingpay/internal/layout"
	"congmingpay/internal/printsvc"
)

func TestRenderSample(t *testing.T) {
	img, err := Render(Params{
		WidthMM:    80,
		SourceJSON: layout.SampleContents,
		HeadLines:  1,
		TailLines:  0,
		Cut:        true,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 576 {
		t.Fatalf("width want 576,got %d", b.Dx())
	}
	if b.Dy() < 50 {
		t.Fatalf("height too small: %d", b.Dy())
	}
	// 应有非白色像素
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("type %T", img)
	}
	ink := false
	for i := 0; i < len(rgba.Pix); i += 4 {
		if rgba.Pix[i] < 250 || rgba.Pix[i+1] < 250 || rgba.Pix[i+2] < 250 {
			ink = true
			break
		}
	}
	if !ink {
		t.Fatal("预览图应有墨迹")
	}
}

func TestRender58(t *testing.T) {
	img, err := Render(Params{
		WidthMM:    58,
		SourceJSON: []byte(`[{"type":"text","cont":"hello"},{"type":"title","cont":"标题"}]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 384 {
		t.Fatalf("58mm width=%d", img.Bounds().Dx())
	}
}

func TestCanPreview(t *testing.T) {
	ok, _ := CanPreview(&printsvc.JobPreview{ContentType: printsvc.ContentESC})
	if ok {
		t.Fatal("ESC 不应可预览")
	}
	ok, _ = CanPreview(&printsvc.JobPreview{ContentType: printsvc.ContentUnknown})
	if ok {
		t.Fatal("无源不应可预览")
	}
	ok, reason := CanPreview(&printsvc.JobPreview{
		ContentType: printsvc.ContentJSON, SourceJSON: []byte(`[]`),
	})
	if !ok {
		t.Fatalf("JSON 应可预览: %s", reason)
	}
}

func TestRenderEmpty(t *testing.T) {
	_, err := Render(Params{WidthMM: 80})
	if err == nil {
		t.Fatal("空源应失败")
	}
}
