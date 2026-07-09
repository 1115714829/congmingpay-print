package ui

import "github.com/lxn/walk"

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
	if err := a.addTrayAction(ni, "退出", func() { walk.App().Exit(0) }); err != nil {
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
	a.mw.Show()
	a.mw.SetFocus()
}
