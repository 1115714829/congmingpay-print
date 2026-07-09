package ui

import (
	"fmt"
	"time"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// b2i 把在线布尔折算为 lastOnline 三态里的 0/1。
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// pingInterval 是每台打印机后台持续检测的间隔(等价 ping -t 的 1 次/秒)。
const pingInterval = 1 * time.Second

// monitorHandle 管理一台打印机的后台监测 goroutine。
type monitorHandle struct {
	stop chan struct{}
	sig  string // 连接身份签名;变了(如改 IP)则停旧起新
}

// monSig 返回打印机的监测签名(连接身份 + 名称)。
// 含 Name:改名后签名变化 → syncMonitors 停旧起新,让监测 goroutine 的 snap 拿到新名称,
// 否则在线注册表写入(SetPrinterOnline 用 snap 身份)与日志会携带旧名,与 state 快照的新名不一致。
func monSig(p *model.Printer) string {
	return string(p.Conn) + "|" + p.IP + "|" + p.Port + "|" + p.USBName + "|" + p.Name
}

// syncMonitors 让后台监测与当前打印机列表对齐:缺的起、变的重启、删的停。
// 只在 UI 线程调用(启动/增删/属性/云端同步),另加锁保 stopAllMonitors 并发。
func (a *App) syncMonitors() {
	a.monitorsMu.Lock()
	defer a.monitorsMu.Unlock()

	live := map[string]bool{}
	for _, p := range a.cfg.PrinterList() {
		live[p.ID] = true
		sig := monSig(p)
		if h, ok := a.monitors[p.ID]; ok {
			if h.sig == sig {
				continue // 已在监测且参数没变
			}
			close(h.stop) // 参数变了(如改了 IP)→ 停旧起新
			delete(a.monitors, p.ID)
		}
		stop := make(chan struct{})
		a.monitors[p.ID] = &monitorHandle{stop: stop, sig: sig}
		go a.monitorLoop(*p, stop) // 传值拷贝,避免循环内读共享指针与属性编辑竞态
	}
	// 停掉已删除打印机的监测
	for id, h := range a.monitors {
		if !live[id] {
			close(h.stop)
			delete(a.monitors, id)
		}
	}
}

// stopAllMonitors 停止所有监测(退出时)。
func (a *App) stopAllMonitors() {
	a.monitorsMu.Lock()
	defer a.monitorsMu.Unlock()
	for id, h := range a.monitors {
		close(h.stop)
		delete(a.monitors, id)
	}
}

// monitorLoop 每台一条:后台不间断检测(网口 ICMP ping、USB winspool),约 1 次/秒。
// 不做防抖——超时即离线、通即在线;仅状态标签变化时回 UI 线程刷新并记日志。
// 每次巡检把结果写入共享在线注册表(定时 state 上报据此合成快照),本函数不直接上报云端。
func (a *App) monitorLoop(snap model.Printer, stop <-chan struct{}) {
	last := ""       // 上次显示的状态标签
	lastOnline := -1 // 上次在线布尔(-1=未定):独立于 label 边沿(就绪↔缺纸等在线内变化不算离线)
	for {
		t0 := time.Now()
		st := printsvc.Status(&snap)
		info := statusInfoFor(st)
		online := st.Reachable && st.Online
		if info.label != last {
			last = info.label
			ic := info
			// 日志在监测 goroutine 里写(文件 I/O 不占 UI 线程,避免拖慢随后的队列刷新);
			// UI 线程只做状态赋值 + 列表重绘。
			logger.Infof("打印机状态: [id=%s] 名称『%s』%s → %s(%s)", snap.ID, snap.Name, snap.Address(), ic.label, ic.detail)
			a.mw.Synchronize(func() {
				a.printerModel.status[snap.ID] = ic
				a.refreshPrinters()
			})
			// 该机变为在线(标签非离线/检测中)→ 催它的「等待重试」任务立即打,不等退避。
			if ic.label != "离线" && ic.label != "—" {
				a.svc.NudgeOnline(snap.ID)
			}
		}
		// 在线布尔边沿 → 系统通知:首查只记基线不通知(启动/监测重建〔改属性/改名停旧起新〕静默,
		// 防启动风暴与假恢复),此后由在线转离线、由离线恢复在线各通知一次。
		switch {
		case lastOnline < 0:
			lastOnline = b2i(online)
		case online != (lastOnline == 1):
			lastOnline = b2i(online)
			if online {
				a.notify(notifyInfo, "打印机已恢复在线",
					fmt.Sprintf("『%s』%s 已恢复在线", snap.Name, snap.Address()))
			} else {
				a.notify(notifyWarn, "打印机离线",
					fmt.Sprintf("『%s』%s 检测离线(%s)", snap.Name, snap.Address(), info.detail))
			}
		}
		// 每次巡检把结果写入共享在线注册表(定时 state 上报据此合成,detail 保持新鲜)。
		a.svc.SetPrinterOnline(snap.ID, online, info.label+"("+info.detail+")")
		wait := pingInterval - time.Since(t0)
		if wait < 0 {
			wait = 0
		}
		select {
		case <-stop:
			return
		case <-time.After(wait):
		}
	}
}
