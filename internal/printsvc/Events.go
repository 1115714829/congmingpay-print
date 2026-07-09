package printsvc

import "congmingpay/internal/model"

// 任务事件类型(供上层组装 report 上行消息的 event 字段)。
const (
	EventDone    = "done"    // 打印成功(终态,恰好一次)
	EventFailed  = "failed"  // 打印失败(终态,恰好一次)
	EventWaiting = "waiting" // 长期等待告警(非终态,每个被拒流至多一次)
)

// JobEvent 是打印任务事件快照,供上层(main 装配 MQTT)上报。
// done/failed 由 setStatus 终态收口触发;waiting 由长期被拒告警触发。
type JobEvent struct {
	Event   string
	Code    int           // 全局错误码:done=0;failed/waiting 见 internal/errcode
	JobNo   int
	CloudID *uint32       // 云端消息 id;nil=本地任务(测试打印/打印样票),由装配层决定是否过滤
	Printer model.Printer // 目标打印机快照(值拷贝,供回执携带身份)
	Err     string        // 失败/等待原因,成功为空
}

// SetOnJobEvent 设置任务事件回调。回调可能在派发 goroutine 中被调用,
// 契约:必须快速返回、不得阻塞(下游 MQTT 发布为非阻塞,满足)。
func (s *Service) SetOnJobEvent(f func(JobEvent)) {
	s.mu.Lock()
	s.onJobEvent = f
	s.mu.Unlock()
}

// fireJobEvent 触发任务事件回调(锁外调用,锁纪律同 fireNotify)。
func (s *Service) fireJobEvent(ev JobEvent) {
	s.mu.Lock()
	f := s.onJobEvent
	s.mu.Unlock()
	if f != nil {
		f(ev)
	}
}

// OnlineInfo 是一台打印机的最近在线检测结果(共享注册表,供 state 上行消息合成)。
type OnlineInfo struct {
	Online bool
	Detail string
}

// SetPrinterOnline 写入一台打印机的在线检测结果(UI 监测 goroutine 每次巡检调用)。
func (s *Service) SetPrinterOnline(id string, online bool, detail string) {
	s.onlineMu.Lock()
	s.online[id] = OnlineInfo{Online: online, Detail: detail}
	s.onlineMu.Unlock()
}

// OnlineSnapshot 返回全部打印机在线检测结果的拷贝(供 state 上行消息合成)。
func (s *Service) OnlineSnapshot() map[string]OnlineInfo {
	s.onlineMu.RLock()
	defer s.onlineMu.RUnlock()
	out := make(map[string]OnlineInfo, len(s.online))
	for k, v := range s.online {
		out[k] = v
	}
	return out
}
