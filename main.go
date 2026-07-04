// congmingpay 是面向餐厅厨房的热敏小票打印服务器(Phase 1:Windows demo)。
//
// 架构:
//   - internal/transport  打印通道抽象(USB 走已装驱动 / 网口 IP:9100)
//   - internal/escpos     ESC/POS 指令构建(中文 GB18030)
//   - internal/config     本地 JSON 配置(含 MQTT 占位)
//   - internal/ui         Walk 原生窗口 + 托盘
//   - internal/logger     全局文件日志
//   - internal/util       通用工具(编码等)
package main

import (
	"os"
	"path/filepath"
	"runtime/debug"

	"congmingpay/internal/api"
	"congmingpay/internal/config"
	"congmingpay/internal/logger"
	"congmingpay/internal/printsvc"
	"congmingpay/internal/ui"
)

func main() {
	logPath := logFilePath()
	if closeLog, err := logger.Init(logPath); err == nil {
		defer closeLog()
	}
	// GUI 程序无控制台,panic 也要落盘,便于在程序目录日志里排查。
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("panic: %v\n%s", r, debug.Stack())
			os.Exit(2)
		}
	}()
	logger.Infof("=== congmingpay 启动 (日志: %s) ===", logPath)

	cfgPath := config.DefaultPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Errorf("加载配置失败(%s),改用默认配置: %v", cfgPath, err)
		cfg = config.Default()
	}

	// 自愈:修复旧配置里空/重复的打印机 ID(历史并发同纳秒撞号),并落盘
	if cfg.HealPrinterIDs() {
		logger.Info("检测到重复/空的打印机 ID,已重新分配唯一 ID 并保存")
		if err := cfg.Save(cfgPath); err != nil {
			logger.Errorf("保存修复后的配置失败: %v", err)
		}
	}
	// 打印机清单(含 ID)日志,便于排查
	for _, p := range cfg.PrinterList() {
		logger.Infof("已加载打印机: [id=%s] 名称『%s』品牌『%s』规格 %s 目标 %s", p.ID, p.Name, p.BrandLabel(), p.WidthLabel(), p.Target())
	}

	svc := printsvc.New()
	// API 与 UI 共享同一个 Server:UI 在 Run 里给它注册打印机列表刷新回调。
	srv := api.NewServer(cfg, svc, cfgPath)
	go startAPI(srv, cfg.Settings.APIPort)

	if err := ui.NewApp(cfg, cfgPath, svc, srv).Run(); err != nil {
		logger.Errorf("启动界面失败: %v", err)
		os.Exit(1)
	}
	logger.Info("=== 正常退出 ===")
}

// startAPI 启动本地 HTTP 打印 API(阻塞;用 goroutine 调用)。
func startAPI(srv *api.Server, port string) {
	if port == "" {
		port = "8080"
	}
	logger.Infof("HTTP API 监听 :%s (/api/print, /api/printers, /swagger/)", port)
	if err := srv.Start(":" + port); err != nil {
		logger.Errorf("HTTP API 启动失败: %v", err)
	}
}

// logFilePath 返回与可执行文件同目录下的日志路径。
func logFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "congmingpay.log"
	}
	return filepath.Join(filepath.Dir(exe), "congmingpay.log")
}
