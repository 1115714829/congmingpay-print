package ui

import (
	"fmt"
	"strconv"
	"time"
	"unicode/utf16"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// notifyLevel 是系统通知级别,映射托盘气泡图标(信息/警告/错误)。
type notifyLevel int

const (
	notifyInfo notifyLevel = iota
	notifyWarn
	notifyError
)

// NOTIFYICONDATA 固定缓冲区为 szInfoTitle=64、szInfo=256 个 UTF-16 码元(含 NUL 终止位);
// walk 对缓冲区做裸 copy 不保证终止符,必须自行截断。
const (
	notifyTitleMax = 63
	notifyBodyMax  = 255
)

// notify 发送一条 Windows 系统通知(Win7=托盘气泡,Win10 及以上由系统渲染为通知中心 Toast)。
// 任意 goroutine 可调用:非阻塞(Synchronize 仅投递消息),满足 printsvc 回调「快速返回」契约;
// UI 未就绪(托盘尚未创建/已退出)或设置关闭系统通知时静默丢弃。
func (a *App) notify(level notifyLevel, title, msg string) {
	if !a.notifyReady.Load() {
		return
	}
	a.mw.Synchronize(func() { // walk 调用与 a.cfg 读一律回 UI 线程
		if a.tray == nil || a.cfg.Settings.NotifyDisabled {
			return
		}
		if msg == "" {
			msg = title // szInfo 空串=撤下气泡而非显示,兜底用标题
		}
		t := truncateUTF16(title, notifyTitleMax)
		m := truncateUTF16(msg, notifyBodyMax)
		var err error
		switch level {
		case notifyError:
			err = a.tray.ShowError(t, m)
		case notifyWarn:
			err = a.tray.ShowWarning(t, m)
		default:
			err = a.tray.ShowInfo(t, m)
		}
		if err != nil {
			logger.Errorf("系统通知发送失败: %v(%s | %s)", err, t, m)
		} else {
			logger.Infof("系统通知: %s | %s", t, m)
		}
	})
}

// truncateUTF16 把 s 截断到至多 n 个 UTF-16 码元;超长时以「…」结尾且不劈开代理对。
func truncateUTF16(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if len(u) <= n {
		return s
	}
	u = u[:n-1] // 留一位给「…」
	if k := len(u); k > 0 && u[k-1] >= 0xD800 && u[k-1] <= 0xDBFF {
		u = u[:k-1] // 尾部是高代理则一并去掉
	}
	return string(utf16.Decode(u)) + "…"
}

// NotifyJobEvent 是打印任务事件的系统通知与告警窗入口,由 main 装配进 printsvc.SetOnJobEvent
// (与 MQTT report 上报并联)。failed(终态)与 waiting(长期卡单,每单至多一次)通知并进告警窗;
// done 不发通知,但消掉此前同任务的等待告警;本地任务(CloudID==nil,测试打印/打印样票)同样通知。
func (a *App) NotifyJobEvent(ev printsvc.JobEvent) {
	key := strconv.Itoa(ev.JobNo)
	switch ev.Event {
	case printsvc.EventFailed:
		txt := fmt.Sprintf("任务#%d 打印机『%s』:%s", ev.JobNo, ev.Printer.Name, ev.Err)
		a.notify(notifyError, "打印失败", txt)
		a.alertResolve(alertJobWaiting, key) // 终态收敛:等待告警随失败转失败行
		a.alertRaise(alertJobFailed, key, alertError, "打印失败 "+txt, time.Now())
	case printsvc.EventWaiting:
		txt := fmt.Sprintf("任务#%d 打印机『%s』:%s", ev.JobNo, ev.Printer.Name, ev.Err)
		a.notify(notifyWarn, "打印任务长时间等待", txt)
		a.alertRaise(alertJobWaiting, key, alertWarn, "长时间等待 "+txt, time.Now())
	case printsvc.EventDone:
		a.alertResolve(alertJobWaiting, key)
		when := time.Now().Format(model.LastPrintTimeLayout)
		id := ev.Printer.ID
		if a.mw != nil {
			a.mw.Synchronize(func() {
				if a.cfg.UpdateLastPrint(id, when) {
					a.save()
					a.refreshPrinters()
				}
			})
		} else if a.cfg.UpdateLastPrint(id, when) {
			a.save()
		}
	}
}

// mqttNotifyEdge 按连接布尔做边沿去重:已连→断开、断开→恢复各通知一次。
// 仅在 UI 线程(onStatus 的 Synchronize 闭包)调用,mqttNotifySt 无需加锁。
// 未启用/已停用(Active()==false)重置为中性——Close/reconnect 置 cli=nil 时不触发
// onStatus 回调,中性判断必须在每次回调里做,不能依赖「停用事件」。
// 首个状态只记基线不通知(启动即弹「已连接」/启动即连不上均静默);
// 同一状态的重复触发(ConnectionLost 连发 setConnected+setErr 两次回调)静默。
func (a *App) mqttNotifyEdge() {
	if a.mc == nil || !a.mc.Active() {
		a.mqttNotifySt = 0
		return
	}
	ok, detail := a.mc.Status()
	switch {
	case ok && a.mqttNotifySt == 2:
		a.mqttNotifySt = 1
		a.notify(notifyInfo, "云端连接已恢复", detail)
		a.alertResolve(alertMqttDown, "")
	case ok:
		a.mqttNotifySt = 1
	case !ok && a.mqttNotifySt == 1:
		a.mqttNotifySt = 2
		a.notify(notifyWarn, "云端连接断开", detail+";正在自动重连")
		a.alertRaise(alertMqttDown, "", alertError, "云端连接断开:"+detail, time.Now())
	}
}
