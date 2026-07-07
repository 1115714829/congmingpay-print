// Package docserver 提供仅局域网、只读的「在线 API 接口文档」HTTP 服务。
//
// 本服务只暴露一个接口文档页面(GET /),不提供任何打印或数据端点:
// 数据通信由 MQTT 承担,本服务不参与。用标准库 net/http,兼容 go1.20 / GOARCH=386 / Windows 7。
package docserver

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"
)

// DefaultPort 是接口文档服务的默认监听端口。
const DefaultPort = 8080

//go:embed apidoc.html
var apiDocHTML []byte

// Server 是只读接口文档服务。始终以指针使用。
type Server struct {
	mu  sync.Mutex
	srv *http.Server
}

// New 创建服务(未启动)。
func New() *Server { return &Server{} }

// Start 按配置启动;未启用则只停不起。可重复调用(先停旧,再起新)。
func (s *Server) Start(cfg model.DocServer) {
	s.Stop()
	if !cfg.Enabled {
		logger.Info("接口文档服务未启用")
		return
	}
	port := cfg.Port
	if port <= 0 {
		port = DefaultPort
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", handle)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.mu.Lock()
	s.srv = srv
	s.mu.Unlock()

	// 绑定与服务放后台 goroutine:首次绑定通配地址(:port)时,Windows 防火墙拦截可能阻塞较久,
	// 不能卡住主程序启动。绑定失败(端口占用等)只记日志、不影响主程序。
	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			logger.Errorf("接口文档服务监听失败(端口 %d,可能被占用): %v", port, err)
			return
		}
		logger.Infof("接口文档服务已启动(只读,仅文档): http://%s:%d/", LANIP(), port)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Errorf("接口文档服务已停止(端口 %d): %v", port, err)
		}
	}()
}

// Reload 配置变更后重启。
func (s *Server) Reload(cfg model.DocServer) { s.Start(cfg) }

// Stop 优雅关闭当前服务。
func (s *Server) Stop() {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.mu.Unlock()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

// handle 只读处理:仅 GET/HEAD / 返回接口文档页;其它方法 405,其它路径 404。
// 本服务不提供任何数据/打印端点。
func handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "本服务仅提供接口文档查看(GET),不提供数据或打印接口。", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(apiDocHTML)
}

// LANIP 返回一个本机私网 IPv4,供界面展示访问地址;取不到时返回占位串。
func LANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "本机IP"
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil && isPrivateIPv4(ip4) {
			return ip4.String()
		}
	}
	return "本机IP"
}

// isPrivateIPv4 判断是否为常见私网网段(10/8、172.16/12、192.168/16)。
func isPrivateIPv4(ip net.IP) bool {
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}
