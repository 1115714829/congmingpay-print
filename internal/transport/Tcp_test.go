package transport

import (
	"strings"
	"testing"
)

// TestOpenRefusedClassified 校验:拨号被拒绝(RST)时,Open 返回的 ConnError 带 Refused=true。
// 这确认 Windows 上 errors.Is(err, syscall.ECONNREFUSED) 能命中 WSAECONNREFUSED。
// 用 127.0.0.1 上一个几乎不可能有人监听的端口,loopback 关闭端口会立即 RST(不会超时)。
func TestOpenRefusedClassified(t *testing.T) {
	p := NewTCPPrinter("127.0.0.1:1")
	err := p.Open()
	if err == nil {
		_ = p.Close()
		t.Skip("127.0.0.1:1 竟然可连,跳过(环境异常)")
	}
	if !IsConnError(err) {
		t.Fatalf("期望 ConnError,得到: %v", err)
	}
	if !IsRefused(err) {
		// 若环境把关闭端口的 RST 过滤成超时/不可达,IsRefused 会为 false;此时跳过而非误判失败。
		if strings.Contains(err.Error(), "超时") || strings.Contains(err.Error(), "timeout") {
			t.Skip("环境未回 RST(超时/被过滤),无法验证 Refused 分类,跳过")
		}
		t.Fatalf("期望 Refused=true(ECONNREFUSED),得到: %v", err)
	}
}

// TestOpenEmptyAddr 空地址应报错(非 ConnError)。
func TestOpenEmptyAddr(t *testing.T) {
	p := NewTCPPrinter("")
	err := p.Open()
	if err == nil {
		t.Fatal("空地址应报错")
	}
	if IsConnError(err) {
		t.Fatalf("空地址不应是 ConnError: %v", err)
	}
}
