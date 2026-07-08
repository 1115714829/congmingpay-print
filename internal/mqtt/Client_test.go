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

	printJSON := `{"gateway":"10.9.9.9","id":770001,"type":5,"pWidth":80,"pCopy":1,` +
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

	// 应收到回执(且携带 merchant 身份——服务端单主题一对多靠它分辨来源)
	select {
	case ack := <-gotAck:
		var a ackMsg
		_ = json.Unmarshal([]byte(ack), &a)
		t.Logf("收到回执: %s (type=%s merchant=%s ok=%v job=%d)", ack, a.Type, a.Merchant, a.OK, a.JobNo)
		if a.Type != "ack" {
			t.Errorf("回执 type 应为 ack, got %q", a.Type)
		}
		if a.Merchant != merchant {
			t.Errorf("回执应携带 merchant=%q, got %q", merchant, a.Merchant)
		}
	case <-time.After(5 * time.Second):
		t.Error("未在 5s 内收到打印回执")
	}
	_ = fmt.Sprint
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

// TestResultRoundTrip:端到端——本地假打印机 + 公共 broker,应先收 ack(已提交)再收 result(打印成功,id 回携)。
// 依赖公共 broker;连不上则跳过。
func TestResultRoundTrip(t *testing.T) {
	merchant := "cmpselftest9272r"
	m := model.MQTT{Enabled: true, Broker: "test.mosquitto.org", Port: 1883, Topic: merchant, ReportTopic: merchant + "/report"}
	if _, err := TestConnect(m); err != nil {
		t.Skipf("公共 broker 不可达,跳过端到端: %v", err)
	}

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
	// 等价 main.go 装配:任务终态 → 上报打印结果(本地任务过滤)。
	svc.SetOnJobFinal(func(ev printsvc.JobFinalEvent) {
		if ev.CloudID == nil {
			return
		}
		c.PublishJobResult(ev)
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

	printJSON := fmt.Sprintf(`{"gateway":"127.0.0.1:%d","id":880001,"type":5,"pWidth":80,"pCopy":1,`+
		`"buzzer":0,"cut":1,"reprint":0,"headLines":0,"tailLines":0,`+
		`"contents":[{"cont":"结果回执自测","type":"title"}]}`, port)
	pub.Publish(merchant, 1, false, printJSON).Wait()

	// 应收到 ack(已提交)、printerList(自动登记触发)与 result(打印成功);全部携带 merchant 身份。
	// 不校验三者先后顺序——ack 由 paho router goroutine 发、result 由 dispatch goroutine 发,
	// 环回打印可 1ms 级完成,二者的 Publish 调用序无同步保证,只要都收到即可。
	var sawAck, sawResult, sawList bool
	timeout := time.After(20 * time.Second)
	for !(sawAck && sawResult && sawList) {
		select {
		case raw := <-got:
			var probe struct {
				Type     string `json:"type"`
				Merchant string `json:"merchant"`
				ID       uint32 `json:"id"`
				OK       bool   `json:"ok"`
				JobNo    int    `json:"jobNo"`
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
			switch probe.Type {
			case "ack":
				if probe.ID == 880001 && probe.OK {
					sawAck = true
				}
			case "printerList":
				if len(probe.Printers) == 0 || probe.Printers[0].Width != 80 {
					t.Errorf("printerList 应含刚登记的 80mm 打印机: %s", raw)
				}
				sawList = true
			case "result":
				if probe.ID != 880001 || !probe.OK || probe.JobNo == 0 {
					t.Errorf("result 内容不符: %s", raw)
				}
				sawResult = true
			}
		case <-timeout:
			t.Fatalf("20s 内未收齐 ack+result+printerList(sawAck=%v sawResult=%v sawList=%v)", sawAck, sawResult, sawList)
		}
	}
}
