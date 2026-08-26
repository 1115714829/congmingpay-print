// Package store 负责 SQLite 数据库的打开、迁移与各表仓储。
// 单连接(SetMaxOpenConns(1))简化并发;时间统一存 UTC RFC3339 字符串。
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store 是数据库句柄。
type Store struct {
	db *sql.DB
}

// Open 打开(不存在则创建)SQLite 并执行建表迁移。path 为 web.db 文件路径。
func Open(path string) (*Store, error) {
	slash := strings.ReplaceAll(filepath.ToSlash(path), "\\", "/")
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", slash)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	return s, s.migrate()
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// DB 暴露底层句柄(供事务使用)。
func (s *Store) DB() *sql.DB { return s.db }

// nowUTC 返回当前 UTC 时间字符串(RFC3339)。
func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// parseUTC 解析库内时间字符串;失败返回零值。
func parseUTC(v string) time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

var schema = `
CREATE TABLE IF NOT EXISTS accounts (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  display_name  TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'operator' CHECK(role IN ('admin','operator')),
  enabled       INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS merchants (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  merchant_no_long  TEXT NOT NULL UNIQUE,
  merchant_no_short TEXT NOT NULL UNIQUE,
  name              TEXT NOT NULL,
  contact_phone     TEXT NOT NULL DEFAULT '',
  address           TEXT NOT NULL DEFAULT '',
  remark            TEXT NOT NULL DEFAULT '',
  created_by        TEXT NOT NULL DEFAULT '',
  created_at        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS devices (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  device_name   TEXT NOT NULL UNIQUE,
  device_secret TEXT NOT NULL DEFAULT '',
  product_key   TEXT NOT NULL DEFAULT '',
  merchant_id   INTEGER,
  allocated_at  TEXT,
  bound_fp_hash TEXT,
  bound_at      TEXT,
  last_seen_at  TEXT,
  os_type       TEXT,
  app_version   TEXT,
  created_at    TEXT NOT NULL,
  FOREIGN KEY(merchant_id) REFERENCES merchants(id)
);
CREATE INDEX IF NOT EXISTS idx_devices_merchant ON devices(merchant_id);
CREATE INDEX IF NOT EXISTS idx_devices_fp ON devices(bound_fp_hash);

CREATE TABLE IF NOT EXISTS fingerprint_bindings (
  fp_hash       TEXT PRIMARY KEY,
  device_name   TEXT NOT NULL,
  raw           TEXT NOT NULL,
  first_seen_at TEXT NOT NULL,
  last_seen_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS login_logs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  username   TEXT NOT NULL,
  ip         TEXT NOT NULL DEFAULT '',
  ua         TEXT NOT NULL DEFAULT '',
  ok         INTEGER NOT NULL,
  reason     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_login_time ON login_logs(created_at);
`

// migrate 建表(幂等)。
func (s *Store) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}
