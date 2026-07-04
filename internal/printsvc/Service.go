// Package printsvc 提供并发打印调度与打印机状态查询。
//
// 每个打印任务在独立 goroutine 中执行,多台打印机可同时打印。
// 任务状态变化通过 notify 回调通知 UI(UI 负责 marshal 回界面线程刷新)。
package printsvc

import (
	"sync"
	"time"

	"congmingpay/internal/escpos"
	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/transport"
)

// Options 是打印任务的可选覆盖参数(nil 字段=用打印机默认)。
type Options struct {
	Cut       *bool
	Buzzer    *bool
	Retry     *bool
	HeadLines *int
	TailLines *int
	RetryMax  *int
}

// entry 是一个任务的内部记录,附带打印机快照、数据与生效参数,便于重试。
type entry struct {
	job     *model.Job
	printer model.Printer
	data    []byte

	// 生效参数(提交时按覆盖/默认解析)
	cut       bool
	buzzer    bool
	retry     bool
	headLines int
	tailLines int
	retryMax  int

	attempts    int           // 已派发次数(含首次)
	reprintNext bool          // 下次派发是否显示"重打"抬头(仅手动「重新打印」时置;自动重试不置)
	cancelled   bool          // 已取消:休眠中的等待重试醒来即停
	backoff     time.Duration // 网络异常等待重试的当前退避间隔
	wake        chan struct{} // 监测到打印机上线时唤醒:立即重试、不等退避(缓冲 1)
}

// Service 是并发打印调度器。
type Service struct {
	mu      sync.Mutex
	entries []*entry
	nextNo  int
	notify  func()
}

// New 创建打印服务。
func New() *Service {
	return &Service{nextNo: 1000}
}

// SetNotify 设置任务变化回调(UI 用它刷新界面)。
func (s *Service) SetNotify(f func()) {
	s.mu.Lock()
	s.notify = f
	s.mu.Unlock()
}

// Jobs 返回当前任务列表快照(新任务在前)。
func (s *Service) Jobs() []*model.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*model.Job, len(s.entries))
	for i, e := range s.entries {
		j := *e.job
		out[i] = &j
	}
	return out
}

// ActiveCount 返回进行中的任务数(排队 + 打印中)。
func (s *Service) ActiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.entries {
		if e.job.Status.Active() {
			n++
		}
	}
	return n
}

// Submit 新建打印任务并在后台并发执行,立即返回任务号。opts 为可选覆盖参数(nil=用打印机默认)。
func (s *Service) Submit(p *model.Printer, doc string, data []byte, opts *Options) int {
	e := &entry{printer: *p, data: data, wake: make(chan struct{}, 1)}
	// 生效参数 = 覆盖?覆盖:打印机默认
	e.cut = p.Cuts()
	e.buzzer = p.BuzzerEnabled
	e.retry = p.Retries()
	e.headLines = p.HeadLines
	e.tailLines = p.TailLines
	e.retryMax = p.MaxRetries()
	if opts != nil {
		if opts.Cut != nil {
			e.cut = *opts.Cut
		}
		if opts.Buzzer != nil {
			e.buzzer = *opts.Buzzer
		}
		if opts.Retry != nil {
			e.retry = *opts.Retry
		}
		if opts.HeadLines != nil {
			e.headLines = *opts.HeadLines
		}
		if opts.TailLines != nil {
			e.tailLines = *opts.TailLines
		}
		if opts.RetryMax != nil && *opts.RetryMax > 0 {
			e.retryMax = *opts.RetryMax
		}
	}

	s.mu.Lock()
	s.nextNo++
	e.job = &model.Job{
		No: s.nextNo, Doc: doc, PrinterID: p.ID, Printer: p.Name,
		Copies: 1, Status: model.JobQueued, Time: time.Now().Format("15:04:05"),
	}
	jobNo := e.job.No
	s.entries = append([]*entry{e}, s.entries...)
	s.mu.Unlock()
	s.fireNotify()

	logger.Infof("任务 #%d 提交 → %s %s (蜂鸣:%s 切刀:%s 重打:%s×%d)",
		jobNo, p.BrandLabel(), p.Target(), onOff(e.buzzer), onOff(e.cut), onOff(e.retry), e.retryMax)
	go s.dispatch(e)
	return jobNo
}

func onOff(v bool) string {
	if v {
		return "开"
	}
	return "关"
}

func (s *Service) dispatch(e *entry) {
	s.mu.Lock()
	e.attempts++
	reprint := e.reprintNext
	s.mu.Unlock()

	s.setStatus(e, model.JobPrinting, "")

	// 组装:蜂鸣 → 重印抬头 → 首部空行 → 内容 → 收尾(尾部走纸 + 按设置切纸)。
	// 收尾统一在此,保证切纸前走纸足够清过切刀;参数用提交时解析的生效值(每台默认或消息覆盖)。
	var payload []byte
	if e.buzzer {
		payload = append(payload, escpos.BuildBuzzer(3, 3)...)
	}
	if reprint {
		payload = append(payload, escpos.ReprintBanner(e.printer.Width)...)
	}
	payload = append(payload, escpos.FeedBytes(e.headLines)...)
	payload = append(payload, e.data...)
	payload = append(payload, escpos.Finish(e.printer.Width, e.tailLines, e.cut)...)

	err := transport.Print(printerFor(&e.printer), payload)
	if err == nil {
		s.setStatus(e, model.JobDone, "")
		tag := ""
		if reprint {
			tag = "(重印)"
		}
		logger.Infof("任务 #%d%s 已发送到 %s %s", e.job.No, tag, e.printer.Name, e.printer.Target())
		return
	}

	// 连接类错误(网络异常)→ 等待重试:退避后持续重试,不计 retryMax、不显"重印",
	// 堆在任务列表,直到网络恢复打成功或被取消。
	if transport.IsConnError(err) {
		s.mu.Lock()
		switch {
		case e.backoff == 0:
			e.backoff = 5 * time.Second
		case e.backoff < 30*time.Second:
			e.backoff *= 2
			if e.backoff > 30*time.Second {
				e.backoff = 30 * time.Second
			}
		}
		wait := e.backoff
		s.mu.Unlock()
		logger.Infof("任务 #%d 网络异常,最多 %.0fs 后重试或上线即打(%s): %v", e.job.No, wait.Seconds(), e.printer.Target(), err)
		s.setStatus(e, model.JobWaiting, err.Error())
		// 退避等待,但一旦监测到该机上线(NudgeOnline)就立即重试、并重置退避。
		select {
		case <-time.After(wait):
		case <-e.wake:
			s.mu.Lock()
			e.backoff = 0
			s.mu.Unlock()
		}
		s.mu.Lock()
		cancelled := e.cancelled
		s.mu.Unlock()
		if cancelled {
			return
		}
		s.dispatch(e)
		return
	}

	// 非连接错误:按重打设置有限重试(最多 retryMax 次;attempts 已含首次)。
	// 自动重试【不】加"重打"抬头——重打标记只在用户手动点「重新打印」时才加(见 Retry)。
	s.mu.Lock()
	canRetry := e.retry && e.attempts <= e.retryMax
	s.mu.Unlock()
	if canRetry {
		logger.Infof("任务 #%d 打印失败,重打(第 %d/%d 次,%s): %v", e.job.No, e.attempts, e.retryMax, e.printer.Target(), err)
		s.setStatus(e, model.JobQueued, "")
		time.Sleep(500 * time.Millisecond)
		s.dispatch(e)
		return
	}
	s.setStatus(e, model.JobFailed, err.Error())
	logger.Errorf("任务 #%d 打印失败(%s %s): %v", e.job.No, e.printer.Name, e.printer.Target(), err)
}

// Retry 重新打印指定任务号。
func (s *Service) Retry(no int) bool {
	s.mu.Lock()
	e := s.find(no)
	if e == nil {
		s.mu.Unlock()
		return false
	}
	e.job.Status = model.JobQueued
	e.job.Time = time.Now().Format("15:04:05")
	e.reprintNext = true // 手动重打是有意重印,显"重印"抬头
	e.cancelled = false
	e.backoff = 0
	s.mu.Unlock()
	s.fireNotify()
	go s.dispatch(e)
	return true
}

// Cancel 移除指定任务(进行中的作业已发送则无法真正撤回,仅从列表移除)。
func (s *Service) Cancel(no int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.job.No == no {
			e.cancelled = true // 让休眠中的等待重试 goroutine 醒来即停
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			s.fireNotifyLocked()
			return true
		}
	}
	return false
}

// ClearDone 清除所有已完成任务。
func (s *Service) ClearDone() {
	s.mu.Lock()
	kept := s.entries[:0]
	for _, e := range s.entries {
		if e.job.Status != model.JobDone {
			kept = append(kept, e)
		}
	}
	s.entries = kept
	s.mu.Unlock()
	s.fireNotify()
}

// NudgeOnline 在监测到某台打印机上线时调用:立即唤醒它所有「等待重试」的任务马上打,不等退避。
// 与自动重打(退避)互为兜底——监测漏了还有退避,监测到了就秒打。
func (s *Service) NudgeOnline(printerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.printer.ID == printerID && e.job.Status == model.JobWaiting {
			select {
			case e.wake <- struct{}{}: // 缓冲 1,非阻塞
			default:
			}
		}
	}
}

func (s *Service) setStatus(e *entry, st model.JobStatus, errMsg string) {
	s.mu.Lock()
	e.job.Status = st
	e.job.Err = errMsg
	s.mu.Unlock()
	s.fireNotify()
}

func (s *Service) find(no int) *entry {
	for _, e := range s.entries {
		if e.job.No == no {
			return e
		}
	}
	return nil
}

func (s *Service) fireNotify() {
	s.mu.Lock()
	f := s.notify
	s.mu.Unlock()
	if f != nil {
		f()
	}
}

// fireNotifyLocked 在已持有锁时读取回调并在解锁语义外调用(调用方随后会解锁)。
func (s *Service) fireNotifyLocked() {
	if s.notify != nil {
		go s.notify()
	}
}

// printerFor 从打印机模型构造传输通道。
func printerFor(p *model.Printer) transport.Printer {
	if p.Conn == model.ConnUSB {
		return transport.NewSpoolerPrinter(p.USBName)
	}
	port := p.Port
	if port == "" {
		port = transport.RawPrintPort
	}
	return transport.NewTCPPrinter(p.IP + ":" + port)
}
