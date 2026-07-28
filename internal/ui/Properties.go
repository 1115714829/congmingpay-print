package ui

import (
	"strconv"
	"strings"

	"congmingpay/internal/model"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// runProperties 运行属性对话框,编辑打印机;点「确定」返回 true。
// 字段用变更事件实时捕获,不在对话框关闭后读控件。
func (a *App) runProperties(p *model.Printer) bool {
	var dlg *walk.Dialog
	var nameLE, ipLE, portLE, headLE, tailLE *walk.LineEdit
	var brandCB *walk.ComboBox
	var buzzerCB, cutCB *walk.CheckBox
	isNet := p.Conn == model.ConnNetwork

	outName, outIP, outPort := p.Name, p.IP, p.Port
	outBrandIdx := model.IndexOfBrand(p.Brand)
	outBuzzer := p.BuzzerEnabled
	outHead, outTail := p.HeadLines, p.TailLines
	outCut := p.Cuts()

	fields := []Widget{
		Label{Text: "名称:"},
		LineEdit{AssignTo: &nameLE, Text: p.Name, OnTextChanged: func() { outName = strings.TrimSpace(nameLE.Text()) }},
		Label{Text: "来源:"},
		Label{Text: p.SourceLabel()},
		Label{Text: "品牌:"},
		ComboBox{AssignTo: &brandCB, Model: brandNames(), CurrentIndex: outBrandIdx, OnCurrentIndexChanged: func() { outBrandIdx = brandCB.CurrentIndex() }},
		Label{Text: "打印时蜂鸣:"},
		CheckBox{AssignTo: &buzzerCB, Text: "启用蜂鸣提示", Checked: p.BuzzerEnabled, OnCheckedChanged: func() { outBuzzer = buzzerCB.Checked() }},
		Label{Text: "切刀:"},
		CheckBox{AssignTo: &cutCB, Text: "启用切刀(打印后切纸)", Checked: p.Cuts(), OnCheckedChanged: func() { outCut = cutCB.Checked() }},
		Label{Text: "规格:"},
		Label{Text: p.WidthLabel()},
		Label{Text: "上次打印:"},
		Label{Text: p.LastPrintLabel()},
		Label{Text: "首部空行:"},
		LineEdit{AssignTo: &headLE, Text: strconv.Itoa(p.HeadLines), OnTextChanged: func() { outHead = atoiOr(headLE.Text(), 0) }},
		Label{Text: "尾部空行:"},
		LineEdit{AssignTo: &tailLE, Text: strconv.Itoa(p.TailLines), OnTextChanged: func() { outTail = atoiOr(tailLE.Text(), 0) }},
	}
	if isNet {
		fields = append(fields,
			Label{Text: "IP 地址:"}, LineEdit{AssignTo: &ipLE, Text: p.IP, OnTextChanged: func() { outIP = strings.TrimSpace(ipLE.Text()) }},
			Label{Text: "端口:"}, LineEdit{AssignTo: &portLE, Text: p.Port, OnTextChanged: func() { outPort = strings.TrimSpace(portLE.Text()) }},
		)
	} else {
		fields = append(fields, Label{Text: "连接:"}, Label{Text: "USB 直连(" + p.USBName + ")"})
	}

	result, _ := (Dialog{
		AssignTo: &dlg,
		Title:    p.Name + " — 属性",
		MinSize:  Size{Width: 420, Height: 280},
		Layout:   VBox{Spacing: 10},
		Children: []Widget{
			Composite{Layout: Grid{Columns: 2, Spacing: 8, MarginsZero: true}, Children: fields},
			VSpacer{},
			Composite{
				Layout: HBox{Spacing: 8},
				Children: []Widget{
					PushButton{Text: "测试打印", OnClicked: func() { a.testPrinter(p) }},
					HSpacer{},
					PushButton{Text: "取消", OnClicked: func() { dlg.Cancel() }},
					PushButton{Text: "确定", OnClicked: func() {
						if outName == "" {
							a.warn("属性", "名称不能为空。")
							return
						}
						dlg.Accept()
					}},
				},
			},
		},
	}).Run(a.mw)

	if result != walk.DlgCmdOK {
		return false
	}
	// 经 Config 写锁更新字段:与 MQTT 上报(PrinterSnapshots 读锁)、云端参数覆盖互斥,避免并发读写撕裂。
	a.cfg.UpdatePrinterFields(p.ID, func(pp *model.Printer) {
		pp.Name = outName
		pp.Brand = model.BrandForIndex(outBrandIdx)
		pp.BuzzerEnabled = outBuzzer
		pp.CutDisabled = !outCut
		pp.HeadLines = outHead
		pp.TailLines = outTail
		if isNet {
			pp.IP = outIP
			pp.Port = outPort
		}
	})
	return true
}
