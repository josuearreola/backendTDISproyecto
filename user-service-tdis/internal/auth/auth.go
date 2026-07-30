package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/luuboon/pti-uteq-backend/user-service-tdis/internal/models"
)

// HashPassword genera un hash bcrypt de la contraseña. bcrypt incluye su propio
// salt y es deliberadamente lento, lo que dificulta los ataques de fuerza bruta.
func HashPassword(plain string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword compara una contraseña en texto plano contra su hash.
// Devuelve nil si coinciden.
func CheckPassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}

// Claims es la estructura del JWT. DEBE coincidir con la del Gateway para que
// este pueda verificar los tokens: mismos nombres de campo JSON (uid, role).
type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// Issuer emite tokens JWT firmados con HS256.
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

func NewIssuer(secret string, ttlMinutes int) *Issuer {
	return &Issuer{
		secret: []byte(secret),
		ttl:    time.Duration(ttlMinutes) * time.Minute,
	}
}

// Issue crea un JWT para el usuario dado. Incluye un jti (ID único del token)
// que es justo lo que el Gateway usa como llave en la denylist para poder
// revocar este token específico más adelante.
func (i *Issuer) Issue(u *models.User) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: u.ID,
		Role:   string(u.Role),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(), // jti: identificador único del token
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(i.secret)
}
