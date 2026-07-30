package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"github.com/luuboon/pti-uteq-backend/api-gateaway-tdis/internal/config"
	"github.com/luuboon/pti-uteq-backend/api-gateaway-tdis/internal/middleware"
	"github.com/luuboon/pti-uteq-backend/api-gateaway-tdis/internal/proxy"
	"github.com/luuboon/pti-uteq-backend/api-gateaway-tdis/internal/token"
)
// @title API Gateway TDI UTEQ
// @version 1.0
// @description API Gateway unificada para el sistema de Formación Integral (TDIs) de la UTEQ.
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @description Escribe 'Bearer ' seguido de tu token JWT obtenido en el login.
func main() {
	// 1. Cargar configuración (falla rápido si falta algo crítico).
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuración inválida: %v", err)
	}

	// 2. Intentar conectar a Redis.
	//    Si falla, continuamos con rdb = nil y usaremos fallback en memoria.
	var rdb *redis.Client
	redisOpt, err := redis.ParseURL(cfg.RedisURL)
	if err == nil {
		client := redis.NewClient(redisOpt)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := client.Ping(ctx).Err(); err == nil {
			rdb = client
			log.Println("conectado a Redis con éxito")
		} else {
			log.Println(" Redis no disponible. Usando fallback en memoria para desarrollo local.")
		}
	} else {
		log.Println(" REDIS_URL inválida. Usando fallback en memoria para desarrollo local.")
	}

	// 3. Preparar el verificador de JWT (usa el secreto y soporta fallback en memoria si rdb es nil).
	verifier := token.NewVerifier(cfg.JWTSecret, rdb)

		// 4. Crear el reverse proxy hacia el servicio de usuarios.
	userProxy, err := proxy.New(cfg.Services["user"], "/api/users")
	if err != nil {
		log.Fatalf("no se pudo crear el proxy de user: %v", err)
	}

	// Crear el proxy hacia el servicio de TDIs (leyendo la URL de entorno o usando localhost:8082)
	tdiURL := os.Getenv("TDI_SERVICE_URL")
	if tdiURL == "" {
		tdiURL = "http://localhost:8082"
	}
	tdiProxy, err := proxy.New(tdiURL, "/api/tdi")
	if err != nil {
		log.Fatalf("no se pudo crear el proxy de tdi: %v", err)
	}


	// 5. Montar el router con Chi.
	r := chi.NewRouter()

	// Middlewares globales: logger, recuperación de panics, CORS y rate limit.
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS(cfg.AllowOrigin))
	r.Use(middleware.RateLimit(rdb, 100, time.Minute)) // 100 req/min por IP

		// Healthcheck público
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Swagger unificado público
	r.Get("/swagger", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(swaggerUIHTML))
	})
	r.Get("/swagger/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger", http.StatusMovedPermanently)
	})



		// Rutas públicas del servicio de usuarios: login y registro NO llevan Auth,
	// porque justamente sirven para obtener el token.
	r.Handle("/api/users/login", userProxy)
	r.Handle("/api/users/register", userProxy)
	
	// Exponer Swagger de los microservicios sin requerir autenticación
	r.Handle("/api/users/swagger/*", userProxy)
	r.Handle("/api/tdi/swagger/*", tdiProxy)

	r.Handle("/api/tdi/swagger/*", tdiProxy)
	r.Handle("/uploads/*", tdiProxy)


// @Summary Cerrar sesión (Logout)


		// Rutas protegidas: todo lo demás bajo /api/users/* y /api/tdi/* exige JWT válido.
		// Rutas protegidas: todo lo demás bajo /api/users/* y /api/tdi/* exige JWT válido.
	r.Group(func(protected chi.Router) {
		protected.Use(middleware.Auth(verifier))

		// NUEVO: Endpoint nativo para cerrar sesión
		protected.Post("/api/users/logout", makeLogoutHandler(verifier))

		protected.Handle("/api/users/*", userProxy)
		protected.Handle("/api/tdi/*", tdiProxy)
	})

	

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	addr := host + ":" + cfg.Port
	log.Printf("Gateway escuchando en %s", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("el servidor se detuvo: %v", err)
	}
}
// @Summary Cerrar sesión (Logout)
// @Description Invalida el token JWT actual agregándolo a la denylist de Redis o de memoria.
// @Tags auth
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {object} map[string]string "Sesión cerrada con éxito"
// @Failure 401 {object} map[string]string "Token inválido o no proporcionado"
// @Failure 500 {object} map[string]string "Error interno al invalidar"
// @Router /api/users/logout [post]
func makeLogoutHandler(v *token.Verifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"no autorizado"}`))
			return
		}

		// Calcular el tiempo de vida restante del token (TTL)
		ttl := time.Until(claims.ExpiresAt.Time)

		// Agregar el identificador único del token (JTI) a la denylist
		err := v.Revoke(r.Context(), claims.ID, ttl)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"no se pudo invalidar la sesión"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"sesión cerrada con éxito. El token ha sido invalidado"}`))
	}
}

const swaggerUIHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Documentación Unificada API - TDI UTEQ</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  <link rel="icon" type="image/png" href="https://unpkg.com/swagger-ui-dist@5/favicon-32x32.png" sizes="32x32" />
  <style>
    html { box-sizing: border-box; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        urls: [
          { url: "/api/users/swagger/doc.json", name: "Servicio de Usuarios y Autenticación" },
          { url: "/api/tdi/swagger/doc.json", name: "Servicio de TDIs y Evidencias" }
        ],
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        layout: "StandaloneLayout"
      });
    };
  </script>
</body>
</html>
`

