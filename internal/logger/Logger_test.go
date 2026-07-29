package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDailyRotateAndRetain(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 7, 21, 10, 0, 0, 0, time.Local)
	cur := base
	oldNow := nowFn
	nowFn = func() time.Time { return cur }
	defer func() { nowFn = oldNow }()

	// 预置 10 天日期文件 + 非日志文件
	for i := 0; i < 10; i++ {
		d := base.AddDate(0, 0, -i).Format(dayLayout)
		if err := os.WriteFile(filepath.Join(dir, d+logFileExt), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "not-a-date.log"), []byte("x"), 0o644)

	closeLog, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer closeLog()

	Info("第一天")
	if got := filepath.Base(CurrentPath()); got != base.Format(dayLayout)+logFileExt {
		t.Fatalf("当天文件: %s", got)
	}

	// 跨天(切换+清理,与后台 0 点轮转同动作)
	cur = base.AddDate(0, 0, 1)
	mu.Lock()
	_ = swapLocked(nowFn())
	pruneLocked(nowFn())
	mu.Unlock()
	Info("第二天")
	if got := filepath.Base(CurrentPath()); got != cur.Format(dayLayout)+logFileExt {
		t.Fatalf("新日期文件: %s", got)
	}

	// 7 天保留:今天 + 前 6 天;第 7 天前已删
	entries, _ := os.ReadDir(dir)
	logs := 0
	var names []string
	for _, e := range entries {
		if logNameRe.MatchString(e.Name()) {
			logs++
			names = append(names, e.Name())
		}
	}
	if logs != 7 {
		t.Fatalf("应剩 7 份日志,got %d: %v", logs, names)
	}
	if _, err := os.Stat(filepath.Join(dir, "readme.txt")); err != nil {
		t.Fatalf("非日志文件不应被删: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "not-a-date.log")); err != nil {
		t.Fatalf("非日期 .log 不应被删: %v", err)
	}

	// 内容各归其日
	b1, _ := os.ReadFile(filepath.Join(dir, base.Format(dayLayout)+logFileExt))
	if !strings.Contains(string(b1), "第一天") {
		t.Fatalf("旧日期文件应含第一天日志: %q", b1)
	}
	b2, _ := os.ReadFile(filepath.Join(dir, cur.Format(dayLayout)+logFileExt))
	if !strings.Contains(string(b2), "第二天") {
		t.Fatalf("新日期文件应含第二天日志: %q", b2)
	}
}
