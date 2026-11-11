package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
)

// AuditQueue la consume tdi-service y la persiste en audit_log.
const AuditQueue = "audit_queue"

// Audit encola un registro por cada petición que modifica estado (POST/PUT/PATCH/DELETE).
// Debe montarse antes de Auth: limpia los headers de identidad que mande el cliente
// y, al terminar, lee los que Auth inyectó (el mapa de headers es el mismo objeto).
// ponytail: si Redis no está, no se audita a nivel gateway (los eventos de dominio sí se guardan).
func Audit(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Del("X-User-Id")
			r.Header.Del("X-User-Role")

			if rdb == nil || r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			entry := map[string]any{
				"origen": "GATEWAY",
				"accion": r.Method + " " + r.URL.Path,
				"status": ww.Status(),
				"ip":     clientIP(r),
				"fecha":  time.Now().UTC(),
			}
			if id := r.Header.Get("X-User-Id"); id != "" {
				entry["usuario_id"] = id
				entry["rol"] = r.Header.Get("X-User-Role")
			}
			payload, _ := json.Marshal(entry)

			// Contexto propio: el de la petición ya puede estar cancelado.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := rdb.LPush(ctx, AuditQueue, payload).Err(); err != nil {
				log.Printf("[GATEWAY] no se pudo encolar auditoría: %v", err)
			}
		})
	}
}

func clientIP(r *http.Request) string {
	// Render pone la IP real del cliente en X-Forwarded-For.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(ip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
