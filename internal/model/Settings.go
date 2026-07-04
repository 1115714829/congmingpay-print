package model

// MQTT 保存云端 MQTT 连接参数(当前阶段仅存储,不实际连接)。
type MQTT struct {
	Broker   string `json:"broker"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Topic    string `json:"topic"`
}

// Settings 是打印服务的全局设置。
type Settings struct {
	ServiceName  string `json:"serviceName"`
	ListenPort   string `json:"listenPort"`
	APIPort      string `json:"apiPort"`      // 本地 HTTP API 端口
	DefaultPaper int    `json:"defaultPaper"` // 58 或 80
	AutoRetry    bool   `json:"autoRetry"`    // 新建打印机默认是否重打
	MQTT         MQTT   `json:"mqtt"`
}

// DefaultSettings 返回默认设置。
func DefaultSettings() Settings {
	return Settings{
		ServiceName:  "票据打印服务",
		ListenPort:   "9100",
		APIPort:      "8080",
		DefaultPaper: 80,
		AutoRetry:    true,
		MQTT:         MQTT{Port: 1883},
	}
}
