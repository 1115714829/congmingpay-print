// Package ui 实现 Walk 原生「票据打印服务器 — 管理控制台」界面。
package ui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"congmingpay/internal/config"
	"congmingpay/internal/docserver"
	"congmingpay/internal/logger"
	"congmingpay/internal/model"
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

	// 系统通知(Notify.go)
	notifyReady  atomic.Bool // 托盘就绪后置 true;守卫早于托盘/晚于退出的通知与告警投递
	mqttNotifySt int         // MQTT 通知边沿状态:0=中性(未启用/未定) 1=已连 2=已断;仅 UI 线程读写

	// 常驻告警窗(AlertWindow.go;alertDlg/alertTV/alertModel 仅 UI 线程读写)
	alertDlg   *walk.Dialog
	alertTV    *walk.TableView
	alertModel *AlertModel

	// 居中悬浮提示窗(Toast.go;仅 UI 线程读写)
	toastDlg         *walk.Dialog
	toastLabel       *walk.Label
	toastButtons     *walk.Composite
	toastBtnYes      *walk.PushButton
	toastBtnNo       *walk.PushButton
	toastTimer       *time.Timer // walk v0.0.0-2021 无 Timer,用标准库 AfterFunc(仅 UI 线程换发/停)
	toastConfirmMode bool // true=确认态(不自动消失,须点按钮)
	toastCb          func(bool)

	// 关闭按钮最小化到托盘(仅 UI 线程读写)
	quitting      bool // 托盘「退出」置位:放行 Closing,不再拦截为隐藏
	trayHintShown bool // 首次最小化到托盘的提示,每次运行至多一条

	navPrinters *walk.PushButton
	navJobs     *walk.PushButton
	navGeneral  *walk.PushButton
	navMqtt     *walk.PushButton
	printersPage *walk.Composite
	jobsPage     *walk.Composite
	generalPage  *walk.Composite
	mqttPage     *walk.Composite

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
	setName         *walk.LineEdit
	notifyEnabled   *walk.CheckBox
	yunheCompat     *walk.CheckBox // 云盒兼容模式(勾选=开=C1–C4)
	jobHistoryDays  *walk.LineEdit
	mqttEnabled     *walk.CheckBox
	mqttProvGeneric *walk.RadioButton // Provider 三选一:自建 / 云消息队列 / 物联网
	mqttProvAli     *walk.RadioButton
	mqttProvIot     *walk.RadioButton
	mqttForms       *walk.Composite // 三组 Provider 表单容器(子项顺序:自建/云消息队列/物联网)
	// 超管解锁(会话态,重启回落):锁定态隐藏接入方式行与 IoT 5 个预置参数行
	mqttProvRow *walk.Composite
	mqttIotRegion   *walk.Composite // IoT 隐藏行:地域
	mqttIotEndpoint *walk.Composite // IoT 隐藏行:MQTT 地址
	mqttIotPort     *walk.Composite // IoT 隐藏行:端口
	mqttIotDown     *walk.Composite // IoT 隐藏行:下行后缀
	mqttIotUp       *walk.Composite // IoT 隐藏行:上行后缀
	// IoT 绑定回填行(常驻隐藏,「获取设备信息」自动填充;超管排查时可展开)
	mqttIotSN      *walk.Composite // DeviceName(与 SN 下拉同步)
	mqttIotProduct *walk.Composite // ProductKey
	mqttIotSecret  *walk.Composite // DeviceSecret
	mqttIotManage  *walk.Composite // 设备源地址
	mqttIotMerchant *walk.Composite // 商户号(保存后上报绑定用)
	// 自建
	mqttBroker *walk.LineEdit
	mqttPort   *walk.LineEdit
	mqttUser   *walk.LineEdit
	mqttPass   *walk.LineEdit
	mqttTopic  *walk.LineEdit
	mqttReport *walk.LineEdit
	// 阿里云云消息队列 MQTT 版
	aliEndpoint   *walk.LineEdit
	aliPort       *walk.LineEdit
	aliInstance   *walk.LineEdit
	aliAccessKey  *walk.LineEdit
	aliSecretKey  *walk.LineEdit
	aliGroupId    *walk.LineEdit
	aliParent     *walk.LineEdit
	aliDeviceId   *walk.LineEdit
	aliDownSuffix *walk.LineEdit
	aliUpSuffix   *walk.LineEdit
	aliPreviewSub *walk.Label
	aliPreviewRep *walk.Label
	aliPreviewCID *walk.Label
	// 阿里云物联网平台(一机一密)
	iotSNBox        *walk.ComboBox // 自由打印SN 下拉(禁止手输,数据从管理端获取)
	iotSnOpts       []iotDevice    // 与 iotSNBox 下拉项一一对应(名称+密钥,选中回填用)
	iotProductKey   *walk.LineEdit
	iotDeviceName   *walk.LineEdit
	iotDeviceSecret *walk.LineEdit
	iotManageServer *walk.LineEdit
	iotMerchantNo   *walk.LineEdit // 获取设备信息时使用的商户号(保存后上报绑定用)
	iotRegionId     *walk.LineEdit
	iotEndpoint     *walk.LineEdit
	iotPort         *walk.LineEdit
	iotDownSuffix   *walk.LineEdit
	iotUpSuffix     *walk.LineEdit
	iotPreviewSub   *walk.Label
	iotPreviewRep   *walk.Label
	iotPreviewHost  *walk.Label
	iotPreviewUser  *walk.Label
	iotPreviewCID   *walk.Label
	mqttStatus      *walk.Label
	docEnabled      *walk.CheckBox
	docPort         *walk.LineEdit
	docURL          *walk.Label
}

// NewApp 创建界面控制器。
func NewApp(cfg *config.Config, cfgPath string, svc *printsvc.Service, mc *mqtt.Client, ds *docserver.Server) *App {
	return &App{
		cfg: cfg, cfgPath: cfgPath, svc: svc, mc: mc, ds: ds,
		printerModel: newPrinterModel(), jobModel: newJobModel(),
		alertModel: &AlertModel{},
		monitors:   map[string]*monitorHandle{},
	}
}

// serviceName 返回设置的「服务名称」(空则回退默认值),用于窗口标题与托盘提示。
func (a *App) serviceName() string {
	if name := strings.TrimSpace(a.cfg.Settings.ServiceName); name != "" {
		return name
	}
	return model.DefaultSettings().ServiceName
}

// windowTitle 返回主窗口标题(服务名称 + 固定后缀)。
func (a *App) windowTitle() string { return a.serviceName() + " — 管理控制台" }

// Run 构建并运行主窗口,阻塞直到退出。
func (a *App) Run() error {
	icon := loadAppIcon()

	if err := (MainWindow{
		AssignTo: &a.mw,
		Title:    a.windowTitle(),
		MinSize:  Size{Width: 780, Height: 560},
		Size:     Size{Width: 800, Height: 600},
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, SpacingZero: true},
				Children: []Widget{
					Composite{
						MinSize: Size{Width: 100},
						MaxSize: Size{Width: 100},
						Layout:  VBox{Margins: Margins{Left: 6, Top: 6, Right: 6, Bottom: 6}, Spacing: 4},
						Children: []Widget{
							PushButton{AssignTo: &a.navPrinters, Text: "打印机", MinSize: Size{Height: 34}, OnClicked: func() { a.showPage(0) }},
							PushButton{AssignTo: &a.navJobs, Text: "打印队列", MinSize: Size{Height: 34}, OnClicked: func() { a.showPage(1) }},
							PushButton{AssignTo: &a.navGeneral, Text: "常规", MinSize: Size{Height: 34}, OnClicked: func() { a.showPage(2) }},
							PushButton{AssignTo: &a.navMqtt, Text: "云端 MQTT", MinSize: Size{Height: 34}, OnClicked: func() { a.showPage(3) }},
							VSpacer{},
						},
					},
					Composite{
						Layout: VBox{MarginsZero: true, SpacingZero: true},
						Children: []Widget{
							a.printersPageWidget(),
							a.jobsPageWidget(),
							a.generalPageWidget(),
							a.mqttPageWidget(),
						},
					},
				},
			},
			a.statusBarWidget(),
		},
	}).Create(); err != nil {
		return err
	}
	// 按已保存 Provider 初始选中对应单选,并挂切换回调(显隐三组表单)
	a.initMqttProviderSelection()

	// 坑:walk 创建时若布局最小尺寸大于设定 Size,窗口会被自动撑大且之后不缩回
	// (FormBase.SetBoundsPixels/startLayout 按 MinSizeForSize 钳制)。非首屏页构建时
	// 已隐藏(Visible=false)、非选中 MQTT 表单也已隐藏,此处按物理像素强制 800×600,
	// 不随 DPI 缩放换算,保证低分辨率目标机首开即用满屏。
	_ = a.mw.SetSizePixels(walk.Size{Width: 800, Height: 600})

	if icon != nil {
		_ = a.mw.SetIcon(icon)
	}
	// 关闭按钮(X/Alt+F4)最小化到系统托盘,程序保持运行;退出仅经托盘右键菜单「退出」
	// (置 quitting 放行;托盘退出走 walk.App().Exit 本就不经 Closing)。
	// 系统关机/注销走 WM_QUERYENDSESSION,不发 WM_CLOSE,此拦截不阻塞关机。
	a.mw.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		if a.quitting {
			return
		}
		*canceled = true
		a.mw.Hide()
		if !a.trayHintShown {
			a.trayHintShown = true
			a.notify(notifyInfo, a.serviceName(),
				"程序已最小化到系统托盘,继续在后台运行。点击托盘图标打开主窗口;退出程序经托盘右键菜单的「退出」。")
		}
	})
	if err := a.setupTray(icon); err != nil {
		logger.Errorf("托盘初始化失败: %v", err)
	} else {
		a.notifyReady.Store(true) // 托盘就绪,系统通知可投递
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
				a.mqttNotifyEdge() // 断线/恢复系统通知(边沿去重)
			})
		})
		// 云端打印受理失败 → 常驻告警窗(无自动消除,仅手动清空)
		a.mc.SetOnAcceptFailed(func(id uint32, code int, message string) {
			a.NotifyAcceptFailed(id, code, message)
		})
	}

	a.showPage(0)
	a.refreshPrinters()
	a.syncMonitors() // 为每台已注册打印机起后台持续 ping 监测

	a.mw.Run()
	a.notifyReady.Store(false) // 窗口已退出,后续通知/告警直接丢弃
	if a.alertDlg != nil {
		a.alertDlg.Dispose() // mw.Run 返回后仍在 UI 线程,可安全销毁
	}
	if a.toastDlg != nil {
		a.toastDlg.Dispose()
	}
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
	a.generalPage.SetVisible(i == 2)
	a.mqttPage.SetVisible(i == 3)
	// 禁用当前页对应的导航按钮以示"选中"
	a.navPrinters.SetEnabled(i != 0)
	a.navJobs.SetEnabled(i != 1)
	a.navGeneral.SetEnabled(i != 2)
	a.navMqtt.SetEnabled(i != 3)
	if i == 1 {
		a.refreshJobs()
	}
	if i == 3 {
		a.refreshMqttStatus() // 打开云端 MQTT 页时刷新连接状态
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
		_ = a.statusText.SetText(fmt.Sprintf("共 %d 台设备 · %d 在线 · %d 个任务进行中 · v%s",
			total, online, active, config.AppVersion))
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
