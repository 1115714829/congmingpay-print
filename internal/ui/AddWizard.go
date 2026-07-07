package ui

import (
	"fmt"
	"strings"

	"congmingpay/internal/config"
	"congmingpay/internal/model"
	"congmingpay/internal/transport"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// brandNames 返回品牌下拉的中文选项(与 model.Brands 顺序一致)。
func brandNames() []string {
	out := make([]string, len(model.Brands))
	for i, b := range model.Brands {
		out[i] = string(b)
	}
	return out
}

// runAddWizard 运行「添加打印机」三步向导,返回新打印机或 nil(取消)。
//
// 字段值用 OnTextChanged 实时写入外层变量(键入即捕获),不在对话框关闭后读控件(那样得空)。
func (a *App) runAddWizard() *model.Printer {
	var dlg *walk.Dialog
	var step1, step2, step3, netGroup, usbGroup *walk.Composite
	var backBtn, nextBtn *walk.PushButton
	var stepLabel *walk.Label
	var nameLE, ipLE, portLE *walk.LineEdit
	var usbCB, brandCB *walk.ComboBox
	var buzzerCB, cutCB *walk.CheckBox

	form := struct {
		width int
		conn  model.Conn
	}{width: 80, conn: model.ConnNetwork}
	step := 1

	usbNames, _ := transport.ListRealPrinters() // 过滤掉 OneNote/PDF 等虚拟打印机

	// 实时捕获的字段值(变更事件写入)
	outName, outIP := "", ""
	outPort := "9100"
	outBrandIdx := 0
	outBuzzer := false
	outUSBIdx := 0
	outCut := true

	stepNames := []string{"纸张规格", "连接方式", "设备信息"}
	update := func() {
		step1.SetVisible(step == 1)
		step2.SetVisible(step == 2)
		step3.SetVisible(step == 3)
		netGroup.SetVisible(form.conn == model.ConnNetwork)
		usbGroup.SetVisible(form.conn == model.ConnUSB)
		backBtn.SetEnabled(step > 1)
		if step < 3 {
			nextBtn.SetText("下一步")
		} else {
			nextBtn.SetText("完成")
		}
		stepLabel.SetText(fmt.Sprintf("第 %d / 3 步 — %s", step, stepNames[step-1]))
	}

	if err := (Dialog{
		AssignTo: &dlg,
		Title:    "添加打印机",
		MinSize:  Size{Width: 460, Height: 300},
		Layout:   VBox{Spacing: 10},
		Children: []Widget{
			Label{AssignTo: &stepLabel, Font: Font{PointSize: 10, Bold: true}},

			Composite{
				AssignTo: &step1,
				Layout:   HBox{Spacing: 12, MarginsZero: true},
				Children: []Widget{
					PushButton{Text: "58mm\n窄幅 / 便携小票", MinSize: Size{Height: 60}, OnClicked: func() { form.width = 58; step = 2; update() }},
					PushButton{Text: "80mm\n标准 / 收银·厨房", MinSize: Size{Height: 60}, OnClicked: func() { form.width = 80; step = 2; update() }},
				},
			},

			Composite{
				AssignTo: &step2,
				Layout:   VBox{Spacing: 10, MarginsZero: true},
				Children: []Widget{
					PushButton{Text: "网口 / IP 地址(RAW 9100)", MinSize: Size{Height: 44}, OnClicked: func() { form.conn = model.ConnNetwork; step = 3; update() }},
					PushButton{Text: "USB 直连(经 Windows 驱动)", MinSize: Size{Height: 44}, OnClicked: func() { form.conn = model.ConnUSB; step = 3; update() }},
				},
			},

			Composite{
				AssignTo: &step3,
				Layout:   Grid{Columns: 2, Spacing: 8, MarginsZero: true},
				Children: []Widget{
					Label{Text: "名称 *:"},
					LineEdit{AssignTo: &nameLE, CueBanner: "如:前台收银台-01", OnTextChanged: func() { outName = strings.TrimSpace(nameLE.Text()) }},
					Label{Text: "品牌:"},
					ComboBox{AssignTo: &brandCB, Model: brandNames(), CurrentIndex: 0, OnCurrentIndexChanged: func() { outBrandIdx = brandCB.CurrentIndex() }},
					Label{Text: "打印时蜂鸣:"},
					CheckBox{AssignTo: &buzzerCB, Text: "启用蜂鸣提示", OnCheckedChanged: func() { outBuzzer = buzzerCB.Checked() }},
					Label{Text: "切刀:"},
					CheckBox{AssignTo: &cutCB, Text: "启用切刀(打印后切纸)", Checked: true, OnCheckedChanged: func() { outCut = cutCB.Checked() }},
					Composite{
						AssignTo:   &netGroup,
						ColumnSpan: 2,
						Layout:     Grid{Columns: 2, Spacing: 8, MarginsZero: true},
						Children: []Widget{
							Label{Text: "IP 地址 *:"},
							LineEdit{AssignTo: &ipLE, CueBanner: "192.168.1.100", OnTextChanged: func() { outIP = strings.TrimSpace(ipLE.Text()) }},
							Label{Text: "端口:"},
							LineEdit{AssignTo: &portLE, Text: "9100", OnTextChanged: func() { outPort = strings.TrimSpace(portLE.Text()) }},
						},
					},
					Composite{
						AssignTo:   &usbGroup,
						ColumnSpan: 2,
						Layout:     Grid{Columns: 2, Spacing: 8, MarginsZero: true},
						Children: []Widget{
							Label{Text: "USB 设备:"},
							ComboBox{AssignTo: &usbCB, Model: usbNames, CurrentIndex: 0, OnCurrentIndexChanged: func() { outUSBIdx = usbCB.CurrentIndex() }},
						},
					},
				},
			},

			VSpacer{},
			Composite{
				Layout: HBox{Spacing: 8},
				Children: []Widget{
					PushButton{AssignTo: &backBtn, Text: "上一步", OnClicked: func() {
						if step > 1 {
							step--
							update()
						}
					}},
					HSpacer{},
					PushButton{Text: "取消", OnClicked: func() { dlg.Cancel() }},
					PushButton{AssignTo: &nextBtn, Text: "下一步", OnClicked: func() {
						if step < 3 {
							step++
							update()
							return
						}
						if outName == "" {
							a.warn("添加打印机", "请填写打印机名称。")
							return
						}
						if form.conn == model.ConnNetwork && outIP == "" {
							a.warn("添加打印机", "请填写 IP 地址。")
							return
						}
						if form.conn == model.ConnUSB && (len(usbNames) == 0 || outUSBIdx < 0 || outUSBIdx >= len(usbNames)) {
							a.warn("添加打印机", "未检测到 / 未选择 USB 打印设备。\r\n请先在 Windows 安装该打印机驱动后重试。")
							return
						}
						dlg.Accept()
					}},
				},
			},
		},
	}).Create(a.mw); err != nil {
		return nil
	}

	update()
	if dlg.Run() != walk.DlgCmdOK {
		return nil
	}

	p := &model.Printer{
		ID:            config.NewPrinterID(),
		Name:          outName,
		Brand:         model.BrandForIndex(outBrandIdx),
		Width:         form.width,
		Conn:          form.conn,
		BuzzerEnabled: outBuzzer,
		CutDisabled:   !outCut,
		LastPrint:     "刚刚添加",
	}
	if form.conn == model.ConnNetwork {
		p.IP = outIP
		p.Port = outPort
		if p.Port == "" {
			p.Port = "9100"
		}
	} else if outUSBIdx >= 0 && outUSBIdx < len(usbNames) {
		p.USBName = usbNames[outUSBIdx]
	}
	return p
}
