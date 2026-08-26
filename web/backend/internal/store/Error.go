package store

import (
	"errors"
	"fmt"
)

// 仓储层哨兵错误(api 层映射为业务错误码)。
var (
	ErrNotFound     = errors.New("记录不存在")
	ErrUsernameUsed = errors.New("用户名已存在")
	ErrNoLongUsed   = errors.New("长商户号已存在")
	ErrNoShortUsed  = errors.New("短商户号已存在")
	ErrInsufficient = errors.New("库存不足")
	ErrNotOwned     = errors.New("DeviceName不存在或不属于该商户")
	ErrBoundElse    = errors.New("该设备(SN)已绑定其他DeviceName")
	ErrOccupied     = errors.New("设备已被占用(抢注失败)")
	ErrNotBound     = errors.New("设备未绑定")
	ErrStillBound   = errors.New("设备已绑定,不可收回(请先解绑)")
)

// BoundElseError 指纹已绑定其他设备;Name 为已绑定的 DeviceName(提示客户端改绑回去)。
type BoundElseError struct{ Name string }

func (e *BoundElseError) Error() string {
	return fmt.Sprintf("该设备(SN)已绑定其他DeviceName: %s", e.Name)
}
