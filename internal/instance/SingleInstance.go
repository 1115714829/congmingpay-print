//go:build windows

// Package instance 提供进程单实例守卫(Windows 命名互斥体)。
package instance

import (
	"golang.org/x/sys/windows"
)

// mutexName 是单实例命名互斥体名。用 Global 命名空间:跨用户会话唯一——
// 打印机、监听端口、MQTT ClientID 都是机器级资源,不允许另一会话再起一份。
// (普通用户可建 Global 互斥体,SeCreateGlobalPrivilege 只限制 section/符号链接。)
const mutexName = `Global\congmingpay-print-server`

// Lock 持有单实例互斥体;进程存活期间保持,退出时 OS 自动回收句柄。
type Lock struct{ h windows.Handle }

// Acquire 尝试成为唯一实例。already=true 表示已有实例在运行(调用方提示后退出);
// err 仅在互斥体创建本身失败时非 nil(不含「已存在」),调用方按守卫失效继续启动。
func Acquire() (*Lock, bool, error) {
	return acquire(mutexName)
}

// acquire 按名创建命名互斥体(仅用存在性判重,不取所有权,无 abandoned 状态问题)。
// x/sys 的 CreateMutex 在 ERROR_ALREADY_EXISTS 时返回「有效句柄 + 该错误」,
// 此路径关掉句柄不留引用。
func acquire(name string) (*Lock, bool, error) {
	name16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, false, err
	}
	h, err := windows.CreateMutex(nil, false, name16)
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		return nil, true, nil
	}
	// 已存在但属其他用户会话:nil 安全属性的互斥体默认 DACL 只授创建者与 SYSTEM,
	// 对已存在互斥体 CreateMutexW 请求 MUTEX_ALL_ACCESS 会得 ERROR_ACCESS_DENIED
	// (句柄为 0,拿不到 ALREADY_EXISTS)。命名互斥体名字被占即已有实例,同样判
	// 「已在运行」——否则快速用户切换后二次启动会被当「守卫失效」放行第二实例。
	if err == windows.ERROR_ACCESS_DENIED {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &Lock{h: h}, false, nil
}

// Release 释放互斥体(正常退出收尾;nil 安全,不调用也无妨——进程退出即回收)。
func (l *Lock) Release() {
	if l == nil || l.h == 0 {
		return
	}
	_ = windows.CloseHandle(l.h)
	l.h = 0
}
