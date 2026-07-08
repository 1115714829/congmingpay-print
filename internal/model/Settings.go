package model

// MQTT 保存云端 MQTT 连接参数(唯一云端通道)。
type MQTT struct {
	Enabled     bool   `json:"enabled"`     // 是否启用 MQTT 连接
	Broker      string `json:"broker"`      // broker 地址(可含 tcp:// 与端口)
	Port        int    `json:"port"`        // 端口(默认 1883)
	Username    string `json:"username"`    // 用户名
	Password    string `json:"password"`    // 密码
	Topic       string `json:"topic"`       // 聪明付短商户号(订阅主题;仅字母数字下划线)
	ReportTopic string `json:"reportTopic"` // 上报(发布)主题:回执/结果/状态全部发往此主题;启用 MQTT 时必填,可含 `/`
}

// DocServer 是本地(局域网)只读「在线 API 接口文档」服务的配置。
type DocServer struct {
	Enabled bool `json:"enabled"` // 是否启动接口文档服务
	Port    int  `json:"port"`    // 监听端口(默认 8080)
}

// Settings 是打印服务的全局设置。
type Settings struct {
	ServiceName string    `json:"serviceName"`
	MQTT        MQTT      `json:"mqtt"`
	DocServer   DocServer `json:"docServer"`
}

// DefaultSettings 返回默认设置。
func DefaultSettings() Settings {
	return Settings{
		ServiceName: "票据打印服务",
		MQTT:        MQTT{Port: 1883},
		DocServer:   DocServer{Enabled: true, Port: 8080},
	}
}
