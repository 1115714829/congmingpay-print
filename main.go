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
	"strings"
	"time"

	"congmingpay/internal/config"
	"congmingpay/internal/docserver"
	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/mqtt"
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
		// 配置损坏:先备份坏文件(打印机注册表可从备份手工恢复),再用默认配置继续启动——不再静默丢弃
		bad := cfgPath + ".bad-" + time.Now().Format("20060102-150405")
		if renameErr := os.Rename(cfgPath, bad); renameErr == nil {
			logger.Errorf("加载配置失败(%s): %v;损坏文件已备份到 %s,改用默认配置", cfgPath, err, bad)
		} else {
			logger.Errorf("加载配置失败(%s): %v(备份损坏文件失败: %v),改用默认配置", cfgPath, err, renameErr)
		}
		cfg = config.Default()
	}

	// 自愈:修复旧配置里空/重复的打印机 ID(历史并发同纳秒撞号),并落盘
	if cfg.HealPrinterIDs() {
		logger.Info("检测到重复/空的打印机 ID,已重新分配唯一 ID 并保存")
		if err := cfg.Save(cfgPath); err != nil {
			logger.Errorf("保存修复后的配置失败: %v", err)
		}
	}
	// 迁移:旧配置无 docServer 段(Port==0)→ 应用默认(启用、端口 8080),并落盘
	if cfg.Settings.DocServer.Port == 0 {
		cfg.Settings.DocServer = model.DefaultSettings().DocServer
		if err := cfg.Save(cfgPath); err != nil {
			logger.Errorf("保存默认接口文档服务配置失败: %v", err)
		}
	}
	// 迁移:旧配置有短商户号但无上报主题 → 自动补「<净化后短商户号>/report」。
	// 用 SanitizeMerchant 与旧版实际发布主题(merchant=SanitizeMerchant(Topic))完全一致,避免手改脏 Topic 时迁移出不同主题。
	if m := mqtt.SanitizeMerchant(cfg.Settings.MQTT.Topic); m != "" && strings.TrimSpace(cfg.Settings.MQTT.ReportTopic) == "" {
		cfg.Settings.MQTT.ReportTopic = m + "/report"
		logger.Infof("旧配置无上报主题,已自动迁移为 %s(可在系统设置里修改)", cfg.Settings.MQTT.ReportTopic)
		if err := cfg.Save(cfgPath); err != nil {
			logger.Errorf("保存迁移后的上报主题失败: %v", err)
		}
	}
	// 打印机清单(含 ID)日志,便于排查
	for _, p := range cfg.PrinterList() {
		logger.Infof("已加载打印机: [id=%s] 名称『%s』品牌『%s』规格 %s 目标 %s", p.ID, p.Name, p.BrandLabel(), p.WidthLabel(), p.Target())
	}

	svc := printsvc.New()
	// 云端唯一通道:MQTT。UI 在 Run 里给它注册打印机列表刷新回调,并在设置保存时 Reload。
	mc := mqtt.New(cfg, svc, cfgPath)
	// 任务终态(成功/失败)→ 上报打印结果;本地任务(测试打印/样票,无云端 id)不上报。
	svc.SetOnJobFinal(func(ev printsvc.JobFinalEvent) {
		if ev.CloudID == nil {
			return
		}
		mc.PublishJobResult(ev)
	})
	mc.Start()
	defer mc.Close()

	// 仅局域网、只读的在线接口文档服务(不参与数据通信)。UI 在设置保存时 Reload。
	ds := docserver.New()
	ds.Start(cfg.Settings.DocServer)
	defer ds.Stop()

	if err := ui.NewApp(cfg, cfgPath, svc, mc, ds).Run(); err != nil {
		logger.Errorf("启动界面失败: %v", err)
		os.Exit(1)
	}
	logger.Info("=== 正常退出 ===")
}

// logFilePath 返回与可执行文件同目录下的日志路径。
func logFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "congmingpay.log"
	}
	return filepath.Join(filepath.Dir(exe), "congmingpay.log")
}
