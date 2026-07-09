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

// TestIPStatusText 锁定 IP_STATUS 译码表:已知码给中文+数字,未知码显数字,0 为未知错误。
func TestIPStatusText(t *testing.T) {
	cases := []struct {
		code uint32
		want string
	}{
		{11002, "目标网络不可达(11002)"},
		{11003, "目标主机不可达(11003)"},
		{11005, "目标端口不可达(11005)"},
		{11010, "请求超时(11010)"},
		{11013, "传输中 TTL 耗尽(11013)"},
		{11018, "目标地址无效(11018)"},
		{0, "未知错误"},
		{5, "错误码 5"},
		{11050, "错误码 11050"},
	}
	for _, c := range cases {
		if got := ipStatusText(c.code); got != c.want {
			t.Errorf("ipStatusText(%d) = %q, want %q", c.code, got, c.want)
		}
	}
}
