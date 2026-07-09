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
	"time"

	"congmingpay/internal/config"
	"congmingpay/internal/docserver"
	"congmingpay/internal/instance"
	"congmingpay/internal/logger"
	"congmingpay/internal/mqtt"
	"congmingpay/internal/printsvc"
	"congmingpay/internal/ui"

	"github.com/lxn/walk"
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

	// 单实例守卫(命名互斥体,跨会话):二次启动友好提示后退出,
	// 不触碰配置/打印/MQTT/端口(避免与运行中实例互抢 MQTT ClientID 顶号、竞争损坏备份)。
	lock, already, err := instance.Acquire()
	if err != nil {
		logger.Errorf("单实例互斥体创建失败(按唯一实例继续启动): %v", err)
	}
	if already {
		logger.Info("程序已在运行,本次启动退出")
		// 标题只读窥探配置里的服务名,与运行中实例的窗口标题/托盘提示一致
		walk.MsgBox(nil, config.PeekServiceName(config.DefaultPath()),
			"程序已在运行,无需重复打开。\r\n可通过屏幕右下角的系统托盘图标打开主窗口。",
			walk.MsgBoxIconInformation|walk.MsgBoxSetForeground)
		return
	}
	defer lock.Release()

	cfgPath := config.DefaultPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		// 配置损坏:备份坏文件(打印机登记信息可从备份手工恢复)并记录日志,以默认配置启动
		bad := cfgPath + ".bad-" + time.Now().Format("20060102-150405")
		if renameErr := os.Rename(cfgPath, bad); renameErr == nil {
			logger.Errorf("加载配置失败(%s): %v;损坏文件已备份到 %s,改用默认配置", cfgPath, err, bad)
		} else {
			logger.Errorf("加载配置失败(%s): %v(备份损坏文件失败: %v),改用默认配置", cfgPath, err, renameErr)
		}
		cfg = config.Default()
	}

	// 配置即当前标准形态,无迁移/自愈逻辑(结构变更时删除旧 config.json 重新配置)。
	// 上报主题无默认值:服务端固定主题由用户在设置页手动填写;
	// 为空时 mqtt.reconnect 只停不连并记日志,启用 MQTT 保存设置时强制必填。
	// 打印机清单(含 ID)日志,便于排查
	for _, p := range cfg.PrinterList() {
		logger.Infof("已加载打印机: [id=%s] 名称『%s』品牌『%s』规格 %s 目标 %s", p.ID, p.Name, p.BrandLabel(), p.WidthLabel(), p.Target())
	}

	svc := printsvc.New()
	// 云端唯一通道:MQTT。UI 在 Run 里给它注册打印机列表刷新回调,并在设置保存时 Reload。
	mc := mqtt.New(cfg, svc, cfgPath)
	// 仅局域网、只读的在线接口文档服务(不参与数据通信)。UI 在设置保存时 Reload。
	ds := docserver.New()
	// NewApp 是纯构造(不碰 walk),先建以便任务事件回调并联系统通知。
	app := ui.NewApp(cfg, cfgPath, svc, mc, ds)
	// 任务事件(终态成功/失败、长期卡单)单回调槽在此组合两个消费者:
	// ① 系统通知(失败/卡单;本地任务也通知;托盘未就绪时由 notify 内部静默丢弃);
	// ② 上报 report——本地任务(测试打印/样票,无云端 id)不上报。
	svc.SetOnJobEvent(func(ev printsvc.JobEvent) {
		app.NotifyJobEvent(ev)
		if ev.CloudID == nil {
			return
		}
		mc.PublishJobEvent(ev)
	})
	mc.Start()
	defer mc.Close()

	ds.Start(cfg.Settings.DocServer)
	defer ds.Stop()

	if err := app.Run(); err != nil {
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
