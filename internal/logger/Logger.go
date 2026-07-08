// Package logger 提供全局文件日志。
//
// GUI(无控制台)程序的崩溃与运行信息都写入程序目录的日志文件,便于排查。
// 使用方式:main 启动时 Init,其余代码统一调用 Info/Infof/Error/Errorf,
// 不要在业务代码里直接使用标准库 log。
package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

var (
	mu   sync.Mutex
	file *os.File
	// 时间戳自行格式化(yyyy-mm-dd HH:MM:SS),故不用 log 的内置 flags。
	std = log.New(os.Stderr, "", 0)
)

// Init 以追加方式打开日志文件,并把后续日志写入其中。
// 返回的关闭函数需在程序退出前调用。
func Init(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	file = f
	std.SetOutput(f)
	mu.Unlock()

	return func() error {
		mu.Lock()
		defer mu.Unlock()
		if file == nil {
			return nil
		}
		err := file.Close()
		file = nil
		return err
	}, nil
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
	std.Printf("%s [%s] %s", time.Now().Format("2006-01-02 15:04:05"), level, msg)
}
