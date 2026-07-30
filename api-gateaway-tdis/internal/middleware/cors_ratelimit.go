package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	localLimitMu sync.Mutex
	localLimits  = make(map[string][]time.Time)
)

// CORS permite que el frontend Angular (en otro dominio) llame al Gateway.
// allowOrigin debe ser el dominio del frontend en producción (no "*").
func CORS(allowOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

			// Las peticiones OPTIONS (preflight) se responden de inmediato.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit limita el número de peticiones por IP usando contadores en Redis.
// Implementa una ventana fija simple: cuenta peticiones por IP durante 'window'
// y rechaza con 429 si se supera 'limit'. Es el uso de Redis que documentamos
// para proteger endpoints como el registro de TDIs.
func RateLimit(rdb *redis.Client, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)

			// Si Redis no está disponible, usamos fallback en memoria local
			if rdb == nil {
				localLimitMu.Lock()
				now := time.Now()
				cutoff := now.Add(-window)

				// Filtrar peticiones fuera de la ventana
				timestamps := localLimits[ip]
				var active []time.Time
				for _, t := range timestamps {
					if t.After(cutoff) {
						active = append(active, t)
					}
				}

				if len(active) >= limit {
					localLimitMu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					w.Write([]byte(`{"error":"demasiadas peticiones, intenta más tarde"}`))
					return
				}

				localLimits[ip] = append(active, now)
				localLimitMu.Unlock()

				next.ServeHTTP(w, r)
				return
			}

			key := "ratelimit:" + ip

			// INCR crea la llave si no existe y la incrementa de forma atómica.
			count, err := rdb.Incr(r.Context(), key).Result()
			if err != nil {
				// Si Redis falla, dejamos pasar la petición (fail-open) para no
				// tumbar el servicio entero por el rate limiter. Es una decisión
				// de diseño: priorizamos disponibilidad sobre el límite estricto.
				next.ServeHTTP(w, r)
				return
			}

			// En la primera petición de la ventana, fijamos la expiración.
			if count == 1 {
				rdb.Expire(r.Context(), key, window)
			}

			if count > int64(limit) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"demasiadas peticiones, intenta más tarde"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	// Railway y otros proxies ponen la IP real en X-Forwarded-For.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}
