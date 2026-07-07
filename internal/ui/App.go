// Package ui 实现 Walk 原生「票据打印服务器 — 管理控制台」界面。
package ui

import (
	"fmt"
	"sync"

	"congmingpay/internal/config"
	"congmingpay/internal/docserver"
	"congmingpay/internal/logger"
	"congmingpay/internal/mqtt"
	"congmingpay/internal/printsvc"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// App 持有全部界面控件与应用状态。
type App struct {
	cfg     *config.Config
	cfgPath string
	svc     *printsvc.Service
	mc      *mqtt.Client
	ds      *docserver.Server

	mw   *walk.MainWindow
	tray *walk.NotifyIcon

	navPrinters  *walk.PushButton
	navJobs      *walk.PushButton
	navSettings  *walk.PushButton
	printersPage *walk.Composite
	jobsPage     *walk.Composite
	settingsPage *walk.Composite

	statusText *walk.Label
	statusMsg  *walk.Label

	// 打印机视图
	printerTV    *walk.TableView
	printerModel *PrinterModel
	filterCB     *walk.ComboBox
	searchLE     *walk.LineEdit

	// 队列视图
	jobTV    *walk.TableView
	jobModel *JobModel

	// 每台打印机一条后台持续 ping 监测(ping -t 式,独立通道)
	monitors   map[string]*monitorHandle // 按打印机 ID
	monitorsMu sync.Mutex

	// 设置视图
	setName     *walk.LineEdit
	mqttEnabled *walk.CheckBox
	mqttBroker  *walk.LineEdit
	mqttPort    *walk.LineEdit
	mqttUser    *walk.LineEdit
	mqttPass    *walk.LineEdit
	mqttTopic   *walk.LineEdit
	mqttStatus  *walk.Label
	docEnabled  *walk.CheckBox
	docPort     *walk.LineEdit
	docURL      *walk.Label
}

// NewApp 创建界面控制器。
func NewApp(cfg *config.Config, cfgPath string, svc *printsvc.Service, mc *mqtt.Client, ds *docserver.Server) *App {
	return &App{
		cfg: cfg, cfgPath: cfgPath, svc: svc, mc: mc, ds: ds,
		printerModel: newPrinterModel(), jobModel: newJobModel(),
		monitors: map[string]*monitorHandle{},
	}
}

// Run 构建并运行主窗口,阻塞直到退出。
func (a *App) Run() error {
	icon := loadAppIcon()

	if err := (MainWindow{
		AssignTo: &a.mw,
		Title:    "票据打印服务器 — 管理控制台",
		MinSize:  Size{Width: 900, Height: 560},
		Size:     Size{Width: 1000, Height: 640},
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, SpacingZero: true},
				Children: []Widget{
					Composite{
						MinSize: Size{Width: 150},
						MaxSize: Size{Width: 150},
						Layout:  VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}, Spacing: 6},
						Children: []Widget{
							PushButton{AssignTo: &a.navPrinters, Text: "打印机", MinSize: Size{Height: 40}, OnClicked: func() { a.showPage(0) }},
							PushButton{AssignTo: &a.navJobs, Text: "打印队列", MinSize: Size{Height: 40}, OnClicked: func() { a.showPage(1) }},
							PushButton{AssignTo: &a.navSettings, Text: "系统设置", MinSize: Size{Height: 40}, OnClicked: func() { a.showPage(2) }},
							VSpacer{},
						},
					},
					Composite{
						Layout: VBox{MarginsZero: true, SpacingZero: true},
						Children: []Widget{
							a.printersPageWidget(),
							a.jobsPageWidget(),
							a.settingsPageWidget(),
						},
					},
				},
			},
			a.statusBarWidget(),
		},
	}).Create(); err != nil {
		return err
	}

	if icon != nil {
		_ = a.mw.SetIcon(icon)
	}
	if err := a.setupTray(icon); err != nil {
		logger.Errorf("托盘初始化失败: %v", err)
	}

	// 任务状态变化 → marshal 回 UI 线程刷新
	a.svc.SetNotify(func() {
		a.mw.Synchronize(func() {
			a.refreshJobs()
			a.updateStatusBar()
		})
	})

	// 云端下发打印机(随打印自动登记)→ marshal 回 UI 线程刷新列表并同步监测
	if a.mc != nil {
		a.mc.SetOnChange(func() {
			a.mw.Synchronize(func() {
				a.refreshPrinters()
				a.syncMonitors()
			})
		})
		// 连接状态一变(连上/断开/首连失败)→ 刷新 MQTT 状态标签,不再卡"连接中…"
		a.mc.SetOnStatus(func() {
			a.mw.Synchronize(func() {
				a.refreshMqttStatus()
			})
		})
	}

	a.showPage(0)
	a.refreshPrinters()
	a.syncMonitors() // 为每台已注册打印机起后台持续 ping 监测

	a.mw.Run()
	a.stopAllMonitors()
	return nil
}

func (a *App) statusBarWidget() Composite {
	return Composite{
		MaxSize: Size{Height: 26},
		Layout:  HBox{Margins: Margins{Left: 10, Right: 10, Top: 2, Bottom: 2}, Spacing: 16},
		Children: []Widget{
			Label{AssignTo: &a.statusText, Text: "就绪"},
			HSpacer{},
			Label{AssignTo: &a.statusMsg, Text: ""},
		},
	}
}

func (a *App) showPage(i int) {
	if a.printersPage == nil {
		return
	}
	a.printersPage.SetVisible(i == 0)
	a.jobsPage.SetVisible(i == 1)
	a.settingsPage.SetVisible(i == 2)
	// 禁用当前页对应的导航按钮以示"选中"
	a.navPrinters.SetEnabled(i != 0)
	a.navJobs.SetEnabled(i != 1)
	a.navSettings.SetEnabled(i != 2)
	if i == 1 {
		a.refreshJobs()
	}
	if i == 2 {
		a.refreshMqttStatus() // 打开设置页时刷新 MQTT 连接状态
	}
}

// warn 弹出警告提示框(并落日志)。
func (a *App) warn(title, msg string) {
	logger.Info(msg)
	if a.mw != nil {
		walk.MsgBox(a.mw, title, msg, walk.MsgBoxIconWarning)
	}
}

func (a *App) updateStatusBar() {
	total := len(a.cfg.Printers)
	online := 0
	for _, p := range a.cfg.Printers {
		if s, ok := a.printerModel.status[p.ID]; ok && s.label == "就绪" {
			online++
		}
	}
	active := a.svc.ActiveCount()
	if a.statusText != nil {
		_ = a.statusText.SetText(fmt.Sprintf("共 %d 台设备 · %d 在线 · %d 个任务进行中", total, online, active))
	}
	if a.navJobs != nil {
		if active > 0 {
			_ = a.navJobs.SetText(fmt.Sprintf("打印队列 (%d)", active))
		} else {
			_ = a.navJobs.SetText("打印队列")
		}
	}
}

// flash 在状态栏右侧显示一条临时消息,并落日志。
func (a *App) flash(msg string) {
	if a.statusMsg != nil {
		_ = a.statusMsg.SetText(msg)
	}
	logger.Info(msg)
}

func (a *App) save() {
	if err := a.cfg.Save(a.cfgPath); err != nil {
		logger.Errorf("保存配置失败: %v", err)
	}
}
