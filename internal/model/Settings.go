package model

// MQTT 通道提供方常量。
const (
	MQTTProviderGeneric = "generic" // 自建/通用 MQTT(用户名密码)
	MQTTProviderAliyun  = "aliyun"  // 阿里云云消息队列 MQTT 版
	MQTTProviderIot     = "iot"     // 阿里云物联网平台(一机一密)
)

// MQTT 保存云端 MQTT 连接参数(唯一云端通道;Provider 三选一)。
type MQTT struct {
	Enabled  bool   `json:"enabled"`  // 是否启用 MQTT 连接
	Provider string `json:"provider"` // "generic" | "aliyun" | "iot";空=generic(旧配置兼容)

	// 自建/通用
	Broker      string `json:"broker"`      // broker 地址(可含 tcp:// 与端口)
	Port        int    `json:"port"`        // 端口(默认 1883)
	Username    string `json:"username"`    // 用户名
	Password    string `json:"password"`    // 密码
	Topic       string `json:"topic"`       // 聪明付短商户号(订阅主题;仅字母数字下划线)
	ReportTopic string `json:"reportTopic"` // 上报(发布)主题;启用自建时必填,可含 `/`

	// 阿里云云消息队列 MQTT 版
	Aliyun AliyunMQTT `json:"aliyun"`

	// 阿里云物联网平台(一机一密)
	Iot IoTMQTT `json:"iot"`
}

// AliyunMQTT 是云消息队列 MQTT 版连接与主题拼接配置。
type AliyunMQTT struct {
	Endpoint    string `json:"endpoint"`    // 接入点域名(手填)
	Port        int    `json:"port"`        // 默认 1883
	InstanceId  string `json:"instanceId"`  // 实例 ID(签名 Username)
	AccessKey   string `json:"accessKey"`   // RAM AccessKey ID
	SecretKey   string `json:"secretKey"`   // RAM AccessKey Secret
	GroupId     string `json:"groupId"`     // 如 GID_xxx;ClientID=GroupId@@@DeviceId
	ParentTopic string `json:"parentTopic"` // 父主题(控制台已建一级 Topic)
	DeviceId    string `json:"deviceId"`    // 自定义 ID/商户号;=merchant;=ClientID 后缀
	DownSuffix  string `json:"downSuffix"`  // 下行后缀(本机 Subscribe)
	UpSuffix    string `json:"upSuffix"`    // 上行后缀(本机 Publish)
}

// IoTMQTT 是物联网平台一机一密连接配置。
// 设备列表拉取为预留能力;当前用手填 ProductKey/DeviceName/DeviceSecret。
// 主题自动拼接为 /{ProductKey}/{DeviceName}/user/{后缀}(官方自定义 Topic 前缀固定含 /user)。
type IoTMQTT struct {
	ProductKey   string `json:"productKey"`
	DeviceName   string `json:"deviceName"`
	DeviceSecret string `json:"deviceSecret"`
	RegionId     string `json:"regionId"`   // 可选;Endpoint 为空时拼接入点,如 cn-shanghai
	Endpoint     string `json:"endpoint"`   // 可选;优先用手填,空则 {ProductKey}.iot-as-mqtt.{RegionId}.aliyuncs.com
	Port         int    `json:"port"`       // 默认 1883(TCP securemode=3)
	DownSuffix   string `json:"downSuffix"` // 下行后缀,如 cmd → /pk/dn/user/cmd
	UpSuffix     string `json:"upSuffix"`   // 上行后缀,如 report → /pk/dn/user/report
}

// DocServer 是本地(局域网)只读「在线 API 接口文档」服务的配置。
type DocServer struct {
	Enabled bool `json:"enabled"` // 是否启动接口文档服务
	Port    int  `json:"port"`    // 监听端口(默认 8080)
}

// 打印历史保留天数默认与合法范围。
const (
	DefaultJobHistoryDays = 7
	MinJobHistoryDays     = 1
	MaxJobHistoryDays     = 365
)

// Settings 是打印服务的全局设置。
type Settings struct {
	ServiceName         string    `json:"serviceName"`
	NotifyDisabled      bool      `json:"notifyDisabled"`      // 关闭 Windows 系统通知;反向字段,零值=启用
	YunheCompatDisabled bool      `json:"yunheCompatDisabled"` // 关闭云盒兼容模式;反向字段,零值=兼容开(C1–C4)
	JobHistoryDays      int       `json:"jobHistoryDays"`      // 已完成/失败任务保留天数;未完成不受限
	MQTT                MQTT      `json:"mqtt"`
	DocServer           DocServer `json:"docServer"`
}

// YunheCompat 返回是否启用云盒兼容模式(C1–C4)。零值/缺字段为启用。
func (s Settings) YunheCompat() bool {
	return !s.YunheCompatDisabled
}

// DefaultSettings 返回默认设置。
func DefaultSettings() Settings {
	return Settings{
		ServiceName:    "票据打印服务",
		JobHistoryDays: DefaultJobHistoryDays,
		MQTT: MQTT{
			Port:     1883,
			Provider: MQTTProviderGeneric,
			Aliyun:   AliyunMQTT{Port: 1883},
			Iot:      IoTMQTT{Port: 1883, RegionId: "cn-shanghai"},
		},
		DocServer: DocServer{Enabled: true, Port: 8080},
	}
}

// ClampJobHistoryDays 将天数钳制到 1–365;非法(≤0 或超上限)回退默认 7。
func ClampJobHistoryDays(n int) int {
	if n < MinJobHistoryDays || n > MaxJobHistoryDays {
		return DefaultJobHistoryDays
	}
	return n
}

// EffectiveProvider 返回规范化 Provider(空视为 generic)。
func (m MQTT) EffectiveProvider() string {
	switch m.Provider {
	case MQTTProviderAliyun:
		return MQTTProviderAliyun
	case MQTTProviderIot:
		return MQTTProviderIot
	default:
		return MQTTProviderGeneric
	}
}
