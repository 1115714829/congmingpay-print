// Package inventory 负责设备 CSV 入库(强校验、整批原子)。
// 模板三列:DeviceName,DeviceSecret,ProductKey(见 web/CLAUDE.md 第 5 节)。
package inventory

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"congmingpay/web/internal/store"
)

// ImportResult 是导入结果。
type ImportResult struct {
	Imported int
	Total    int
}

// ImportCSV 解析并整批入库。任一校验失败 → 零写入(先校验后单事务 INSERT)。
// err 信息带首个出错行号;重复类错误返回 store 对应哨兵(由 api 层映射 4003/4004)。
func ImportCSV(s *store.Store, r io.Reader) (*ImportResult, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, errors.New("文件读取失败: 不是合法 CSV")
	}
	if len(rows) == 0 {
		return nil, errors.New("模板格式错误: 空文件")
	}
	// 表头(允许 BOM 已由 csv 包处理,首行首列再剥一次 BOM 保险)
	header := rows[0]
	if len(header) != 3 ||
		trimBOM(strings.TrimSpace(header[0])) != "DeviceName" ||
		strings.TrimSpace(header[1]) != "DeviceSecret" ||
		strings.TrimSpace(header[2]) != "ProductKey" {
		return nil, errors.New("模板格式错误: 首行表头必须为 DeviceName,DeviceSecret,ProductKey")
	}
	if len(rows) < 2 {
		return nil, errors.New("模板格式错误: 缺少数据行")
	}

	type rec struct {
		name, secret, pk string
		line             int
	}
	recs := make([]rec, 0, len(rows)-1)
	seen := make(map[string]int) // 文件内排重
	for i, row := range rows[1:] {
		line := i + 2 // 1 基行号,含表头
		if len(row) != 3 {
			return nil, fmt.Errorf("模板格式错误: 第%d行列数不是3", line)
		}
		name := strings.TrimSpace(row[0])
		secret := strings.TrimSpace(row[1])
		pk := strings.TrimSpace(row[2])
		if name == "" || secret == "" || pk == "" {
			return nil, fmt.Errorf("模板格式错误: 第%d行存在空列", line)
		}
		if !nameRe.MatchString(name) || len(name) > 64 {
			return nil, fmt.Errorf("模板格式错误: 第%d行 DeviceName 须为字母数字且长度1-64", line)
		}
		if len(secret) > 128 || len(pk) > 128 {
			return nil, fmt.Errorf("模板格式错误: 第%d行 DeviceSecret/ProductKey 长度须1-128", line)
		}
		if prev, dup := seen[name]; dup {
			return nil, &DupError{Code: 4003, Line: line, Detail: name + "(与第" + itoa(prev) + "行重复)"}
		}
		seen[name] = line
		recs = append(recs, rec{name, secret, pk, line})
	}

	// 与库中已有 DeviceName 排重(库存+已分配+已绑定)
	names := make([]string, 0, len(recs))
	for _, r := range recs {
		names = append(names, r.name)
	}
	existing, err := s.ExistingDeviceNames(names)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		if existing[r.name] {
			return nil, &DupError{Code: 4004, Line: r.line, Detail: r.name + " 与库存重复"}
		}
	}

	// 整批单事务入库(库存态:merchant_id/allocated_at 保持 NULL)
	imports := make([]store.ImportedDevice, 0, len(recs))
	for _, r := range recs {
		imports = append(imports, store.ImportedDevice{Name: r.name, Secret: r.secret, PK: r.pk})
	}
	res, err := s.ImportDevices(imports)
	if err != nil {
		return nil, err
	}
	return &ImportResult{Imported: res, Total: len(recs)}, nil
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func trimBOM(s string) string { return strings.TrimPrefix(s, "\uFEFF") }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
