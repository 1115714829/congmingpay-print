//go:build windows

package transport

import "testing"

// TestICMPPingLoopback:127.0.0.1 必然可 ping 通 → 在线。验证 IcmpSendEcho 调用与解析正确。
func TestICMPPingLoopback(t *testing.T) {
	st := QueryICMPPing("127.0.0.1")
	if !st.Online {
		t.Fatalf("127.0.0.1 应在线, got %+v", st)
	}
	t.Logf("127.0.0.1 -> %s", st.Detail)
}

// TestICMPPingUnreachable:192.0.2.1(TEST-NET-1,保留不可路由)应 ping 不通 → 离线。
func TestICMPPingUnreachable(t *testing.T) {
	st := QueryICMPPing("192.0.2.1")
	if st.Online {
		t.Fatalf("192.0.2.1 应离线, got %+v", st)
	}
	t.Logf("192.0.2.1 -> %s", st.Detail)
}
