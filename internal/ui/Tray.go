package ui

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

// setupTray 创建系统托盘图标与右键菜单(显示主窗口 / 退出)。
func (a *App) setupTray(icon *walk.Icon) error {
	ni, err := walk.NewNotifyIcon(a.mw)
	if err != nil {
		return err
	}
	a.tray = ni

	if icon != nil {
		if err := ni.SetIcon(icon); err != nil {
			return err
		}
	}
	if err := ni.SetToolTip(a.serviceName()); err != nil {
		return err
	}

	if err := a.addTrayAction(ni, "显示主窗口", a.showMainWindow); err != nil {
		return err
	}
	if err := a.addTrayAction(ni, "退出", func() {
		a.quitting = true // 放行主窗口 Closing 拦截(Exit 本不经 Closing,置位为保险)
		walk.App().Exit(0)
	}); err != nil {
		return err
	}

	ni.MouseUp().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			a.showMainWindow()
		}
	})

	// 点击系统通知(气泡/Toast)→ 显示主窗口
	ni.MessageClicked().Attach(a.showMainWindow)

	return ni.SetVisible(true)
}

func (a *App) addTrayAction(ni *walk.NotifyIcon, text string, handler func()) error {
	action := walk.NewAction()
	if err := action.SetText(text); err != nil {
		return err
	}
	action.Triggered().Attach(handler)
	return ni.ContextMenu().Actions().Add(action)
}

func (a *App) showMainWindow() {
	// walk 的 Show 底层是 SW_SHOWNA:对最小化窗口保持最小化,SetFocus 也不还原——
	// 最小化状态须先 SW_RESTORE,否则托盘/通知/告警窗的「打开主窗口」点了没反应。
	if win.IsIconic(a.mw.Handle()) {
		win.ShowWindow(a.mw.Handle(), win.SW_RESTORE)
	}
	a.mw.Show()
	a.mw.SetFocus()
}
