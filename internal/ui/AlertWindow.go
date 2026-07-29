package ui

import (
	"fmt"
	"time"
	"unsafe"

	"congmingpay/internal/logger"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

// 常驻告警窗:屏幕右下角置顶小窗,不抢焦点、不进任务栏,须手动处理或对应事件恢复才消失。
// 系统 Toast/气泡的停留时长由系统全局设置控制、应用无权常驻,故自绘此窗承担「持久醒目」职责。

// alertKind+key 唯一标识一条告警:raise 幂等更新(不重复加行),resolve 按标识移除。
type alertKind string

const (
	alertJobFailed      alertKind = "job-failed"      // key=任务号
	alertJobWaiting     alertKind = "job-waiting"     // key=任务号
	alertPrinterOffline alertKind = "printer-offline" // key=打印机 ID
	alertMqttDown       alertKind = "mqtt-down"       // key=""
	alertAcceptFailed   alertKind = "accept-failed"   // key=请求 id(无自动消除,仅手动清空)
)

// alertSeverity 决定行颜色:warn=橙,error=红。
type alertSeverity int

const (
	alertWarn alertSeverity = iota
	alertError
)

// alertMaxItems 是告警窗条目上限(故障风暴封顶,超出丢最旧)。
const alertMaxItems = 200

type alertItem struct {
	kind alertKind
	key  string
	sev  alertSeverity
	text string
	when time.Time // 事件起始时刻(打印机离线=断连起点 failSince,其余=挂起时刻);「已持续」列据此动态计时
}

// durText 把时长格式化为中文短文本(告警窗「已持续」列)。
func durText(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d / time.Second)
	switch {
	case s < 60:
		return fmt.Sprintf("%d秒", s)
	case s < 3600:
		return fmt.Sprintf("%d分%02d秒", s/60, s%60)
	default:
		return fmt.Sprintf("%d时%d分", s/3600, s%3600/60)
	}
}

// AlertModel 是告警窗 TableView 模型(列:时间/内容;按 severity 上色)。仅 UI 线程读写。
type AlertModel struct {
	walk.TableModelBase
	items []*alertItem
}

func (m *AlertModel) RowCount() int { return len(m.items) }

func (m *AlertModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.items) {
		return ""
	}
	it := m.items[row]
	switch col {
	case 0:
		return it.when.Format("15:04:05")
	case 1:
		return durText(time.Since(it.when)) // 绘制时实时计算,由 alertTickLoop 每秒触发重绘
	case 2:
		return it.text
	}
	return ""
}

// StyleCell 按告警级别上色:error 红、warn 橙。
func (m *AlertModel) StyleCell(style *walk.CellStyle) {
	if r := style.Row(); r >= 0 && r < len(m.items) {
		if m.items[r].sev == alertError {
			style.TextColor = colRed
		} else {
			style.TextColor = colOrange
		}
	}
}

// alertRaise 挂起/更新一条告警并弹出告警窗。任意 goroutine 可调:
// notifyReady 守卫 + Synchronize 只投递,满足 printsvc 回调「快速返回不阻塞」契约。
// 同 kind+key 原位更新(时间/文本/级别),新告警插最上。since=事件起始时刻(「已持续」列起点)。
func (a *App) alertRaise(kind alertKind, key string, sev alertSeverity, text string, since time.Time) {
	if !a.notifyReady.Load() {
		return
	}
	a.mw.Synchronize(func() {
		for _, it := range a.alertModel.items {
			if it.kind == kind && it.key == key {
				it.sev, it.text, it.when = sev, text, since
				publishTableRefresh(&a.alertModel.TableModelBase, len(a.alertModel.items))
				a.alertShow()
				return
			}
		}
		items := append([]*alertItem{{kind: kind, key: key, sev: sev, text: text, when: since}},
			a.alertModel.items...)
		if len(items) > alertMaxItems {
			items = items[:alertMaxItems]
		}
		a.alertModel.items = items
		publishTableRefresh(&a.alertModel.TableModelBase, len(a.alertModel.items))
		a.alertShow()
	})
}

// alertResolve 按 kind+key 移除告警(事件恢复时自动消除);不存在则忽略;清空后自动收起。
func (a *App) alertResolve(kind alertKind, key string) {
	if !a.notifyReady.Load() {
		return
	}
	a.mw.Synchronize(func() {
		kept := a.alertModel.items[:0]
		for _, it := range a.alertModel.items {
			if it.kind != kind || it.key != key {
				kept = append(kept, it)
			}
		}
		if len(kept) == len(a.alertModel.items) {
			return
		}
		a.alertModel.items = kept
		publishTableRefresh(&a.alertModel.TableModelBase, len(kept))
		if len(kept) == 0 && a.alertDlg != nil {
			a.alertDlg.Hide()
		}
	})
}

// alertShow 惰性创建告警窗,定位到工作区右下角并以「置顶、不抢焦点」显示。
// 每次都重算位置(任务栏可能移动)并重申 TOPMOST(被其他置顶窗压下后仍回最前)。仅 UI 线程调用。
func (a *App) alertShow() {
	if a.alertDlg == nil {
		if err := a.alertCreate(); err != nil {
			logger.Errorf("告警窗创建失败: %v", err)
			return
		}
	}
	h := a.alertDlg.Handle()
	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	win.GetMonitorInfo(win.MonitorFromWindow(h, win.MONITOR_DEFAULTTONEAREST), &mi)
	var r win.RECT
	win.GetWindowRect(h, &r)
	w, hgt := r.Right-r.Left, r.Bottom-r.Top
	const margin = 12
	win.SetWindowPos(h, win.HWND_TOPMOST,
		mi.RcWork.Right-w-margin, mi.RcWork.Bottom-hgt-margin, w, hgt,
		win.SWP_NOACTIVATE)
	a.alertDlg.Show() // 非模态、底层 SW_SHOWNA 不激活;nil owner 不会被拉回主窗中心
}

// alertCreate 构建告警窗(仅首条告警时调用一次,此后 Hide/Show 复用)。
func (a *App) alertCreate() error {
	if err := (Dialog{
		AssignTo:  &a.alertDlg,
		Title:     "打印告警",
		FixedSize: true, // WS_CAPTION|WS_SYSMENU:自带关闭按钮 X,无缩放边框
		MinSize:   Size{Width: 400, Height: 260},
		Size:      Size{Width: 400, Height: 260},
		Layout:    VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}, Spacing: 6},
		Children: []Widget{
			TableView{
				AssignTo:            &a.alertTV,
				Model:               a.alertModel,
				StyleCell:           a.alertModel.StyleCell,
				LastColumnStretched: true,
				Columns: []TableViewColumn{
					{Title: "发生时间", Width: 64},
					{Title: "已持续", Width: 64},
					{Title: "内容", Width: 230},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					HSpacer{},
					PushButton{Text: "打开主窗口", OnClicked: a.showMainWindow},
					PushButton{Text: "全部知道了", OnClicked: a.alertDismissAll},
				},
			},
		},
		// nil owner:避开 Dialog.Show 的 centerInOwnerWhenRun 把窗口拉回主窗中心,
		// 也不随主窗最小化被一并隐藏。
	}).Create(nil); err != nil {
		// declarative Create 在构造成功后、初始化控件前就写 AssignTo:
		// 部分失败时 alertDlg 已指向半初始化窗口,必须销毁并置 nil,
		// 否则后续 alertShow 会跳过创建、对着残窗操作。
		if a.alertDlg != nil {
			a.alertDlg.Dispose()
			a.alertDlg = nil
		}
		return err
	}
	if icon := loadAppIcon(); icon != nil {
		_ = a.alertDlg.SetIcon(icon)
	}
	// 双击告警行 → 打开主窗口
	a.alertTV.ItemActivated().Attach(a.showMainWindow)
	// X 等价「全部知道了」:拦截关闭改为清空+隐藏(窗口复用不销毁)
	a.alertDlg.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		*canceled = true
		a.alertDismissAll()
	})
	// 工具窗(不进任务栏/Alt-Tab)+ 弹出时不抢焦点(SW_SHOWNA/SWP_NOACTIVATE;
	// 用户点击窗内控件会正常取得焦点,属交互本义)。
	// 置顶不在此设:WS_EX_TOPMOST 位经 SetWindowLong 不生效(MSDN:topmost 只能经
	// SetWindowPos 修改),由 alertShow 每次 SetWindowPos(HWND_TOPMOST) 保证。
	h := a.alertDlg.Handle()
	ex := win.GetWindowLong(h, win.GWL_EXSTYLE)
	win.SetWindowLong(h, win.GWL_EXSTYLE, ex|win.WS_EX_TOOLWINDOW|win.WS_EX_NOACTIVATE)
	go a.alertTickLoop() // 与窗口同生:每秒刷新「已持续」列
	return nil
}

// alertTickLoop 每秒触发一次告警窗重绘,让「已持续」列动态走秒。
// 只重绘行(PublishRowsChanged→LVM_REDRAWITEMS),不 reset、不丢选中;
// 窗口隐藏或无告警时空转;notifyReady 变 false(程序退出)即退出。
func (a *App) alertTickLoop() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for range tick.C {
		if !a.notifyReady.Load() {
			return
		}
		a.mw.Synchronize(func() {
			if n := len(a.alertModel.items); n > 0 && a.alertDlg != nil && a.alertDlg.Visible() {
				a.alertModel.PublishRowsChanged(0, n-1)
			}
		})
	}
}

// alertDismissAll 清空全部告警并收起(「全部知道了」与 X 共用)。
func (a *App) alertDismissAll() {
	a.alertModel.items = nil
	publishTableRefresh(&a.alertModel.TableModelBase, 0)
	if a.alertDlg != nil {
		a.alertDlg.Hide()
	}
}
