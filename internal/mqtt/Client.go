// Package mqtt 是云端通道:MQTT 3.1.1 客户端。
//
// 订阅「聪明付短商户号」主题收打印消息(复用 api.ParseRequests/Process),
// 向配置的「上报主题」(Settings.MQTT.ReportTopic)发布打印回执/打印结果/
// 打印机在线离线/APP 上下线状态(LWT)。明文连接、用户名/密码鉴权。
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

	mu          sync.Mutex
	cli         pmqtt.Client
	merchant    string // 当前订阅的短商户号(已净化)
	reportTopic string // 当前上报(发布)主题(配置 Settings.MQTT.ReportTopic)
	enabled     bool   // 当前配置是否启用 MQTT(供 publish 决定未发送时是否落错误日志)
	connected   bool
	lastErr     string
	onChange    func()
	onStatus    func() // 连接状态变化回调(UI 刷新状态标签用)
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

// statusMsg 上下线状态(上行到配置的上报主题)。
// merchant=短商户号:服务端单主题一对多,所有上行靠它分辨来源(下同)。
type statusMsg struct {
	Type     string `json:"type"`     // "status"
	Merchant string `json:"merchant"` // 短商户号(来源身份)
	Event    string `json:"event"`    // "online" / "offline"
	TS       int64  `json:"ts,omitempty"`
}

// ackMsg 打印回执(上行到配置的上报主题)。id 恒回显(不加 omitempty,id=0 也保留字段)。
type ackMsg struct {
	Type     string `json:"type"`     // "ack"
	Merchant string `json:"merchant"` // 短商户号(来源身份)
	ID       uint32 `json:"id"`
	OK       bool   `json:"ok"`
	JobNo    int    `json:"jobNo,omitempty"`
	Message  string `json:"message"`
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
	report := strings.TrimSpace(m.ReportTopic)
	c.merchant = merchant
	c.reportTopic = report
	c.enabled = m.Enabled
	c.lastErr = ""
	// 上报主题与 broker/短商户号同级必要:没有它,回执/结果/状态全部无处可发,不连接。
	if !m.Enabled || strings.TrimSpace(m.Broker) == "" || merchant == "" || report == "" {
		c.mu.Unlock()
		logger.Info("MQTT 未启用或 broker/短商户号/上报主题 为空,不连接")
		return
	}
	// 自订阅回环守卫:上报主题若等于订阅主题(短商户号),ack 会被自己收到→解析失败→再发失败 ack,
	// 形成无限消息风暴。设置页已拦截,此处兜底手改配置文件的情况。
	if report == merchant {
		c.mu.Unlock()
		logger.Errorf("MQTT 上报主题(%s)不能与订阅主题(短商户号)相同——会造成自订阅回环,不连接", report)
		return
	}
	// 发布主题不得含通配符(设置页已挡,此处兜底手改配置):+/# 会致 broker 拒绝或断连循环。
	if strings.ContainsAny(report, "+#") {
		c.mu.Unlock()
		logger.Errorf("MQTT 上报主题(%s)含通配符 +/#,非法发布主题,不连接", report)
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
	will, _ := json.Marshal(statusMsg{Type: "status", Merchant: merchant, Event: "offline"})
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
	opts.SetWill(strings.TrimSpace(m.ReportTopic), string(will), 1, false) // 遗嘱:异常断开时 broker 代发离线(发到配置的上报主题)
	opts.SetOnConnectHandler(func(cli pmqtt.Client) {
		c.setConnected(true)
		// 订阅与上线 publish 放后台 goroutine:t.Wait() 会阻塞 paho 回调线程,别卡在这里。
		go func() {
			if t := cli.Subscribe(merchant, 1, c.onPrint); t.Wait() && t.Error() != nil {
				logger.Errorf("MQTT 订阅 %s 失败: %v", merchant, t.Error())
			} else {
				logger.Infof("MQTT 已连接·订阅 %s", merchant)
			}
			c.publish(cli, statusMsg{Type: "status", Merchant: merchant, Event: "online", TS: time.Now().UnixMilli()})
			c.PublishPrinterList() // 上线基线:随 online 发一次全量打印机列表(含每台配置参数)
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

	_, merchant, _ := c.snapshot()
	reqs, wasArray, err := api.ParseRequests(payload)
	if err != nil {
		logger.Errorf("MQTT 打印消息解析失败: %v", err)
		c.publish(cli, ackMsg{Type: "ack", Merchant: merchant, OK: false, Message: "解析失败: " + err.Error()})
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
			c.publish(cli, ackMsg{Type: "ack", Merchant: merchant, ID: id, OK: false, Message: e.Error()})
			continue
		}
		logger.Infof("MQTT id=%d 已提交 任务#%d(登记/更新=%v)", id, no, reg)
		c.publish(cli, ackMsg{Type: "ack", Merchant: merchant, ID: id, OK: true, JobNo: no, Message: "已提交"})
	}
	if changed {
		c.save()
		c.fireChange()         // 有新增/更新打印机 → 保存并刷新列表 + 起监测
		c.PublishPrinterList() // 列表/参数有变 → 上报最新全量列表
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

// publish 向配置的「上报主题」发布(best-effort:不排队不补发,失败原因落日志)。
// 未连接/主题为空时跳过;若配置为启用状态,跳过与失败都 logger.Errorf 记原因,做到有据可查。
//
// 必须用 IsConnectionOpen(仅真正 connected 才 true)而非只判 cli!=nil:
// paho v1.4.3 在 ConnectRetry 下「连接中」状态 IsConnected() 也为 true,此时 QoS1 Publish
// 只进内部存储、token 永不完成,且 CleanSession 连上后 persist.Reset() 直接丢弃——
// 消息静默丢失、tok.Wait() goroutine 永久泄漏。判 IsConnectionOpen 把这类消息转为「跳过+日志」;
// 判后瞬断的残余窗口由 WaitTimeout 兜底(超时记日志、goroutine 退出,不泄漏)。
func (c *Client) publish(cli pmqtt.Client, v interface{}) {
	c.mu.Lock()
	topic := c.reportTopic
	enabled := c.enabled
	c.mu.Unlock()
	b, _ := json.Marshal(v)
	if cli == nil || topic == "" || !cli.IsConnectionOpen() {
		if enabled {
			logger.Errorf("MQTT 上报未发送(未连接或上报主题为空): %s", b)
		}
		return
	}
	tok := cli.Publish(topic, 1, false, b)
	go func() {
		if !tok.WaitTimeout(30 * time.Second) {
			logger.Errorf("MQTT 上报超时未确认(疑似发布时断线被丢弃) topic=%s: %s", topic, b)
			return
		}
		if err := tok.Error(); err != nil {
			logger.Errorf("MQTT 上报失败 topic=%s: %v(消息: %s)", topic, err, b)
		}
	}()
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
	merchant := c.merchant
	c.cli = nil
	c.connected = false
	c.mu.Unlock()
	if cli != nil {
		c.publish(cli, statusMsg{Type: "status", Merchant: merchant, Event: "offline"})
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
