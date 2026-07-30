package layout

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

// —— 拒绝用例(全部严格:字段存在但非法 → 整单拒) ——

func TestRejectUnknownType(t *testing.T) {
	mustErr(t, `[{"type":"foo","cont":"x"}]`, `未知元素 type "foo"`)
}

// 云盒兼容开:contents type=cut 接受;返回切纸意图;不写入 GS V。
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
	if err != nil || cut0 == nil || *cut0 {
		t.Fatalf("cut cont=0 意图应为 false,got %v err=%v", cut0, err)
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
	mustErr(t, `[{"type":"bc128","cont":"123","size":"44"}]`, "条码仅支持")
	mustErr(t, `[{"type":"bc128","cont":"123","hri":4}]`, "hri 4 非法")
	mustErr(t, `[{"type":"code39","cont":""}]`, "条码内容为空")
	// 超长内容会让 GS k 长度字节回绕产出损坏流 → 拒
	mustErr(t, `[{"type":"bc128","cont":"`+strings.Repeat("9", 300)+`"}]`, "过长")
}

// 二维码内容超 QR 容量上限 → 拒(避免损坏指令流)。
func TestRejectQRTooLong(t *testing.T) {
	mustErr(t, `[{"type":"qrcode","cont":"`+strings.Repeat("A", 3000)+`"}]`, "过长")
}

func TestRejectPNG(t *testing.T) {
	mustErr(t, `[{"type":"png","cont":"ftp://x"}]`, "来源非法")
	mustErr(t, `[{"type":"png","cont":""}]`, "png 内容为空")
	mustErr(t, `[{"type":"png","cont":"data:image/png;base64,%%%"}]`, "base64 解码失败")
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
	mustOK(t, `[{"cont":"x"}]`, 80)                                               // 无 type = text
	mustOK(t, `[{"type":"text"}]`, 80)                                            // 无 cont = 空行
	mustOK(t, `[{"cont":"x","size":""}]`, 80)                                     // size 空 = 不放大
	mustOK(t, `[{"cont":"x","align":"center","size":"11","bold":true}]`, 80)      // 全合法属性
	mustOK(t, `[{"type":"div_line"},{"type":"div_star","cont":"标题"}]`, 80)      // 分割线
	mustOK(t, `[{"both_sides":["收款金额","258.00"]}]`, 80)                       // 恰 2 段
	mustOK(t, `[{"type":"bc128","cont":"123456789"}]`, 80)                        // 条码缺 size = 默认
	mustOK(t, `[{"type":"code39","cont":"123456789","hri":2,"size":"22"}]`, 80)   // 合法条码
	mustOK(t, `[{"type":"qrcode","cont":"http://x"}]`, 80)                        // 单码
	mustOK(t, `[{"type":"qrcode","cont":["http://x","12345"]}]`, 80)              // 双码恰 2 项
	mustOK(t, `[{"type":"qrcode","cont":"http://x","size":"60"}]`, 80)            // size 仅警告不拒
	mustOK(t, `[{"thead":{"a":"50%","b":"50%"},"tbody":[["x","y"],["z",0.01],["w",true]]}]`, 80) // 标量单元格
	mustOK(t, `[{"thead":["60%","40%"],"tbody":[["序号1","内容AAA"]]}]`, 80)      // 数组表头
}

// 显式 null 字段视作缺失(云端全字段序列化常输出 null),不得误判为表格分支。
func TestNullFieldsTreatedAsMissing(t *testing.T) {
	mustOK(t, `[{"type":"text","cont":"普通文本","thead":null}]`, 80)   // thead:null 应走 text,不误拒
	mustOK(t, `[{"cont":"x","thead":null,"both_sides":null}]`, 80)     // 多个 null 字段并存 → text
	mustOK(t, `[{"type":"title","cont":"标题","tbody":null}]`, 80)      // tbody:null 不影响
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
