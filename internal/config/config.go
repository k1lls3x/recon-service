package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Host         string
	Port         int
	AllowOrigins []string
	LogLevel     string
	MaxUploadMB  int
	LogFile      string
}

func Load() Config {
	port, _ := strconv.Atoi(getenv("PORT", "8082"))
	mb, _ := strconv.Atoi(getenv("MAX_UPLOAD_MB", "256"))
	origins := strings.Split(getenv("ALLOW_ORIGINS", "*"), ",")
	return Config{
		Host:         getenv("HOST", "127.0.0.1"),
		Port:         port,
		AllowOrigins: origins,
		LogLevel:     getenv("LOG_LEVEL", "info"),
		MaxUploadMB:  mb,
		LogFile:      getenv("LOG_FILE", "logs/recon-service.log"),
	}
}

func (c Config) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
