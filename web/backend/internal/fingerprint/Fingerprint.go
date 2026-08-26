// Package fingerprint 负责设备硬件指纹的归一化、摘要与合法性校验。
// 规则见 web/CLAUDE.md 第 4 节:身份=硬件级字段,重装系统不变。
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Fingerprint 是设备端上报的硬件指纹。
// 参与摘要(身份)的字段必须跨重装不变;展示字段不参与摘要。
// 不含网卡 MAC(网卡不稳定、USB 网卡干扰);diskSerials 只含系统盘(避免多盘干扰)。
type Fingerprint struct {
	OsType      string   `json:"osType"`      // "win" | "android"(参与摘要)
	BoardSerial string   `json:"boardSerial"` // 主板序列号(参与摘要)
	CpuID       string   `json:"cpuId"`       // CPU ID(参与摘要)
	DiskSerials []string `json:"diskSerials"` // 系统盘序列号数组(参与摘要,内部排序)
	OsBuild     string   `json:"osBuild"`     // 仅展示
	AppVersion  string   `json:"appVersion"`  // 仅展示
	DeviceModel string   `json:"deviceModel"` // 仅展示
}

// Validate 校验指纹合法性:boardSerial/diskSerials 二者至少一项非空白
// (与 canonical 的 TrimSpace 口径一致,纯空白序列号视为空)。
func (f Fingerprint) Validate() error {
	if strings.TrimSpace(f.BoardSerial) == "" &&
		len(sortedSerials(f.DiskSerials)) == 0 {
		return ErrInvalid
	}
	return nil
}

// Hash 计算身份摘要 SHA256(canonicalJSON):
// 只含 4 个身份字段,字段按名字典序排序,值 TrimSpace,diskSerials 排序后输出。
func (f Fingerprint) Hash() string {
	type item struct {
		K string
		V interface{}
	}
	items := []item{
		{"boardSerial", strings.TrimSpace(f.BoardSerial)},
		{"cpuId", strings.TrimSpace(f.CpuID)},
		{"diskSerials", sortedSerials(f.DiskSerials)},
		{"osType", strings.TrimSpace(f.OsType)},
	}
	sort.Slice(items, func(i, j int) bool { return items[i].K < items[j].K })
	buf := make([]byte, 0, 128)
	buf = append(buf, '{')
	for i, it := range items {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = appendJSONString(buf, it.K)
		buf = append(buf, ':')
		switch v := it.V.(type) {
		case string:
			buf = appendJSONString(buf, v)
		case []string:
			buf = append(buf, '[')
			for j, s := range v {
				if j > 0 {
					buf = append(buf, ',')
				}
				buf = appendJSONString(buf, s)
			}
			buf = append(buf, ']')
		}
	}
	buf = append(buf, '}')
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

// sortedSerials 去空白、过滤空值、字典序排序。
func sortedSerials(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// appendJSONString 以 JSON 字符串形式追加(处理引号与反斜杠转义)。
func appendJSONString(buf []byte, s string) []byte {
	buf = append(buf, '"')
	for _, r := range s {
		switch r {
		case '"':
			buf = append(buf, '\\', '"')
		case '\\':
			buf = append(buf, '\\', '\\')
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
		default:
			buf = appendRune(buf, r)
		}
	}
	return append(buf, '"')
}

func appendRune(buf []byte, r rune) []byte {
	var b [4]byte
	n := appendRuneLen(b[:], r)
	return append(buf, b[:n]...)
}

func appendRuneLen(b []byte, r rune) int {
	switch {
	case r < 0x80:
		b[0] = byte(r)
		return 1
	case r < 0x800:
		b[0] = 0xC0 | byte(r>>6)
		b[1] = 0x80 | byte(r)&0x3F
		return 2
	default:
		b[0] = 0xE0 | byte(r>>12)
		b[1] = 0x80 | byte(r>>6)&0x3F
		b[2] = 0x80 | byte(r)&0x3F
		return 3
	}
}
