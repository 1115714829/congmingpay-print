package ui

import (
	"strings"
	"time"

	"congmingpay/internal/escpos"
	"congmingpay/internal/layout"
	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func (a *App) printersPageWidget() Composite {
	return Composite{
		AssignTo: &a.printersPage,
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			// 工具栏总最小宽须 ≤ 内容宽 686(800-100 导航-14 边框),且每个页面都满足——
			// 否则切页时布局最小宽度变化会把主窗口撑大(walk 按 MinSizeForSize 钳制,且只撑不缩)。
			Composite{
				Layout: HBox{Margins: Margins{Left: 6, Top: 4, Right: 6, Bottom: 4}, Spacing: 2},
				Children: []Widget{
					PushButton{Text: "新增", MinSize: Size{Width: 50}, MaxSize: Size{Width: 58}, OnClicked: a.onAddPrinter},
					PushButton{Text: "刷新", MinSize: Size{Width: 50}, MaxSize: Size{Width: 58}, OnClicked: a.onRefreshPrinters},
					PushButton{Text: "测试打印", MinSize: Size{Width: 64}, MaxSize: Size{Width: 72}, OnClicked: a.onTestPrint},
					PushButton{Text: "打印样票", MinSize: Size{Width: 64}, MaxSize: Size{Width: 72}, OnClicked: a.onSampleLayout},
					PushButton{Text: "JSON测试", MinSize: Size{Width: 64}, MaxSize: Size{Width: 72}, OnClicked: a.onJSONTest},
					PushButton{Text: "属性", MinSize: Size{Width: 50}, MaxSize: Size{Width: 58}, OnClicked: a.onProperties},
					PushButton{Text: "删除", MinSize: Size{Width: 50}, MaxSize: Size{Width: 58}, OnClicked: a.onDeletePrinter},
					HSpacer{},
					ComboBox{
						AssignTo:              &a.filterCB,
						MinSize:               Size{Width: 78},
						MaxSize:               Size{Width: 78},
						Model:                 []string{"全部", "58mm", "80mm", "在线"},
						CurrentIndex:          0,
						OnCurrentIndexChanged: a.onFilterChanged,
					},
					LineEdit{
						AssignTo:      &a.searchLE,
						MinSize:       Size{Width: 110},
						MaxSize:       Size{Width: 110},
						CueBanner:     "搜索名称/品牌/IP",
						OnTextChanged: a.onFilterChanged,
					},
				},
			},
			TableView{
				AssignTo:            &a.printerTV,
				Model:               a.printerModel,
				StyleCell:           a.printerModel.StyleCell,
				LastColumnStretched: true, // 窗口放大时末列(上次打印时间)吃掉剩余宽度,表格填满
				OnItemActivated:     a.onProperties,
				Columns: []TableViewColumn{
					{Title: "名称", Width: 118},
					{Title: "来源", Width: 56},
					{Title: "品牌", Width: 52},
					{Title: "规格", Width: 46},
					{Title: "连接", Width: 44},
					{Title: "地址 / 设备", Width: 118},
					{Title: "状态", Width: 56},
					{Title: "上次打印时间", Width: 118},
				},
			},
		},
	}
}

func (a *App) selectedPrinter() *model.Printer {
	i := a.printerTV.CurrentIndex()
	if i < 0 || i >= len(a.printerModel.items) {
		return nil
	}
	return a.printerModel.items[i]
}

func (a *App) refreshPrinters() {
	// 记住当前选中,刷新后恢复(PublishRowsReset 会清空选择)
	selID := ""
	if p := a.selectedPrinter(); p != nil {
		selID = p.ID
	}
	a.printerModel.items = a.filteredPrinters()
	publishTableRefresh(&a.printerModel.TableModelBase, len(a.printerModel.items))
	a.updateStatusBar()
	if selID != "" && a.printerTV != nil {
		for i, p := range a.printerModel.items {
			if p.ID == selID {
				_ = a.printerTV.SetCurrentIndex(i)
				break
			}
		}
	}
}

func (a *App) filteredPrinters() []*model.Printer {
	filter := 0
	if a.filterCB != nil {
		filter = a.filterCB.CurrentIndex()
	}
	query := ""
	if a.searchLE != nil {
		query = strings.ToLower(strings.TrimSpace(a.searchLE.Text()))
	}
	var out []*model.Printer
	for _, p := range a.cfg.Printers {
		switch filter {
		case 1:
			if p.Width != 58 {
				continue
			}
		case 2:
			if p.Width != 80 {
				continue
			}
		case 3:
			if s, ok := a.printerModel.status[p.ID]; !ok || s.label != "就绪" {
				continue
			}
		}
		if query != "" &&
			!strings.Contains(strings.ToLower(p.Name), query) &&
			!strings.Contains(strings.ToLower(p.BrandLabel()), query) &&
			!strings.Contains(strings.ToLower(p.IP), query) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (a *App) onFilterChanged() { a.refreshPrinters() }

func (a *App) onRefreshPrinters() {
	a.syncMonitors() // 确保每台监测在跑(状态本就每秒持续检测)
	a.refreshPrinters()
	a.flash("设备状态持续检测中(每秒 ping)")
}

func (a *App) onAddPrinter() {
	p := a.runAddWizard()
	if p == nil {
		return
	}
	a.cfg.AddPrinter(p)
	a.save()
	a.refreshPrinters()
	a.syncMonitors()
	logger.Infof("已添加打印机 名称『%s』品牌『%s』规格 %s 连接 %s %s", p.Name, p.BrandLabel(), p.WidthLabel(), p.ConnLabel(), p.Target())
	a.flash("已添加打印机「" + p.Name + "」")
}

func (a *App) onTestPrint() {
	p := a.selectedPrinter()
	if p == nil {
		a.warn("测试打印", "请先在列表中选择一台打印机。")
		return
	}
	a.testPrinter(p)
}

func (a *App) testPrinter(p *model.Printer) {
	// 打印前预检,避免提交必然失败的任务
	if p.Conn == model.ConnUSB && p.USBName == "" {
		a.warn("测试打印", "打印机「"+p.Name+"」未绑定 USB 设备。\r\n请在属性中重新选择,或删除后重新添加。")
		return
	}
	if p.Conn == model.ConnNetwork && p.IP == "" {
		a.warn("测试打印", "打印机「"+p.Name+"」未填写 IP 地址。")
		return
	}
	data, err := escpos.BuildTestReceipt(time.Now(), p.Width)
	if err != nil {
		a.warn("测试打印", "生成小票失败: "+err.Error())
		return
	}
	a.svc.Submit(p, "测试页", data, nil)
	a.refreshPrinters()
	a.flash("已向「" + p.Name + "」发送测试页(见打印队列)")
}

// onSampleLayout 用 JSON 排版渲染器打印一张覆盖各元素的样票(将来 MQTT 亦走 layout.Render)。
func (a *App) onSampleLayout() {
	p := a.selectedPrinter()
	if p == nil {
		a.warn("打印样票", "请先在列表中选择一台打印机。")
		return
	}
	data, _, err := layout.Render(layout.SampleContents, p.Width, a.cfg.Settings.YunheCompat())
	if err != nil {
		a.warn("打印样票", "生成样票失败: "+err.Error())
		return
	}
	ct := printsvc.ContentJSON
	a.svc.Submit(p, "排版样票", data, &printsvc.Options{
		ContentType: &ct,
		SourceJSON:  append([]byte(nil), layout.SampleContents...),
	})
	a.refreshPrinters()
	a.flash("已向「" + p.Name + "」发送排版样票")
}

func (a *App) onProperties() {
	p := a.selectedPrinter()
	if p == nil {
		a.warn("属性", "请先选择一台打印机。")
		return
	}
	if a.runProperties(p) {
		a.save()
		a.refreshPrinters()
		a.syncMonitors() // 改了 IP 等 → 按 sig 变化重启该机监测
		a.flash("已保存「" + p.Name + "」的属性")
	}
}

func (a *App) onDeletePrinter() {
	p := a.selectedPrinter()
	if p == nil {
		a.warn("删除打印机", "请先选择要删除的打印机。")
		return
	}
	if walk.MsgBox(a.mw, "删除打印机", "确定删除「"+p.Name+"」?", walk.MsgBoxIconQuestion|walk.MsgBoxYesNo) != walk.DlgCmdYes {
		return
	}
	a.cfg.RemovePrinter(p.ID)
	a.save()
	a.refreshPrinters()
	a.syncMonitors() // 停掉被删打印机的监测
	// 事件源已消失,恢复边沿永不再来:挂起的离线告警在此收尾,否则永久残留
	a.alertResolve(alertPrinterOffline, p.ID)
	a.flash("已删除「" + p.Name + "」")
}
