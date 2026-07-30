package config

import (
	"fmt"
	"os"
	"strings"
)

// Config agrupa toda la configuración del Gateway, leída de variables de entorno.
// En Railway estas variables se definen en el panel del servicio.
type Config struct {
	Port        string            // Puerto donde escucha el Gateway (Railway inyecta PORT)
	JWTSecret   string            // Secreto para firmar/verificar los JWT (HS256)
	RedisURL    string            // URL de conexión a Redis (Railway: REDIS_URL)
	Services    map[string]string // Mapa nombre-de-servicio -> URL base (para el proxy)
	AllowOrigin string            // Origen permitido para CORS (el frontend Angular)
}

// Load lee la configuración del entorno y valida lo mínimo indispensable.
// Devuelve error si falta algo crítico, para fallar rápido al arrancar.
func Load() (*Config, error) {
	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		RedisURL:    os.Getenv("REDIS_URL"),
		AllowOrigin: getEnv("ALLOW_ORIGIN", "*"),
		Services:    map[string]string{},
	}

	// URL base del servicio de usuarios/auth. En Railway, cada servicio
	// expone una URL interna; aquí la recibimos por variable de entorno.
	if userURL := os.Getenv("USER_SERVICE_URL"); userURL != "" {
		cfg.Services["user"] = strings.TrimRight(userURL, "/")
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("falta JWT_SECRET")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("falta REDIS_URL")
	}
	if cfg.Services["user"] == "" {
		return nil, fmt.Errorf("falta USER_SERVICE_URL")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
