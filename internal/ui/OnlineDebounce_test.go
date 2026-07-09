package ui

import (
	"strings"
	"testing"
	"time"
)

// TestDebouncerBasic 锁定防抖核心语义:成功即在线、连败满窗才离线、中途成功重置窗口。
func TestDebouncerBasic(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)
	d := newOnlineDebouncer(30 * time.Second)

	// 首次成功立即在线
	if got := d.observe(t0, true, "ping 1ms"); got != 1 {
		t.Fatalf("首次成功应 eff=1, got %d", got)
	}

	// 连败(起点 t0+1s)持续 29 秒内仍在线(容错窗口)
	for i := 1; i <= 30; i++ {
		if got := d.observe(t0.Add(time.Duration(i)*time.Second), false, "请求超时(11010)"); got != 1 {
			t.Fatalf("连败持续 %ds(<30s)应仍在线, got %d", i-1, got)
		}
	}
	// 自首败起持续满 30 秒(t0+31s)判离线,failFirst 保留首因
	if got := d.observe(t0.Add(31*time.Second), false, "目标主机不可达(11003)"); got != 0 {
		t.Fatalf("连败满 30s 应离线, got %d", got)
	}
	if d.failFirst != "请求超时(11010)" {
		t.Fatalf("failFirst 应保留首次失败原因, got %q", d.failFirst)
	}
	if d.failFor(t0.Add(31*time.Second)) != 30*time.Second {
		t.Fatalf("failFor 应为 30s, got %v", d.failFor(t0.Add(31*time.Second)))
	}

	// 离线后一次成功立即恢复
	if got := d.observe(t0.Add(32*time.Second), true, "ping 2ms"); got != 1 {
		t.Fatalf("成功应立即恢复在线, got %d", got)
	}
	if d.failFor(t0.Add(32*time.Second)) != 0 {
		t.Fatal("成功后连败时长应归零")
	}
}

// TestDebouncerResetOnSuccess 中途一次成功后,再连败需重新计满窗口。
func TestDebouncerResetOnSuccess(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)
	d := newOnlineDebouncer(30 * time.Second)
	d.observe(t0, true, "")
	// 连败 10s → 一次成功 → 再连败 29s:全程在线
	for i := 1; i <= 10; i++ {
		d.observe(t0.Add(time.Duration(i)*time.Second), false, "x")
	}
	d.observe(t0.Add(11*time.Second), true, "ping 1ms")
	for i := 12; i <= 41; i++ {
		if got := d.observe(t0.Add(time.Duration(i)*time.Second), false, "x"); got != 1 {
			t.Fatalf("重置后连败 %ds(<30s)应仍在线, got %d", i-12, got)
		}
	}
	// 第 42 秒自新连败起点(12s)持续满 30s → 离线
	if got := d.observe(t0.Add(42*time.Second), false, "x"); got != 0 {
		t.Fatalf("重置后连败满 30s 应离线, got %d", got)
	}
}

// TestDebouncerStartupUnknown 启动即连败:未定(-1)保持到满窗转离线,期间不出现在线。
func TestDebouncerStartupUnknown(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)
	d := newOnlineDebouncer(30 * time.Second)
	for i := 0; i < 30; i++ {
		if got := d.observe(t0.Add(time.Duration(i)*time.Second), false, "x"); got != -1 {
			t.Fatalf("启动连败 %ds 应保持未定(-1), got %d", i, got)
		}
	}
	if got := d.observe(t0.Add(30*time.Second), false, "x"); got != 0 {
		t.Fatalf("启动连败满 30s 应离线, got %d", got)
	}
}

// TestDebouncerZeroWindow window=0(USB):失败即时离线、成功即时在线。
func TestDebouncerZeroWindow(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)
	d := newOnlineDebouncer(0)
	if got := d.observe(t0, false, "查询失败"); got != 0 {
		t.Fatalf("零窗口首败应立即离线, got %d", got)
	}
	if got := d.observe(t0.Add(time.Second), true, "就绪"); got != 1 {
		t.Fatalf("零窗口成功应立即在线, got %d", got)
	}
}

// TestPingStats 锁定统计/抖动日志:阈值、节流、窗口、峰值与均值、无丢包不出统计行。
func TestPingStats(t *testing.T) {
	origEvery, origMin, origGap := pingStatsEvery, jitterLogMin, jitterLogGap
	pingStatsEvery, jitterLogMin, jitterLogGap = 60*time.Second, 5*time.Second, 20*time.Second
	defer func() { pingStatsEvery, jitterLogMin, jitterLogGap = origEvery, origMin, origGap }()

	t0 := time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)
	var ps pingStats

	// 连败 3s 后恢复:低于 5s 抖动阈值,不记抖动
	ps.record(t0, true, 2, "")
	for i := 1; i <= 3; i++ {
		ps.record(t0.Add(time.Duration(i)*time.Second), false, 0, "请求超时(11010)")
	}
	if _, jl := ps.record(t0.Add(4*time.Second), true, 4, ""); jl != "" {
		t.Fatalf("连败 3s 不应记抖动, got %q", jl)
	}

	// 连败 6s 后恢复:记一条抖动,含持续时长与首因
	for i := 5; i <= 10; i++ {
		ps.record(t0.Add(time.Duration(i)*time.Second), false, 0, "请求超时(11010)")
	}
	_, jl := ps.record(t0.Add(11*time.Second), true, 2, "")
	if jl == "" || !strings.Contains(jl, "6s") || !strings.Contains(jl, "请求超时(11010)") {
		t.Fatalf("连败 6s 应记抖动(含时长与首因), got %q", jl)
	}

	// 20s 节流窗口内再次抖动:不重复记
	for i := 12; i <= 18; i++ {
		ps.record(t0.Add(time.Duration(i)*time.Second), false, 0, "x")
	}
	if _, jl2 := ps.record(t0.Add(19*time.Second), true, 2, ""); jl2 != "" {
		t.Fatalf("节流间隔内不应再记抖动, got %q", jl2)
	}

	// 统计窗口期满:有丢包 → 出统计行(丢失数、峰值)
	var statsLine string
	for i := 20; i <= 61; i++ {
		sl, _ := ps.record(t0.Add(time.Duration(i)*time.Second), true, 3, "")
		if sl != "" {
			statsLine = sl
		}
	}
	if statsLine == "" {
		t.Fatal("统计窗口期满且窗口内有丢包,应出统计行")
	}
	if !strings.Contains(statsLine, "连败峰值 7") { // 12..18 共 7 连败为峰值
		t.Fatalf("统计行应含连败峰值 7, got %q", statsLine)
	}

	// 新窗口零丢包:期满不出统计行
	var sl2 string
	for i := 62; i <= 125; i++ {
		sl, _ := ps.record(t0.Add(time.Duration(i)*time.Second), true, 3, "")
		if sl != "" {
			sl2 = sl
		}
	}
	if sl2 != "" {
		t.Fatalf("零丢包窗口不应出统计行, got %q", sl2)
	}
}

// TestPingStatsRealOfflineNoJitter 连败持续 ≥ 离线防抖窗口(已被判离线)的恢复不算「抖动」,
// 不产生自相矛盾的「未判离线」日志(离线/恢复已有状态日志与通知)。
func TestPingStatsRealOfflineNoJitter(t *testing.T) {
	origMin, origGap := jitterLogMin, jitterLogGap
	jitterLogMin, jitterLogGap = 5*time.Second, 20*time.Second
	defer func() { jitterLogMin, jitterLogGap = origMin, origGap }()

	t0 := time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)
	ps := pingStats{offlineWindow: 30 * time.Second}

	// 连败 10s(< 窗口)后恢复:仍记抖动
	ps.record(t0, true, 2, "")
	for i := 1; i <= 10; i++ {
		ps.record(t0.Add(time.Duration(i)*time.Second), false, 0, "请求超时(11010)")
	}
	if _, jl := ps.record(t0.Add(11*time.Second), true, 2, ""); jl == "" {
		t.Fatal("连败 10s(<30s 窗口)应记抖动")
	}

	// 连败 40s(≥ 窗口,已判离线)后恢复:不记抖动
	for i := 40; i <= 79; i++ {
		ps.record(t0.Add(time.Duration(i)*time.Second), false, 0, "请求超时(11010)")
	}
	if _, jl := ps.record(t0.Add(80*time.Second), true, 2, ""); jl != "" {
		t.Fatalf("连败 40s(≥30s 窗口,已判离线)不应记抖动, got %q", jl)
	}
}
