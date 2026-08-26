package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// AllocTTL 是商户独占期:分配后 30 天未绑定自动回库存。
const AllocTTL = 30 * 24 * time.Hour

// OnlineWindow 是"在线"判定窗口:lastSeen 距今 < 120s。
const OnlineWindow = 120 * time.Second

// State 枚举(由列推导,不单独存储)。
const (
	StateInventory = "inventory" // 库存:merchant_id IS NULL
	StateAllocated = "allocated" // 已分配未激活:merchant_id IS NOT NULL AND bound_fp_hash IS NULL
	StateBound     = "bound"     // 已绑定:bound_fp_hash IS NOT NULL
)

// Device 是设备档案 + 展示字段。
type Device struct {
	ID           int64
	Name         string // DeviceName(设备SN)
	DeviceSecret string
	ProductKey   string
	MerchantID   sql.NullInt64
	AllocatedAt  sql.NullString
	BoundFPHash  sql.NullString
	BoundAt      sql.NullString
	LastSeenAt   sql.NullString
	OsType       sql.NullString
	AppVersion   sql.NullString
	CreatedAt    string

	// 查询填充
	MerchantNoLong    sql.NullString
	MerchantNoShort   sql.NullString
	MerchantName      sql.NullString
	AllocatedLeftDays int
	Online            bool
}

// State 推导设备状态。
func (d *Device) State() string {
	if d.BoundFPHash.Valid {
		return StateBound
	}
	if d.MerchantID.Valid {
		return StateAllocated
	}
	return StateInventory
}

func scanDevice(row interface{ Scan(...interface{}) error }, d *Device) error {
	return row.Scan(&d.ID, &d.Name, &d.DeviceSecret, &d.ProductKey, &d.MerchantID,
		&d.AllocatedAt, &d.BoundFPHash, &d.BoundAt, &d.LastSeenAt, &d.OsType, &d.AppVersion, &d.CreatedAt,
		&d.MerchantNoLong, &d.MerchantNoShort, &d.MerchantName)
}

const deviceCols = `d.id,d.device_name,d.device_secret,d.product_key,d.merchant_id,
	d.allocated_at,d.bound_fp_hash,d.bound_at,d.last_seen_at,d.os_type,d.app_version,d.created_at,
	m.merchant_no_long,m.merchant_no_short,m.name`

const deviceFrom = `FROM devices d LEFT JOIN merchants m ON m.id = d.merchant_id`

func fillDerived(d *Device) {
	now := time.Now()
	if d.LastSeenAt.Valid {
		t := parseUTC(d.LastSeenAt.String)
		d.Online = !t.IsZero() && now.Sub(t) < OnlineWindow
	}
	if d.State() == StateAllocated && d.AllocatedAt.Valid {
		deadline := parseUTC(d.AllocatedAt.String).Add(AllocTTL)
		d.AllocatedLeftDays = int(deadline.Sub(now).Hours() / 24)
		if d.AllocatedLeftDays < 0 {
			d.AllocatedLeftDays = 0
		}
	}
}

// ListDevices 分页总表。
// keyword:匹配 DeviceName/长/短商户号/商户名;state:inventory|allocated|bound。
func (s *Store) ListDevices(page, pageSize int, keyword, state string, merchantID int64) (int, []*Device, error) {
	where, args := buildDeviceFilter(keyword, state, merchantID)
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) `+deviceFrom+where, args...).Scan(&total); err != nil {
		return 0, nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	rows, err := s.db.Query(`SELECT `+deviceCols+` `+deviceFrom+where+` ORDER BY d.id ASC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	out := make([]*Device, 0, pageSize)
	for rows.Next() {
		d := &Device{}
		if err := scanDevice(rows, d); err != nil {
			return 0, nil, err
		}
		fillDerived(d)
		out = append(out, d)
	}
	return total, out, rows.Err()
}

func buildDeviceFilter(keyword, state string, merchantID int64) (string, []interface{}) {
	conds := make([]string, 0, 3)
	args := make([]interface{}, 0, 5)
	if keyword != "" {
		conds = append(conds, "(d.device_name LIKE ? OR m.merchant_no_long LIKE ? OR m.merchant_no_short LIKE ? OR m.name LIKE ?)")
		k := "%" + keyword + "%"
		args = append(args, k, k, k, k)
	}
	if merchantID > 0 {
		conds = append(conds, "d.merchant_id = ?")
		args = append(args, merchantID)
	}
	switch state {
	case StateInventory:
		conds = append(conds, "d.merchant_id IS NULL")
	case StateAllocated:
		conds = append(conds, "d.merchant_id IS NOT NULL AND d.bound_fp_hash IS NULL")
	case StateBound:
		conds = append(conds, "d.bound_fp_hash IS NOT NULL")
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	return where, args
}

// GetDevice 按 DeviceName 查(附带指纹 raw);不存在返回 ErrNotFound。
func (s *Store) GetDevice(name string) (*Device, string, error) {
	d := &Device{}
	var raw sql.NullString
	err := s.db.QueryRow(`SELECT `+deviceCols+`, (SELECT raw FROM fingerprint_bindings WHERE fp_hash = d.bound_fp_hash)
		`+deviceFrom+` WHERE d.device_name = ?`, name).
		Scan(&d.ID, &d.Name, &d.DeviceSecret, &d.ProductKey, &d.MerchantID,
			&d.AllocatedAt, &d.BoundFPHash, &d.BoundAt, &d.LastSeenAt, &d.OsType, &d.AppVersion, &d.CreatedAt,
			&d.MerchantNoLong, &d.MerchantNoShort, &d.MerchantName, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	fillDerived(d)
	return d, raw.String, nil
}

// ListMerchantDevices 某商户名下设备列表。
func (s *Store) ListMerchantDevices(merchantID int64) ([]*Device, error) {
	rows, err := s.db.Query(`SELECT `+deviceCols+` `+deviceFrom+` WHERE d.merchant_id = ? ORDER BY d.id ASC`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Device, 0)
	for rows.Next() {
		d := &Device{}
		if err := scanDevice(rows, d); err != nil {
			return nil, err
		}
		fillDerived(d)
		out = append(out, d)
	}
	return out, rows.Err()
}

// Allocate 从库存按入库顺序取 count 个分配给商户(独占 30 天);不足返回 ErrInsufficient(一个不分配)。
func (s *Store) Allocate(merchantID int64, count int) ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM devices WHERE merchant_id IS NULL ORDER BY id ASC LIMIT ?`, count)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) < count {
		return nil, ErrInsufficient
	}
	now := nowUTC()
	ph := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+2)
	args = append(args, merchantID, now)
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, id)
	}
	if _, err := tx.Exec(`UPDATE devices SET merchant_id = ?, allocated_at = ? WHERE id IN (`+strings.Join(ph, ",")+`)`, args...); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		var n string
		if err := s.db.QueryRow(`SELECT device_name FROM devices WHERE id = ?`, id).Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// Reclaim 收回"已分配未绑定"设备回库存;已绑定返回 ErrStillBound;不属于商户返回 ErrNotOwned。
func (s *Store) Reclaim(merchantID int64, name string) error {
	res, err := s.db.Exec(`UPDATE devices SET merchant_id = NULL, allocated_at = NULL
		WHERE device_name = ? AND merchant_id = ? AND bound_fp_hash IS NULL`, name, merchantID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		d, _, err := s.GetDevice(name)
		if err != nil {
			return ErrNotOwned
		}
		if d.BoundFPHash.Valid {
			return ErrStillBound
		}
		return ErrNotOwned
	}
	return nil
}

// Unbind 解绑(换硬件/回收):清指纹绑定与商户归属,设备回库存;未绑定返回 ErrNotBound。
func (s *Store) Unbind(name string) error {
	res, err := s.db.Exec(`UPDATE devices SET merchant_id = NULL, allocated_at = NULL,
		bound_fp_hash = NULL, bound_at = NULL WHERE device_name = ? AND bound_fp_hash IS NOT NULL`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotBound
	}
	// 同步清理指纹反向索引
	_, _ = s.db.Exec(`UPDATE fingerprint_bindings SET device_name = '' WHERE device_name = ?`, name)
	return nil
}

// BatchUnbind 批量解绑:逐台执行,单台失败不影响其余。
// 返回成功列表与被跳过明细(原因:未绑定/设备不存在)。
func (s *Store) BatchUnbind(names []string) ([]string, map[string]string) {
	unbound := make([]string, 0, len(names))
	skipped := map[string]string{}
	seen := map[string]bool{}
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		d, _, err := s.GetDevice(n)
		if err == ErrNotFound {
			skipped[n] = "设备不存在"
			continue
		}
		if err != nil || !d.BoundFPHash.Valid {
			skipped[n] = "未绑定"
			continue
		}
		if err := s.Unbind(n); err == nil {
			unbound = append(unbound, n)
		} else {
			skipped[n] = "解绑失败"
		}
	}
	return unbound, skipped
}

// BatchReclaim 批量回收:把所选「已分配未激活」设备逐台收回库存;单台失败不影响其余。
// 返回成功列表与被跳过明细(原因:设备不存在/已绑定/未分配)。
func (s *Store) BatchReclaim(names []string) ([]string, map[string]string) {
	reclaimed := make([]string, 0, len(names))
	skipped := map[string]string{}
	seen := map[string]bool{}
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		res, err := s.db.Exec(`UPDATE devices SET merchant_id = NULL, allocated_at = NULL
			WHERE device_name = ? AND merchant_id IS NOT NULL AND bound_fp_hash IS NULL`, n)
		if err != nil {
			skipped[n] = "回收失败"
			continue
		}
		if aff, _ := res.RowsAffected(); aff > 0 {
			reclaimed = append(reclaimed, n)
			continue
		}
		// 未命中:查明细给出跳过原因
		var owner sql.NullInt64
		var bound sql.NullString
		err = s.db.QueryRow(`SELECT merchant_id, bound_fp_hash FROM devices WHERE device_name = ?`, n).
			Scan(&owner, &bound)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			skipped[n] = "设备不存在"
		case err != nil:
			skipped[n] = "回收失败"
		case bound.Valid:
			skipped[n] = "已绑定"
		default:
			skipped[n] = "未分配"
		}
	}
	return reclaimed, skipped
}

// Bind 设备端绑定(核心,单事务):
//   - 指纹已绑其他设备且请求不同 → ErrBoundElse(附带已绑定设备名);
//   - 设备不存在/不属于该商户/已过期回库存 → ErrNotOwned;
//   - 原子 UPDATE(条件 未绑定 或 本指纹幂等重放),抢注失败 rows=0 → ErrOccupied;
//   - 成功:写入指纹反向索引,刷新 os_type/app_version/last_seen_at,返回设备档案(含密钥)。
func (s *Store) Bind(merchantID int64, name, fpHash, rawJSON, osType, appVersion string) (*Device, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1) 指纹是否已绑定其他设备(device_name='' 为解绑墓碑行,视同未绑定)
	var boundName string
	err = tx.QueryRow(`SELECT device_name FROM fingerprint_bindings WHERE fp_hash = ? AND device_name != ''`, fpHash).Scan(&boundName)
	if err == nil {
		if boundName != name {
			tx.Rollback()
			return nil, &BoundElseError{Name: boundName}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// 2) 设备必须存在且属于该商户(不含已过期回库存的),否则 3002
	var ownerID sql.NullInt64
	err = tx.QueryRow(`SELECT merchant_id FROM devices WHERE device_name = ?`, name).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) || !ownerID.Valid || ownerID.Int64 != merchantID {
		tx.Rollback()
		return nil, ErrNotOwned
	}

	now := nowUTC()
	// 3) 原子绑定:未绑定 或 本指纹的幂等重放;rows=0 即并发抢注失败(3005)
	res, err := tx.Exec(`UPDATE devices SET bound_fp_hash = ?, bound_at = COALESCE(bound_at, ?),
		last_seen_at = ?, os_type = ?, app_version = ?
		WHERE device_name = ? AND merchant_id = ? AND (bound_fp_hash IS NULL OR bound_fp_hash = ?)`,
		fpHash, now, now, nullStr(osType), nullStr(appVersion), name, merchantID, fpHash)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		tx.Rollback()
		return nil, ErrOccupied
	}

	// 3) 指纹反向索引(upsert)
	if _, err := tx.Exec(`INSERT INTO fingerprint_bindings(fp_hash,device_name,raw,first_seen_at,last_seen_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(fp_hash) DO UPDATE SET last_seen_at = excluded.last_seen_at, device_name = excluded.device_name`,
		fpHash, name, rawJSON, now, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	d, _, err := s.GetDevice(name)
	return d, err
}

// nullStr 空串转 NULL(os_type/app_version 允许为空)。
func nullStr(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}

// TouchSeen lookup 侧心跳:指纹已绑定设备时,刷新设备 last_seen 与 os_type/app_version
// (每次上报都覆盖为最新);指纹未绑定过则无事可做。
func (s *Store) TouchSeen(fpHash, osType, appVersion string) error {
	now := nowUTC()
	if _, err := s.db.Exec(`UPDATE fingerprint_bindings SET last_seen_at = ? WHERE fp_hash = ?`, now, fpHash); err != nil {
		return err
	}
	name, err := s.FindBoundDeviceByFP(fpHash)
	if err != nil || name == "" {
		return err
	}
	fields := "last_seen_at = ?"
	args := []interface{}{now}
	if osType != "" {
		fields += ", os_type = ?"
		args = append(args, osType)
	}
	if appVersion != "" {
		fields += ", app_version = ?"
		args = append(args, appVersion)
	}
	args = append(args, name)
	_, err = s.db.Exec(`UPDATE devices SET `+fields+` WHERE device_name = ?`, args...)
	return err
}

// FindBoundDeviceByFP 指纹已绑定的设备名(lookup 重装恢复场景);未绑定返回空串。
func (s *Store) FindBoundDeviceByFP(fpHash string) (string, error) {
	var n string
	err := s.db.QueryRow(`SELECT device_name FROM fingerprint_bindings WHERE fp_hash = ? AND device_name != ''`, fpHash).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return n, nil
}

// ExpireAllocations 到期回收:超过 30 天未绑定的分配回库存。返回回收条数。
func (s *Store) ExpireAllocations() (int, error) {
	cutoff := time.Now().UTC().Add(-AllocTTL).Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE devices SET merchant_id = NULL, allocated_at = NULL
		WHERE merchant_id IS NOT NULL AND bound_fp_hash IS NULL AND allocated_at IS NOT NULL AND allocated_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Counts 总览统计:商户数/库存/已分配未激活/已绑定。
func (s *Store) Counts() (merchants, inventory, allocated, bound int, err error) {
	err = s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM merchants),
		(SELECT COUNT(*) FROM devices WHERE merchant_id IS NULL),
		(SELECT COUNT(*) FROM devices WHERE merchant_id IS NOT NULL AND bound_fp_hash IS NULL),
		(SELECT COUNT(*) FROM devices WHERE bound_fp_hash IS NOT NULL)`).
		Scan(&merchants, &inventory, &allocated, &bound)
	return merchants, inventory, allocated, bound, err
}

// LoginStats 近 days 天(含今天)登录成功/失败按日统计(成功数,失败数)。
func (s *Store) LoginStats(days int) (map[string][2]int, error) {
	start := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02T00:00:00Z")
	rows, err := s.db.Query(`SELECT substr(created_at,1,10), ok, COUNT(*) FROM login_logs WHERE created_at >= ? GROUP BY 1,2`, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][2]int)
	for rows.Next() {
		var date string
		var ok, n int
		if err := rows.Scan(&date, &ok, &n); err != nil {
			return nil, err
		}
		v := out[date]
		if ok == 1 {
			v[0] += n
		} else {
			v[1] += n
		}
		out[date] = v
	}
	return out, rows.Err()
}

// WriteLoginLog 写登录日志(成功/失败)。
func (s *Store) WriteLoginLog(username, ip, ua string, ok bool, reason string) {
	v := 0
	if ok {
		v = 1
	}
	_, _ = s.db.Exec(`INSERT INTO login_logs(username,ip,ua,ok,reason,created_at) VALUES(?,?,?,?,?,?)`,
		username, ip, ua, v, reason, nowUTC())
}
