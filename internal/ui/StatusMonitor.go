package ui

import (
	"time"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// pingInterval 是每台打印机后台持续检测的间隔(等价 ping -t 的 1 次/秒)。
const pingInterval = 1 * time.Second

// monitorHandle 管理一台打印机的后台监测 goroutine。
type monitorHandle struct {
	stop chan struct{}
	sig  string // 连接身份签名;变了(如改 IP)则停旧起新
}

// monSig 返回打印机的连接身份签名(监测相关字段)。
func monSig(p *model.Printer) string {
	return string(p.Conn) + "|" + p.IP + "|" + p.Port + "|" + p.USBName
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
func (a *App) monitorLoop(snap model.Printer, stop <-chan struct{}) {
	last := "" // 上次显示的状态标签
	for {
		t0 := time.Now()
		info := statusInfoFor(printsvc.Status(&snap))
		if info.label != last {
			last = info.label
			ic := info
			a.mw.Synchronize(func() {
				a.printerModel.status[snap.ID] = ic
				a.refreshPrinters()
				logger.Infof("打印机状态: [id=%s] 名称『%s』%s → %s(%s)", snap.ID, snap.Name, snap.Address(), ic.label, ic.detail)
			})
			// 该机变为在线(标签非离线/检测中)→ 催它的「等待重试」任务立即打,不等退避。
			if ic.label != "离线" && ic.label != "—" {
				a.svc.NudgeOnline(snap.ID)
			}
		}
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
