// Package config 负责应用数据(设置 + 打印机列表)的读取与保存(JSON)。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"congmingpay/internal/model"
)

// printerIDSeq 是全局递增序号,保证并发/同纳秒生成的打印机 ID 也唯一。
var printerIDSeq uint64

// NewPrinterID 生成全局唯一的打印机 ID:时间戳 + 原子递增序号。
// 【勿再用裸 time.Now().UnixNano()】——Windows 时钟精度粗(~100ns),两次相邻调用会撞出同一值,
// 曾导致数组一次登记多台时两台共用 ID、状态互相绑定的 bug。
func NewPrinterID() string {
	return fmt.Sprintf("p%d-%d", time.Now().UnixNano(), atomic.AddUint64(&printerIDSeq, 1))
}

// Config 是持久化的应用数据。始终以指针使用(勿拷贝,含锁)。
type Config struct {
	Settings model.Settings   `json:"settings"`
	Printers []*model.Printer `json:"printers"`

	mu sync.RWMutex // 保护 Printers:UI(增删)与 API(读)并发
}

// Default 返回默认配置(默认设置,无打印机)。
func Default() *Config {
	return &Config{Settings: model.DefaultSettings()}
}

// DefaultPath 返回配置文件默认路径(与可执行文件同目录的 config.json)。
func DefaultPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

// Load 从 path 读取配置;文件不存在时返回默认配置。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, err
	}
	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save 将配置以 JSON 写入 path。
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// HealPrinterIDs 修复空/重复的打印机 ID(旧配置可能因并发同纳秒撞号),给冲突项重分配唯一 ID。
// 返回是否有改动(调用方据此决定要不要落盘)。启动加载后调用,单线程,不加锁。
func (c *Config) HealPrinterIDs() bool {
	seen := make(map[string]bool, len(c.Printers))
	changed := false
	for _, p := range c.Printers {
		if p.ID == "" || seen[p.ID] {
			p.ID = NewPrinterID()
			changed = true
		}
		seen[p.ID] = true
	}
	return changed
}

// AddPrinter 追加一台打印机(加写锁)。
func (c *Config) AddPrinter(p *model.Printer) {
	c.mu.Lock()
	c.Printers = append(c.Printers, p)
	c.mu.Unlock()
}

// UpsertPrinter 按身份增量插入或更新一台打印机(云端下发同步用,替代手动创建)。
// 身份:网口按 IP 匹配、USB 按 USBName 匹配。命中则只更新展示字段(名称/品牌/规格/端口),
// 保留本地个性化设置(蜂鸣/切刀/重打/首尾行);未命中则补全默认并追加。
// 返回落库后的打印机与是否为新增(加写锁)。
func (c *Config) UpsertPrinter(in *model.Printer) (*model.Printer, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.Printers {
		hit := false
		if in.Conn == model.ConnUSB {
			hit = p.Conn == model.ConnUSB && in.USBName != "" && p.USBName == in.USBName
		} else {
			hit = p.Conn == model.ConnNetwork && in.IP != "" && p.IP == in.IP
		}
		if hit {
			applyPrinterUpdate(p, in)
			return p, false
		}
	}
	if in.ID == "" {
		in.ID = NewPrinterID()
	}
	if in.Brand == "" {
		in.Brand = model.BrandOther
	}
	if in.Width != 58 && in.Width != 80 {
		in.Width = 80
	}
	if in.Conn == model.ConnNetwork && in.Port == "" {
		in.Port = "9100"
	}
	if in.LastPrint == "" {
		in.LastPrint = "云端下发"
	}
	c.Printers = append(c.Printers, in)
	return in, true
}

// PrinterUpdate 是云端下发要覆盖到本地打印机的字段(身份 + 参数)。
type PrinterUpdate struct {
	Name  string      // 空=不改
	Brand model.Brand // 空=不改
	Width int         // 非 58/80=不改
	// 以下参数总是覆盖(打印消息必填):
	BuzzerEnabled bool
	CutDisabled   bool
	HeadLines     int
	TailLines     int
}

// UpdatePrinterFromCloud 用云端下发覆盖并持久化打印机的身份+参数(cloud authoritative)。
// 身份字段有才改;5 个参数总覆盖。逐字段比较,有实际改动才返回 true(供调用方决定是否落盘/刷新)。加写锁。
func (c *Config) UpdatePrinterFromCloud(id string, u PrinterUpdate) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.Printers {
		if p.ID != id {
			continue
		}
		changed := false
		set := func(cond bool) {
			if cond {
				changed = true
			}
		}
		if u.Name != "" && p.Name != u.Name {
			p.Name = u.Name
			set(true)
		}
		if u.Brand != "" && p.Brand != u.Brand {
			p.Brand = u.Brand
			set(true)
		}
		if (u.Width == 58 || u.Width == 80) && p.Width != u.Width {
			p.Width = u.Width
			set(true)
		}
		if p.BuzzerEnabled != u.BuzzerEnabled {
			p.BuzzerEnabled = u.BuzzerEnabled
			set(true)
		}
		if p.CutDisabled != u.CutDisabled {
			p.CutDisabled = u.CutDisabled
			set(true)
		}
		if p.HeadLines != u.HeadLines {
			p.HeadLines = u.HeadLines
			set(true)
		}
		if p.TailLines != u.TailLines {
			p.TailLines = u.TailLines
			set(true)
		}
		return changed
	}
	return false
}

// applyPrinterUpdate 用下发定义更新已注册机的展示字段,保留本地个性化设置。
func applyPrinterUpdate(p, in *model.Printer) {
	if in.Name != "" {
		p.Name = in.Name
	}
	if in.Brand != "" {
		p.Brand = in.Brand
	}
	if in.Width == 58 || in.Width == 80 {
		p.Width = in.Width
	}
	if in.Conn == model.ConnNetwork && in.Port != "" {
		p.Port = in.Port
	}
}

// FindPrinter 按 ID 或名称返回打印机指针,未找到返回 nil(加读锁,供 API 用)。
func (c *Config) FindPrinter(idOrName string) *model.Printer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, p := range c.Printers {
		if p.ID == idOrName || p.Name == idOrName {
			return p
		}
	}
	return nil
}

// PrinterList 返回打印机列表快照(加读锁,供 API 用)。
func (c *Config) PrinterList() []*model.Printer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*model.Printer, len(c.Printers))
	copy(out, c.Printers)
	return out
}

// RemovePrinter 按 ID 移除打印机,返回是否移除成功(加写锁)。
func (c *Config) RemovePrinter(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, p := range c.Printers {
		if p.ID == id {
			c.Printers = append(c.Printers[:i], c.Printers[i+1:]...)
			return true
		}
	}
	return false
}
