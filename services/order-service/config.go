package main

import (
	"os"
	"strings"
)

type Config struct {
	Port            string
	DatabaseURL     string
	RedisURL        string
	UserServiceAddr string
	LogLevel        string
	CertFile        string
	KeyFile         string
	CAFile          string
	VaultAddr       string
	VaultToken      string
	VaultDBRole     string
}

func (c Config) UseVault() bool {
	return c.VaultAddr != "" && c.VaultToken != ""
}

func LoadConfig() Config {
	return Config{
		Port:            getEnv("PORT", "50052"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/orders?sslmode=disable"),
		RedisURL:        getEnv("REDIS_URL", "redis://localhost:6379/0"),
		UserServiceAddr: getEnv("USER_SERVICE_ADDR", "localhost:50051"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		CertFile:        getEnv("TLS_CERT_FILE", "certs/order-service/cert.pem"),
		KeyFile:         getEnv("TLS_KEY_FILE", "certs/order-service/key.pem"),
		CAFile:          getEnv("TLS_CA_FILE", "certs/ca/ca.pem"),
		VaultAddr:       getEnv("VAULT_ADDR", ""),
		VaultToken:      loadVaultToken(),
		VaultDBRole:     getEnv("VAULT_DB_ROLE", "order-service"),
	}
}

func loadVaultToken() string {
	if token := os.Getenv("VAULT_TOKEN"); token != "" {
		return token
	}
	if path := os.Getenv("VAULT_TOKEN_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
