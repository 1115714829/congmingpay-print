// Package printsvc 提供并发打印调度与打印机状态查询。
//
// 每个打印任务在独立 goroutine 中执行,多台打印机可同时打印。
// 任务状态变化通过 notify 回调通知 UI(UI 负责 marshal 回界面线程刷新)。
package printsvc

import (
	"fmt"
	"sync"
	"time"

	"congmingpay/internal/escpos"
	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/transport"
)

// 退避档与告警阈值:占用(RST=被占/端口未就绪)用短退避尽快抓释放;离线(超时/不可达)用长退避。
const (
	occBackoffStart = 1 * time.Second  // 占用短退避起点
	occBackoffCap   = 3 * time.Second  // 占用短退避上限
	offBackoffStart = 5 * time.Second  // 离线退避起点
	offBackoffCap   = 30 * time.Second // 离线退避上限
	longWaitWarn    = 20 * time.Second // 连续被拒超此时长 → 任务行/日志可见告警(疑似端口未就绪/故障),但不自动失败
)

// Options 是打印任务的可选覆盖参数(nil 字段=用打印机默认)。
type Options struct {
	Cut       *bool
	Buzzer    *bool
	Reprint   *bool // 本单是否带"重打"醒目抬头(消息 reprint:1 或手动重打)
	HeadLines *int
	TailLines *int
}

// entry 是一个任务的内部记录,附带打印机快照、数据与生效参数。
type entry struct {
	job     *model.Job
	printer model.Printer
	data    []byte

	// 生效参数(提交时按覆盖/默认解析)
	cut       bool
	buzzer    bool
	headLines int
	tailLines int

	reprintNext bool          // 下次派发是否显示"重打"抬头(消息 reprint:1 或手动「重新打印」)
	cancelled   bool          // 已取消:休眠中的等待重试醒来即停
	backoff     time.Duration // 等待重试的当前退避间隔(占用档/离线档共用此字段,按错误类型夹取)
	wake        chan struct{} // 监测到打印机上线时唤醒:立即重试、不等退避(缓冲 1)

	gated          bool      // 是否已做过首次"判在线"预检(仅首次门控;重试一律直接真打为最终裁决)
	firstRefusedAt time.Time // 连续被拒(占用/端口未就绪)流的起点;零值=当前不在被拒流。用于长期可见告警
	warned         bool      // 长期被拒告警是否已落日志(节流,只记一次)
	gen            int       // 派发代次:Retry 自增以作废仍在跑的旧派发 goroutine(旧 goroutine 检查点发现代次不符即退出,杜绝重复打印)
}

// Service 是并发打印调度器。
type Service struct {
	mu      sync.Mutex
	entries []*entry
	nextNo  int
	notify  func()

	// 每台打印机一把打印串行锁(printerID → *sync.Mutex):本进程对同一台同时只开一个 9100 连接,
	// pCopy 多份/并发任务串行打印,彼此不自撞 RST;不同打印机仍并行。仅在真打(transport.Print)那步持锁。
	printLocks sync.Map
}

// printerLock 返回某台打印机的打印串行锁(懒创建)。
func (s *Service) printerLock(id string) *sync.Mutex {
	v, _ := s.printLocks.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
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
	e.headLines = p.HeadLines
	e.tailLines = p.TailLines
	if opts != nil {
		if opts.Cut != nil {
			e.cut = *opts.Cut
		}
		if opts.Buzzer != nil {
			e.buzzer = *opts.Buzzer
		}
		if opts.Reprint != nil {
			e.reprintNext = *opts.Reprint // 消息 reprint:1 → 首次派发即带"重打"抬头
		}
		if opts.HeadLines != nil {
			e.headLines = *opts.HeadLines
		}
		if opts.TailLines != nil {
			e.tailLines = *opts.TailLines
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

	logger.Infof("任务 #%d 提交 → %s %s (蜂鸣:%s 切刀:%s 重打抬头:%s)",
		jobNo, p.BrandLabel(), p.Target(), onOff(e.buzzer), onOff(e.cut), onOff(e.reprintNext))
	go s.dispatch(e)
	return jobNo
}

func onOff(v bool) string {
	if v {
		return "开"
	}
	return "关"
}

// dispatch 是一个任务的派发循环(单 goroutine,**迭代非递归**,避免长期等待时栈无限增长)。
// 每轮:要求5「判在线→分支」+ 真打为最终裁决 + 按错误分类排队等待。
//   - 离线/被占 → 排队等待(两档退避),联通/释放即打(要求3);
//   - 已连上但写失败 → 直接失败(避免重复出纸)。
//
// 用 e.gen 代次守卫:Retry 自增代次并唤醒旧 goroutine,旧 goroutine 在检查点发现代次不符即退出,
// 杜绝「Retry 一个仍在等待/排队的任务时又起一个派发,导致重复打印/goroutine 泄漏」(H1)。
func (s *Service) dispatch(e *entry) {
	s.mu.Lock()
	myGen := e.gen
	s.mu.Unlock()

	for {
		s.mu.Lock()
		if e.cancelled || e.gen != myGen { // 已取消 / 已被 Retry 作废 → 退出
			s.mu.Unlock()
			return
		}
		reprint := e.reprintNext
		firstGate := !e.gated
		e.gated = true
		s.mu.Unlock()

		// 要求5:首次派发先按【公共检测 Status】判在线(仅网口:ICMP,不碰 9100),离线则直接进队等待,
		// 省去对确定离线机的 5s 拨号空等。**仅首次门控**——重试一律直接真打,真打为最终裁决,
		// 避免 ICMP 被防火墙/固件过滤但 9100 可打的机被假离线永久排队(红蓝对抗要点)。
		// USB 交后台 spooler 排队,不做 ICMP 门控(避免 spooler 瞬时查询失败误判离线)。
		if firstGate && e.printer.Conn == model.ConnNetwork {
			if st := Status(&e.printer); !st.Online {
				logger.Infof("任务 #%d 判在线: 离线 → 进队等待联通(%s)", e.job.No, e.printer.Target())
				if !s.waitAndContinue(e, myGen, false, fmt.Errorf("打印机离线(%s),等待联通", e.printer.Target())) {
					return
				}
				continue
			}
		}

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

		// 要求2/3:每台一把打印串行锁,本进程对同一台同时只开一个 9100 连接(pCopy 多份/并发任务串行、
		// 彼此不自撞 RST;不同打印机仍并行)。仅在真打这步持锁,退避 sleep 不持锁。
		lock := s.printerLock(e.printer.ID)
		lock.Lock()
		// 取锁后(可能已排在同机上一份之后等了一会)再确认未取消/未作废,避免打出已取消或旧代次的单。
		s.mu.Lock()
		stop := e.cancelled || e.gen != myGen
		s.mu.Unlock()
		if stop {
			lock.Unlock()
			return
		}
		s.setStatus(e, model.JobPrinting, "")
		logger.Infof("任务 #%d 开始发送(%s %s)", e.job.No, e.printer.Name, e.printer.Target())
		t0 := time.Now()
		err := transport.Print(printerFor(&e.printer), payload)
		took := time.Since(t0)
		lock.Unlock()

		tag := ""
		if reprint {
			tag = "(重印)"
		}

		// USB 显式走 Windows 驱动/打印后台:不参与 9100 的在线检测/占用/离线重试。
		// 交后台成功 = 已入 Windows 打印队列(打印机离线则上电后由 Windows 补打)→ 完成;
		// 失败 = 驱动/打印机名/配置类错误,重试无益 → 失败。
		if e.printer.Conn == model.ConnUSB {
			if err == nil {
				s.setStatus(e, model.JobDone, "")
				logger.Infof("任务 #%d%s 已交打印后台(USB『%s』,耗时 %dms)", e.job.No, tag, e.printer.USBName, took.Milliseconds())
				return
			}
			s.setStatus(e, model.JobFailed, err.Error())
			logger.Errorf("任务 #%d USB 打印失败(『%s』): %v", e.job.No, e.printer.USBName, err)
			return
		}

		// 以下仅网口:真打为最终裁决,按错误分类。
		if err == nil {
			s.setStatus(e, model.JobDone, "")
			logger.Infof("任务 #%d%s 已发送到 %s %s(耗时 %dms)", e.job.No, tag, e.printer.Name, e.printer.Target(), took.Milliseconds())
			return
		}
		// ① RST(被占/端口未就绪)→ 占用短退避排队等待(不失败;串行锁下自撞不可能,故必是第三方占用或端口未就绪)。
		if transport.IsRefused(err) {
			if !s.waitAndContinue(e, myGen, true, err) {
				return
			}
			continue
		}
		// ② 超时/不可达 → 离线退避排队等待(联通即打)。
		if transport.IsConnError(err) {
			if !s.waitAndContinue(e, myGen, false, err) {
				return
			}
			continue
		}
		// ③ 已连上但写失败/渲染类 → 直接失败,不自动重试(避免重复出纸)。
		s.setStatus(e, model.JobFailed, err.Error())
		logger.Errorf("任务 #%d 打印失败(%s %s): %v", e.job.No, e.printer.Name, e.printer.Target(), err)
		return
	}
}

// waitAndContinue 把任务转入「等待重试」并退避;返回 true=应继续重试,false=应停止(已取消或被 Retry 作废)。
// occupancy=true 为占用/端口未就绪(短退避档),否则为离线(长退避档)。
// 占用流长期持续 → 任务行/日志给可见告警(疑似端口未就绪/故障),但永不自动失败。
func (s *Service) waitAndContinue(e *entry, myGen int, occupancy bool, cause error) bool {
	start, capBackoff := offBackoffStart, offBackoffCap
	if occupancy {
		start, capBackoff = occBackoffStart, occBackoffCap
	}

	s.mu.Lock()
	if e.backoff < start {
		e.backoff = start
	} else {
		e.backoff *= 2
		if e.backoff > capBackoff {
			e.backoff = capBackoff
		}
	}
	wait := e.backoff

	detail := cause.Error()
	warnNow := false
	if occupancy {
		if e.firstRefusedAt.IsZero() {
			e.firstRefusedAt = time.Now()
		}
		if time.Since(e.firstRefusedAt) >= longWaitWarn {
			detail = "长期等待·疑似端口未就绪/故障(请检查 IP/端口/打印机): " + cause.Error()
			if !e.warned {
				e.warned = true
				warnNow = true
			}
		}
	} else {
		// 离线流不计入"端口未就绪"告警,重置被拒流计时。
		e.firstRefusedAt = time.Time{}
		e.warned = false
	}
	s.mu.Unlock()

	s.setStatus(e, model.JobWaiting, detail)
	reason := "离线"
	if occupancy {
		reason = "打印口被占/未就绪"
	}
	if warnNow {
		logger.Errorf("任务 #%d 长期等待·疑似端口未就绪/故障(%s %s): %v", e.job.No, e.printer.Name, e.printer.Target(), cause)
	} else {
		logger.Infof("任务 #%d %s,最多 %.0fs 后重试或联通/释放即打(%s): %v", e.job.No, reason, wait.Seconds(), e.printer.Target(), cause)
	}

	// 退避等待,但一旦被唤醒(监测到上线 NudgeOnline / 被 Cancel / 被 Retry)就立即醒来复查。
	select {
	case <-time.After(wait):
	case <-e.wake:
		s.mu.Lock()
		e.backoff = 0
		s.mu.Unlock()
		logger.Infof("任务 #%d 被唤醒(上线/释放),立即重打(%s)", e.job.No, e.printer.Target())
	}
	s.mu.Lock()
	stop := e.cancelled || e.gen != myGen
	s.mu.Unlock()
	return !stop
}

// Retry 重新打印指定任务号(手动/接口触发)。走全新派发,遵循 dispatch 的判在线/排队规则(要求4)。
// 正在打印中的任务拒绝重打(避免与在途真打叠出一份重复);等待/排队/已完成/失败均可重打。
func (s *Service) Retry(no int) bool {
	s.mu.Lock()
	e := s.find(no)
	if e == nil {
		s.mu.Unlock()
		return false
	}
	if e.job.Status == model.JobPrinting {
		s.mu.Unlock()
		return false // 正在打印中,不重复触发
	}
	// 代次自增:作废任何仍在跑的旧派发 goroutine(等待/排队中的),它们在检查点会发现代次不符而退出,
	// 从而杜绝「重打一个仍在等待的任务时又起一个派发 → 重复打印/goroutine 泄漏」(H1)。
	e.gen++
	e.job.Status = model.JobQueued
	e.job.Time = time.Now().Format("15:04:05")
	e.reprintNext = true // 手动重打是有意重印,显"重印"抬头
	e.cancelled = false
	e.backoff = 0
	// 重打视作全新派发:重做首次判在线门控、清被拒流与告警计时。
	e.gated = false
	e.firstRefusedAt = time.Time{}
	e.warned = false
	// 唤醒可能在休眠的旧 goroutine,让它按新代次立即醒来并退出(不必干等退避)。
	select {
	case e.wake <- struct{}{}:
	default:
	}
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
			// 立即唤醒可能在退避休眠的 goroutine,让它马上复查并退出(不必干等最长 30s 退避)。
			select {
			case e.wake <- struct{}{}:
			default:
			}
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
