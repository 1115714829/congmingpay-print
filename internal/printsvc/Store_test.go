package printsvc

import (
	"path/filepath"
	"testing"
	"time"

	"congmingpay/internal/model"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	cid := uint32(42)
	j := persistedJob{
		No: 1001, Doc: "测试", Status: model.JobWaiting, TimeLabel: "12:00:00",
		Err: "offline", CreatedAt: time.Now().Add(-time.Hour),
		Printer: model.Printer{ID: "p1", Name: "厨打", Width: 58, Conn: model.ConnNetwork, IP: "1.2.3.4"},
		Data: []byte{0x1b, 0x40}, Cut: true, Buzzer: false, HeadLines: 1, TailLines: 2,
		ReprintNext: true, CloudID: &cid,
	}
	if err := s.UpsertJob(j); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNextNo(1001); err != nil {
		t.Fatal(err)
	}

	got, err := s.LoadAll()
	if err != nil || len(got) != 1 {
		t.Fatalf("load %v len=%d", err, len(got))
	}
	g := got[0]
	if g.No != 1001 || g.Status != model.JobWaiting || string(g.Data) != string(j.Data) {
		t.Fatalf("%+v", g)
	}
	if g.CloudID == nil || *g.CloudID != 42 || !g.ReprintNext || !g.Cut {
		t.Fatalf("fields %+v", g)
	}
	n, _ := s.GetNextNo()
	if n != 1001 {
		t.Fatalf("nextNo %d", n)
	}
}

func TestStorePruneKeepsActive(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	old := time.Now().AddDate(0, 0, -10)
	_ = s.UpsertJob(persistedJob{
		No: 1, Doc: "old-done", Status: model.JobDone, CreatedAt: old,
		Printer: model.Printer{ID: "p"}, Data: []byte{1},
	})
	_ = s.UpsertJob(persistedJob{
		No: 2, Doc: "old-wait", Status: model.JobWaiting, CreatedAt: old,
		Printer: model.Printer{ID: "p"}, Data: []byte{1},
	})
	_ = s.UpsertJob(persistedJob{
		No: 3, Doc: "new-done", Status: model.JobDone, CreatedAt: time.Now(),
		Printer: model.Printer{ID: "p"}, Data: []byte{1},
	})
	n, err := s.PruneTerminal(7, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("prune n=%d err=%v", n, err)
	}
	all, _ := s.LoadAll()
	if len(all) != 2 {
		t.Fatalf("left %d", len(all))
	}
	for _, j := range all {
		if j.No == 1 {
			t.Fatal("old done should be gone")
		}
	}
}

func TestRestorePrintingBecomesQueued(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.db")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.UpsertJob(persistedJob{
		No: 1005, Doc: "mid", Status: model.JobPrinting, CreatedAt: time.Now(),
		Printer: model.Printer{ID: "p", Name: "x", Conn: model.ConnUSB, USBName: "U"},
		Data: []byte{0x0a},
	})
	_ = st.SetNextNo(1005)
	_ = st.Close()

	st2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewWithStore(st2, 7)
	defer svc.CloseStore()
	svc.RestoreAndResume()
	jobs := svc.Jobs()
	if len(jobs) != 1 || jobs[0].Status != model.JobQueued {
		t.Fatalf("%+v", jobs)
	}
	if svc.nextNo < 1005 {
		t.Fatalf("nextNo %d", svc.nextNo)
	}
}

func TestClearDonePersists(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewWithStore(st, 7)
	defer svc.CloseStore()
	p := &model.Printer{ID: "p", Name: "n", Conn: model.ConnUSB, USBName: "U"}
	// Submit will try USB print and fail fast or succeed depending on env — use Restore path instead
	_ = st.UpsertJob(persistedJob{
		No: 2, Doc: "d", Status: model.JobDone, CreatedAt: time.Now(),
		Printer: *p, Data: []byte{1},
	})
	_ = st.UpsertJob(persistedJob{
		No: 3, Doc: "f", Status: model.JobFailed, CreatedAt: time.Now(),
		Printer: *p, Data: []byte{1},
	})
	svc.RestoreAndResume()
	svc.ClearDone()
	all, _ := st.LoadAll()
	if len(all) != 1 || all[0].Status != model.JobFailed {
		t.Fatalf("%+v", all)
	}
}

func TestClampJobHistoryDays(t *testing.T) {
	if model.ClampJobHistoryDays(0) != model.DefaultJobHistoryDays {
		t.Fatal()
	}
	if model.ClampJobHistoryDays(30) != 30 {
		t.Fatal()
	}
	if model.ClampJobHistoryDays(999) != model.DefaultJobHistoryDays {
		t.Fatal()
	}
}
