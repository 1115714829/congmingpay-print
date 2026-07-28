package printsvc

import (
	"time"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"
)

// SetHistoryDays 更新保留天数并立即 prune。
func (s *Service) SetHistoryDays(days int) {
	s.mu.Lock()
	s.historyDays = model.ClampJobHistoryDays(days)
	s.mu.Unlock()
	s.PruneHistory()
}

// HistoryDays 返回当前保留天数。
func (s *Service) HistoryDays() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.historyDays
}

// CloseStore 刷盘并关闭 DB。
func (s *Service) CloseStore() {
	s.flushPersist()
	if s.store != nil {
		_ = s.store.Close()
		s.store = nil
	}
}

// PruneHistory 按保留天数删除超龄 done/failed(内存 + DB);Active 保留。
func (s *Service) PruneHistory() {
	s.mu.Lock()
	days := s.historyDays
	cutoff := time.Now().AddDate(0, 0, -days)
	kept := s.entries[:0]
	for _, e := range s.entries {
		st := e.job.Status
		if (st == model.JobDone || st == model.JobFailed) && !e.createdAt.IsZero() && e.createdAt.Before(cutoff) {
			continue
		}
		kept = append(kept, e)
	}
	s.entries = kept
	s.mu.Unlock()
	if s.store != nil {
		n, err := s.store.PruneTerminal(days, time.Now())
		persistErr("prune", err)
		if n > 0 {
			logger.Infof("打印历史清理:删除 %d 条超 %d 天的已完成/失败任务", n, days)
		}
	}
	s.fireNotify()
}

// RestoreAndResume 从 DB 加载任务并恢复 Active 派发。进程中断的 printing → queued。
func (s *Service) RestoreAndResume() {
	if s.store == nil {
		return
	}
	s.PruneHistory()
	jobs, err := s.store.LoadAll()
	if err != nil {
		logger.Errorf("加载打印任务失败: %v", err)
		return
	}
	next, _ := s.store.GetNextNo()
	var resume []*entry
	s.mu.Lock()
	s.entries = s.entries[:0]
	maxNo := next
	for _, j := range jobs {
		if j.No > maxNo {
			maxNo = j.No
		}
		st := j.Status
		if st == model.JobPrinting {
			st = model.JobQueued // 进程已死,不可能仍在打印
		}
		e := &entry{
			job: &model.Job{
				No: j.No, Doc: j.Doc, Printer: j.Printer.Name,
				Status: st, Time: j.TimeLabel, Err: j.Err,
			},
			printer: j.Printer, data: j.Data, cloudID: j.CloudID,
			createdAt: j.CreatedAt, cut: j.Cut, buzzer: j.Buzzer,
			headLines: j.HeadLines, tailLines: j.TailLines,
			reprintNext: j.ReprintNext, wake: make(chan struct{}, 1),
			contentType: j.ContentType, sourceJSON: j.SourceJSON,
		}
		s.entries = append(s.entries, e)
		if st.Active() {
			resume = append(resume, e)
		}
	}
	s.nextNo = maxNo
	s.mu.Unlock()
	persistErr("set-next", s.store.SetNextNo(maxNo))
	if len(jobs) > 0 {
		logger.Infof("已从 jobs.db 恢复 %d 条任务(其中 %d 条将继续派发)", len(jobs), len(resume))
	}
	for _, e := range resume {
		go s.dispatch(e)
	}
	s.fireNotify()
}

func (s *Service) markDirtyLocked(no int) {
	if s.dirty == nil {
		s.dirty = map[int]struct{}{}
	}
	s.dirty[no] = struct{}{}
}

func (s *Service) schedulePersist() {
	if s.store == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.persistTimer != nil {
		s.persistTimer.Stop()
	}
	s.persistTimer = time.AfterFunc(persistDebounce, s.flushPersist)
}

// FlushPersist 立即将脏任务与 nextNo 写入 DB(退出前调用)。
func (s *Service) FlushPersist() { s.flushPersist() }

func (s *Service) flushPersist() {
	if s.store == nil {
		return
	}
	s.mu.Lock()
	if s.persistTimer != nil {
		s.persistTimer.Stop()
		s.persistTimer = nil
	}
	nos := make([]int, 0, len(s.dirty))
	for no := range s.dirty {
		nos = append(nos, no)
	}
	s.dirty = map[int]struct{}{}
	next := s.nextNo
	var batch []persistedJob
	for _, no := range nos {
		if e := s.find(no); e != nil {
			batch = append(batch, entryToPersisted(e))
		}
	}
	s.mu.Unlock()
	for _, j := range batch {
		persistErr("upsert", s.store.UpsertJob(j))
	}
	persistErr("next-no", s.store.SetNextNo(next))
}
