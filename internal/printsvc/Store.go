package printsvc

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"

	_ "modernc.org/sqlite"
)

const (
	metaNextNo = "next_no"
)

// DefaultJobsPath 返回 jobs.db 默认路径(与 exe 同目录)。
func DefaultJobsPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "jobs.db"
	}
	return filepath.Join(filepath.Dir(exe), "jobs.db")
}

// Store 是打印任务 SQLite 持久化。
type Store struct {
	db *sql.DB
}

// OpenStore 打开或创建 jobs.db 并迁移表结构。
func OpenStore(path string) (*Store, error) {
	// busy_timeout 毫秒;modernc 用 _pragma 查询参数。
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite 写串行即可
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS jobs (
  no INTEGER PRIMARY KEY,
  doc TEXT NOT NULL,
  status TEXT NOT NULL,
  time_label TEXT,
  err TEXT,
  created_at TEXT NOT NULL,
  printer_json TEXT NOT NULL,
  data_blob BLOB NOT NULL,
  cut INTEGER NOT NULL DEFAULT 0,
  buzzer INTEGER NOT NULL DEFAULT 0,
  head_lines INTEGER NOT NULL DEFAULT 0,
  tail_lines INTEGER NOT NULL DEFAULT 0,
  reprint_next INTEGER NOT NULL DEFAULT 0,
  cloud_id INTEGER,
  content_type INTEGER NOT NULL DEFAULT -1,
  source_json BLOB
);
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`)
	if err != nil {
		return err
	}
	// 旧库补列(CREATE IF NOT EXISTS 不会改已有表)
	if err := s.ensureColumn("jobs", "content_type", "INTEGER NOT NULL DEFAULT -1"); err != nil {
		return err
	}
	return s.ensureColumn("jobs", "source_json", "BLOB")
}

func (s *Store) ensureColumn(table, col, decl string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == col {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` ` + decl)
	return err
}

// persistedJob 是落库中间结构(与 entry 互转)。
type persistedJob struct {
	No          int
	Doc         string
	Status      model.JobStatus
	TimeLabel   string
	Err         string
	CreatedAt   time.Time
	Printer     model.Printer
	Data        []byte
	Cut         bool
	Buzzer      bool
	HeadLines   int
	TailLines   int
	ReprintNext bool
	CloudID     *uint32
	ContentType int
	SourceJSON  []byte
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// UpsertJob 插入或更新一行任务。
func (s *Store) UpsertJob(j persistedJob) error {
	if s == nil || s.db == nil {
		return nil
	}
	pj, err := json.Marshal(j.Printer)
	if err != nil {
		return err
	}
	var cloud sql.NullInt64
	if j.CloudID != nil {
		cloud = sql.NullInt64{Int64: int64(*j.CloudID), Valid: true}
	}
	_, err = s.db.Exec(`
INSERT INTO jobs(no,doc,status,time_label,err,created_at,printer_json,data_blob,
  cut,buzzer,head_lines,tail_lines,reprint_next,cloud_id,content_type,source_json)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(no) DO UPDATE SET
  doc=excluded.doc, status=excluded.status, time_label=excluded.time_label, err=excluded.err,
  created_at=excluded.created_at, printer_json=excluded.printer_json, data_blob=excluded.data_blob,
  cut=excluded.cut, buzzer=excluded.buzzer, head_lines=excluded.head_lines, tail_lines=excluded.tail_lines,
  reprint_next=excluded.reprint_next, cloud_id=excluded.cloud_id,
  content_type=excluded.content_type, source_json=excluded.source_json
`, j.No, j.Doc, string(j.Status), j.TimeLabel, j.Err, j.CreatedAt.Format(time.RFC3339),
		string(pj), j.Data, boolInt(j.Cut), boolInt(j.Buzzer), j.HeadLines, j.TailLines,
		boolInt(j.ReprintNext), cloud, j.ContentType, j.SourceJSON)
	return err
}

// DeleteJob 按任务号删除。
func (s *Store) DeleteJob(no int) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM jobs WHERE no=?`, no)
	return err
}

// DeleteDone 删除全部已完成任务。
func (s *Store) DeleteDone() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM jobs WHERE status=?`, string(model.JobDone))
	return err
}

// PruneTerminal 删除超龄的 done/failed;days 须已钳制。返回删除行数。
func (s *Store) PruneTerminal(days int, now time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	cutoff := now.AddDate(0, 0, -days).Format(time.RFC3339)
	res, err := s.db.Exec(`
DELETE FROM jobs WHERE status IN (?,?) AND created_at < ?
`, string(model.JobDone), string(model.JobFailed), cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// LoadAll 按 no 降序加载全部任务(新任务在前)。
func (s *Store) LoadAll() ([]persistedJob, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`
SELECT no,doc,status,time_label,err,created_at,printer_json,data_blob,
  cut,buzzer,head_lines,tail_lines,reprint_next,cloud_id,content_type,source_json
FROM jobs ORDER BY no DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []persistedJob
	for rows.Next() {
		var j persistedJob
		var status, created, pj string
		var cut, buzzer, reprint int
		var cloud sql.NullInt64
		var srcBlob []byte
		if err := rows.Scan(&j.No, &j.Doc, &status, &j.TimeLabel, &j.Err, &created, &pj, &j.Data,
			&cut, &buzzer, &j.HeadLines, &j.TailLines, &reprint, &cloud, &j.ContentType, &srcBlob); err != nil {
			return nil, err
		}
		j.Status = model.JobStatus(status)
		j.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if j.CreatedAt.IsZero() {
			j.CreatedAt = time.Now()
		}
		_ = json.Unmarshal([]byte(pj), &j.Printer)
		j.Cut, j.Buzzer, j.ReprintNext = cut != 0, buzzer != 0, reprint != 0
		if cloud.Valid {
			id := uint32(cloud.Int64)
			j.CloudID = &id
		}
		j.SourceJSON = srcBlob
		out = append(out, j)
	}
	return out, rows.Err()
}

// GetNextNo 读取 meta next_no;缺省返回 1000。
func (s *Store) GetNextNo() (int, error) {
	if s == nil || s.db == nil {
		return 1000, nil
	}
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, metaNextNo).Scan(&v)
	if err == sql.ErrNoRows {
		return 1000, nil
	}
	if err != nil {
		return 1000, err
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1000 {
		return 1000, nil
	}
	return n, nil
}

// SetNextNo 写入 next_no。
func (s *Store) SetNextNo(n int) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`
INSERT INTO meta(key,value) VALUES(?,?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value
`, metaNextNo, strconv.Itoa(n))
	return err
}

// OpenStoreOrRecover 打开库;损坏时备份并重建空库。
func OpenStoreOrRecover(path string) *Store {
	s, err := OpenStore(path)
	if err == nil {
		return s
	}
	logger.Errorf("打开 jobs.db 失败(%s): %v", path, err)
	bad := path + ".bad-" + time.Now().Format("20060102-150405")
	if renameErr := os.Rename(path, bad); renameErr == nil {
		logger.Errorf("损坏 jobs.db 已备份到 %s,重建空库", bad)
	} else {
		logger.Errorf("备份损坏 jobs.db 失败: %v;尝试删除后重建", renameErr)
		_ = os.Remove(path)
	}
	s, err = OpenStore(path)
	if err != nil {
		logger.Errorf("重建 jobs.db 仍失败: %v;本会话不落盘", err)
		return nil
	}
	return s
}

// entryToPersisted 从 entry 快照(调用方须已持锁或保证不并发改)。
func entryToPersisted(e *entry) persistedJob {
	j := persistedJob{
		No: e.job.No, Doc: e.job.Doc, Status: e.job.Status,
		TimeLabel: e.job.Time, Err: e.job.Err, CreatedAt: e.createdAt,
		Printer: e.printer, Data: e.data,
		Cut: e.cut, Buzzer: e.buzzer, HeadLines: e.headLines, TailLines: e.tailLines,
		ReprintNext: e.reprintNext, CloudID: e.cloudID,
		ContentType: e.contentType, SourceJSON: e.sourceJSON,
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now()
	}
	return j
}

func persistErr(op string, err error) {
	if err != nil {
		logger.Errorf("jobs.db %s: %v", op, err)
	}
}
