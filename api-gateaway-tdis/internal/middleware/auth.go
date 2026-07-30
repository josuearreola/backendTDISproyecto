package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/luuboon/pti-uteq-backend/api-gateaway-tdis/internal/token"
)

// ctxKey es un tipo privado para evitar colisiones en el context.
type ctxKey string

const claimsKey ctxKey = "claims"

// Auth crea un middleware que exige un JWT válido. Verifica firma, expiración
// y denylist (a través del Verifier). Si el token es válido, inyecta la
// identidad del usuario en headers para que el servicio downstream la reciba.
//
// Este es el corazón del patrón del documento: el Gateway valida una sola vez
// y los servicios confían en los headers X-User-Id y X-User-Role.
func Auth(v *token.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				writeUnauthorized(w, "falta el token Bearer")
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			claims, err := v.Verify(r.Context(), tokenStr)
			if err != nil {
				writeUnauthorized(w, "token inválido o revocado")
				return
			}

			// Guardamos los claims en el context por si el Gateway los necesita
			// (por ejemplo, para el endpoint de logout).
			ctx := context.WithValue(r.Context(), claimsKey, claims)

			// Inyectamos la identidad en headers. Importante: limpiamos primero
			// cualquier header de identidad que venga del cliente, para que nadie
			// pueda suplantar a otro usuario mandando estos headers a mano.
			r.Header.Del("X-User-Id")
			r.Header.Del("X-User-Role")
			r.Header.Set("X-User-Id", claims.UserID)
			r.Header.Set("X-User-Role", claims.Role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext recupera los claims inyectados por el middleware Auth.
func ClaimsFromContext(ctx context.Context) (*token.Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*token.Claims)
	return c, ok
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}
