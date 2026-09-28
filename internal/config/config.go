package config

import (
	"os"
	"time"
)

type Config struct {
	MySQLDSN string
	HTTPAddr string
	HoldTTL  time.Duration
}

func FromEnv() Config {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root:ottodot@tcp(127.0.0.1:3306)/ottodot?parseTime=true&loc=UTC&timeout=10s"
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	ttl := 8 * time.Minute
	if v := os.Getenv("HOLD_TTL"); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			ttl = parsed
		}
	}
	return Config{MySQLDSN: dsn, HTTPAddr: addr, HoldTTL: ttl}
}
