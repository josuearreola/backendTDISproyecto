package config

import (
	"fmt"
	"os"
)

// Config del servicio User/Auth.
type Config struct {
	Port        string // Puerto (Railway inyecta PORT)
	DatabaseURL string // Connection string de Neon (Postgres)
	JWTSecret   string // MISMO secreto que el Gateway, para que pueda verificar
	JWTTTL      int    // Vigencia del token en minutos
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:        getEnv("PORT", "8081"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		JWTTTL:      getEnvInt("JWT_TTL_MINUTES", 60),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("falta DATABASE_URL")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("falta JWT_SECRET")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
