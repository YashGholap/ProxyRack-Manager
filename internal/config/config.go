package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	//server
	Port int

	//ProxyRack
	ProxyrackBaseURL string
	ProxyrackAPIKey  string

	//pool
	MaxThreads     int
	RequestTimeout time.Duration

	// Queue depth limit — if this many requests are already waiting
	// for a slot, new arrivals get 503 instead of queuing forever
	MaxQueueDepth int
}

func Load() (*Config, error) {
	apiKey := os.Getenv("PROXYRACK_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("PROXYRACK_API_KEY is required")
	}

	cfg := &Config{
		Port:             getEnvInt("PORT", 8080),
		ProxyrackBaseURL: getEnvStr("PROXYRACK_BASE_URL", "https://scrape.proxyrack.net/api/v1/"),
		ProxyrackAPIKey:  apiKey,
		MaxThreads:       getEnvInt("MAX_THREADS", 25),
		RequestTimeout:   time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 120)) * time.Second,
		MaxQueueDepth:    getEnvInt("MAX_QUEUE_DEPTH", 100),
	}

	return cfg, nil
}

func getEnvStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
