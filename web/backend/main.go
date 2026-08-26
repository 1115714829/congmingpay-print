// CMF打印服务器 web 后端入口。
// 独立 module congmingpay/web,与主工程零耦合;Linux/Windows 均可部署。
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"congmingpay/web/internal/api"
	"congmingpay/web/internal/auth"
	"congmingpay/web/internal/config"
	"congmingpay/web/internal/store"
)

func main() {
	cfg := config.Load()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	// 文件日志(data/log/日期.log)+ stdout 双写,GUI 无控制台靠文件排查
	if err := setupFileLog(filepath.Join(cfg.DataDir, "log")); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	log.Printf("web 管理系统启动,数据目录 %s", cfg.DataDir)

	dbPath := filepath.Join(cfg.DataDir, "web.db")
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败(%s): %v", dbPath, err)
	}
	defer st.Close()

	// 首次启动种子管理员
	created, err := st.SeedAdmin(cfg.AdminUser, mustHash(cfg.AdminPass))
	if err != nil {
		log.Fatalf("种子管理员失败: %v", err)
	}
	if created {
		log.Printf("[警告] 已创建初始管理员 %s(初始密码来自 ADMIN_PASSWORD 或默认 admin123),请尽快登录后修改密码!", cfg.AdminUser)
	}
	if cfg.JWTSecret == config.DefaultJWTSecret {
		log.Printf("[警告] 未设置环境变量 JWT_SECRET,正在使用内置默认密钥——生产环境务必通过 JWT_SECRET 注入随机密钥!")
	}

	// 到期回收:启动跑一次 + 每小时一次
	go expireLoop(st)

	srv := &api.Server{Store: st, JWTSecret: cfg.JWTSecret}
	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("web 管理系统监听 :%d,数据目录 %s", cfg.Port, cfg.DataDir)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号,正在关闭…")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	log.Println("已退出")
}

// expireLoop 分配独占期回收:立即一次,此后每 1h。
func expireLoop(st *store.Store) {
	run := func() {
		n, err := st.ExpireAllocations()
		if err != nil {
			log.Printf("到期回收失败: %v", err)
			return
		}
		if n > 0 {
			log.Printf("到期回收: %d 台超 30 天未绑定设备已回库存", n)
		}
	}
	run()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for range t.C {
		run()
	}
}

func mustHash(p string) string {
	h, err := auth.HashPassword(p)
	if err != nil {
		log.Fatalf("密码哈希失败: %v", err)
	}
	return h
}

// setupFileLog 日志双写到 data/log/日期.log 与 stdout。
func setupFileLog(logDir string) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(logDir, time.Now().Format("2006-01-02")+".log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	return nil
}
