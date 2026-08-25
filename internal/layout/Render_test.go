package layout

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"congmingpay/internal/escpos"
	"congmingpay/internal/util"
)

// mustErr 断言整单拒绝:err 非空、含 contents[i] 定位与期望片段、不返回字节。
func mustErr(t *testing.T, contents, want string) {
	t.Helper()
	data, _, err := Render([]byte(contents), 80, true)
	if err == nil {
		t.Fatalf("应整单拒绝,却渲染成功: %s", contents)
	}
	if data != nil {
		t.Fatalf("拒绝时不应返回字节,got %d 字节", len(data))
	}
	if !strings.Contains(err.Error(), "contents[") {
		t.Fatalf("错误应含 contents[i] 定位,got: %v", err)
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("错误应含 %q,got: %v", want, err)
	}
}

// mustOK 断言渲染成功且有输出。
func mustOK(t *testing.T, contents string, width int) {
	t.Helper()
	data, _, err := Render([]byte(contents), width, true)
	if err != nil {
		t.Fatalf("应渲染成功,got err: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("渲染结果为空")
	}
}

// testPNG 生成一张合法的 4x4 灰度 PNG,返回 (data URI, 原始字节)。
func testPNG(t *testing.T) (string, []byte) {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), buf.Bytes()
}

// genBMPBytes 生成 w×h 24 位未压缩 BMP(自底向上,棋盘格图案),返回原始字节。
func genBMPBytes(w, h int) []byte {
	bpp := 3
	rowSize := (w*bpp + 3) &^ 3
	pixOff := 14 + 40
	total := pixOff + rowSize*h
	buf := new(bytes.Buffer)
	buf.Write([]byte{'B', 'M'})
	_ = binary.Write(buf, binary.LittleEndian, uint32(total))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(buf, binary.LittleEndian, uint32(pixOff))
	_ = binary.Write(buf, binary.LittleEndian, uint32(40))
	_ = binary.Write(buf, binary.LittleEndian, int32(w))
	_ = binary.Write(buf, binary.LittleEndian, int32(h))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(24))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(buf, binary.LittleEndian, int32(0))
	_ = binary.Write(buf, binary.LittleEndian, int32(0))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	px := make([]byte, rowSize)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x+y)%2 == 0 {
				px[x*bpp+0], px[x*bpp+1], px[x*bpp+2] = 0, 0, 0
			} else {
				px[x*bpp+0], px[x*bpp+1], px[x*bpp+2] = 255, 255, 255
			}
		}
		buf.Write(px)
	}
	return buf.Bytes()
}

// testBMP 生成一张合法的 4x4 24 位 BMP,返回 (data URI, 原始字节)。
func testBMP(t *testing.T) (string, []byte) {
	t.Helper()
	raw := genBMPBytes(4, 4)
	return "data:image/bmp;base64," + base64.StdEncoding.EncodeToString(raw), raw
}

// —— 拒绝用例(全部严格:字段存在但非法 → 整单拒) ——

func TestRejectUnknownType(t *testing.T) {
	mustErr(t, `[{"type":"foo","cont":"x"}]`, `未知元素 type "foo"`)
}

// 云盒兼容开:contents type=cut 接受;返回切纸意图;不写入 GS V。
// 厂商语义:cont "0"=全切、"1"=半切,均触发切纸;false 保留「不切」。
func TestLegacyCompatCutElement(t *testing.T) {
	data, cut, err := Render([]byte(`[{"type":"text","cont":"hi"},{"cont":"1","type":"cut"}]`), 80, true)
	if err != nil {
		t.Fatalf("cut 应接受,got: %v", err)
	}
	if cut == nil || !*cut {
		t.Fatalf("cut cont=1 意图应为 true,got %v", cut)
	}
	if bytes.Contains(data, []byte{0x1D, 0x56, 0x01}) {
		t.Fatal("layout 不应写入 GS V(由 Finish 收尾)")
	}
	_, cut0, err := Render([]byte(`[{"type":"cut","cont":"0"}]`), 80, true)
	if err != nil || cut0 == nil || !*cut0 {
		t.Fatalf("cut cont=\"0\" 意图应为 true(全切),got %v err=%v", cut0, err)
	}
	_, cutN, err := Render([]byte(`[{"type":"cut","cont":0}]`), 80, true)
	if err != nil || cutN == nil || !*cutN {
		t.Fatalf("cut cont=0 意图应为 true(全切),got %v err=%v", cutN, err)
	}
	_, cutF, err := Render([]byte(`[{"type":"cut","cont":false}]`), 80, true)
	if err != nil || cutF == nil || *cutF {
		t.Fatalf("cut cont=false 意图应为 false,got %v err=%v", cutF, err)
	}
}

func TestStrictRejectCutElement(t *testing.T) {
	mustErrCompatOff(t, `[{"type":"text","cont":"hi"},{"cont":"1","type":"cut"}]`, `未知元素 type "cut"`)
}

// mustErrCompatOff 断言兼容关时整单拒绝。
func mustErrCompatOff(t *testing.T, contents, want string) {
	t.Helper()
	data, _, err := Render([]byte(contents), 80, false)
	if err == nil {
		t.Fatalf("应整单拒绝,却渲染成功: %s", contents)
	}
	if data != nil {
		t.Fatalf("拒绝时不应返回字节,got %d 字节", len(data))
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("错误应含 %q,got: %v", want, err)
	}
}

func TestRejectTbodyWithoutThead(t *testing.T) {
	mustErr(t, `[{"tbody":[["a","b"]]}]`, "缺 thead")
}

func TestRejectBadSize(t *testing.T) {
	for _, s := range []string{"1", "18", "ab", "123", "1 "} {
		mustErr(t, `[{"cont":"x","size":"`+s+`"}]`, "size")
	}
}

func TestRejectBadAlign(t *testing.T) {
	mustErr(t, `[{"cont":"x","align":"middle"}]`, "align")
}

func TestRejectBadCont(t *testing.T) {
	mustErr(t, `[{"cont":123}]`, "cont 需为字符串")
	mustErr(t, `[{"cont":{"a":1}}]`, "cont 需为字符串")
}

func TestRejectBothSides(t *testing.T) {
	mustErr(t, `[{"both_sides":["a"]}]`, "2 段")
	mustErr(t, `[{"both_sides":["a","b","c"]}]`, "2 段")
	mustErr(t, `[{"both_sides":[]}]`, "2 段")
	mustErr(t, `[{"both_sides":["a","b"],"size":"9"}]`, "size")
}

func TestRejectBadThead(t *testing.T) {
	mustErr(t, `[{"thead":{},"tbody":[]}]`, "thead 为空")
	mustErr(t, `[{"thead":[],"tbody":[]}]`, "thead 为空")
	mustErr(t, `[{"thead":"abc","tbody":[]}]`, "thead 需为对象")
	mustErr(t, `[{"thead":5,"tbody":[]}]`, "thead 需为对象")
	mustErr(t, `[{"thead":{"名称":50},"tbody":[]}]`, "需为字符串")
}

func TestRejectBadPct(t *testing.T) {
	mustErr(t, `[{"thead":{"a":"abc%","b":"50%"},"tbody":[]}]`, "非法")
	mustErr(t, `[{"thead":{"a":"50.5%","b":"50%"},"tbody":[]}]`, "非法")
	mustErr(t, `[{"thead":{"a":"0%","b":"100%"},"tbody":[]}]`, "超出 1-100")
	mustErr(t, `[{"thead":{"a":"-10%","b":"100%"},"tbody":[]}]`, "超出 1-100")
	mustErr(t, `[{"thead":{"a":"120%"},"tbody":[]}]`, "超出 1-100")
	mustErr(t, `[{"thead":["60%","30%"],"tbody":[]}]`, "≠ 100")
}

func TestRejectRowMismatch(t *testing.T) {
	mustErr(t, `[{"thead":{"a":"50%","b":"50%"},"tbody":[["x"]]}]`, "与列数 2 不符")
	mustErr(t, `[{"thead":{"a":"50%","b":"50%"},"tbody":[["x","y","z"]]}]`, "与列数 2 不符")
}

func TestRejectBadCell(t *testing.T) {
	mustErr(t, `[{"thead":{"a":"50%","b":"50%"},"tbody":[["x",null]]}]`, "值非法")
	mustErr(t, `[{"thead":{"a":"50%","b":"50%"},"tbody":[["x",{}]]}]`, "值非法")
	mustErr(t, `[{"thead":{"a":"50%","b":"50%"},"tbody":[["x",[1]]]}]`, "值非法")
}

func TestRejectQRCode(t *testing.T) {
	mustErr(t, `[{"type":"qrcode","cont":""}]`, "二维码内容为空")
	mustErr(t, `[{"type":"qrcode"}]`, "二维码内容为空")
	mustErr(t, `[{"type":"qrcode","cont":["a"]}]`, "恰含 2 项")
	mustErr(t, `[{"type":"qrcode","cont":["a","b","c"]}]`, "恰含 2 项")
	mustErr(t, `[{"type":"qrcode","cont":["a",1]}]`, "元素需为字符串")
}

func TestRejectBarcode(t *testing.T) {
	mustErr(t, `[{"type":"bc128","cont":"123","size":"70"}]`, "条码需两位")
	mustErr(t, `[{"type":"bc128","cont":"123","size":"2x"}]`, "条码需两位")
	mustErr(t, `[{"type":"bc128","cont":"123","hri":4}]`, "hri 4 非法")
	mustErr(t, `[{"type":"code39","cont":""}]`, "条码内容为空")
	// 超长内容会让 GS k 长度字节回绕产出损坏流 → 拒
	mustErr(t, `[{"type":"bc128","cont":"`+strings.Repeat("9", 300)+`"}]`, "过长")
}

// bc128/bc128a 为 Code128 码集 A:仅大写字母与数字、≤14 位。
func TestRejectBarcode128A(t *testing.T) {
	mustErr(t, `[{"type":"bc128","cont":"abc"}]`, "仅支持大写字母与数字")
	mustErr(t, `[{"type":"bc128a","cont":"Ab1_"}]`, "仅支持大写字母与数字")
	mustErr(t, `[{"type":"bc128","cont":"`+strings.Repeat("A", 15)+`"}]`, "上限 14")
	mustErr(t, `[{"type":"bc128a","cont":"`+strings.Repeat("1", 15)+`"}]`, "上限 14")
}

// bc128c 为 Code128 码集 C:仅数字、≤28 位。
func TestRejectBarcode128C(t *testing.T) {
	mustErr(t, `[{"type":"bc128c","cont":"12a"}]`, "仅支持数字")
	mustErr(t, `[{"type":"bc128c","cont":"12.3"}]`, "仅支持数字")
	mustErr(t, `[{"type":"bc128c","cont":"`+strings.Repeat("1", 29)+`"}]`, "上限 28")
}

// 二维码内容超 512 字节 → 拒(单码与双码均适用)。
func TestRejectQRTooLong512(t *testing.T) {
	mustOK(t, `[{"type":"qrcode","cont":"`+strings.Repeat("A", 512)+`"}]`, 80) // 恰 512 合法
	mustErr(t, `[{"type":"qrcode","cont":"`+strings.Repeat("A", 513)+`"}]`, "过长")
	mustErr(t, `[{"type":"qrcode","cont":["`+strings.Repeat("A", 513)+`","x"]}]`, "过长")
	mustErr(t, `[{"type":"qrcode","cont":["x","`+strings.Repeat("A", 600)+`"]}]`, "过长")
}

// qrcode size "01"-"09" 控制模块大小;其他取值整单拒。
func TestRejectQRCodeSize(t *testing.T) {
	mustErr(t, `[{"type":"qrcode","cont":"x","size":"60"}]`, "qrcode 需两位 01-09")
	mustErr(t, `[{"type":"qrcode","cont":"x","size":"00"}]`, "qrcode 需两位 01-09")
	mustErr(t, `[{"type":"qrcode","cont":"x","size":"10"}]`, "qrcode 需两位 01-09")
	mustErr(t, `[{"type":"qrcode","cont":"x","size":"9"}]`, "qrcode 需两位 01-09")
}

func TestRejectPNG(t *testing.T) {
	mustErr(t, `[{"type":"png","cont":"ftp://x"}]`, "来源非法")
	mustErr(t, `[{"type":"png","cont":""}]`, "png 内容为空")
	mustErr(t, `[{"type":"png","cont":"data:image/png;base64,%%%"}]`, "base64 解码失败")
}

// png 解码后字节 >60K 整单拒(data URI 与 URL 均适用)。
func TestRejectPNGTooLarge(t *testing.T) {
	big := bytes.Repeat([]byte{'x'}, 60*1024+1)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(big)
	mustErr(t, `[{"type":"png","cont":"`+uri+`"}]`, "上限 60K")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(big)
	}))
	defer srv.Close()
	mustErr(t, `[{"type":"png","cont":"`+srv.URL+`"}]`, "上限 60K")
}

// bmp 元素:合法 24/32 位未压缩 BMP 可渲染;超 6K、非 BMP、不支持的位深拒。
func TestOKBMPDataURI(t *testing.T) {
	uri, _ := testBMP(t)
	mustOK(t, `[{"type":"bmp","cont":"`+uri+`","align":"center"}]`, 80)
}

func TestOKBMPHTTP(t *testing.T) {
	_, raw := testBMP(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/bmp")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()
	mustOK(t, `[{"type":"bmp","cont":"`+srv.URL+`"}]`, 80)
}

func TestRejectBMP(t *testing.T) {
	mustErr(t, `[{"type":"bmp","cont":""}]`, "bmp 内容为空")
	mustErr(t, `[{"type":"bmp","cont":"ftp://x"}]`, "来源非法")
	mustErr(t, `[{"type":"bmp","cont":"data:image/bmp;base64,%%%"}]`, "base64 解码失败")
	// 非 BMP 数据(如 PNG 字节)→ 拒
	uri, _ := testPNG(t)
	mustErr(t, `[{"type":"bmp","cont":"`+uri+`"}]`, "非 BMP 数据")
	// 超 6K:48×48 24 位 ≈ 6.9K
	big := genBMPBytes(48, 48)
	if len(big) <= 6*1024 {
		t.Fatal("测试数据应超 6K")
	}
	bigURI := "data:image/bmp;base64," + base64.StdEncoding.EncodeToString(big)
	mustErr(t, `[{"type":"bmp","cont":"`+bigURI+`"}]`, "上限 6K")
	// 16 位 BMP:仅支持 24/32 位
	raw16 := append([]byte(nil), big[:54]...)
	raw16[28] = 16 // bpp 字段 → 16
	uri16 := "data:image/bmp;base64," + base64.StdEncoding.EncodeToString(raw16)
	mustErr(t, `[{"type":"bmp","cont":"`+uri16+`"}]`, "仅支持 24/32 位")
}

// plugin 开钱箱:ESC p m 25 25(m=cont 0/1,缺失=0)。
func TestPluginCashDrawer(t *testing.T) {
	data, _, err := Render([]byte(`[{"type":"plugin","cont":"1"}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte{0x1B, 0x70, 0x01, 0x19, 0x19}) {
		t.Fatalf("plugin cont=1 应输出 ESC p 1 25 25,got % x", data)
	}
	data0, _, err := Render([]byte(`[{"type":"plugin","cont":"0"}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data0, []byte{0x1B, 0x70, 0x00, 0x19, 0x19}) {
		t.Fatalf("plugin cont=0 应输出 ESC p 0 25 25,got % x", data0)
	}
	mustOK(t, `[{"type":"plugin"}]`, 80) // 缺失 cont = 引脚 0
}

func TestRejectPlugin(t *testing.T) {
	mustErr(t, `[{"type":"plugin","cont":"2"}]`, "plugin.cont 需为 0 或 1")
	mustErr(t, `[{"type":"plugin","cont":true}]`, "plugin.cont 需为 0 或 1")
}

// text 的 cont 数组 = 多行,每行同 bold/size/align。
func TestTextContArray(t *testing.T) {
	data, _, err := Render([]byte(`[{"type":"text","cont":["a","b","c"]}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(data, []byte{0x0A}); n != 3 {
		t.Fatalf("cont 数组应按行输出 3 行,got %d 个换行", n)
	}
	// 样式对每行一致:放大指令输出一次
	dataMag, _, err := Render([]byte(`[{"type":"text","cont":["a","b"],"size":"11"}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(dataMag, []byte{0x1D, 0x21, 0x11}) != 1 {
		t.Fatalf("多行应共用一次放大指令,got % x", dataMag)
	}
	mustErr(t, `[{"type":"text","cont":[1]}]`, "数组元素需为字符串")
	mustErr(t, `[{"type":"text","cont":["a",{}]}]`, "数组元素需为字符串")
}

// title 去加粗:居中 + 二倍字,无 ESC E 1。
func TestTitleNotBold(t *testing.T) {
	data, _, err := Render([]byte(`[{"type":"title","cont":"结账单"}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte{0x1B, 0x45, 0x01}) {
		t.Fatal("title 不应加粗(ESC E 1)")
	}
	if !bytes.Contains(data, []byte{0x1B, 0x61, 0x01}) {
		t.Fatal("title 应居中(ESC a 1)")
	}
	if !bytes.Contains(data, []byte{0x1D, 0x21, 0x11}) {
		t.Fatal("title 应二倍字(GS ! 0x11)")
	}
}

// line_space 行间距 = ESC J 点数(8 点=1mm),不再是行数 LF。
func TestLineSpaceFeedDots(t *testing.T) {
	js := `[{"thead":{"a":"50%","b":"50%"},"tbody":[["x","y"]],"line_space":3}]`
	data, _, err := Render([]byte(js), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte{0x1B, 0x4A, 0x03}) {
		t.Fatalf("line_space 应输出 ESC J 3,got % x", data)
	}
	if n := bytes.Count(data, []byte{0x0A}); n != 2 {
		t.Fatalf("应只有 2 个换行(表头+数据行),got %d", n)
	}
}

// bc128a/bc128c 内容校验通过后,输出带 {A/{C 前缀的 GS k 73 指令。
func TestBarcodeSetBytes(t *testing.T) {
	dataA, _, err := Render([]byte(`[{"type":"bc128a","cont":"ORD12345"}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	payloadA := append([]byte("{A"), []byte("ORD12345")...)
	seqA := append([]byte{0x1D, 0x6B, 73, byte(len(payloadA))}, payloadA...)
	if !bytes.Contains(dataA, seqA) {
		t.Fatalf("bc128a 应输出码集 A 前缀,got % x", dataA)
	}
	dataC, _, err := Render([]byte(`[{"type":"bc128c","cont":"123456789012"}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	payloadC := append([]byte("{C"), []byte("123456789012")...)
	seqC := append([]byte{0x1D, 0x6B, 73, byte(len(payloadC))}, payloadC...)
	if !bytes.Contains(dataC, seqC) {
		t.Fatalf("bc128c 应输出码集 C 前缀,got % x", dataC)
	}
}

// 条码尺寸新语义 "AB":A=模块宽(夹取 ≥2)、B=高度档(高度=(B+1)×24 点)。
func TestBarcodeSizeBytes(t *testing.T) {
	dataDef, _, err := Render([]byte(`[{"type":"bc128","cont":"123"}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(dataDef, []byte{0x1D, 0x68, 72, 0x1D, 0x77, 2}) {
		t.Fatalf("缺省应为 高72/宽2,got % x", dataDef)
	}
	data33, _, err := Render([]byte(`[{"type":"bc128","cont":"123","size":"33"}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data33, []byte{0x1D, 0x68, 96, 0x1D, 0x77, 3}) {
		t.Fatalf("size 33 应为 高96/宽3,got % x", data33)
	}
	data10, _, err := Render([]byte(`[{"type":"bc128","cont":"123","size":"10"}]`), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data10, []byte{0x1D, 0x68, 24, 0x1D, 0x77, 2}) {
		t.Fatalf("size 10 应为 高24/宽2(夹取),got % x", data10)
	}
}

// qrcode size 映射到 GS ( K 模块大小字节:01-09,缺省 04。
func TestQRCodeSizeBytes(t *testing.T) {
	for size, want := range map[string]byte{"": 4, "01": 1, "04": 4, "09": 9, "06": 6} {
		js := `[{"type":"qrcode","cont":"x","size":"` + size + `"}]`
		if size == "" {
			js = `[{"type":"qrcode","cont":"x"}]`
		}
		data, _, err := Render([]byte(js), 80, true)
		if err != nil {
			t.Fatalf("size %q 应合法,got: %v", size, err)
		}
		seq := []byte{0x1D, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x43, want}
		if !bytes.Contains(data, seq) {
			t.Fatalf("size %q 模块应为 %d,got % x", size, want, data)
		}
	}
}

func TestRejectPNGHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	mustErr(t, `[{"type":"png","cont":"`+srv.URL+`"}]`, "HTTP 404")
}

// 整单语义:中间一个非法 → 整体报错(含定位),不产出部分字节。
func TestRejectWholeRequest(t *testing.T) {
	data, _, err := Render([]byte(`[{"cont":"ok"},{"type":"foo"},{"cont":"ok2"}]`), 80, true)
	if err == nil || data != nil {
		t.Fatalf("应整单拒绝,got data=%v err=%v", data != nil, err)
	}
	if !strings.Contains(err.Error(), "contents[1]") {
		t.Fatalf("错误应定位 contents[1],got: %v", err)
	}
}

// —— 合法默认用例(字段缺失 = 文档声明默认,不拒) ——

func TestOKDefaults(t *testing.T) {
	mustOK(t, `[{"cont":"x"}]`, 80)                                             // 无 type = text
	mustOK(t, `[{"type":"text"}]`, 80)                                          // 无 cont = 空行
	mustOK(t, `[{"cont":"x","size":""}]`, 80)                                   // size 空 = 不放大
	mustOK(t, `[{"cont":"x","align":"center","size":"11","bold":true}]`, 80)    // 全合法属性
	mustOK(t, `[{"type":"div_line"},{"type":"div_star","cont":"标题"}]`, 80)      // 分割线
	mustOK(t, `[{"both_sides":["收款金额","258.00"]}]`, 80)                         // 恰 2 段
	mustOK(t, `[{"type":"bc128","cont":"123456789"}]`, 80)                      // 条码缺 size = 默认
	mustOK(t, `[{"type":"code39","cont":"123456789","hri":2,"size":"22"}]`, 80) // 合法条码
	mustOK(t, `[{"type":"qrcode","cont":"http://x"}]`, 80)                      // 单码
	mustOK(t, `[{"type":"qrcode","cont":["http://x","12345"]}]`, 80)            // 双码恰 2 项
	mustOK(t, `[{"type":"qrcode","cont":"http://x","size":"06"}]`, 80)          // size 01-09 合法
	mustOK(t, `[{"type":"bc128","cont":"1234567890"}]`, 80)                     // bc128 默认码集 A
	mustOK(t, `[{"type":"bc128a","cont":"ORDER123"}]`, 80)
	mustOK(t, `[{"type":"bc128c","cont":"1234567890123456789012345678"}]`, 80)                   // 28 位上限
	mustOK(t, `[{"type":"bc128","cont":"123","size":"33"}]`, 80)                                 // 新 AB 尺寸语义
	mustOK(t, `[{"type":"plugin"}]`, 80)                                                         // 开钱箱
	mustOK(t, `[{"type":"text","cont":["第一行","第二行"]}]`, 80)                                      // text 数组多行
	mustOK(t, `[{"thead":{"a":"50%","b":"50%"},"tbody":[["x","y"],["z",0.01],["w",true]]}]`, 80) // 标量单元格
	mustOK(t, `[{"thead":["60%","40%"],"tbody":[["序号1","内容AAA"]]}]`, 80)                         // 数组表头
}

// 显式 null 字段视作缺失(云端全字段序列化常输出 null),不得误判为表格分支。
func TestNullFieldsTreatedAsMissing(t *testing.T) {
	mustOK(t, `[{"type":"text","cont":"普通文本","thead":null}]`, 80)  // thead:null 应走 text,不误拒
	mustOK(t, `[{"cont":"x","thead":null,"both_sides":null}]`, 80) // 多个 null 字段并存 → text
	mustOK(t, `[{"type":"title","cont":"标题","tbody":null}]`, 80)   // tbody:null 不影响
	// thead:null 但确有 tbody → 仍应报「缺 thead」(证明 null 被当缺失,而非当空表格)
	mustErr(t, `[{"thead":null,"tbody":[["a","b"]]}]`, "缺 thead")
}

func TestOKPNGDataURI(t *testing.T) {
	uri, _ := testPNG(t)
	mustOK(t, `[{"type":"png","cont":"`+uri+`","align":"center"}]`, 80)
}

func TestOKPNGHTTP(t *testing.T) {
	_, raw := testPNG(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()
	mustOK(t, `[{"type":"png","cont":"`+srv.URL+`"}]`, 80)
}

// —— 样例硬性验收:内置样票与 测试JSON.txt 的 contents 必须全部通过 ——

func TestSampleContentsPass(t *testing.T) {
	for _, w := range []int{58, 80} {
		if _, _, err := Render(SampleContents, w, true); err != nil {
			t.Fatalf("SampleContents 在 %dmm 下应通过,got: %v", w, err)
		}
	}
}

// 测试JSON.txt 各段的 contents(与文件保持一致的代表性 fixture)。
const fixtureKitchen = `[
  {"size":"22","bold":true,"type":"text","align":"center","cont":"一菜一切(合并)"},
  {"size":"11","bold":true,"type":"text","align":"center","cont":"(面点)"},
  {"size":"11","type":"text","align":"center","cont":"【堂食】"},
  {"size":"11","type":"text","cont":"桌号:A08"},
  {"type":"text","cont":"商户名称:谷养元"},
  {"type":"div_line"},
  {"size":"11","tbody":[["1.手工猪肉大葱小笼包(3个)","1笼"]],"thead":{"商品名称":"85%","数量":"15%"}},
  {"type":"div_line"},
  {"size":"11","bold":true,"type":"text","align":"right","cont":"总计:1件"},
  {"type":"text"},
  {"type":"text"}
]`

const fixtureBill = `[
  {"size":"22","bold":true,"type":"text","align":"center","cont":"结账单"},
  {"size":"11","type":"text","cont":"桌号:null"},
  {"type":"div_line"},
  {"size":"00","tbody":[["1.青椒肉丝(标准|不辣)","1份",0.01],["2.规格测试","1份",0.01]],"thead":{"商品名称":"70%","数量":"15%","金额":"15%"}},
  {"type":"div_line"},
  {"size":"00","bold":true,"type":"text","align":"right","cont":"应付金额:0.02"},
  {"size":"11","bold":true,"type":"text","align":"right","cont":"实付金额:0.02"},
  {"type":"text"}
]`

const fixture58 = `[
  {"size":"11","bold":true,"type":"text","align":"center","cont":"一菜一切(合并)"},
  {"type":"text","cont":"桌号:A08"},
  {"type":"div_line"},
  {"size":"11","tbody":[["1.手工猪肉大葱小笼包(3个)","1笼"]],"thead":{"商品名称":"75%","数量":"25%"}},
  {"type":"div_line"},
  {"size":"11","bold":true,"type":"text","align":"right","cont":"总计:1件"}
]`

func TestCloudFixturesPass(t *testing.T) {
	mustOK(t, fixtureKitchen, 80)
	mustOK(t, fixtureBill, 80)
	mustOK(t, fixture58, 58)
}

// 表格表头恒为正常字,不随 size 放大;tbody 仍随 size 放大。
// 用仅含该表格的 JSON:锁定 GS ! 0x11(放大)出现前必须已输出表头「商品名称」(GB18030),
// 且放大区(GS ! 0x11 起,到下一 GS ! 0x00 止)内不得再出现表头内容。
func TestTableHeaderNotMagnified(t *testing.T) {
	tableOnly := `[{"size":"11","tbody":[["1.手工猪肉大葱小笼包(3个)","1笼"]],"thead":{"商品名称":"85%","数量":"15%"}}]`
	data, _, err := Render([]byte(tableOnly), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	gsMagnify := []byte{0x1D, 0x21, 0x11}
	gsNormal := []byte{0x1D, 0x21, 0x00}
	headerGBK := []byte{0xC9, 0xCC, 0xC6, 0xB7, 0xC3, 0xFB, 0xB3, 0xC6} // "商品名称" GB18030

	idxMag := bytes.Index(data, gsMagnify)
	if idxMag < 0 {
		t.Fatal("预期存在放大指令 GS ! 0x11")
	}
	idxHeader := bytes.Index(data, headerGBK)
	if idxHeader < 0 {
		t.Fatal("预期存在表头 商品名称")
	}
	if idxHeader >= idxMag {
		t.Fatalf("表头须在放大指令之前输出: header@%d, magnify@%d", idxHeader, idxMag)
	}
	rest := data[idxMag+len(gsMagnify):]
	idxReset := bytes.Index(rest, gsNormal)
	if idxReset < 0 {
		t.Fatal("预期存在恢复字号指令 GS ! 0x00")
	}
	if bytes.Contains(rest[:idxReset], headerGBK) {
		t.Fatal("放大区间内不得再出现表头内容")
	}
}

// 列宽按纸宽满行计算:58mm 下 85%/15% 分列为 [27,5](满行 32 字符),
// 表头正常字一行内排下「商品名称」+「数量」,数量不再因列宽 3 而竖折。
func TestTableColumnsFullPaperWidth58(t *testing.T) {
	tableOnly := `[{"size":"11","tbody":[["1.手工猪肉大葱小笼包(3个)","1笼"]],"thead":{"商品名称":"85%","数量":"15%"}}]`
	data, _, err := Render([]byte(tableOnly), 58, true)
	if err != nil {
		t.Fatal(err)
	}
	gsMagnify := []byte{0x1D, 0x21, 0x11}
	idxMag := bytes.Index(data, gsMagnify)
	if idxMag < 0 {
		t.Fatal("预期存在放大指令")
	}
	header := data[:idxMag]
	// 表头行须同时含两列且「数量」与「商品名称」之间有大量补位(85% 列)
	headerGBK := []byte{0xC9, 0xCC, 0xC6, 0xB7, 0xC3, 0xFB, 0xB3, 0xC6}
	qtyGBK := []byte{0xCA, 0xFD, 0xC1, 0xBF} // "数量"
	idxName := bytes.Index(header, headerGBK)
	idxQty := bytes.Index(header, qtyGBK)
	if idxName < 0 || idxQty < 0 {
		t.Fatal("表头须含 商品名称 与 数量")
	}
	if idxQty <= idxName {
		t.Fatal("数量应位于商品名称之后")
	}
	// 58mm: cpl=32 → 85% 列宽 27;表头正常字下商品名称占 8 显示宽,应补约 19 个空格
	gap := idxQty - (idxName + len(headerGBK))
	if gap < 10 {
		t.Fatalf("85%% 列应有明显补位(gap=%d,预期约 19)", gap)
	}
}

// 表格末列须贴右边缘:行尾补位空格不得被 TrimRight 裁掉(对齐厂商输出),
// 且末列内容自带的尾随空格必须原样透传(不修改传输数据)。
// 80mm cpl=48:85%/15% → cols=[40,8];表头行「金额」(显示宽 4)后应补 8-4=4 空格,
// 数据行(size 11,eff=[20,4])「1份」(显示宽 3)后应补 4-3=1 空格,整行物理宽=纸宽。
func TestTableLastColumnFlushRight(t *testing.T) {
	tableOnly := `[{"size":"11","thead":{"商品名称":"85%","金额":"15%"},"tbody":[["1.规格测试","1份"]]}]`

	// 80mm
	data, _, err := Render([]byte(tableOnly), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	amtGBK := gb(t, "金额")
	idx := bytes.Index(data, amtGBK)
	if idx < 0 {
		t.Fatal("表头应含 金额")
	}
	if n := countSpaces(data[idx+len(amtGBK):]); n != 4 {
		t.Fatalf("80mm 表头末列应补 4 空格贴右(列宽 8),got %d", n)
	}
	fenGBK := gb(t, "1份")
	idxF := bytes.Index(data, fenGBK)
	if idxF < 0 {
		t.Fatal("数据行应含 1份")
	}
	if n := countSpaces(data[idxF+len(fenGBK):]); n != 1 {
		t.Fatalf("80mm 放大末列应补 1 空格贴右(eff 4),got %d", n)
	}

	// 58mm:85%/15% → cols=[27,5],表头「金额」(显示宽 4)后应补 5-4=1 空格
	data58, _, err := Render([]byte(tableOnly), 58, true)
	if err != nil {
		t.Fatal(err)
	}
	idx58 := bytes.Index(data58, amtGBK)
	if idx58 < 0 {
		t.Fatal("58mm 表头应含 金额")
	}
	if n := countSpaces(data58[idx58+len(amtGBK):]); n != 1 {
		t.Fatalf("58mm 表头末列应补 1 空格贴右(列宽 5),got %d", n)
	}

	// 末列内容自带尾随空格原样透传:50%/50% → cols=[24,24],
	// "b " 之后为补位 22 空格(内容空格不得被裁),整行 48 显示宽
	trail := `[{"thead":{"a":"50%","b":"50%"},"tbody":[["x","b "]]}]`
	dataT, _, err := Render([]byte(trail), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	bSpGBK := gb(t, "b ")
	idxT := bytes.Index(dataT, bSpGBK)
	if idxT < 0 {
		t.Fatal("末列应含内容 b 及自带尾随空格")
	}
	if n := countSpaces(dataT[idxT+len(bSpGBK):]); n != 22 {
		t.Fatalf("末列内容尾随空格应保留、补位 22 空格,got %d", n)
	}
}

// countSpaces 统计从 b 起始的连续空格数。
func countSpaces(b []byte) int {
	n := 0
	for i := 0; i < len(b) && b[i] == 0x20; i++ {
		n++
	}
	return n
}

func gb(t *testing.T, s string) []byte {
	t.Helper()
	b, err := util.ToGB18030(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// padCell 放大宽度回归:size:"11" 表格的 bug 是 padCell 用 (cw-dw)*scale 补空格,
// 空格又被 GS ! 放大 → 整行物理宽远超纸宽触发打印机换行。Render 层旧 bug 不报错
// (正常吐超宽字节,换行在硬件),mustOK 抓不到;故直接断言 padCell 输出显示宽==列宽(放大单位)。
func TestPadCellWidth(t *testing.T) {
	cases := []struct {
		s  string
		cw int
	}{
		{"1.规格测试", 20}, // 放大2倍:cols=40, eff=20; dw=10, 应补 10 空格 → 宽 20
		{"1份", 4},      // 放大2倍:cols=8, eff=4; dw=3, 应补 1 空格 → 宽 4
		{"商品名称", 40},   // scale=1 表头列:eff=cols=40; dw=8, 补 32 → 宽 40
		{"x", 1},       // 最小列
		{"", 5},        // 空单元格补满
	}
	for _, c := range cases {
		got := padCell(c.s, c.cw)
		w := escpos.DisplayWidth(got)
		if w != c.cw {
			t.Errorf("padCell(%q,%d) 显示宽=%d, 预期 %d (bug:旧逻辑会放大空格导致超宽)",
				c.s, c.cw, w, c.cw)
		}
	}
}

// size:"11" 放大表格数据行物理宽回归:整行拼接后放大倍率下不超过 cpl。
// cols=[40,8] 80mm,eff=[20,4];行 dw=10(pad10)+3(pad1)=23,×2=46 ≤ 48 ✓。
// 旧 bug 下 dw=(10+60)+(3+10)=83,×2=166 >> 48 → 换行。
func TestZoomTableRowFitsPaperWidth(t *testing.T) {
	// 直接验证 tableRow 拼接行的放大物理宽 ≤ cpl:构造 cols/scale,
	// 逐格 padCell 拼接后乘 scale 应 ≤ cpl(末列含取整补偿,cpl=48)。
	cols := []int{40, 8}
	scale := 2
	cpl := 48
	var total int
	for i, s := range []string{"1.规格测试", "1份"} {
		total += escpos.DisplayWidth(padCell(s, cols[i]/scale))
	}
	physWidth := total * scale
	if physWidth > cpl {
		t.Fatalf("放大后行物理宽 %d > 纸宽 %d,打印机会自动换行(bug)",
			physWidth, cpl)
	}
}
