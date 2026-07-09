package ui

import (
	"fmt"
	"time"
)

// offlineAfter 是网口打印机判离线所需的连续失败持续时长(防抖窗口,测试可调小)。
// 用户现场一台路由带数百设备,偶发丢包频繁,「丢一包即离线」误报;
// 连败满窗才判离线、任一次成功立即回在线。USB 走 winspool 本地查询无丢包问题,窗口为 0。
var offlineAfter = 30 * time.Second

// 抖动与统计日志节奏(包级 var,测试可调):
//   pingStatsEvery — 每台统计窗口,窗口内有丢包才落一条 INFO(健康机零日志);
//   jitterLogMin   — 连败持续满该时长后恢复才记「网络抖动」INFO(1-2 包偶发丢失不刷屏);
//   jitterLogGap   — 同台抖动 INFO 最小间隔(反复抖动时靠统计行兜底)。
var (
	pingStatsEvery = 5 * time.Minute
	jitterLogMin   = 5 * time.Second
	jitterLogGap   = time.Minute
)

// onlineDebouncer 把每秒原始探测折算为「生效在线态」:
// 任一次成功立即在线;连续失败持续满 window 才离线;window=0 时失败即时生效(USB)。
type onlineDebouncer struct {
	window    time.Duration
	eff       int       // 生效在线态:-1 未定(启动以来无成功且连败未满窗)/ 0 离线 / 1 在线
	failSince time.Time // 当前连败起点(零值=上次探测成功)
	failFirst string    // 连败首次失败原因(离线 detail/通知用)
}

func newOnlineDebouncer(window time.Duration) *onlineDebouncer {
	return &onlineDebouncer{window: window, eff: -1}
}

// observe 录入一次原始探测结果,返回最新生效在线态。
func (d *onlineDebouncer) observe(now time.Time, ok bool, detail string) int {
	if ok {
		d.failSince, d.failFirst = time.Time{}, ""
		d.eff = 1
		return d.eff
	}
	if d.failSince.IsZero() {
		d.failSince, d.failFirst = now, detail
	}
	if now.Sub(d.failSince) >= d.window {
		d.eff = 0
	}
	return d.eff
}

// failFor 返回当前连败已持续时长(无连败=0)。
func (d *onlineDebouncer) failFor(now time.Time) time.Duration {
	if d.failSince.IsZero() {
		return 0
	}
	return now.Sub(d.failSince)
}

// pingStats 单台网口打印机滚动丢包统计(监测 goroutine 私有,无锁)。
// offlineWindow 为该机的离线防抖窗口:连败持续达到它即已被判离线,
// 恢复时不再算「抖动」(离线/恢复已有状态日志与通知,抖动行只报未判离线的短中断)。
type pingStats struct {
	offlineWindow time.Duration
	winStart      time.Time
	sent, lost    int
	curFails      int
	peakFails     int
	rttSum, rttN  int
	failStart     time.Time
	failDetail    string
	lastJitterLog time.Time
}

// record 录入一次探测;返回的 statsLine/jitterLine 非空即由调用方落 INFO 日志。
// 统计窗口重置时保留跨窗口的连败上下文与抖动节流时钟。
func (ps *pingStats) record(now time.Time, ok bool, rttMs int, detail string) (statsLine, jitterLine string) {
	if ps.winStart.IsZero() {
		ps.winStart = now
	}
	ps.sent++
	if !ok {
		ps.lost++
		if ps.curFails == 0 {
			ps.failStart, ps.failDetail = now, detail
		}
		ps.curFails++
		if ps.curFails > ps.peakFails {
			ps.peakFails = ps.curFails
		}
	} else {
		dur := now.Sub(ps.failStart)
		if ps.curFails > 0 && dur >= jitterLogMin &&
			(ps.offlineWindow <= 0 || dur < ps.offlineWindow) &&
			now.Sub(ps.lastJitterLog) >= jitterLogGap {
			ps.lastJitterLog = now
			jitterLine = fmt.Sprintf("连续失败 %v 后恢复(未判离线;首因: %s)",
				dur.Truncate(time.Second), ps.failDetail)
		}
		ps.curFails = 0
		ps.rttSum += rttMs
		ps.rttN++
	}
	if now.Sub(ps.winStart) >= pingStatsEvery {
		if ps.lost > 0 {
			avg := 0
			if ps.rttN > 0 {
				avg = ps.rttSum / ps.rttN
			}
			statsLine = fmt.Sprintf("发送 %d 丢失 %d(%.1f%%) 连败峰值 %d RTT均值 %dms",
				ps.sent, ps.lost, float64(ps.lost)*100/float64(ps.sent), ps.peakFails, avg)
		}
		*ps = pingStats{offlineWindow: ps.offlineWindow, winStart: now, curFails: ps.curFails,
			failStart: ps.failStart, failDetail: ps.failDetail, lastJitterLog: ps.lastJitterLog}
	}
	return
}
