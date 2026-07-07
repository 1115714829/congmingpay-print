package mqtt

import (
	"encoding/json"
	"fmt"
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
	m := model.MQTT{Enabled: true, Broker: "test.mosquitto.org", Port: 1883, Topic: merchant}

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
	pub.Subscribe(merchant+"/report", 1, func(_ pmqtt.Client, msg pmqtt.Message) {
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

	// 应收到回执
	select {
	case ack := <-gotAck:
		var a ackMsg
		_ = json.Unmarshal([]byte(ack), &a)
		t.Logf("收到回执: %s (type=%s ok=%v job=%d)", ack, a.Type, a.OK, a.JobNo)
		if a.Type != "ack" {
			t.Errorf("回执 type 应为 ack, got %q", a.Type)
		}
	case <-time.After(5 * time.Second):
		t.Error("未在 5s 内收到打印回执")
	}
	_ = fmt.Sprint
}
