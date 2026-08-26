package store

import (
	"database/sql"
	"errors"
	"strings"
)

// Account 是登录账号(密码哈希)。
type Account struct {
	ID           int64
	Username     string
	PasswordHash string
	DisplayName  string
	Role         string // admin | operator
	Enabled      bool
	CreatedAt    string
	UpdatedAt    string
}

// SeedAdmin 首次启动种子管理员(accounts 为空时调用)。返回是否创建。
func (s *Store) SeedAdmin(username, passwordHash string) (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	now := nowUTC()
	_, err := s.db.Exec(`INSERT INTO accounts(username,password_hash,display_name,role,enabled,created_at,updated_at)
		VALUES(?,?,?,?,1,?,?)`, username, passwordHash, "超级管理员", "admin", now, now)
	if err != nil {
		return false, err
	}
	return true, nil
}

// CreateAccount 新建账号;用户名已存在返回 ErrUsernameUsed。
func (s *Store) CreateAccount(username, passwordHash, displayName, role string) (*Account, error) {
	if role != "admin" && role != "operator" {
		role = "operator"
	}
	now := nowUTC()
	res, err := s.db.Exec(`INSERT INTO accounts(username,password_hash,display_name,role,enabled,created_at,updated_at)
		VALUES(?,?,?, ?,1,?,?)`, username, passwordHash, displayName, role, now, now)
	if err != nil {
		if isUniqueError(err) {
			return nil, ErrUsernameUsed
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetAccountByID(id)
}

// GetAccountByID 按 ID 查账号。
func (s *Store) GetAccountByID(id int64) (*Account, error) {
	a := &Account{}
	err := s.db.QueryRow(`SELECT id,username,password_hash,display_name,role,enabled,created_at,updated_at
		FROM accounts WHERE id=?`, id).
		Scan(&a.ID, &a.Username, &a.PasswordHash, &a.DisplayName, &a.Role, &a.Enabled, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// GetAccountByUsername 按用户名查账号(登录用)。
func (s *Store) GetAccountByUsername(username string) (*Account, error) {
	a := &Account{}
	err := s.db.QueryRow(`SELECT id,username,password_hash,display_name,role,enabled,created_at,updated_at
		FROM accounts WHERE username=?`, username).
		Scan(&a.ID, &a.Username, &a.PasswordHash, &a.DisplayName, &a.Role, &a.Enabled, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateAccount 按 ID 更新 displayName/role/enabled(仅非零值字段)。
func (s *Store) UpdateAccount(id int64, displayName *string, role *string, enabled *bool) (*Account, error) {
	if displayName != nil {
		if _, err := s.db.Exec(`UPDATE accounts SET display_name=?,updated_at=? WHERE id=?`, *displayName, nowUTC(), id); err != nil {
			return nil, err
		}
	}
	if role != nil && (*role == "admin" || *role == "operator") {
		if _, err := s.db.Exec(`UPDATE accounts SET role=?,updated_at=? WHERE id=?`, *role, nowUTC(), id); err != nil {
			return nil, err
		}
	}
	if enabled != nil {
		v := 0
		if *enabled {
			v = 1
		}
		if _, err := s.db.Exec(`UPDATE accounts SET enabled=?,updated_at=? WHERE id=?`, v, nowUTC(), id); err != nil {
			return nil, err
		}
	}
	return s.GetAccountByID(id)
}

// SetPassword 重置密码。
func (s *Store) SetPassword(id int64, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE accounts SET password_hash=?,updated_at=? WHERE id=?`, passwordHash, nowUTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAccounts 分页列表(keyword 匹配用户名/显示名)。
func (s *Store) ListAccounts(page, pageSize int, keyword string) (int, []*Account, error) {
	where := ""
	args := []interface{}{}
	if keyword != "" {
		where = ` WHERE username LIKE ? OR display_name LIKE ?`
		k := "%" + keyword + "%"
		args = append(args, k, k)
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts`+where, args...).Scan(&total); err != nil {
		return 0, nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	rows, err := s.db.Query(`SELECT id,username,password_hash,display_name,role,enabled,created_at,updated_at
		FROM accounts`+where+` ORDER BY id ASC LIMIT ? OFFSET ?`, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	out := make([]*Account, 0, pageSize)
	for rows.Next() {
		a := &Account{}
		if err := rows.Scan(&a.ID, &a.Username, &a.PasswordHash, &a.DisplayName, &a.Role, &a.Enabled, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return 0, nil, err
		}
		out = append(out, a)
	}
	return total, out, rows.Err()
}

func isUniqueError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
