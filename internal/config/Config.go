// Package config 负责应用数据(设置 + 打印机列表)的读取与保存(JSON)。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"congmingpay/internal/model"
)

// printerIDSeq 是全局递增序号,保证并发/同纳秒生成的打印机 ID 也唯一。
var printerIDSeq uint64

// NewPrinterID 生成全局唯一的打印机 ID:时间戳 + 原子递增序号。
// 原子序号保证并发/同纳秒调用也互不重复(Windows 时钟精度约 100ns,裸 UnixNano 不满足唯一性)。
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

// PeekServiceName 只读窥探配置中的服务名(供二次启动提示弹窗取标题,与运行中
// 实例的窗口标题/托盘提示一致)。任何失败(不存在/损坏/为空)一律回退默认服务名;
// 不备份、不改写、零副作用——此刻另一实例正在运行,损坏处理归其正常启动路径。
func PeekServiceName(path string) string {
	def := model.DefaultSettings().ServiceName
	data, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	var c struct {
		Settings struct {
			ServiceName string `json:"serviceName"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return def
	}
	if name := strings.TrimSpace(c.Settings.ServiceName); name != "" {
		return name
	}
	return def
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

// AddPrinter 追加一台打印机(加写锁)。
func (c *Config) AddPrinter(p *model.Printer) {
	c.mu.Lock()
	c.Printers = append(c.Printers, p)
	c.mu.Unlock()
}

// UpsertPrinter 按身份增量插入或更新一台打印机(云端下发同步用,替代手动创建)。
// 身份:网口按 IP 匹配(调用方仅传网口机;USB 机不走云端登记)。命中则只更新展示字段(名称/品牌/规格/端口),
// 保留本地个性化设置(蜂鸣/切刀/重打/首尾行);未命中则补全默认并追加。
// 返回落库后的打印机与是否为新增(加写锁)。
func (c *Config) UpsertPrinter(in *model.Printer) (*model.Printer, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.Printers {
		if p.Conn == model.ConnNetwork && in.IP != "" && p.IP == in.IP {
			applyPrinterUpdate(p, in)
			return p, false
		}
	}
	// 调用方契约:in.Width 须为 58/80(api.resolveTarget 已强校验,双缺即拒,不在此兜底默认)。
	if in.ID == "" {
		in.ID = NewPrinterID()
	}
	if in.Brand == "" {
		in.Brand = model.BrandOther
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

// PrinterSnapshots 返回打印机列表的**值拷贝**快照(读锁内完成深拷贝)。
// model.Printer 全为值类型字段,`*p` 即深拷贝——供跨 goroutine 安全读取(如 MQTT 上报),
// 不共享指针、不与 UI 写(经 UpdatePrinterFields)撕裂。
func (c *Config) PrinterSnapshots() []model.Printer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]model.Printer, len(c.Printers))
	for i, p := range c.Printers {
		out[i] = *p
	}
	return out
}

// UpdatePrinterFields 在写锁内对指定打印机执行 apply(供 UI 属性编辑安全写字段,
// 与 PrinterSnapshots 的读锁、UpdatePrinterFromCloud 的写锁互斥)。返回是否命中。
func (c *Config) UpdatePrinterFields(id string, apply func(*model.Printer)) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.Printers {
		if p.ID == id {
			apply(p)
			return true
		}
	}
	return false
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
