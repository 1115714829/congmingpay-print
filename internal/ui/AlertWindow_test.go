package ui

import (
	"testing"
	"time"
)

// TestDurText 锁定「已持续」列的中文时长格式:秒/分/时三档与边界。
func TestDurText(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{-3 * time.Second, "0秒"}, // 时钟回拨兜底
		{0, "0秒"},
		{37 * time.Second, "37秒"},
		{59 * time.Second, "59秒"},
		{60 * time.Second, "1分00秒"},
		{5*time.Minute + 12*time.Second, "5分12秒"},
		{59*time.Minute + 59*time.Second, "59分59秒"},
		{time.Hour, "1时0分"},
		{time.Hour + 3*time.Minute, "1时3分"},
		{25*time.Hour + 42*time.Minute, "25时42分"},
	}
	for _, c := range cases {
		if got := durText(c.d); got != c.want {
			t.Errorf("durText(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
