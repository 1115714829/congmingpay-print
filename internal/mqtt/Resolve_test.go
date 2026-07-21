package mqtt

import (
	"strings"
	"testing"

	"congmingpay/internal/model"
)

func TestJoinParentTopic(t *testing.T) {
	got, err := JoinParentTopic("server", "M1", "cmd")
	if err != nil || got != "server/M1/cmd" {
		t.Fatalf("got %q err=%v", got, err)
	}
	got, err = JoinParentTopic("/server/", "/M1/", "/report/")
	if err != nil || got != "server/M1/report" {
		t.Fatalf("trim: got %q err=%v", got, err)
	}
	if _, err := JoinParentTopic("server", "M1", "a+#"); err == nil {
		t.Fatal("期望通配符错误")
	}
	long := strings.Repeat("x", 30)
	if _, err := JoinParentTopic(long, long, "cmd"); err == nil {
		t.Fatal("期望超长错误")
	}
}

func TestMacSignatureKnownVector(t *testing.T) {
	got := MacSignature("secret", "GID_Test@@@device1")
	if got == "" || strings.Contains(got, " ") {
		t.Fatalf("签名异常: %q", got)
	}
	if MacSignature("secret", "GID_Test@@@device1") != got {
		t.Fatal("签名不稳定")
	}
	if MacSignature("other", "GID_Test@@@device1") == got {
		t.Fatal("不同密钥应不同")
	}
}

func TestResolveAliyun(t *testing.T) {
	m := model.MQTT{
		Enabled:  true,
		Provider: model.MQTTProviderAliyun,
		Aliyun: model.AliyunMQTT{
			Endpoint: "post-cn-xxx.mqtt.aliyuncs.com", Port: 1883,
			InstanceId: "post-cn-xxx", AccessKey: "LTAI", SecretKey: "sk",
			GroupId: "GID_Print", ParentTopic: "server", DeviceId: "M100001",
			DownSuffix: "cmd", UpSuffix: "report",
		},
	}
	p, ok, err := Resolve(m)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.SubscribeTopic != "server/M100001/cmd" || p.ReportTopic != "server/M100001/report" {
		t.Fatalf("topics %q %q", p.SubscribeTopic, p.ReportTopic)
	}
	if p.ClientID != "GID_Print@@@M100001" || p.Merchant != "M100001" {
		t.Fatalf("id/merchant %q / %q", p.ClientID, p.Merchant)
	}
	if p.Username != "Signature|LTAI|post-cn-xxx" {
		t.Fatalf("username %q", p.Username)
	}
	if p.Password != MacSignature("sk", p.ClientID) {
		t.Fatal("password 不符")
	}
}

func TestResolveAliyunSameTopics(t *testing.T) {
	_, ok, err := Resolve(model.MQTT{
		Enabled: true, Provider: model.MQTTProviderAliyun,
		Aliyun: model.AliyunMQTT{
			Endpoint: "e", InstanceId: "i", AccessKey: "ak", SecretKey: "sk",
			GroupId: "GID", ParentTopic: "server", DeviceId: "d",
			DownSuffix: "x", UpSuffix: "x",
		},
	})
	if ok || err == nil {
		t.Fatalf("期望相同主题错误 ok=%v err=%v", ok, err)
	}
}

func TestResolveAliyunIncomplete(t *testing.T) {
	_, ok, err := Resolve(model.MQTT{
		Enabled: true, Provider: model.MQTTProviderAliyun,
		Aliyun: model.AliyunMQTT{Endpoint: "e", ParentTopic: "server"},
	})
	if err != nil || ok {
		t.Fatalf("不全应 ok=false ok=%v err=%v", ok, err)
	}
}

func TestResolveGeneric(t *testing.T) {
	p, ok, err := Resolve(model.MQTT{
		Enabled: true, Broker: "mqtt.example.com", Port: 1883,
		Topic: "M1", ReportTopic: "server", Username: "u", Password: "p",
	})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.SubscribeTopic != "M1" || p.ReportTopic != "server" {
		t.Fatalf("%+v", p)
	}
}

func TestPreviewTopics(t *testing.T) {
	sub, rep, cid, err := PreviewTopics(model.AliyunMQTT{
		GroupId: "GID", ParentTopic: "server", DeviceId: "M1",
		DownSuffix: "cmd", UpSuffix: "report",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sub != "server/M1/cmd" || rep != "server/M1/report" || cid != "GID@@@M1" {
		t.Fatalf("%s %s %s", sub, rep, cid)
	}
}
