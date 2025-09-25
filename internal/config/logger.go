package config

import (
	"os"
	"time"

	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// SetupLogger: человекочитаемый вывод в консоль + файл с ротацией.
func SetupLogger(cfg Config) zerolog.Logger {
	_ = os.MkdirAll("logs", 0o755)

	console := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	file := &lumberjack.Logger{
		Filename:   cfg.LogFile,
		MaxSize:    50,  // MB
		MaxBackups: 5,
		MaxAge:     30,  // days
		Compress:   true,
	}

	mw := zerolog.MultiLevelWriter(console, file)
	lvl, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	logger := zerolog.New(mw).With().Timestamp().Logger()
	log.Logger = logger
	return logger
}
