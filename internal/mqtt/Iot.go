package mqtt

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"congmingpay/internal/model"
)

// IoT 一机一密(官方):ClientID=brief|securemode=3,signmethod=hmacsha1,timestamp=…|
// Username=DeviceName&ProductKey;Password=hex(HMAC-SHA1(DeviceSecret, content))
// content=按参数名字典序拼接 key+value(含 clientId/deviceName/productKey/timestamp)。
// securemode=3 对应明文 TCP 1883(与本服务现有通道一致;TLS=2 待后续)。
// 自定义 Topic 官方前缀固定 /{productKey}/{deviceName}/user/{后缀}。

// ResolveIotEndpoint 解析接入点:手填优先,否则 {pk}.iot-as-mqtt.{region}.aliyuncs.com。
func ResolveIotEndpoint(productKey, regionID, endpoint string) (string, error) {
	ep := strings.TrimSpace(endpoint)
	if ep != "" {
		return ep, nil
	}
	pk := strings.TrimSpace(productKey)
	region := strings.TrimSpace(regionID)
	if pk == "" {
		return "", fmt.Errorf("ProductKey 未填写")
	}
	if region == "" {
		return "", fmt.Errorf("地域 RegionId 未填写(或手填 MQTT 地址)")
	}
	return pk + ".iot-as-mqtt." + region + ".aliyuncs.com", nil
}

// JoinIotUserTopic 拼物联网自定义 Topic:/{productKey}/{deviceName}/user/{suffix}。
func JoinIotUserTopic(productKey, deviceName, suffix string) (string, error) {
	pk := strings.Trim(strings.TrimSpace(productKey), "/")
	dn := strings.Trim(strings.TrimSpace(deviceName), "/")
	suf := strings.Trim(strings.TrimSpace(suffix), "/")
	if pk == "" {
		return "", fmt.Errorf("ProductKey 未填写")
	}
	if dn == "" {
		return "", fmt.Errorf("DeviceName 未填写")
	}
	if suf == "" {
		return "", fmt.Errorf("主题后缀未填写")
	}
	if strings.ContainsAny(pk+dn+suf, "+#") {
		return "", fmt.Errorf("主题段含通配符 +/#")
	}
	return "/" + pk + "/" + dn + "/user/" + suf, nil
}

// IoTSignPassword 按物联网平台一机一密计算 MQTT Password(十六进制小写)。
func IoTSignPassword(deviceSecret string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(params[k])
	}
	mac := hmac.New(sha1.New, []byte(deviceSecret))
	_, _ = mac.Write([]byte(b.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// BuildIotConn 组装物联网平台连接参数(TCP securemode=3)。
func BuildIotConn(a model.IoTMQTT, now time.Time) (ConnParams, bool, error) {
	pk := strings.TrimSpace(a.ProductKey)
	dn := strings.TrimSpace(a.DeviceName)
	secret := strings.TrimSpace(a.DeviceSecret)
	down := strings.TrimSpace(a.DownSuffix)
	up := strings.TrimSpace(a.UpSuffix)
	if pk == "" || dn == "" || secret == "" || down == "" || up == "" {
		return ConnParams{}, false, nil
	}
	sub, err := JoinIotUserTopic(pk, dn, down)
	if err != nil {
		return ConnParams{}, false, fmt.Errorf("订阅主题: %w", err)
	}
	rep, err := JoinIotUserTopic(pk, dn, up)
	if err != nil {
		return ConnParams{}, false, fmt.Errorf("上报主题: %w", err)
	}
	if sub == rep {
		return ConnParams{}, false, fmt.Errorf("上报主题不能与订阅主题相同(会造成自订阅回环)")
	}
	host, err := ResolveIotEndpoint(pk, a.RegionId, a.Endpoint)
	if err != nil {
		return ConnParams{}, false, err
	}
	port := a.Port
	if port <= 0 {
		port = 1883
	}
	// brief clientId:ProductKey.DeviceName(官方建议可自定义,≤64)
	brief := pk + "." + dn
	if len(brief) > 64 {
		brief = dn
		if len(brief) > 64 {
			brief = brief[:64]
		}
	}
	ts := strconv.FormatInt(now.UnixMilli(), 10)
	mqttClientID := brief + "|securemode=3,signmethod=hmacsha1,timestamp=" + ts + "|"
	params := map[string]string{
		"clientId":   brief,
		"deviceName": dn,
		"productKey": pk,
		"timestamp":  ts,
	}
	return ConnParams{
		BrokerURL:      brokerURL(host, port),
		Username:       dn + "&" + pk,
		Password:       IoTSignPassword(secret, params),
		ClientID:       mqttClientID,
		SubscribeTopic: sub,
		ReportTopic:    rep,
		Merchant:       dn,
	}, true, nil
}

func resolveIot(a model.IoTMQTT) (ConnParams, bool, error) {
	return BuildIotConn(a, time.Now())
}

// PreviewIot 供 UI 预览:接入点、用户名、简版 ClientID、上下行完整主题。
func PreviewIot(a model.IoTMQTT) (host, username, briefID, sub, rep string, err error) {
	pk := strings.TrimSpace(a.ProductKey)
	dn := strings.TrimSpace(a.DeviceName)
	if pk != "" && dn != "" {
		username = dn + "&" + pk
		briefID = pk + "." + dn
		if len(briefID) > 64 {
			briefID = dn
		}
	}
	host, err = ResolveIotEndpoint(pk, a.RegionId, a.Endpoint)
	if err != nil {
		return "", username, briefID, "", "", err
	}
	if pk == "" || dn == "" {
		return host, username, briefID, "", "", fmt.Errorf("填写 ProductKey 与 DeviceName")
	}
	down := strings.TrimSpace(a.DownSuffix)
	up := strings.TrimSpace(a.UpSuffix)
	if down == "" && up == "" {
		return host, username, briefID, "", "", fmt.Errorf("填写下行/上行后缀")
	}
	if down != "" {
		sub, err = JoinIotUserTopic(pk, dn, down)
		if err != nil {
			return host, username, briefID, "", "", err
		}
	}
	if up != "" {
		rep, err = JoinIotUserTopic(pk, dn, up)
		if err != nil {
			return host, username, briefID, "", "", err
		}
	}
	return host, username, briefID, sub, rep, nil
}
