package mqtt

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"

	"congmingpay/internal/model"
)

// aliyunTopicMaxLen 是云消息队列 MQTT 版父子 Topic 全路径长度上限。
const aliyunTopicMaxLen = 64

// ConnParams 是 Resolve 产出的统一连接参数(自建与阿里云共用)。
type ConnParams struct {
	BrokerURL      string
	Username       string
	Password       string
	ClientID       string
	SubscribeTopic string
	ReportTopic    string
	Merchant       string // 上行 JSON merchant 字段
}

// JoinParentTopic 拼接 父主题/自定义ID/后缀。
func JoinParentTopic(parent, deviceID, suffix string) (string, error) {
	p := strings.TrimSpace(parent)
	id := strings.TrimSpace(deviceID)
	suf := strings.TrimSpace(suffix)
	if p == "" {
		return "", fmt.Errorf("父主题未填写")
	}
	if id == "" {
		return "", fmt.Errorf("自定义 ID 未填写")
	}
	if suf == "" {
		return "", fmt.Errorf("主题后缀未填写")
	}
	if strings.ContainsAny(p+id+suf, "+#") {
		return "", fmt.Errorf("主题段含通配符 +/#")
	}
	// 去掉各段首尾多余 /
	p = strings.Trim(p, "/")
	id = strings.Trim(id, "/")
	suf = strings.Trim(suf, "/")
	topic := p + "/" + id + "/" + suf
	if len(topic) > aliyunTopicMaxLen {
		return "", fmt.Errorf("主题长度 %d 超过阿里云上限 %d", len(topic), aliyunTopicMaxLen)
	}
	return topic, nil
}

// MacSignature 计算云消息队列签名鉴权 Password:Base64(HMAC-SHA1(secret, clientID))。
func MacSignature(secretKey, clientID string) string {
	mac := hmac.New(sha1.New, []byte(secretKey))
	_, _ = mac.Write([]byte(clientID))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Resolve 按 Provider 解析连接参数;未启用时返回 ok=false 且 err=nil。
func Resolve(m model.MQTT) (p ConnParams, ok bool, err error) {
	if !m.Enabled {
		return ConnParams{}, false, nil
	}
	switch m.EffectiveProvider() {
	case model.MQTTProviderAliyun:
		return resolveAliyun(m.Aliyun)
	default:
		return resolveGeneric(m)
	}
}

func resolveGeneric(m model.MQTT) (ConnParams, bool, error) {
	merchant := SanitizeMerchant(m.Topic)
	report := strings.TrimSpace(m.ReportTopic)
	if strings.TrimSpace(m.Broker) == "" || merchant == "" || report == "" {
		return ConnParams{}, false, nil
	}
	if report == merchant {
		return ConnParams{}, false, fmt.Errorf("上报主题不能与订阅主题(短商户号)相同")
	}
	if strings.ContainsAny(report, "+#") {
		return ConnParams{}, false, fmt.Errorf("上报主题含通配符 +/#")
	}
	return ConnParams{
		BrokerURL:      brokerURL(m.Broker, m.Port),
		Username:       m.Username,
		Password:       m.Password,
		ClientID:       merchant,
		SubscribeTopic: merchant,
		ReportTopic:    report,
		Merchant:       merchant,
	}, true, nil
}

func resolveAliyun(a model.AliyunMQTT) (ConnParams, bool, error) {
	endpoint := strings.TrimSpace(a.Endpoint)
	instanceID := strings.TrimSpace(a.InstanceId)
	accessKey := strings.TrimSpace(a.AccessKey)
	secretKey := a.SecretKey
	groupID := strings.TrimSpace(a.GroupId)
	parent := strings.TrimSpace(a.ParentTopic)
	deviceID := strings.TrimSpace(a.DeviceId)
	down := strings.TrimSpace(a.DownSuffix)
	up := strings.TrimSpace(a.UpSuffix)
	if endpoint == "" || instanceID == "" || accessKey == "" || strings.TrimSpace(secretKey) == "" ||
		groupID == "" || parent == "" || deviceID == "" || down == "" || up == "" {
		return ConnParams{}, false, nil
	}
	port := a.Port
	if port <= 0 {
		port = 1883
	}
	sub, err := JoinParentTopic(parent, deviceID, down)
	if err != nil {
		return ConnParams{}, false, fmt.Errorf("订阅主题: %w", err)
	}
	rep, err := JoinParentTopic(parent, deviceID, up)
	if err != nil {
		return ConnParams{}, false, fmt.Errorf("上报主题: %w", err)
	}
	if sub == rep {
		return ConnParams{}, false, fmt.Errorf("上报主题不能与订阅主题相同(会造成自订阅回环)")
	}
	clientID := groupID + "@@@" + deviceID
	if len(clientID) > aliyunTopicMaxLen {
		return ConnParams{}, false, fmt.Errorf("ClientID 长度超过 %d: %s", aliyunTopicMaxLen, clientID)
	}
	return ConnParams{
		BrokerURL:      brokerURL(endpoint, port),
		Username:       "Signature|" + accessKey + "|" + instanceID,
		Password:       MacSignature(secretKey, clientID),
		ClientID:       clientID,
		SubscribeTopic: sub,
		ReportTopic:    rep,
		Merchant:       deviceID,
	}, true, nil
}

// PreviewTopics 供 UI 预览:拼主题与 ClientID;缺字段时返回可读错误。
func PreviewTopics(a model.AliyunMQTT) (sub, rep, clientID string, err error) {
	groupID := strings.TrimSpace(a.GroupId)
	deviceID := strings.TrimSpace(a.DeviceId)
	if groupID != "" && deviceID != "" {
		clientID = groupID + "@@@" + deviceID
	}
	parent := strings.TrimSpace(a.ParentTopic)
	down := strings.TrimSpace(a.DownSuffix)
	up := strings.TrimSpace(a.UpSuffix)
	if parent == "" || deviceID == "" {
		return "", "", clientID, fmt.Errorf("填写父主题与自定义 ID")
	}
	if down == "" && up == "" {
		return "", "", clientID, fmt.Errorf("填写下行/上行后缀")
	}
	if down != "" {
		sub, err = JoinParentTopic(parent, deviceID, down)
		if err != nil {
			return "", "", clientID, err
		}
	}
	if up != "" {
		rep, err = JoinParentTopic(parent, deviceID, up)
		if err != nil {
			return "", "", clientID, err
		}
	}
	return sub, rep, clientID, nil
}
