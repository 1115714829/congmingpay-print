package docserver

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 端到端:经真实 socket(loopback,避免通配绑定的防火墙延迟)验证 Serve + 内嵌 HTML + 只读契约。
func TestServeLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(handle)}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	base := "http://" + ln.Addr().String()
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(base + "/")
	if err != nil {
		t.Fatalf("GET 失败: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET / 应 200,实际 %d", resp.StatusCode)
	}
	if !bytes.Contains(body, []byte("接口文档")) {
		t.Fatal("响应应包含接口文档内容")
	}

	resp2, err := client.Post(base+"/", "text/plain", nil)
	if err != nil {
		t.Fatalf("POST 失败: %v", err)
	}
	code := resp2.StatusCode
	_ = resp2.Body.Close()
	if code != 405 {
		t.Fatalf("POST / 应 405(无数据端点),实际 %d", code)
	}
}

// GET / 返回接口文档页。
func TestHandleServesDocOnGetRoot(t *testing.T) {
	w := httptest.NewRecorder()
	handle(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatalf("GET / 应返回 200,实际 %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("接口文档")) {
		t.Fatal("响应应包含接口文档内容")
	}
}

// 只读保证:POST 一律 405(无任何数据/打印端点)。
func TestHandleRejectsPost(t *testing.T) {
	w := httptest.NewRecorder()
	handle(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 405 {
		t.Fatalf("POST / 应返回 405,实际 %d", w.Code)
	}
}

// 只读保证:非 / 路径一律 404(不存在 /print 等数据端点)。
func TestHandleNotFoundOtherPath(t *testing.T) {
	for _, p := range []string{"/print", "/api/print", "/api", "/report"} {
		w := httptest.NewRecorder()
		handle(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 404 {
			t.Fatalf("GET %s 应返回 404,实际 %d", p, w.Code)
		}
	}
}

// LANIP 不 panic,返回非空。
func TestLANIP(t *testing.T) {
	if LANIP() == "" {
		t.Fatal("LANIP 不应为空")
	}
}
