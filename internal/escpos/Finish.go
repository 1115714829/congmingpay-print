package escpos

import "bytes"

// FeedBytes 返回 n 个换行(走纸 n 行);n<=0 返回空。不含初始化。
func FeedBytes(n int) []byte {
	if n <= 0 {
		return nil
	}
	return bytes.Repeat([]byte{0x0A}, n)
}

// Finish 生成打印收尾字节:尾部走纸(TailFeed=按纸宽的基数+偏移)+ 可选半切(GS V 1)。
// 不含初始化;由 printsvc 统一追加到内容之后,保证切纸前走纸足够清过切刀。
func Finish(widthMM, tailOffset int, cut bool) []byte {
	out := FeedBytes(TailFeed(widthMM, tailOffset))
	if cut {
		out = append(out, 0x1D, 0x56, 0x01)
	}
	return out
}
