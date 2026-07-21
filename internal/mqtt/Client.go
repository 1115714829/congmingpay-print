// Package mqtt 是云端通道:MQTT 3.1.1 客户端。
//
// 支持自建(用户名密码)与阿里云(签名鉴权 + 主题模板)二选一(model.MQTT.Provider)。
// 订阅 Resolve 产出的主题收打印/查询,向上报主题发 report/state(定时 state + LWT 离线)。
package mqtt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	pmqtt "github.com/eclipse/paho.mqtt.golang"

	"congmingpay/internal/api"
	"congmingpay/internal/config"
	"congmingpay/internal/errcode"
	"congmingpay/internal/logger"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

// stateInterval 是 state 状态快照的定时上报间隔(包级 var 仅供测试临时调小,产品固定 1 分钟)。
var stateInterval = time.Minute

// Client 是云端 MQTT 客户端(唯一云端通道)。
// ClientID/订阅/上报主题均来自 Resolve(自建=短商户号;阿里云=Group@@@Device + 模板展开)。
type Client struct {
	cfg     *config.Config
	svc     *printsvc.Service
	cfgPath string

	mu             sync.Mutex
	cli            pmqtt.Client
	merchant       string // 上行 JSON merchant(自建=短商户号;阿里云=DeviceId)
	subscribeTopic string // 实际订阅主题
	reportTopic    string // 实际上报(发布)主题
	enabled        bool   // 当前配置是否启用 MQTT(供 publish 决定未发送时是否落错误日志)
	connected      bool
	lastErr        string
	onChange       func()
	onStatus       func()        // 连接状态变化回调(UI 刷新状态标签用)
	stopTick       chan struct{} // 定时 state 上报 goroutine 的停止信号(随连接生命周期)
}

// New 创建客户端(未连接;由 Start 按配置连接)。
func New(cfg *config.Config, svc *printsvc.Service, cfgPath string) *Client {
	return &Client{cfg: cfg, svc: svc, cfgPath: cfgPath}
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
		old, oldMerchant, oldTopic := c.cli, c.merchant, c.reportTopic
		c.cli = nil
		c.connected = false
		// 旧身份先发简版离线再断开:干净 DISCONNECT 会让 broker 丢弃遗嘱,
		// 停用/换配置若不主动发,云端将永久滞留 online:true。
		go func() {
			sendOffline(old, oldMerchant, oldTopic)
			old.Disconnect(250)
		}()
	}
	if c.stopTick != nil {
		close(c.stopTick)
		c.stopTick = nil
	}
	c.enabled = m.Enabled
	c.lastErr = ""
	c.merchant = ""
	c.subscribeTopic = ""
	c.reportTopic = ""

	p, ok, err := Resolve(m)
	if err != nil {
		c.mu.Unlock()
		logger.Errorf("MQTT 配置无效,不连接: %v", err)
		return
	}
	if !ok {
		c.mu.Unlock()
		logger.Info("MQTT 未启用或必填参数为空,不连接")
		return
	}
	c.merchant = p.Merchant
	c.subscribeTopic = p.SubscribeTopic
	c.reportTopic = p.ReportTopic
	cli := pmqtt.NewClient(c.buildOpts(p))
	c.cli = cli
	c.stopTick = make(chan struct{})
	go c.stateLoop(c.stopTick) // 定时 state 上报,随本次连接生命周期
	c.mu.Unlock()

	logger.Infof("MQTT 连接 %s(ClientID %s · 订阅 %s)…", p.BrokerURL, p.ClientID, p.SubscribeTopic)
	tok := cli.Connect()
	go func() {
		tok.Wait()
		if err := tok.Error(); err != nil {
			c.setErr(err.Error())
			logger.Errorf("MQTT 首次连接失败(将后台重试): %v", err)
		}
	}()
}

func (c *Client) buildOpts(p ConnParams) *pmqtt.ClientOptions {
	// 遗嘱为连接时固化的静态内容:简版 state(仅服务离线,不含打印机数组)。
	will, _ := json.Marshal(stateMsg{Type: "state", Merchant: p.Merchant, Online: false})
	subTopic := p.SubscribeTopic
	opts := pmqtt.NewClientOptions()
	opts.AddBroker(p.BrokerURL)
	opts.SetClientID(p.ClientID)
	if p.Username != "" {
		opts.SetUsername(p.Username)
	}
	if p.Password != "" {
		opts.SetPassword(p.Password)
	}
	opts.SetKeepAlive(60 * time.Second)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true) // 已连后断开自动重连
	opts.SetConnectRetry(true)  // 首次连不上也后台重试
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetWill(p.ReportTopic, string(will), 1, false) // 遗嘱发到上报主题
	opts.SetOnConnectHandler(func(cli pmqtt.Client) {
		c.setConnected(true)
		// 订阅与上线 publish 放后台 goroutine:t.Wait() 会阻塞 paho 回调线程,别卡在这里。
		go func() {
			if t := cli.Subscribe(subTopic, 1, c.onPrint); t.Wait() && t.Error() != nil {
				logger.Errorf("MQTT 订阅 %s 失败: %v", subTopic, t.Error())
			} else {
				logger.Infof("MQTT 已连接·订阅 %s", subTopic)
			}
			c.PublishState() // 上线首拍(定时流起点):一条 state 全量(服务在线 + 全部打印机配置与在线态)
		}()
	})
	opts.SetConnectionLostHandler(func(_ pmqtt.Client, err error) {
		c.setConnected(false)
		c.setErr(err.Error())
		logger.Errorf("MQTT 连接断开(自动重连中): %v", err)
	})
	return opts
}

// stateLoop 定时上报 state:每 stateInterval 一条全量快照(连接首拍由 OnConnect 发)。
// 未启用/断线时静默跳过——断线本身已有连接日志,不逐拍刷错误。
func (c *Client) stateLoop(stop <-chan struct{}) {
	t := time.NewTicker(stateInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			cli, _, enabled := c.snapshot()
			if !enabled || cli == nil || !cli.IsConnectionOpen() {
				continue
			}
			c.PublishState()
		}
	}
}

// parseQuery 探测下行 payload 是否为查询消息:单对象且顶层 query 为非空字符串。
func parseQuery(payload []byte) (string, bool) {
	t := bytes.TrimSpace(payload)
	if len(t) == 0 || t[0] != '{' {
		return "", false
	}
	var probe struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(t, &probe); err != nil || probe.Query == "" {
		return "", false
	}
	return probe.Query, true
}

// onPrint 收到下行消息:含顶层 query 字段的单对象为查询(printers=拉取全量 state);
// 其余复用 api.ParseRequests/Process,逐条处理并回执。
// 【每条都打】不做 id 去重——收到消息即打印/登记(与 UI「JSON测试」一致);id 仅用于回执/日志对应。
func (c *Client) onPrint(cli pmqtt.Client, msg pmqtt.Message) {
	payload := msg.Payload()
	logger.Infof("MQTT 收到消息 topic=%s 字节=%d", msg.Topic(), len(payload))

	_, merchant, _ := c.snapshot()
	if q, isQuery := parseQuery(payload); isQuery {
		if q == "printers" {
			logger.Info("MQTT 收到查询 printers → 回发全量 state")
			c.PublishState()
		} else {
			logger.Errorf("MQTT 不支持的 query: %q", q)
			c.publish(cli, reportMsg{Type: "report", Merchant: merchant, Event: printsvc.EventFailed,
				Code: errcode.UnsupportedQuery, Message: `不支持的 query: ` + q + `(支持 "printers")`, TS: time.Now().UnixMilli()})
		}
		return
	}
	reqs, wasArray, err := api.ParseRequests(payload)
	if err != nil {
		logger.Errorf("MQTT 打印消息解析失败: %v", err)
		c.publish(cli, reportMsg{Type: "report", Merchant: merchant, Event: printsvc.EventFailed,
			Code: errcode.ParseFailed, Message: "解析失败: " + err.Error(), TS: time.Now().UnixMilli()})
		return
	}
	logger.Infof("MQTT 解析出 %d 条打印请求(数组=%v)", len(reqs), wasArray)

	changed := false
	for i := range reqs {
		req := &reqs[i]
		id := req.ID
		logger.Infof("MQTT 处理 id=%d 目标=%s type=%d", id, reqTarget(req), req.Type)
		res, reg, e := api.Process(c.cfg, c.svc, req)
		if reg {
			changed = true
		}
		if e != nil {
			logger.Errorf("MQTT 打印受理失败 id=%d code=%d: %v", id, errcode.CodeOf(e), e)
			c.publish(cli, reportMsg{Type: "report", Merchant: merchant, Event: printsvc.EventFailed,
				ID: id, Code: errcode.CodeOf(e), Message: e.Error(), TS: time.Now().UnixMilli()})
			continue
		}
		logger.Infof("MQTT id=%d 已提交 任务#%d(登记/更新=%v)", id, res.JobNo, reg)
		copies := req.PCopy
		if copies <= 0 {
			copies = 1
		}
		width := res.Printer.Width
		if req.PWidth == 58 || req.PWidth == 80 {
			width = req.PWidth
		}
		c.publish(cli, reportMsg{Type: "report", Merchant: merchant, Event: eventAccepted,
			ID: id, JobNo: res.JobNo, Code: 0, Message: "已提交",
			Printer: reportPrinterOf(&res.Printer),
			Params: &reportParams{
				Buzzer: *req.Buzzer, Cut: *req.Cut, Reprint: *req.Reprint,
				HeadLines: *req.HeadLines, TailLines: *req.TailLines,
				PWidth: width, PCopy: copies, ContentType: req.Type,
			},
			TS: time.Now().UnixMilli()})
	}
	if changed {
		c.save()
		c.fireChange() // 有新增/更新打印机 → 保存并刷新列表 + 起监测(state 由定时拍带出)
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

// sendOffline 直接经指定连接同步发布简版离线 state(短超时),供 Close/Reload 收尾:
// 绕过 publish 的代际校验——收尾时 c.cli 已置换/置 nil,常规 publish 会拒发。
func sendOffline(cli pmqtt.Client, merchant, topic string) {
	if cli == nil || strings.TrimSpace(topic) == "" || !cli.IsConnectionOpen() {
		return
	}
	b, _ := json.Marshal(stateMsg{Type: "state", Merchant: merchant, Online: false})
	if tok := cli.Publish(topic, 1, false, b); !tok.WaitTimeout(3 * time.Second) {
		logger.Errorf("MQTT 离线上报超时未确认: %s", b)
	}
}

// publish 向配置的「上报主题」发布(best-effort:不排队不补发,失败原因落日志)。
// 未连接/主题为空时跳过;若配置为启用状态,跳过与失败都 logger.Errorf 记原因,做到有据可查。
//
// 必须用 IsConnectionOpen(仅真正 connected 才 true)而非只判 cli!=nil:
// paho v1.4.3 在 ConnectRetry 下「连接中」状态 IsConnected() 也为 true,此时 QoS1 Publish
// 只进内部存储、token 永不完成,且 CleanSession 连上后 persist.Reset() 直接丢弃——
// 消息静默丢失、tok.Wait() goroutine 永久泄漏。判 IsConnectionOpen 把这类消息转为「跳过+日志」;
// 判后瞬断的残余窗口由 WaitTimeout 兜底(超时记日志、goroutine 退出,不泄漏)。
//
// 代际校验:cli 不等于当前 c.cli(Reload 已换连接/Close 已关闭)时静默跳过——
// 封死「Close 发完离线后,在途 PublishState 又经旧连接补发 online:true」的交错窗口。
func (c *Client) publish(cli pmqtt.Client, v interface{}) {
	c.mu.Lock()
	topic := c.reportTopic
	enabled := c.enabled
	cur := c.cli
	c.mu.Unlock()
	b, _ := json.Marshal(v)
	if cli != nil && cli != cur {
		logger.Infof("MQTT 上报跳过(连接已更换或关闭): %s", b)
		return
	}
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
	m.Enabled = true // 测试时视为启用,便于 Resolve
	p, ok, err := Resolve(m)
	if err != nil {
		return 0, err
	}
	if !ok {
		if m.EffectiveProvider() == model.MQTTProviderAliyun {
			return 0, fmt.Errorf("阿里云参数不完整(地址/端口/InstanceId/AK/SK/GroupId/父主题/自定义ID/上下行后缀)")
		}
		return 0, fmt.Errorf("Broker/短商户号/上报主题 不完整")
	}
	opts := pmqtt.NewClientOptions()
	opts.AddBroker(p.BrokerURL)
	opts.SetClientID(p.ClientID)
	if p.Username != "" {
		opts.SetUsername(p.Username)
	}
	if p.Password != "" {
		opts.SetPassword(p.Password)
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
		return true, "已连接 · 订阅 " + c.subscribeTopic
	case c.lastErr != "":
		return false, "未连接 · " + c.lastErr
	default:
		return false, "连接中…"
	}
}

// Active 返回当前是否存在连接对象(启用且参数齐全;不代表已连上)。
// 供 UI 区分「断线」与「未启用/已停用」——Close/reconnect 置 cli=nil 时不触发 onStatus,
// UI 须在每次状态回调里主动判别中性态。
func (c *Client) Active() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cli != nil
}

// Close 优雅关闭:停定时上报、主动发离线并断开。
// enabled 置 false:收尾期在途上报(任务终态等)静默跳过,不再刷错误日志。
func (c *Client) Close() {
	c.mu.Lock()
	cli, merchant, topic := c.cli, c.merchant, c.reportTopic
	c.cli = nil
	c.connected = false
	c.enabled = false
	if c.stopTick != nil {
		close(c.stopTick)
		c.stopTick = nil
	}
	c.mu.Unlock()
	if cli != nil {
		sendOffline(cli, merchant, topic)
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
