package mqtt

import (
	"testing"

	"congmingpay/internal/config"
	"congmingpay/internal/printsvc"
)

// 云盒兼容开(默认):waiting 在 PublishJobEvent 入口即丢弃。
func TestLegacyCompatC4PublishJobEventSkipsWaiting(t *testing.T) {
	cfg := config.Default() // YunheCompat()=true
	c := &Client{cfg: cfg, enabled: true, merchant: "t"}
	c.PublishJobEvent(printsvc.JobEvent{Event: printsvc.EventWaiting, JobNo: 1, Err: "x"})
	c.PublishJobEvent(printsvc.JobEvent{Event: printsvc.EventDone, JobNo: 2})
	c.PublishJobEvent(printsvc.JobEvent{Event: printsvc.EventFailed, JobNo: 3, Err: "fail"})
}

// 兼容关:waiting 进入 publish 路径(cli 为 nil 时安全跳过,不 panic)。
func TestStrictPublishJobEventAllowsWaiting(t *testing.T) {
	cfg := config.Default()
	cfg.Settings.YunheCompatDisabled = true
	c := &Client{cfg: cfg, enabled: true, merchant: "t"}
	c.PublishJobEvent(printsvc.JobEvent{Event: printsvc.EventWaiting, JobNo: 1, Err: "长期等待"})
	c.PublishJobEvent(printsvc.JobEvent{Event: printsvc.EventDone, JobNo: 2})
}
