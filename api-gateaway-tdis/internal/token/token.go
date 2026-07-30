package token

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// Claims son los datos que viajan dentro del JWT.
// Siguen el patrón del documento: el Gateway es el único que los lee y verifica,
// y luego pasa la identidad a los servicios por headers.
type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"` // estudiante, coordinador, secretario, administrador
	jwt.RegisteredClaims
}

// Verifier verifica tokens y consulta la denylist en Redis.
// La denylist permite revocar un JWT antes de que expire: cuando se hace
// logout o se revoca una sesión, el jti (ID del token) se guarda en Redis
// con un TTL igual al tiempo que le quedaba de vida al token.
type Verifier struct {
	secret        []byte
	rdb           *redis.Client
	localDenylist sync.Map // fallback local en memoria: jti (string) -> expiración (time.Time)
}

func NewVerifier(secret string, rdb *redis.Client) *Verifier {
	return &Verifier{secret: []byte(secret), rdb: rdb}
}

// denylistKey construye la llave en Redis para un token revocado.
func denylistKey(jti string) string {
	return "denylist:" + jti
}

var (
	ErrInvalidToken = errors.New("token inválido")
	ErrRevokedToken = errors.New("token revocado")
)

// Verify valida la firma y la expiración del token, y comprueba que no esté
// en la denylist. Devuelve los claims si todo está correcto.
func (v *Verifier) Verify(ctx context.Context, tokenStr string) (*Claims, error) {
	claims := &Claims{}

	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		// Aseguramos que el algoritmo de firma sea el esperado (HS256).
		// Esto evita el ataque de "algorithm confusion".
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("algoritmo de firma inesperado: %v", t.Header["alg"])
		}
		return v.secret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}

	// El jti (JWT ID) identifica de forma única al token. Lo usamos como
	// llave en la denylist. Si el token no trae jti, no podemos revocarlo
	// de forma granular, así que lo tratamos como inválido por seguridad.
	if claims.ID == "" {
		return nil, ErrInvalidToken
	}

	// Si Redis está disponible, verificamos ahí
	if v.rdb != nil {
		revoked, err := v.rdb.Exists(ctx, denylistKey(claims.ID)).Result()
		if err != nil {
			// Si Redis no responde, fallamos cerrado (negamos el acceso) en lugar
			// de abierto, porque no podemos garantizar que el token no esté revocado.
			return nil, fmt.Errorf("no se pudo consultar la denylist: %w", err)
		}
		if revoked > 0 {
			return nil, ErrRevokedToken
		}
	} else {
		// Fallback local en memoria
		if expVal, ok := v.localDenylist.Load(claims.ID); ok {
			if exp, ok := expVal.(time.Time); ok {
				if time.Now().Before(exp) {
					return nil, ErrRevokedToken
				}
				// Limpieza si ya venció la expiración local
				v.localDenylist.Delete(claims.ID)
			}
		}
	}

	return claims, nil
}

// Revoke agrega un token a la denylist. ttl debe ser el tiempo que le queda
// de vida al token; así Redis lo elimina solo cuando ya habría expirado y no
// acumulamos basura. Esta función la usará el endpoint de logout.
func (v *Verifier) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		// El token ya expiró; no hace falta guardarlo.
		return nil
	}

	if v.rdb != nil {
		return v.rdb.Set(ctx, denylistKey(jti), "1", ttl).Err()
	}

	// Fallback local en memoria
	v.localDenylist.Store(jti, time.Now().Add(ttl))
	return nil
}
