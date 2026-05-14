package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port        string
	DatabaseURL string
	LogLevel    string
	CertFile    string
	KeyFile     string
	CAFile      string
	VaultAddr   string
	VaultToken  string
	VaultDBRole string

	OTLPEndpoint string
	MetricsPort  string

	DBPoolMaxConns         int32
	DBPoolMinConns         int32
	DBPoolMaxConnLifetime  time.Duration
	DBPoolMaxConnIdleTime  time.Duration
	DBPoolHealthCheckPeriod time.Duration
	DBQueryTimeout         time.Duration
}

func (c Config) UseVault() bool {
	return c.VaultAddr != "" && c.VaultToken != ""
}

func LoadConfig() Config {
	return Config{
		Port:        getEnv("PORT", "50051"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/users?sslmode=disable"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		CertFile:    getEnv("TLS_CERT_FILE", "certs/user-service/cert.pem"),
		KeyFile:     getEnv("TLS_KEY_FILE", "certs/user-service/key.pem"),
		CAFile:      getEnv("TLS_CA_FILE", "certs/ca/ca.pem"),
		VaultAddr:   getEnv("VAULT_ADDR", ""),
		VaultToken:  loadVaultToken(),
		VaultDBRole: getEnv("VAULT_DB_ROLE", "user-service"),

		OTLPEndpoint: getEnv("OTLP_ENDPOINT", "jaeger:4317"),
		MetricsPort:  getEnv("METRICS_PORT", "9091"),

		DBPoolMaxConns:          int32(getEnvInt("DB_POOL_MAX_CONNS", 25)),
		DBPoolMinConns:          int32(getEnvInt("DB_POOL_MIN_CONNS", 5)),
		DBPoolMaxConnLifetime:   getEnvDuration("DB_POOL_MAX_CONN_LIFETIME", 30*time.Minute),
		DBPoolMaxConnIdleTime:   getEnvDuration("DB_POOL_MAX_CONN_IDLE_TIME", 5*time.Minute),
		DBPoolHealthCheckPeriod: getEnvDuration("DB_POOL_HEALTH_CHECK_PERIOD", 30*time.Second),
		DBQueryTimeout:          getEnvDuration("DB_QUERY_TIMEOUT", 10*time.Second),
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

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		d, err := time.ParseDuration(val)
		if err == nil {
			return d
		}
	}
	return fallback
}
