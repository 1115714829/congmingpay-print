package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"congmingpay/internal/api"
	"congmingpay/internal/model"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// onJSONTest 打开对话框填写打印 JSON(与 MQTT 打印消息同格式),本地直接走 api.Process,等价云端下发。
func (a *App) onJSONTest() {
	sel := a.selectedPrinter()
	sample := strings.ReplaceAll(jsonTestSample(sel), "\n", "\r\n")

	var dlg *walk.Dialog
	var te *walk.TextEdit
	_, _ = (Dialog{
		AssignTo: &dlg,
		Title:    "JSON 打印测试(模拟 MQTT 下发)",
		MinSize:  Size{Width: 580, Height: 500},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "编辑打印 JSON(与 MQTT 打印消息同格式,等价云端下发)。单个对象=打一台,数组 [ {…},{…} ]=打多台。"},
			Label{Text: "目标 printer 可为对象 {name,ip,brand,width}(未注册自动新增)或名字字符串;或用 gateway(IP);留空=选中机。"},
			TextEdit{AssignTo: &te, Text: sample, VScroll: true, Font: Font{Family: "Consolas", PointSize: 9}},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{Text: "打印", OnClicked: func() {
						reqs, wasArray, err := api.ParseRequests([]byte(te.Text()))
						if err != nil {
							a.warn("JSON 打印测试", "JSON 解析失败: "+err.Error())
							return
						}
						ok, fail, firstNo, lastErr := 0, 0, 0, ""
						registered := false
						for i := range reqs {
							if reqs[i].Printer.Empty() && reqs[i].Gateway == "" && sel != nil {
								reqs[i].Printer = api.PrinterRef{Name: sel.Name}
							}
							res, reg, e := api.Process(a.cfg, a.svc, &reqs[i])
							if reg {
								registered = true
							}
							if e != nil {
								fail++
								lastErr = e.Error()
								continue
							}
							ok++
							if firstNo == 0 {
								firstNo = res.JobNo
							}
						}
						// 有目标随打印自动登记 → 持久化、刷新列表并为新机起监测(已在 UI 线程)
						if registered {
							a.save()
							a.refreshPrinters()
							a.syncMonitors()
						}
						if ok == 0 {
							a.warn("JSON 打印测试", lastErr)
							return
						}
						if wasArray {
							a.flash(fmt.Sprintf("已提交 %d 台(失败 %d,见打印队列)", ok, fail))
						} else {
							a.flash(fmt.Sprintf("已提交任务 #%d(见打印队列)", firstNo))
						}
						dlg.Accept()
					}},
					PushButton{Text: "取消", OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}).Run(a.mw)
}

// jsonTestSample 返回「JSON测试」默认模板:云端真实 MQTT 小票(type=0)。
// 目标 printer 用带身份的对象:选中网口机→带其 name/ip/brand/width;选中 USB 机→用名字;
// 没选中→用「飞蛾1」示例(演示未注册自动新增)。pWidth 跟随纸宽,id/pCopy 保留 MQTT 结构。
func jsonTestSample(sel *model.Printer) string {
	target := `"printer": {"name":"飞蛾1","ip":"192.168.68.128","brand":"飞蛾","width":80}`
	width := 80
	if sel != nil {
		width = sel.Width
		if width != 58 && width != 80 {
			width = 80
		}
		if sel.Conn == model.ConnNetwork && sel.IP != "" {
			target = `"printer": {"name":` + jstr(sel.Name) + `,"ip":` + jstr(sel.IP) +
				`,"brand":` + jstr(sel.BrandLabel()) + `,"width":` + strconv.Itoa(width) + `}`
		} else {
			target = `"printer": ` + jstr(sel.Name) // USB 机用名字字符串
		}
	}
	return `{
  ` + target + `,
  "id": 1295873547,
  "type": 0,
  "pWidth": ` + strconv.Itoa(width) + `,
  "pCopy": 1,
  "buzzer": 0, "cut": 1, "reprint": 0, "headLines": 0, "tailLines": 0,
  "contents": [
    {"size":"22","bold":true,"type":"text","align":"center","cont":"一菜一切(合并)"},
    {"size":"11","bold":true,"type":"text","align":"center","cont":"(面点)"},
    {"size":"11","type":"text","align":"center","cont":"【堂食】"},
    {"size":"11","type":"text","cont":"桌号：A08"},
    {"type":"text","cont":"商户名称：谷养元"},
    {"type":"text","cont":"订单编号：10132607031628482179225"},
    {"type":"text","cont":"下单时间：2026-07-03 16:28:50"},
    {"type":"text","cont":"支付时间：2026-07-03 16:29:25"},
    {"type":"div_line"},
    {"size":"11","bold":true,"type":"text","cont":"备注："},
    {"type":"div_line"},
    {"size":"11","tbody":[["1.手工猪肉大葱小笼包（3个）","1笼"]],"thead":{"商品名称":"85%","数量":"15%"}},
    {"type":"div_line"},
    {"size":"11","bold":true,"type":"text","align":"right","cont":"总计：1件"},
    {"type":"text"},
    {"type":"text"},
    {"type":"text"}
  ]
}`
}

// jstr 把字符串编码为 JSON 字面量(含引号,正确转义),用于拼模板。
func jstr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
