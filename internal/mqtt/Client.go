// Package mqtt 是云端通道:MQTT 3.1.1 客户端。
//
// 订阅「聪明付短商户号」主题收打印消息(复用 api.ParseRequests/Process),
// 向 <短商户号>/report 发布打印回执与上下线状态(LWT)。明文连接、用户名/密码鉴权。
package mqtt

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	pmqtt "github.com/eclipse/paho.mqtt.golang"

	"congmingpay/internal/api"
	"congmingpay/internal/config"
	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// Client 是云端 MQTT 客户端(唯一云端通道)。
type Client struct {
	cfg     *config.Config
	svc     *printsvc.Service
	cfgPath string

	id string // 本进程一次性随机后缀,拼进 ClientID 防同 ID 顶号

	mu        sync.Mutex
	cli       pmqtt.Client
	merchant  string // 当前订阅的短商户号(已净化)
	connected bool
	lastErr   string
	onChange  func()
	onStatus  func() // 连接状态变化回调(UI 刷新状态标签用)
}

// New 创建客户端(未连接;由 Start 按配置连接)。
func New(cfg *config.Config, svc *printsvc.Service, cfgPath string) *Client {
	return &Client{cfg: cfg, svc: svc, cfgPath: cfgPath, id: randSuffix()}
}

// randSuffix 返回一次性 4 字节 hex(进程内稳定),用于让 ClientID 跨进程/跨机唯一。
func randSuffix() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "0"
	}
	return hex.EncodeToString(b)
}

// SetOnChange 设置打印机列表变化回调(UI 刷新用)。
func (c *Client) SetOnChange(f func()) {
	c.mu.Lock()
	c.onChange = f
	c.mu.Unlock()
}

// SetOnStatus 设置连接状态变化回调(连上/断开/首连失败时触发,UI 刷新状态标签用)。
func (c *Client) SetOnStatus(f func()) {
	c.mu.Lock()
	c.onStatus = f
	c.mu.Unlock()
}

// statusMsg 上下线状态(上行到 <短商户号>/report)。
type statusMsg struct {
	Type  string `json:"type"`  // "status"
	Event string `json:"event"` // "online" / "offline"
	TS    int64  `json:"ts,omitempty"`
}

// ackMsg 打印回执(上行到 <短商户号>/report)。
type ackMsg struct {
	Type    string `json:"type"` // "ack"
	ID      uint32 `json:"id,omitempty"`
	OK      bool   `json:"ok"`
	JobNo   int    `json:"jobNo,omitempty"`
	Message string `json:"message"`
}

// SanitizeMerchant 净化短商户号:去空格/Tab/换行等,仅保留字母数字与下划线。
func SanitizeMerchant(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// brokerURL 归一 broker 地址:补 tcp:// 与端口。
func brokerURL(broker string, port int) string {
	b := strings.TrimSpace(broker)
	if port <= 0 {
		port = 1883
	}
	if !strings.Contains(b, "://") {
		b = "tcp://" + b
	}
	rest := b[strings.Index(b, "://")+3:]
	if !strings.Contains(rest, ":") {
		b = b + ":" + strconv.Itoa(port)
	}
	return b
}

// Start 按当前配置(重)连接。
func (c *Client) Start() { c.reconnect(c.cfg.Settings.MQTT) }

// Reload 配置变更后热切(先断后连)。
func (c *Client) Reload(m model.MQTT) { c.reconnect(m) }

// reconnect 停旧连接、按 m 重新连接(未启用/参数不全则只停不连)。
func (c *Client) reconnect(m model.MQTT) {
	c.mu.Lock()
	if c.cli != nil {
		old := c.cli
		c.cli = nil
		c.connected = false
		go old.Disconnect(200)
	}
	merchant := SanitizeMerchant(m.Topic)
	c.merchant = merchant
	c.lastErr = ""
	if !m.Enabled || strings.TrimSpace(m.Broker) == "" || merchant == "" {
		c.mu.Unlock()
		logger.Info("MQTT 未启用或 broker/短商户号 为空,不连接")
		return
	}
	cli := pmqtt.NewClient(c.buildOpts(m, merchant))
	c.cli = cli
	c.mu.Unlock()

	logger.Infof("MQTT 连接 %s(短商户号 %s,ClientID cmp-%s-%s)…", brokerURL(m.Broker, m.Port), merchant, merchant, c.id)
	tok := cli.Connect()
	go func() {
		tok.Wait()
		if err := tok.Error(); err != nil {
			c.setErr(err.Error())
			logger.Errorf("MQTT 首次连接失败(将后台重试): %v", err)
		}
	}()
}

func (c *Client) buildOpts(m model.MQTT, merchant string) *pmqtt.ClientOptions {
	will, _ := json.Marshal(statusMsg{Type: "status", Event: "offline"})
	opts := pmqtt.NewClientOptions()
	opts.AddBroker(brokerURL(m.Broker, m.Port))
	opts.SetClientID("cmp-" + merchant + "-" + c.id) // 加随机后缀,避免同商户号多客户端互相顶号(顶号会表现为"用一会就不收")
	if m.Username != "" {
		opts.SetUsername(m.Username)
	}
	if m.Password != "" {
		opts.SetPassword(m.Password)
	}
	opts.SetKeepAlive(60 * time.Second)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true) // 已连后断开自动重连
	opts.SetConnectRetry(true)  // 首次连不上也后台重试
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetWill(merchant+"/report", string(will), 1, false) // 遗嘱:异常断开时 broker 代发离线
	opts.SetOnConnectHandler(func(cli pmqtt.Client) {
		c.setConnected(true)
		// 订阅与上线 publish 放后台 goroutine:t.Wait() 会阻塞 paho 回调线程,别卡在这里。
		go func() {
			if t := cli.Subscribe(merchant, 1, c.onPrint); t.Wait() && t.Error() != nil {
				logger.Errorf("MQTT 订阅 %s 失败: %v", merchant, t.Error())
			} else {
				logger.Infof("MQTT 已连接·订阅 %s", merchant)
			}
			c.publish(cli, statusMsg{Type: "status", Event: "online", TS: time.Now().UnixMilli()})
		}()
	})
	opts.SetConnectionLostHandler(func(_ pmqtt.Client, err error) {
		c.setConnected(false)
		c.setErr(err.Error())
		logger.Errorf("MQTT 连接断开(自动重连中): %v", err)
	})
	return opts
}

// onPrint 收到打印消息:复用 api.ParseRequests/Process,逐条处理并回执。
// 【每条都打】不做 id 去重——收到消息即打印/登记(与 UI「JSON测试」一致);id 仅用于回执/日志对应。
func (c *Client) onPrint(cli pmqtt.Client, msg pmqtt.Message) {
	payload := msg.Payload()
	logger.Infof("MQTT 收到消息 topic=%s 字节=%d", msg.Topic(), len(payload))

	reqs, wasArray, err := api.ParseRequests(payload)
	if err != nil {
		logger.Errorf("MQTT 打印消息解析失败: %v", err)
		c.publish(cli, ackMsg{Type: "ack", OK: false, Message: "解析失败: " + err.Error()})
		return
	}
	logger.Infof("MQTT 解析出 %d 条打印请求(数组=%v)", len(reqs), wasArray)

	changed := false
	for i := range reqs {
		id := reqs[i].ID
		logger.Infof("MQTT 处理 id=%d 目标=%s type=%d", id, reqTarget(&reqs[i]), reqs[i].Type)
		no, reg, e := api.Process(c.cfg, c.svc, &reqs[i])
		if reg {
			changed = true
		}
		if e != nil {
			logger.Errorf("MQTT 打印失败 id=%d: %v", id, e)
			c.publish(cli, ackMsg{Type: "ack", ID: id, OK: false, Message: e.Error()})
			continue
		}
		logger.Infof("MQTT id=%d 已提交 任务#%d(登记/更新=%v)", id, no, reg)
		c.publish(cli, ackMsg{Type: "ack", ID: id, OK: true, JobNo: no, Message: "已提交"})
	}
	if changed {
		c.save()
		c.fireChange() // 有新增/更新打印机 → 保存并刷新列表 + 起监测
	}
}

// reqTarget 返回打印请求的目标描述(供日志),printer 名/IP 或 gateway。
func reqTarget(r *api.PrintRequest) string {
	if s := strings.TrimSpace(r.Gateway); s != "" {
		return "gateway=" + s
	}
	if r.Printer.IP != "" {
		return "printer=" + r.Printer.Name + "@" + r.Printer.IP
	}
	if r.Printer.Name != "" {
		return "printer=" + r.Printer.Name
	}
	return "(选中机/未指定)"
}

// publish 向 <短商户号>/report 发布(best-effort,未连则跳过)。
func (c *Client) publish(cli pmqtt.Client, v interface{}) {
	c.mu.Lock()
	merchant := c.merchant
	c.mu.Unlock()
	if cli == nil || merchant == "" {
		return
	}
	b, _ := json.Marshal(v)
	cli.Publish(merchant+"/report", 1, false, b)
}

// TestConnect 一次性连接+断开,供设置页「连接测试」。返回耗时与错误。
func TestConnect(m model.MQTT) (time.Duration, error) {
	merchant := SanitizeMerchant(m.Topic)
	if strings.TrimSpace(m.Broker) == "" {
		return 0, fmt.Errorf("Broker 为空")
	}
	if merchant == "" {
		return 0, fmt.Errorf("短商户号为空")
	}
	opts := pmqtt.NewClientOptions()
	opts.AddBroker(brokerURL(m.Broker, m.Port))
	opts.SetClientID("cmp-" + merchant + "-test")
	if m.Username != "" {
		opts.SetUsername(m.Username)
	}
	if m.Password != "" {
		opts.SetPassword(m.Password)
	}
	opts.SetConnectTimeout(8 * time.Second)
	cli := pmqtt.NewClient(opts)
	start := time.Now()
	tok := cli.Connect()
	if !tok.WaitTimeout(9 * time.Second) {
		return time.Since(start), fmt.Errorf("连接超时")
	}
	if err := tok.Error(); err != nil {
		return time.Since(start), err
	}
	d := time.Since(start)
	cli.Disconnect(100)
	return d, nil
}

// Status 返回连接状态与描述(供 UI)。
func (c *Client) Status() (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.cli == nil:
		return false, "未启用"
	case c.connected:
		return true, "已连接 · 订阅 " + c.merchant
	case c.lastErr != "":
		return false, "未连接 · " + c.lastErr
	default:
		return false, "连接中…"
	}
}

// Close 优雅关闭:主动发离线并断开。
func (c *Client) Close() {
	c.mu.Lock()
	cli := c.cli
	c.cli = nil
	c.connected = false
	c.mu.Unlock()
	if cli != nil {
		c.publish(cli, statusMsg{Type: "status", Event: "offline"})
		cli.Disconnect(250)
	}
}

func (c *Client) setConnected(v bool) {
	c.mu.Lock()
	c.connected = v
	if v {
		c.lastErr = ""
	}
	c.mu.Unlock()
	c.fireStatus()
}

func (c *Client) setErr(s string) {
	c.mu.Lock()
	c.lastErr = s
	c.mu.Unlock()
	c.fireStatus()
}

func (c *Client) save() {
	if err := c.cfg.Save(c.cfgPath); err != nil {
		logger.Errorf("保存配置失败: %v", err)
	}
}

func (c *Client) fireChange() {
	c.mu.Lock()
	f := c.onChange
	c.mu.Unlock()
	if f != nil {
		f()
	}
}

// fireStatus 通知 UI 连接状态已变(锁外调用,避免与回调里的读锁互锁)。
func (c *Client) fireStatus() {
	c.mu.Lock()
	f := c.onStatus
	c.mu.Unlock()
	if f != nil {
		f()
	}
}
