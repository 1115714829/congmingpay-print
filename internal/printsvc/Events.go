package printsvc

// JobFinalEvent 是打印任务到达终态(成功/失败)时的事件快照,供上层(main 装配 MQTT)上报打印结果。
// 仅终态触发:JobDone → OK=true、JobFailed → OK=false;「等待重试」不触发(待其最终打成/失败再报)。
type JobFinalEvent struct {
	JobNo   int
	CloudID *uint32 // 云端消息 id;nil=本地任务(测试打印/打印样票),由装配层决定是否过滤
	Printer string  // 打印机名称
	Target  string  // 打印目标(IP:port 或 USB『…』),供日志/排查
	OK      bool    // true=打印成功(JobDone) false=打印失败(JobFailed)
	Err     string  // 失败原因,成功为空
}

// SetOnJobFinal 设置任务终态回调。回调可能在派发 goroutine 中被调用,
// 契约:必须快速返回、不得阻塞(下游 MQTT 发布为非阻塞,满足)。
func (s *Service) SetOnJobFinal(f func(JobFinalEvent)) {
	s.mu.Lock()
	s.onJobFinal = f
	s.mu.Unlock()
}

// fireJobFinal 触发终态回调(锁外调用,锁纪律同 fireNotify)。
func (s *Service) fireJobFinal(ev JobFinalEvent) {
	s.mu.Lock()
	f := s.onJobFinal
	s.mu.Unlock()
	if f != nil {
		f(ev)
	}
}
