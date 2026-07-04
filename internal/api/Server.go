// Package api 提供本地 HTTP 打印接口(供第三方对接与 Swagger 文档)。
//
// @title        聪明付 打印服务 API
// @version      1.0
// @description  本地 HTTP 打印接口:提交 JSON 排版打印任务、查询已注册打印机。同一请求模型将来供 MQTT 复用。
// @BasePath     /
package api

import (
	"net/http"
	"sync"

	"congmingpay/internal/config"
	"congmingpay/internal/logger"
	"congmingpay/internal/printsvc"
)

// Server 是本地 HTTP 打印 API。
type Server struct {
	cfg      *config.Config
	svc      *printsvc.Service
	cfgPath  string
	mu       sync.Mutex
	seen     map[uint32]bool // 已受理的任务 id,防重打
	onChange func()          // 打印机列表变化(云端同步后)回调,UI 用它刷新
}

// NewServer 创建 API 服务。
func NewServer(cfg *config.Config, svc *printsvc.Service, cfgPath string) *Server {
	return &Server{cfg: cfg, svc: svc, cfgPath: cfgPath, seen: map[uint32]bool{}}
}

// SetOnChange 设置打印机列表变化回调(UI 用它刷新界面)。
func (s *Server) SetOnChange(f func()) { s.onChange = f }

// save 持久化配置(云端同步打印机后调用)。
func (s *Server) save() {
	if s.cfgPath == "" {
		return
	}
	if err := s.cfg.Save(s.cfgPath); err != nil {
		logger.Errorf("保存配置失败: %v", err)
	}
}

// fireChange 通知 UI 刷新打印机列表(若已设回调)。
func (s *Server) fireChange() {
	if s.onChange != nil {
		s.onChange()
	}
}

// Handler 返回 HTTP 路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/print", s.handlePrint)
	mux.HandleFunc("/api/printers", s.handlePrinters)
	s.mountSwagger(mux)
	return mux
}

// Start 在 addr 监听(阻塞;调用方用 goroutine)。
func (s *Server) Start(addr string) error {
	return http.ListenAndServe(addr, s.Handler())
}
