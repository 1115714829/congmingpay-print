// Package util 提供通用工具函数。
package util

import "golang.org/x/text/encoding/simplifiedchinese"

// ToGB18030 将 UTF-8 字符串编码为 GB18030 字节。
// 热敏小票打印机的中文通常按 GB18030/GBK 处理,而非 UTF-8。
func ToGB18030(s string) ([]byte, error) {
	return simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(s))
}
