// Toast.go 屏幕居中的悬浮提示窗:无标题栏、置顶、不抢焦点、不进任务栏。
// 两种形态:自动消失的提示(成功/失败/警告/信息)与带「确定/取消」的确认窗。
// 提示与警告统一走本窗,不再弹系统 MsgBox;状态栏 flash 仅保留纯状态文字。
package ui

import (
	"time"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

// toastKind 决定提示窗配色。
type toastKind int

const (
	toastInfo toastKind = iota
	toastSuccess
	toastWarning
	toastError
)

// toast 显示一条居中悬浮提示,自动消失(3.5s/5s)。任意 goroutine 可调。
func (a *App) toast(kind toastKind, text string) {
	if !a.notifyReady.Load() || a.mw == nil {
		return
	}
	a.mw.Synchronize(func() { a.toastShow(kind, text, false, nil) })
}

// toastConfirm 居中悬浮确认窗(「确定/取消」),不自动消失,须用户点击;
// 回调在 UI 线程执行。任意 goroutine 可调。
func (a *App) toastConfirm(text string, cb func(yes bool)) {
	if !a.notifyReady.Load() || a.mw == nil {
		return
	}
	a.mw.Synchronize(func() { a.toastShow(toastWarning, text, true, cb) })
}

// toastShow 渲染并显示提示窗(仅 UI 线程调用)。confirm=true 时显示按钮行且不自动消失。
func (a *App) toastShow(kind toastKind, text string, confirm bool, cb func(yes bool)) {
	if a.toastDlg == nil {
		if err := a.toastCreate(); err != nil {
			a.flash(text) // 窗口创建失败退回状态栏,不丢信息
			return
		}
	}
	a.toastConfirmMode = confirm
	a.toastCb = cb

	bg, fg := toastColor(kind)
	if br, err := walk.NewSolidColorBrush(bg); err == nil {
		a.toastDlg.SetBackground(br)
	}
	a.toastLabel.SetTextColor(fg)
	_ = a.toastLabel.SetText(text)

	w, h := toastSize(text, confirm)
	a.toastButtons.SetVisible(confirm)

	// 定位到主显示器工作区中心(略偏上),重申置顶再显示(不激活、不抢焦点)
	hwnd := a.toastDlg.Handle()
	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	win.GetMonitorInfo(win.MonitorFromWindow(hwnd, win.MONITOR_DEFAULTTONEAREST), &mi)
	l, t := int(mi.RcWork.Left), int(mi.RcWork.Top)
	r, b := int(mi.RcWork.Right), int(mi.RcWork.Bottom)
	x := l + (r-l-w)/2
	y := t + (b-t-h)/2 - 40
	win.SetWindowPos(hwnd, win.HWND_TOPMOST, int32(x), int32(y), int32(w), int32(h), win.SWP_NOACTIVATE|win.SWP_FRAMECHANGED)
	a.toastDlg.Show()

	if a.toastTimer != nil {
		a.toastTimer.Stop()
	}
	if !confirm {
		d := 3500 * time.Millisecond
		if kind == toastError || kind == toastWarning {
			d = 5000 * time.Millisecond
		}
		// walk v0.0.0-2021 无 Timer 类型,用标准库 AfterFunc 定时,回 UI 线程收起。
		a.toastTimer = time.AfterFunc(d, func() {
			a.mw.Synchronize(func() {
				if !a.toastConfirmMode && a.toastDlg != nil && a.toastDlg.Visible() {
					a.toastHide()
				}
			})
		})
	}
}

// toastHide 收起提示窗(窗口复用,不销毁)。
func (a *App) toastHide() {
	if a.toastTimer != nil {
		a.toastTimer.Stop()
	}
	if a.toastDlg != nil {
		a.toastDlg.Hide()
	}
	a.toastCb = nil
}

// toastConfirmAnswer 确认窗按钮回调(「确定」=true)。
func (a *App) toastConfirmAnswer(yes bool) {
	cb := a.toastCb
	a.toastHide()
	if cb != nil {
		cb(yes)
	}
}

// toastColor 按级别返回 背景/文字 色。
func toastColor(kind toastKind) (walk.Color, walk.Color) {
	white := walk.RGB(0xff, 0xff, 0xff)
	switch kind {
	case toastSuccess:
		return walk.RGB(0x18, 0x80, 0x38), white // 绿
	case toastWarning:
		return walk.RGB(0xd9, 0x7c, 0x0b), white // 橙
	case toastError:
		return walk.RGB(0xd1, 0x34, 0x38), white // 红
	default:
		return walk.RGB(0x2d, 0x7d, 0xd2), white // 蓝
	}
}

// toastEstWidth 估算文本像素宽(9pt:中文≈13px,ASCII≈7px)。
func toastEstWidth(text string) int {
	w := 0
	for _, r := range text {
		if r >= 0x2E80 {
			w += 13
		} else {
			w += 7
		}
	}
	return w
}

// toastSize 按文本与形态算窗口尺寸:单行 44 高,超宽折两行 68 高,确认态加按钮行。
func toastSize(text string, confirm bool) (int, int) {
	const pad = 28
	const maxw = 460
	pw := toastEstWidth(text)
	w := pw + pad
	if w > maxw {
		w = maxw
	}
	if w < 220 {
		w = 220
	}
	h := 44
	if pw > maxw-pad {
		h = 68
	}
	if confirm {
		h += 40
	}
	return w, h
}

// toastCreate 构建提示窗(首条提示时创建一次,此后 Hide/Show 复用)。
// 无标题栏(WS_POPUP)+工具窗(不进任务栏)+不抢焦点(SW_SHOWNA/SWP_NOACTIVATE)。
func (a *App) toastCreate() error {
	if err := (Dialog{
		AssignTo: &a.toastDlg,
		Title:    "提示",
		Layout:   VBox{Margins: Margins{Left: 12, Top: 8, Right: 12, Bottom: 8}, Spacing: 6},
		Children: []Widget{
			Label{AssignTo: &a.toastLabel, TextAlignment: AlignCenter},
			Composite{
				AssignTo: &a.toastButtons,
				Visible:  false,
				Layout:   HBox{Spacing: 12, MarginsZero: true},
				Children: []Widget{
					HSpacer{},
					PushButton{AssignTo: &a.toastBtnYes, Text: "确定", OnClicked: func() { a.toastConfirmAnswer(true) }},
					PushButton{AssignTo: &a.toastBtnNo, Text: "取消", OnClicked: func() { a.toastConfirmAnswer(false) }},
				},
			},
		},
		// nil owner:不随主窗最小化隐藏,也不被 centerInOwnerWhenRun 拉回主窗中心。
	}).Create(nil); err != nil {
		// declarative Create 部分失败时 toastDlg 可能已指向半初始化窗口,须销毁置 nil。
		if a.toastDlg != nil {
			a.toastDlg.Dispose()
			a.toastDlg = nil
		}
		return err
	}
	h := a.toastDlg.Handle()
	// 无标题栏:去掉 caption/边框,改为弹出窗(WS_POPUP=0x80000000,须以负数 int32 传)
	win.SetWindowLong(h, win.GWL_STYLE, -0x80000000)
	ex := win.GetWindowLong(h, win.GWL_EXSTYLE)
	win.SetWindowLong(h, win.GWL_EXSTYLE, ex|win.WS_EX_TOOLWINDOW|win.WS_EX_NOACTIVATE)
	// 点击提示窗本身 = 关闭(仅自动消失态;确认态须点按钮)
	a.toastDlg.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if !a.toastConfirmMode {
			a.toastHide()
		}
	})
	return nil
}
