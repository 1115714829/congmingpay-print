package fingerprint

import "errors"

// ErrInvalid 指纹无效:boardSerial/diskSerials 全空。
var ErrInvalid = errors.New("指纹无效")
