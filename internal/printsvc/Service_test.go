package printsvc

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"congmingpay/internal/errcode"
	"congmingpay/internal/model"
)

func jobStatus(s *Service, no int) model.JobStatus {
	for _, j := range s.Jobs() {
		if j.No == no {
			return j.Status
		}
	}
	return ""
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(30 * time.Millisecond)
	}
	return false
}

// 端口被拒绝(RST:127.0.0.1:1 端口关)必须进「等待重试」并**持续等待、永不 JobFailed**。
// 这锁定核心行为:被占/端口未就绪 → 排队等待(不再快速失败)。
func TestRefusedGoesWaitingNeverFail(t *testing.T) {
	svc := New()
	p := &model.Printer{ID: "t1", Name: "t1", Conn: model.ConnNetwork, IP: "127.0.0.1", Port: "1", Width: 80}
	no := svc.Submit(p, "test", []byte("x"), nil)

	if !waitFor(t, func() bool { return jobStatus(svc, no) == model.JobWaiting }, 3*time.Second) {
		t.Fatalf("被拒任务应进入 JobWaiting,当前=%s", jobStatus(svc, no))
	}
	// 再观察一段时间:必须始终不是 JobFailed(占用档持续等待)。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := jobStatus(svc, no); s == model.JobFailed {
			t.Fatalf("被拒任务不应变为 JobFailed,应持续等待")
		}
		time.Sleep(50 * time.Millisecond)
	}
	svc.Cancel(no)
}

// 同一台并发多份(pCopy 场景):被拒时全部进等待,**无一被误杀为 JobFailed**(串行锁 → 无自撞)。
func TestConcurrentCopiesNoFalseFail(t *testing.T) {
	svc := New()
	p := &model.Printer{ID: "t2", Name: "t2", Conn: model.ConnNetwork, IP: "127.0.0.1", Port: "1", Width: 80}
	nos := []int{}
	for i := 0; i < 3; i++ {
		nos = append(nos, svc.Submit(p, "copy", []byte("x"), nil))
	}
	// 给足首轮真打 + 分类
	time.Sleep(1500 * time.Millisecond)
	for _, no := range nos {
		if s := jobStatus(svc, no); s == model.JobFailed {
			t.Fatalf("并发副本 #%d 被误杀为 JobFailed(应为等待)", no)
		}
	}
	for _, no := range nos {
		svc.Cancel(no)
	}
}

// H1 回归:对一个仍在「等待重试」的任务点重打,**不能**产生重复打印
// (旧派发 goroutine 必须因代次作废而退出,只留新派发那一份)。
// 手法:先让任务打到一个"当前拒绝连接"的端口 → 进等待;点 Retry(代次+1);
// 再在同一端口起一个计数监听(打印机"上线");最终真正连上的打印**只应有 1 次**。
func TestRetryWhileWaitingNoDuplicatePrint(t *testing.T) {
	// 取一个空闲端口后立即关闭 → 该端口此刻"拒绝连接"。
	l0, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("无法取端口: %v", err)
	}
	port := l0.Addr().(*net.TCPAddr).Port
	_ = l0.Close()

	svc := New()
	p := &model.Printer{ID: "h1", Name: "h1", Conn: model.ConnNetwork, IP: "127.0.0.1", Port: fmt.Sprintf("%d", port), Width: 80}
	no := svc.Submit(p, "test", []byte("x"), nil)

	// 等它因被拒进入等待重试。
	if !waitFor(t, func() bool { return jobStatus(svc, no) == model.JobWaiting }, 3*time.Second) {
		t.Fatalf("应进入 JobWaiting,当前=%s", jobStatus(svc, no))
	}
	// 点重打:代次+1,旧 goroutine 应被唤醒并因代次不符退出。
	if !svc.Retry(no) {
		t.Fatal("Retry 应成功")
	}

	// 现在在同一端口起计数监听(打印机"上线"),统计真正连上的打印次数。
	var conns int32
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Skipf("端口被抢占,跳过: %v", err)
	}
	defer l.Close()
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			atomic.AddInt32(&conns, 1)
			_, _ = io.Copy(io.Discard, c)
			_ = c.Close()
		}
	}()

	// 等任务打成功(占用短退避 1~3s 内应重试连上)。
	if !waitFor(t, func() bool { return jobStatus(svc, no) == model.JobDone }, 8*time.Second) {
		t.Fatalf("上线后应打成功 JobDone,当前=%s", jobStatus(svc, no))
	}
	// 关键断言:只应有 1 次真正的打印连接(无重复派发导致的第 2 次)。
	// 再稍等,确认没有第二个 goroutine 事后又打一次。
	time.Sleep(1500 * time.Millisecond)
	if n := atomic.LoadInt32(&conns); n != 1 {
		t.Fatalf("重打等待中的任务后应只打印 1 次,实际连接 %d 次(重复派发/H1 回归)", n)
	}
}

// 终态事件:打印成功应触发 OnJobEvent(done、code=0、CloudID 回携、Printer 快照);
// 本地任务(无 CloudID)事件照发、CloudID 为 nil。
func TestJobEventOnDone(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("无法起本地监听: %v", err)
	}
	defer l.Close()
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			_, _ = io.Copy(io.Discard, c)
			_ = c.Close()
		}
	}()
	port := l.Addr().(*net.TCPAddr).Port

	svc := New()
	events := make(chan JobEvent, 4)
	svc.SetOnJobEvent(func(ev JobEvent) { events <- ev })

	p := &model.Printer{ID: "e1", Name: "e1", Conn: model.ConnNetwork, IP: "127.0.0.1", Port: fmt.Sprintf("%d", port), Width: 80}
	id := uint32(880001)
	noCloud := svc.Submit(p, "cloud", []byte("x"), &Options{CloudID: &id})
	noLocal := svc.Submit(p, "local", []byte("y"), nil)

	got := map[int]JobEvent{}
	deadline := time.After(8 * time.Second)
	for len(got) < 2 {
		select {
		case ev := <-events:
			got[ev.JobNo] = ev
		case <-deadline:
			t.Fatalf("8s 内未收齐 2 个终态事件,已收 %d", len(got))
		}
	}
	ev := got[noCloud]
	if ev.Event != EventDone || ev.Code != 0 {
		t.Errorf("云端任务应 done/code=0,got %+v", ev)
	}
	if ev.CloudID == nil || *ev.CloudID != id {
		t.Errorf("终态事件应回携 CloudID=%d,got %+v", id, ev.CloudID)
	}
	if ev.Printer.ID != "e1" {
		t.Errorf("终态事件应携带打印机快照,got %+v", ev.Printer)
	}
	evL := got[noLocal]
	if evL.Event != EventDone {
		t.Errorf("本地任务应 done,got %+v", evL)
	}
	if evL.CloudID != nil {
		t.Errorf("本地任务 CloudID 应为 nil,got %d", *evL.CloudID)
	}
}

// 等待重试(拒连)在告警阈值前不触发任何事件——终态与 20s 卡单告警之外无上报。
func TestNoEventWhileWaiting(t *testing.T) {
	svc := New()
	events := make(chan JobEvent, 1)
	svc.SetOnJobEvent(func(ev JobEvent) { events <- ev })
	p := &model.Printer{ID: "e2", Name: "e2", Conn: model.ConnNetwork, IP: "127.0.0.1", Port: "1", Width: 80}
	no := svc.Submit(p, "test", []byte("x"), nil)
	if !waitFor(t, func() bool { return jobStatus(svc, no) == model.JobWaiting }, 3*time.Second) {
		t.Fatalf("应进入 JobWaiting,当前=%s", jobStatus(svc, no))
	}
	select {
	case ev := <-events:
		t.Fatalf("告警阈值前不应触发事件: %+v", ev)
	case <-time.After(2 * time.Second):
	}
	svc.Cancel(no)
}

// 持续被拒超阈值 → 触发一次 waiting 事件(code=LongWaiting)。阈值经包级变量临时调小。
func TestWaitingEventOnLongRefused(t *testing.T) {
	old := longWaitWarn
	longWaitWarn = 300 * time.Millisecond
	t.Cleanup(func() { longWaitWarn = old })

	svc := New()
	events := make(chan JobEvent, 4)
	svc.SetOnJobEvent(func(ev JobEvent) { events <- ev })
	id := uint32(990001)
	p := &model.Printer{ID: "w1", Name: "w1", Conn: model.ConnNetwork, IP: "127.0.0.1", Port: "1", Width: 80}
	no := svc.Submit(p, "test", []byte("x"), &Options{CloudID: &id})

	select {
	case ev := <-events:
		if ev.Event != EventWaiting {
			t.Fatalf("被拒任务应先收到 waiting 事件,got %+v", ev)
		}
		if ev.Code != errcode.LongWaiting || ev.JobNo != no || ev.CloudID == nil || *ev.CloudID != id {
			t.Fatalf("waiting 事件内容不符: %+v", ev)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("8s 内未收到 waiting 事件")
	}
	svc.Cancel(no)
}

// Cancel 应让等待中的任务停止(从列表移除)。
func TestCancelStopsWaiting(t *testing.T) {
	svc := New()
	p := &model.Printer{ID: "t3", Name: "t3", Conn: model.ConnNetwork, IP: "127.0.0.1", Port: "1", Width: 80}
	no := svc.Submit(p, "test", []byte("x"), nil)
	waitFor(t, func() bool { return jobStatus(svc, no) == model.JobWaiting }, 3*time.Second)
	if !svc.Cancel(no) {
		t.Fatal("Cancel 应返回 true")
	}
	if jobStatus(svc, no) != "" {
		t.Fatalf("取消后任务应从列表移除")
	}
}
