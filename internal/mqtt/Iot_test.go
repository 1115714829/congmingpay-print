package mqtt

import (
	"strings"
	"testing"
	"time"

	"congmingpay/internal/model"
)

func TestResolveIotEndpoint(t *testing.T) {
	got, err := ResolveIotEndpoint("a1pk", "cn-shanghai", "")
	if err != nil || got != "a1pk.iot-as-mqtt.cn-shanghai.aliyuncs.com" {
		t.Fatalf("got %q err=%v", got, err)
	}
	got, err = ResolveIotEndpoint("a1pk", "cn-shanghai", "custom.example.com")
	if err != nil || got != "custom.example.com" {
		t.Fatalf("override: got %q err=%v", got, err)
	}
	if _, err := ResolveIotEndpoint("", "cn-shanghai", ""); err == nil {
		t.Fatal("期望缺 ProductKey")
	}
	if _, err := ResolveIotEndpoint("pk", "", ""); err == nil {
		t.Fatal("期望缺地域")
	}
}

func TestJoinIotUserTopic(t *testing.T) {
	got, err := JoinIotUserTopic("pk", "dn", "cmd")
	if err != nil || got != "/pk/dn/user/cmd" {
		t.Fatalf("got %q err=%v", got, err)
	}
	got, err = JoinIotUserTopic("/pk/", "/dn/", "/report/")
	if err != nil || got != "/pk/dn/user/report" {
		t.Fatalf("trim: got %q err=%v", got, err)
	}
	if _, err := JoinIotUserTopic("pk", "dn", "a+#"); err == nil {
		t.Fatal("期望通配符错误")
	}
	if _, err := JoinIotUserTopic("", "dn", "cmd"); err == nil {
		t.Fatal("期望缺 ProductKey")
	}
}

func TestIoTSignPasswordKnownOrder(t *testing.T) {
	// 参数字典序:clientId, deviceName, productKey, timestamp
	got := IoTSignPassword("secret", map[string]string{
		"timestamp":  "1",
		"productKey": "pk",
		"deviceName": "dn",
		"clientId":   "pk.dn",
	})
	if got == "" || strings.Contains(got, " ") {
		t.Fatalf("签名异常: %q", got)
	}
	if len(got) != 40 { // sha1 hex
		t.Fatalf("期望 40 hex, got len=%d %q", len(got), got)
	}
	again := IoTSignPassword("secret", map[string]string{
		"clientId": "pk.dn", "deviceName": "dn", "productKey": "pk", "timestamp": "1",
	})
	if again != got {
		t.Fatal("签名不稳定")
	}
}

func TestBuildIotConn(t *testing.T) {
	now := time.UnixMilli(1700000000123)
	p, ok, err := BuildIotConn(model.IoTMQTT{
		ProductKey: "pk", DeviceName: "device", DeviceSecret: "sec",
		RegionId: "cn-shanghai", DownSuffix: "cmd", UpSuffix: "report",
	}, now)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.BrokerURL != "tcp://pk.iot-as-mqtt.cn-shanghai.aliyuncs.com:1883" {
		t.Fatalf("broker %q", p.BrokerURL)
	}
	if p.Username != "device&pk" {
		t.Fatalf("user %q", p.Username)
	}
	wantCID := "pk.device|securemode=3,signmethod=hmacsha1,timestamp=1700000000123|"
	if p.ClientID != wantCID {
		t.Fatalf("clientID %q", p.ClientID)
	}
	if p.Merchant != "device" {
		t.Fatalf("merchant %q", p.Merchant)
	}
	if p.SubscribeTopic != "/pk/device/user/cmd" || p.ReportTopic != "/pk/device/user/report" {
		t.Fatalf("topics %q %q", p.SubscribeTopic, p.ReportTopic)
	}
	params := map[string]string{
		"clientId": "pk.device", "deviceName": "device", "productKey": "pk", "timestamp": "1700000000123",
	}
	if p.Password != IoTSignPassword("sec", params) {
		t.Fatal("password 不符")
	}
}

func TestBuildIotConnSameSuffix(t *testing.T) {
	_, ok, err := BuildIotConn(model.IoTMQTT{
		ProductKey: "pk", DeviceName: "dn", DeviceSecret: "s",
		RegionId: "cn-shanghai", DownSuffix: "x", UpSuffix: "x",
	}, time.Now())
	if ok || err == nil {
		t.Fatalf("期望相同后缀错误 ok=%v err=%v", ok, err)
	}
}

func TestBuildIotConnIncomplete(t *testing.T) {
	_, ok, err := BuildIotConn(model.IoTMQTT{
		ProductKey: "pk", DeviceName: "dn", RegionId: "cn-shanghai",
	}, time.Now())
	if err != nil || ok {
		t.Fatalf("不全应 ok=false ok=%v err=%v", ok, err)
	}
}

func TestResolveIot(t *testing.T) {
	p, ok, err := Resolve(model.MQTT{
		Enabled: true, Provider: model.MQTTProviderIot,
		Iot: model.IoTMQTT{
			ProductKey: "pk", DeviceName: "dn", DeviceSecret: "s",
			Endpoint: "iot.example.com", Port: 1883,
			DownSuffix: "cmd", UpSuffix: "report",
		},
	})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.SubscribeTopic != "/pk/dn/user/cmd" || p.ReportTopic != "/pk/dn/user/report" {
		t.Fatalf("%+v", p)
	}
}

func TestPreviewIot(t *testing.T) {
	host, user, id, sub, rep, err := PreviewIot(model.IoTMQTT{
		ProductKey: "pk", DeviceName: "dn", RegionId: "cn-shanghai",
		DownSuffix: "cmd", UpSuffix: "report",
	})
	if err != nil {
		t.Fatal(err)
	}
	if host != "pk.iot-as-mqtt.cn-shanghai.aliyuncs.com" || user != "dn&pk" || id != "pk.dn" {
		t.Fatalf("%s %s %s", host, user, id)
	}
	if sub != "/pk/dn/user/cmd" || rep != "/pk/dn/user/report" {
		t.Fatalf("topics %s %s", sub, rep)
	}
}
