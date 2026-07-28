package mqtt

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	pmqtt "github.com/eclipse/paho.mqtt.golang"

	"congmingpay/internal/config"
	"congmingpay/internal/model"
	"congmingpay/internal/printsvc"
)

func TestSanitizeMerchant(t *testing.T) {
	cases := map[string]string{
		" a b\tc\n#!_x ": "abc_x",
		"商户_A1":         "_A1",
		"  M-100/x  ":    "M100x",
		"":               "",
	}
	for in, want := range cases {
		if got := SanitizeMerchant(in); got != want {
			t.Errorf("SanitizeMerchant(%q)=%q, 期望 %q", in, got, want)
		}
	}
}

func TestBrokerURL(t *testing.T) {
	cases := []struct {
		broker string
		port   int
		want   string
	}{
		{"mqtt.example.com", 1883, "tcp://mqtt.example.com:1883"},
		{"tcp://host:1883", 0, "tcp://host:1883"},
		{"host", 8883, "tcp://host:8883"},
		{"ssl://host", 8883, "ssl://host:8883"},
	}
	for _, c := range cases {
		if got := brokerURL(c.broker, c.port); got != c.want {
			t.Errorf("brokerURL(%q,%d)=%q, 期望 %q", c.broker, c.port, got, c.want)
		}
	}
}

// TestRoundTrip:端到端——本 APP 订阅短商户号,外部发一条打印 JSON → 产生打印任务 + 收到回执。
// 依赖公共 broker(test.mosquitto.org);连不上则跳过(不算失败)。
func TestRoundTrip(t *testing.T) {
	merchant := "cmpselftest9271"
	// ReportTopic 为测试专用唯一主题(公共 broker 防串扰);产品无默认上报主题,由用户手填。
	m := model.MQTT{Enabled: true, Broker: "test.mosquitto.org", Port: 1883, Topic: merchant, ReportTopic: merchant + "/report"}

	if _, err := TestConnect(m); err != nil {
		t.Skipf("公共 broker 不可达,跳过端到端: %v", err)
	}

	cfg := config.Default()
	svc := printsvc.New()
	c := New(cfg, svc, "")
	cfg.Settings.MQTT = m
	c.Start()
	defer c.Close()

	// 等本 APP 连上并订阅
	deadline := time.Now().Add(10 * time.Second)
	for {
		if ok, _ := c.Status(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Skip("本 APP 未能连上 broker,跳过")
		}
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond) // 订阅稳定

	// 另一个客户端:订阅回执主题 + 向短商户号发打印
	gotAck := make(chan string, 1)
	pub := pmqtt.NewClient(pmqtt.NewClientOptions().AddBroker("tcp://test.mosquitto.org:1883").SetClientID("cmpselftest-pub"))
	if tok := pub.Connect(); !tok.WaitTimeout(8*time.Second) || tok.Error() != nil {
		t.Skip("发布端连不上,跳过")
	}
	defer pub.Disconnect(100)
	pub.Subscribe(m.ReportTopic, 1, func(_ pmqtt.Client, msg pmqtt.Message) {
		select {
		case gotAck <- string(msg.Payload()):
		default:
		}
	}).Wait()

	printJSON := `{"gateway":"10.9.9.9","id":770001,"type":0,"pWidth":80,"pCopy":1,` +
		`"buzzer":0,"cut":1,"reprint":0,"headLines":0,"tailLines":0,` +
		`"contents":[{"cont":"MQTT自测","type":"title"}]}`
	pub.Publish(merchant, 1, false, printJSON).Wait()

	// 应产生打印任务(目标不可达 → 进等待重试,但任务已建)
	ok := false
	for i := 0; i < 30; i++ {
		if len(svc.Jobs()) > 0 {
			ok = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ok {
		t.Fatal("MQTT 打印消息未产生打印任务")
	}
	t.Logf("已产生打印任务 %d 个", len(svc.Jobs()))

	// LegacyCompat C4: 不上报 accepted;目标不可达亦不上报 waiting → 短时不应有 report。
	select {
	case raw := <-gotAck:
		var r struct {
			Type  string `json:"type"`
			Event string `json:"event"`
		}
		_ = json.Unmarshal([]byte(raw), &r)
		if r.Type == "report" && (r.Event == "accepted" || r.Event == "waiting") {
			t.Errorf("LegacyCompat C4 不应上报 %s: %s", r.Event, raw)
		}
		t.Logf("收到上行(非 accepted/waiting 可接受): %s", raw)
	case <-time.After(2 * time.Second):
		t.Log("2s 内无 report,符合 C4(无 accepted/waiting)")
	}
}

// TestParseQuery 锁定下行查询消息的分流边界:仅「单对象 + 顶层 query 非空字符串」为查询。
func TestParseQuery(t *testing.T) {
	cases := []struct {
		payload string
		want    string
		isQuery bool
	}{
		{`{"query":"printers"}`, "printers", true},
		{`  {"query":"foo"} `, "foo", true},
		{`{"query":"printers","buzzer":0}`, "printers", true}, // 含 query 即查询,其余字段忽略
		{`{"query":""}`, "", false},                           // 空字符串不是查询
		{`{"query":123}`, "", false},                          // 非字符串不是查询
		{`{"query":null}`, "", false},
		{`[{"query":"printers"}]`, "", false}, // 数组不做查询探测
		{`{"buzzer":0}`, "", false},
		{``, "", false},
	}
	for _, c := range cases {
		q, ok := parseQuery([]byte(c.payload))
		if q != c.want || ok != c.isQuery {
			t.Errorf("parseQuery(%q) = (%q,%v), 期望 (%q,%v)", c.payload, q, ok, c.want, c.isQuery)
		}
	}
}

// 启用但缺上报主题 → 拒绝连接(与 broker/短商户号同级必要)。
func TestReconnectRequiresReportTopic(t *testing.T) {
	cfg := config.Default()
	svc := printsvc.New()
	c := New(cfg, svc, "")
	c.Reload(model.MQTT{Enabled: true, Broker: "mqtt.example.com", Port: 1883, Topic: "m123"}) // 无 ReportTopic
	if ok, detail := c.Status(); ok || detail != "未启用" {
		t.Fatalf("缺上报主题应不连接(状态“未启用”),got ok=%v detail=%q", ok, detail)
	}
}

// TestReportRoundTrip:端到端——本地假打印机 + 公共 broker,应收到
// state 与 report(done);LegacyCompat C4 下不应有 accepted。
// 另验证 {"query":"printers"} 拉取与未知 query 的 1002 回执。依赖公共 broker;连不上则跳过。
func TestReportRoundTrip(t *testing.T) {
	merchant := "cmpselftest9272r"
	// ReportTopic 为测试专用唯一主题(公共 broker 防串扰);产品无默认上报主题,由用户手填。
	m := model.MQTT{Enabled: true, Broker: "test.mosquitto.org", Port: 1883, Topic: merchant, ReportTopic: merchant + "/report"}
	if _, err := TestConnect(m); err != nil {
		t.Skipf("公共 broker 不可达,跳过端到端: %v", err)
	}

	// 定时上报间隔临时调小:登记后的 state 由定时拍带出(产品固定 1 分钟)。
	oldInterval := stateInterval
	stateInterval = 2 * time.Second
	t.Cleanup(func() { stateInterval = oldInterval })

	// 本地假打印机:接收并丢弃打印字节。
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("无法起本地监听: %v", err)
	}
	defer l.Close()
	go func() {
		for {
			conn, e := l.Accept()
			if e != nil {
				return
			}
			_, _ = io.Copy(io.Discard, conn)
			_ = conn.Close()
		}
	}()
	port := l.Addr().(*net.TCPAddr).Port

	cfg := config.Default()
	svc := printsvc.New()
	c := New(cfg, svc, "")
	// 等价 main.go 装配:任务事件 → 上报 report(本地任务过滤)。
	svc.SetOnJobEvent(func(ev printsvc.JobEvent) {
		if ev.CloudID == nil {
			return
		}
		c.PublishJobEvent(ev)
	})
	cfg.Settings.MQTT = m
	c.Start()
	defer c.Close()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if ok, _ := c.Status(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Skip("本 APP 未能连上 broker,跳过")
		}
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)

	got := make(chan string, 10)
	pub := pmqtt.NewClient(pmqtt.NewClientOptions().AddBroker("tcp://test.mosquitto.org:1883").SetClientID("cmpselftest-pub-r"))
	if tok := pub.Connect(); !tok.WaitTimeout(8*time.Second) || tok.Error() != nil {
		t.Skip("发布端连不上,跳过")
	}
	defer pub.Disconnect(100)
	pub.Subscribe(m.ReportTopic, 1, func(_ pmqtt.Client, msg pmqtt.Message) {
		select {
		case got <- string(msg.Payload()):
		default:
		}
	}).Wait()

	printJSON := fmt.Sprintf(`{"gateway":"127.0.0.1:%d","id":880001,"type":0,"pWidth":80,"pCopy":1,`+
		`"buzzer":0,"cut":1,"reprint":0,"headLines":0,"tailLines":0,`+
		`"contents":[{"cont":"结果回执自测","type":"title"}]}`, port)
	pub.Publish(merchant, 1, false, printJSON).Wait()

	// LegacyCompat C4: 应收 done + state;不应收 accepted。
	var sawAccepted, sawDone, sawState bool
	timeout := time.After(20 * time.Second)
	for !(sawDone && sawState) {
		select {
		case raw := <-got:
			var probe struct {
				Type     string `json:"type"`
				Merchant string `json:"merchant"`
				Event    string `json:"event"`
				ID       uint32 `json:"id"`
				JobNo    int    `json:"jobNo"`
				Code     int    `json:"code"`
				Printers []struct {
					Printer string `json:"printer"`
					Width   int    `json:"width"`
				} `json:"printers"`
			}
			_ = json.Unmarshal([]byte(raw), &probe)
			t.Logf("收到上行: %s", raw)
			if probe.Merchant != merchant {
				t.Errorf("上行消息应携带 merchant=%q,got %q(%s)", merchant, probe.Merchant, raw)
			}
			switch {
			case probe.Type == "report" && probe.Event == "accepted":
				sawAccepted = true
				t.Errorf("LegacyCompat C4 不应上报 accepted: %s", raw)
			case probe.Type == "report" && probe.Event == "waiting":
				t.Errorf("LegacyCompat C4 不应上报 waiting: %s", raw)
			case probe.Type == "report" && probe.Event == "done":
				if probe.ID != 880001 || probe.Code != 0 || probe.JobNo == 0 {
					t.Errorf("done 内容不符: %s", raw)
				}
				sawDone = true
			case probe.Type == "state":
				if len(probe.Printers) == 0 {
					break
				}
				if probe.Printers[0].Width != 80 {
					t.Errorf("state 应含刚登记的 80mm 打印机: %s", raw)
				}
				sawState = true
			}
		case <-timeout:
			t.Fatalf("20s 内未收齐 done+state(done=%v state=%v accepted误报=%v)", sawDone, sawState, sawAccepted)
		}
	}

	// 按需拉取:{"query":"printers"} → 收到全量 state;未知 query → report(failed, code=1002)。
	pub.Publish(merchant, 1, false, `{"query":"printers"}`).Wait()
	pub.Publish(merchant, 1, false, `{"query":"foo"}`).Wait()
	var sawQueryState, saw1002 bool
	qTimeout := time.After(10 * time.Second)
	for !(sawQueryState && saw1002) {
		select {
		case raw := <-got:
			var probe struct {
				Type  string `json:"type"`
				Event string `json:"event"`
				ID    uint32 `json:"id"`
				Code  int    `json:"code"`
			}
			_ = json.Unmarshal([]byte(raw), &probe)
			t.Logf("收到上行: %s", raw)
			switch {
			case probe.Type == "state":
				sawQueryState = true
			case probe.Type == "report" && probe.Event == "failed" && probe.Code == 1002:
				if probe.ID != 0 {
					t.Errorf("查询回执 id 应为 0: %s", raw)
				}
				saw1002 = true
			}
		case <-qTimeout:
			t.Fatalf("10s 内未收齐查询响应(state=%v code1002=%v)", sawQueryState, saw1002)
		}
	}
}
