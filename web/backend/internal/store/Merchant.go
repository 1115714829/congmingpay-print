package store

import (
	"database/sql"
	"errors"
	"strings"
)

// Merchant 是商户(长/短商户号各自全局唯一)。
type Merchant struct {
	ID              int64
	MerchantNoLong  string
	MerchantNoShort string
	Name            string
	ContactPhone    string
	Address         string
	Remark          string
	CreatedBy       string
	CreatedAt       string
	// 列表统计(查询时填充)
	AllocatedCount int
	BoundCount     int
}

// CreateMerchant 新增商户(新增完成即入库);长号/短号冲突分别返回 ErrNoLongUsed/ErrNoShortUsed。
func (s *Store) CreateMerchant(longNo, shortNo, name, phone, addr, remark, createdBy string) (*Merchant, error) {
	now := nowUTC()
	res, err := s.db.Exec(`INSERT INTO merchants(merchant_no_long,merchant_no_short,name,contact_phone,address,remark,created_by,created_at)
		VALUES(?,?,?,?,?,?,?,?)`, longNo, shortNo, name, phone, addr, remark, createdBy, now)
	if err != nil {
		msg := err.Error()
		if containsField(msg, "merchant_no_long") {
			return nil, ErrNoLongUsed
		}
		if containsField(msg, "merchant_no_short") {
			return nil, ErrNoShortUsed
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetMerchantByID(id)
}

// GetMerchantByID 按 ID 查商户。
func (s *Store) GetMerchantByID(id int64) (*Merchant, error) {
	m := &Merchant{}
	err := s.db.QueryRow(`SELECT id,merchant_no_long,merchant_no_short,name,contact_phone,address,remark,created_by,created_at
		FROM merchants WHERE id=?`, id).
		Scan(&m.ID, &m.MerchantNoLong, &m.MerchantNoShort, &m.Name, &m.ContactPhone, &m.Address, &m.Remark, &m.CreatedBy, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// FindMerchantByNo 按商户号查(先 long 精确匹配,未中再 short 精确匹配)。
func (s *Store) FindMerchantByNo(no string) (*Merchant, error) {
	m := &Merchant{}
	err := s.db.QueryRow(`SELECT id,merchant_no_long,merchant_no_short,name,contact_phone,address,remark,created_by,created_at
		FROM merchants WHERE merchant_no_long=? OR merchant_no_short=?`, no, no).
		Scan(&m.ID, &m.MerchantNoLong, &m.MerchantNoShort, &m.Name, &m.ContactPhone, &m.Address, &m.Remark, &m.CreatedBy, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ListMerchants 列表(含名下设备统计)。
func (s *Store) ListMerchants() ([]*Merchant, error) {
	rows, err := s.db.Query(`
		SELECT m.id,m.merchant_no_long,m.merchant_no_short,m.name,m.contact_phone,m.address,m.remark,m.created_by,m.created_at,
			COALESCE(SUM(CASE WHEN d.merchant_id IS NOT NULL THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN d.bound_fp_hash IS NOT NULL THEN 1 ELSE 0 END),0)
		FROM merchants m
		LEFT JOIN devices d ON d.merchant_id = m.id
		GROUP BY m.id ORDER BY m.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Merchant, 0)
	for rows.Next() {
		m := &Merchant{}
		if err := rows.Scan(&m.ID, &m.MerchantNoLong, &m.MerchantNoShort, &m.Name, &m.ContactPhone, &m.Address,
			&m.Remark, &m.CreatedBy, &m.CreatedAt, &m.AllocatedCount, &m.BoundCount); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func containsField(msg, field string) bool {
	return strings.Contains(msg, field)
}

// DeleteMerchant 删除商户:名下有已绑定设备 → ErrStillBound(须先解绑);
// 未绑定(已分配)设备同事务回库存。返回回库存设备数;商户不存在 → ErrNotFound。
func (s *Store) DeleteMerchant(id int64) (int, error) {
	var bound int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE merchant_id = ? AND bound_fp_hash IS NOT NULL`, id).Scan(&bound); err != nil {
		return 0, err
	}
	if bound > 0 {
		return 0, ErrStillBound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE devices SET merchant_id = NULL, allocated_at = NULL WHERE merchant_id = ?`, id)
	if err != nil {
		return 0, err
	}
	released, _ := res.RowsAffected()
	del, err := tx.Exec(`DELETE FROM merchants WHERE id = ?`, id)
	if err != nil {
		return 0, err
	}
	n, _ := del.RowsAffected()
	if n == 0 {
		return 0, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(released), nil
}
