package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/luuboon/pti-uteq-backend/user-service-tdis/internal/auth"
	"github.com/luuboon/pti-uteq-backend/user-service-tdis/internal/config"
	"github.com/luuboon/pti-uteq-backend/user-service-tdis/internal/db"
	"github.com/luuboon/pti-uteq-backend/user-service-tdis/internal/handlers"

	_ "github.com/luuboon/pti-uteq-backend/user-service-tdis/docs"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// @title User Service API
// @version 1.0
// @description Microservicio de autenticación y gestión de usuarios.
// @BasePath /
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @description Escribe 'Bearer ' seguido de tu token JWT para autenticarte.
func main() {
	// 1. Configuración.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuración inválida: %v", err)
	}

	// 2. Conexión a Neon (Postgres) y migración de la tabla.
	ctx := context.Background()
	store, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("error de base de datos: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		log.Fatalf("no se pudo migrar: %v", err)
	}
	log.Println("conectado a Neon y tabla lista")

	// 3. Emisor de JWT (mismo secreto que el Gateway).
	issuer := auth.NewIssuer(cfg.JWTSecret, cfg.JWTTTL)

	h := &handlers.Handler{Store: store, Issuer: issuer}

	// 4. Router.
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Rutas. Nota: NO llevan el prefijo /api/users porque el Gateway ya lo
	// quita antes de reenviar (stripPrefix). Aquí las rutas son "limpias".
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Get("/me", h.Me)
	r.Post("/alumnos/completar-perfil", h.CompletarPerfilAlumno)
	r.Post("/administrativos/completar-perfil", h.CompletarPerfilAdministrativo)
	r.Post("/creadores/completar-perfil", h.CompletarPerfilCreador)

	r.Delete("/users/{id}", h.DeleteUserHandler)

	r.Get("/notificaciones", h.GetNotificationsHandler)
	r.Put("/notificaciones/{id}/leer", h.MarkNotificationReadHandler)



		// Swagger UI
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("doc.json"),
	))



	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	addr := host + ":" + cfg.Port

	log.Printf("User/Auth escuchando en %s", addr)
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

