package main

import "os"

type Config struct {
	HTTPPort         string
	OrdersServiceURL string
}

func loadConfig() Config {
	return Config{
		HTTPPort:         getEnv("GATEWAY_HTTP_PORT", "8080"),
		OrdersServiceURL: getEnv("ORDERS_SERVICE_URL", "http://localhost:8081"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
