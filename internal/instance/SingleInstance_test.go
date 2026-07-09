//go:build windows

package instance

import (
	"fmt"
	"os"
	"testing"
)

// TestAcquireRelease 锁定单实例语义:首次可得、重复报「已存在」、释放后可再得。
// 用带 PID 的测试专用互斥体名,避免与正在运行的应用或并行测试相撞。
func TestAcquireRelease(t *testing.T) {
	name := fmt.Sprintf(`Global\congmingpay-instance-test-%d`, os.Getpid())

	l1, already, err := acquire(name)
	if err != nil {
		t.Fatalf("首次 acquire 失败: %v", err)
	}
	if already {
		t.Fatal("首次 acquire 不应报已存在")
	}
	if l1 == nil {
		t.Fatal("首次 acquire 应返回 Lock")
	}

	l2, already2, err2 := acquire(name)
	if err2 != nil {
		t.Fatalf("重复 acquire 报错: %v", err2)
	}
	if !already2 {
		t.Fatal("重复 acquire 应报已存在")
	}
	if l2 != nil {
		t.Fatal("重复 acquire 不应返回 Lock")
	}

	l1.Release()

	l3, already3, err3 := acquire(name)
	if err3 != nil {
		t.Fatalf("释放后再 acquire 失败: %v", err3)
	}
	if already3 {
		t.Fatal("释放后再 acquire 不应报已存在")
	}
	l3.Release()

	// nil 安全
	var nilLock *Lock
	nilLock.Release()
}
