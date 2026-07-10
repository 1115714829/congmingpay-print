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
// 网口做离线防抖(offlineAfter=30s):连续失败持续满窗才判离线,期间任一次 ping 通立即回在线;
// USB 本地查询无丢包问题,不防抖(窗口 0,结果即时生效)。
// 三个独立边沿:①原始成功边沿 → NudgeOnline(「联通即打」不受防抖拖慢,容错窗口内恢复也秒打);
// ②生效标签边沿 → 表格刷新 + 状态日志(容错/离线持续期间标签不变,零刷屏);
// ③生效在线布尔边沿 → 系统通知 + 常驻告警窗(首个生效态记基线静默:启动/监测重建不通知)。
// 每轮把生效态写入共享在线注册表(定时 state 上报据此合成快照),本函数不直接上报云端。
func (a *App) monitorLoop(snap model.Printer, stop <-chan struct{}) {
	window := offlineAfter
	isNet := snap.Conn != model.ConnUSB
	if !isNet {
		window = 0
	}
	deb := newOnlineDebouncer(window)
	stats := pingStats{offlineWindow: window}
	lastLabel := ""         // 生效标签边沿(UI 表格 + 状态日志)
	lastEff := -1           // 生效在线布尔边沿(通知/告警窗;-1=基线未定)
	lastRaw := -1           // 原始探测边沿(NudgeOnline)
	var lastGood statusInfo // 最近一次原始在线的显示信息(容错窗口沿用其标签防闪烁)

	for {
		t0 := time.Now()
		st := printsvc.Status(&snap)
		// Status 的 1s ping 超时可能横跨 close(stop):被停(删除/重建)后不再产出任何
		// 边沿动作——否则删除后迟到的 raise 会留下无人能自动消除的孤儿告警。
		select {
		case <-stop:
			return
		default:
		}
		raw := st.Reachable && st.Online
		eff := deb.observe(t0, raw, st.Detail)

		// ① 原始成功边沿 → 立即催打等待任务(容错窗口内恢复不产生 eff 边沿,等待任务仍应秒打)。
		if raw && lastRaw != 1 {
			a.svc.NudgeOnline(snap.ID)
		}
		lastRaw = b2i(raw)

		// ② 折算生效显示信息。
		var info statusInfo
		switch {
		case eff == 1 && raw: // 正常在线
			info = statusInfoFor(st)
			lastGood = info
		case eff == 1: // 容错窗口:保持在线标签防闪烁,detail 记抖动进展
			info = lastGood
			info.detail = fmt.Sprintf("网络抖动: 连续失败 %v(%s)",
				deb.failFor(t0).Truncate(time.Second), st.Detail)
		case eff == 0:
			info = statusInfo{label: "离线", color: colGray, detail: fmt.Sprintf("%s;已持续 %v",
				deb.failFirst, deb.failFor(t0).Truncate(time.Second))}
		default: // eff == -1 启动即连败,防抖窗内未定:表格维持「—」
			info = statusInfo{label: "—", color: colGray, detail: "检测中(" + st.Detail + ")"}
		}

		// ③ 生效标签边沿 → 日志 + 回 UI 线程刷表格。
		// 日志在监测 goroutine 里写(文件 I/O 不占 UI 线程),UI 线程只做状态赋值 + 列表重绘。
		if info.label != lastLabel {
			lastLabel = info.label
			ic := info
			logger.Infof("打印机状态: [id=%s] 名称『%s』%s → %s(%s)", snap.ID, snap.Name, snap.Address(), ic.label, ic.detail)
			a.mw.Synchronize(func() {
				a.printerModel.status[snap.ID] = ic
				a.refreshPrinters()
			})
		}

		// ④ 生效在线布尔边沿 → 系统通知 + 告警窗(基线静默:启动/监测重建〔改属性/改名〕不通知)。
		switch {
		case eff < 0: // 未定:不通知不告警
		case lastEff < 0:
			lastEff = eff
			if eff == 1 {
				// 基线静默只免通知,不免收尾:监测重建(改对 IP/改名)前挂起的
				// 离线告警,重建后首个在线基线即消除(resolve 幂等,不存在则忽略)。
				a.alertResolve(alertPrinterOffline, snap.ID)
			} else {
				// 启动/监测重建即离线:不弹系统通知(防启动风暴),但进常驻告警窗;
				// 「已持续」按程序工作时间算(程序启动后首次探测失败起)。
				a.alertRaise(alertPrinterOffline, snap.ID, alertError,
					fmt.Sprintf("打印机离线 『%s』%s(%s)", snap.Name, snap.Address(), deb.failFirst),
					t0.Add(-deb.failFor(t0)))
			}
		case eff != lastEff:
			lastEff = eff
			if eff == 1 {
				a.notify(notifyInfo, "打印机已恢复在线",
					fmt.Sprintf("『%s』%s 已恢复在线", snap.Name, snap.Address()))
				a.alertResolve(alertPrinterOffline, snap.ID)
			} else {
				a.notify(notifyWarn, "打印机离线",
					fmt.Sprintf("『%s』%s 检测离线(%s)", snap.Name, snap.Address(), info.detail))
				// 告警行文本只带首因,时长交给「已持续」列动态显示;起点=断连开始时刻(failSince)
				a.alertRaise(alertPrinterOffline, snap.ID, alertError,
					fmt.Sprintf("打印机离线 『%s』%s(%s)", snap.Name, snap.Address(), deb.failFirst),
					t0.Add(-deb.failFor(t0)))
			}
		}

		// ⑤ 在线注册表写生效态(detail 每轮新鲜:抖动/离线持续时长可被 state 上报带出)。
		// 未定态(-1)不写:监测重建首轮恰逢丢包时,沿用旧监测写入的生效态,
		// 避免向定时 state 上报暴露本功能要压制的单包假离线;冷启动缺项本就默认离线,不劣化。
		if eff >= 0 {
			a.svc.SetPrinterOnline(snap.ID, eff == 1, info.label+"("+info.detail+")")
		}

		// ⑥ 丢包统计与抖动日志(仅网口;窗口内无丢包不落日志)。
		if isNet {
			sl, jl := stats.record(t0, raw, st.RTTms, st.Detail)
			if jl != "" {
				logger.Infof("网络抖动: [id=%s] 名称『%s』%s %s", snap.ID, snap.Name, snap.Address(), jl)
			}
			if sl != "" {
				logger.Infof("ping 统计: [id=%s] 名称『%s』%s 最近 %v %s",
					snap.ID, snap.Name, snap.Address(), pingStatsEvery, sl)
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
