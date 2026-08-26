package inventory

import "fmt"

// DupError 是带行号的重复错误;Code 对应业务码(4003 文件内 / 4004 与库存)。
type DupError struct {
	Code   int
	Line   int
	Detail string
}

func (e *DupError) Error() string {
	return fmt.Sprintf("第%d行 %s", e.Line, e.Detail)
}
