package store

import (
	"strings"
)

// ExistingDeviceNames 批量查询已存在的 DeviceName(库存+已分配+已绑定),返回 名称→true。
func (s *Store) ExistingDeviceNames(names []string) (map[string]bool, error) {
	out := make(map[string]bool, len(names))
	if len(names) == 0 {
		return out, nil
	}
	ph := make([]string, len(names))
	args := make([]interface{}, len(names))
	for i, n := range names {
		ph[i] = "?"
		args[i] = n
	}
	rows, err := s.db.Query(`SELECT device_name FROM devices WHERE device_name IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}

// ImportedDevice 是一条待入库设备(仅入库四列,其余保持 NULL/默认)。
type ImportedDevice struct {
	Name   string
	Secret string
	PK     string
}

// ImportDevices 整批单事务入库(库存态);任何失败回滚,零写入。返回入库条数。
func (s *Store) ImportDevices(recs []ImportedDevice) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := nowUTC()
	stmt, err := tx.Prepare(`INSERT INTO devices(device_name,device_secret,product_key,created_at) VALUES(?,?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, r := range recs {
		if _, err := stmt.Exec(r.Name, r.Secret, r.PK, now); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(recs), nil
}
