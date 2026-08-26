// Package config 负责 web 后端的环境变量配置读取。
package config

import (
	"os"
	"strconv"
)

// Config 是 web 后端的运行配置(全部来自环境变量,带默认值)。
type Config struct {
	Port         int    // WEB_PORT,HTTP 监听端口,默认 9000
	DataDir      string // WEB_DATA,数据目录(web.db、log),默认 ./data
	JWTSecret    string // JWT_SECRET,HS256 密钥;缺省用内置值(启动强提醒)
	AdminUser    string // ADMIN_USERNAME,首次启动种子管理员用户名,默认 admin
	AdminPass    string // ADMIN_PASSWORD,首次启动种子管理员密码,默认 admin123
}

// DefaultJWTSecret 是未设置 JWT_SECRET 时的内置密钥(仅开发期可用,启动日志强提醒)。
const DefaultJWTSecret = "congmingpay-web-default-secret-change-me"

// Load 从环境变量加载配置。
func Load() *Config {
	return &Config{
		Port:      envInt("WEB_PORT", 9000),
		DataDir:   envStr("WEB_DATA", "data"),
		JWTSecret: envStr("JWT_SECRET", DefaultJWTSecret),
		AdminUser: envStr("ADMIN_USERNAME", "admin"),
		AdminPass: envStr("ADMIN_PASSWORD", "admin123"),
	}
}

func envStr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
