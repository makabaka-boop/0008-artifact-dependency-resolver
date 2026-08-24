package config

import (
	"errors"
	"os"
)

// Config 汇总服务运行所需的全部配置。
type Config struct {
	ListenAddr string
	DBPath     string
	LogLevel   string
}

// Load 从环境变量读取配置，未设置时使用默认值。
func Load() Config {
	return Config{
		ListenAddr: getenv("LISTEN_ADDR", ":8080"),
		DBPath:     getenv("DB_PATH", "./data/app.db"),
		LogLevel:   getenv("LOG_LEVEL", "info"),
	}
}

// Validate 校验必填配置的合法性。
func (c Config) Validate() error {
	if c.ListenAddr == "" {
		return errors.New("LISTEN_ADDR must not be empty")
	}
	if c.DBPath == "" {
		return errors.New("DB_PATH must not be empty")
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
