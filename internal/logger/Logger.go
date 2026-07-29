// Package logger 提供全局文件日志。
//
// GUI(无控制台)程序的崩溃与运行信息都写入程序目录下的日志文件,便于排查。
// 使用方式:main 启动时 Init,其余代码统一调用 Info/Infof/Error/Errorf,
// 不要在业务代码里直接使用标准库 log。
//
// 日志按天分文件:log\<yyyy-MM-dd>.log;后台每天本地 0 点整切换新文件,
// 仅保留最近 7 天(含当天)。切换与每条日志写入共用同一把互斥锁,跨天不丢行。
package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

const (
	// dayLayout 是日志文件名中的日期格式与文件名后缀。
	dayLayout   = "2006-01-02"
	logFileExt  = ".log"
	retainDays  = 7 // 保留最近天数(含当天)
	filePattern = `^\d{4}-\d{2}-\d{2}\.log$`
)

var (
	mu   sync.Mutex
	file *os.File
	// 时间戳自行格式化(yyyy-mm-dd HH:MM:SS),故不用 log 的内置 flags。
	std = log.New(os.Stderr, "", 0)

	logDir  string
	curDate string
	stopCh  chan struct{}

	nowFn = time.Now // 供测试注入

	logNameRe = regexp.MustCompile(filePattern)
)

// Init 以追加方式打开当天日志文件(logDir 自动创建),启动后台按天轮转,
// 并立即清理超过保留天数的日志。返回的关闭函数需在程序退出前调用。
func Init(dir string) (func() error, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	mu.Lock()
	logDir = dir
	stopCh = make(chan struct{})
	if err := swapLocked(nowFn()); err != nil {
		mu.Unlock()
		return nil, err
	}
	pruneLocked(nowFn())
	mu.Unlock()

	go rotateLoop(stopCh)

	return func() error {
		mu.Lock()
		defer mu.Unlock()
		if stopCh != nil {
			close(stopCh)
			stopCh = nil
		}
		if file == nil {
			return nil
		}
		err := file.Close()
		file = nil
		return err
	}, nil
}

// CurrentPath 返回当前写入的日志文件完整路径(未初始化时为空)。
func CurrentPath() string {
	mu.Lock()
	defer mu.Unlock()
	if logDir == "" || curDate == "" {
		return ""
	}
	return filepath.Join(logDir, curDate+logFileExt)
}

// rotateLoop 每天本地 0 点整切换到新日期文件并清理过期日志。
func rotateLoop(stop <-chan struct{}) {
	for {
		now := nowFn()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		t := time.NewTimer(time.Until(next))
		select {
		case <-stop:
			t.Stop()
			return
		case <-t.C:
		}
		mu.Lock()
		_ = swapLocked(nowFn())
		pruneLocked(nowFn())
		mu.Unlock()
	}
}

// swapLocked 在日期变化时切换到当天文件(调用方须持锁)。
// 打开失败保留旧文件继续写,返回错误供记录。
func swapLocked(now time.Time) error {
	d := now.Format(dayLayout)
	if d == curDate && file != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(logDir, d+logFileExt), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if file != nil {
		_ = file.Close()
	}
	file = f
	curDate = d
	std.SetOutput(f)
	return nil
}

// pruneLocked 删除超过保留天数的日期日志文件(调用方须持锁)。
func pruneLocked(now time.Time) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	cutoff := today.AddDate(0, 0, -(retainDays - 1))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !logNameRe.MatchString(name) {
			continue
		}
		d, err := time.ParseInLocation(dayLayout, name[:len(name)-len(logFileExt)], time.Local)
		if err != nil {
			continue
		}
		if d.Before(cutoff) {
			_ = os.Remove(filepath.Join(logDir, name)) // 占用/权限失败仅跳过,不中断
		}
	}
}

// Info 记录一条信息级日志。
func Info(msg string) { output("INFO", msg) }

// Infof 按格式记录一条信息级日志。
func Infof(format string, args ...interface{}) { output("INFO", fmt.Sprintf(format, args...)) }

// Warn 记录一条警告级日志。
func Warn(msg string) { output("WARN", msg) }

// Warnf 按格式记录一条警告级日志。
func Warnf(format string, args ...interface{}) { output("WARN", fmt.Sprintf(format, args...)) }

// Error 记录一条错误级日志。
func Error(msg string) { output("ERROR", msg) }

// Errorf 按格式记录一条错误级日志。
func Errorf(format string, args ...interface{}) { output("ERROR", fmt.Sprintf(format, args...)) }

func output(level, msg string) {
	mu.Lock()
	defer mu.Unlock()
	std.Printf("%s [%s] %s", nowFn().Format("2006-01-02 15:04:05"), level, msg)
}
