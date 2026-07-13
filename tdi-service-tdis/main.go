package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9" // <-- NUEVO
	"github.com/luuboon/pti-uteq-backend/tdi-service-tdis/internal/config"
	"github.com/luuboon/pti-uteq-backend/tdi-service-tdis/internal/db"
	"github.com/luuboon/pti-uteq-backend/tdi-service-tdis/internal/handlers"

	_ "github.com/luuboon/pti-uteq-backend/tdi-service-tdis/docs"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)



// @title TDI Service API
// @version 1.0
// @description Servicio de Gestión de Actividades e Integración de TDIs.

// @BasePath /
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @description Escribe 'Bearer ' seguido de tu token JWT para autenticarte.
func main() {
	// 1. Cargar configuración
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuración inválida: %v", err)
	}

		// 2. Conectar a Postgres (Neon)
	ctx := context.Background()
	store, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("error al conectar con base de datos: %v", err)
	}
	defer store.Close()

	// 2.5 Intentar conectar a Redis (si falla, continuará con rdb = nil en local)
	var rdb *redis.Client
	if cfg.RedisURL != "" {
		redisOpt, err := redis.ParseURL(cfg.RedisURL)
		if err == nil {
			client := redis.NewClient(redisOpt)
			pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := client.Ping(pingCtx).Err(); err == nil {
				rdb = client
				log.Println("[TDI-SERVICE ] Conectado a Redis con éxito")
			} else {
				log.Println("[TDI-SERVICE ] Redis no disponible. Continuando en modo desarrollo sin colas.")
			}
		} else {
			log.Println("[TDI-SERVICE ] REDIS_URL inválida. Continuando en modo desarrollo sin colas.")
		}
	} else {
		log.Println("[TDI-SERVICE ] REDIS_URL no configurada. Continuando en modo desarrollo sin colas.")
	}

	h := &handlers.Handler{
		Store: store,
		RDB:   rdb, // <-- INYECTAR REDIS CLIENT
	}


	// 3. Configurar el router
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	// Healthcheck público
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// 4. Registrar Rutas del Catálogo (CRUD Completo y Selección)
	// Nota: El Gateway valida los tokens e inyecta los headers. Aquí las rutas son limpias.
		r.Get("/catalogo", h.ListCatalogHandler)
	r.Get("/catalogo/{id}", h.GetCatalogItemHandler)
	r.Post("/catalogo", h.CreateCatalogItemHandler)
	r.Put("/catalogo/{id}", h.UpdateCatalogItemHandler)
	r.Delete("/catalogo/{id}", h.DeleteCatalogItemHandler)
	r.Post("/registro/seleccionar", h.SeleccionarTDIHandler)
	r.Post("/registro/{id}/subir-evidencia", h.SubirEvidenciaHandler)
	r.Get("/registro/mis-registros", h.GetAlumnoRegistrosHandler)
	r.Get("/alumnos/progreso", h.GetAlumnoProgresoHandler)

	// Servidor estático para la carpeta uploads
	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "uploads"))
	fileServer(r, "/uploads", filesDir)

		// Swagger UI
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("doc.json"),
	))


	// 5. Configurar el host local para evitar alertas de Windows Firewall

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	addr := host + ":" + cfg.Port

	log.Printf("TDI-Service escuchando en %s", addr)
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
func fileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer no permite parámetros URL.")
	}

	if path != "/" && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	fs := http.StripPrefix(path, http.FileServer(root))

	r.Get(path+"/*", func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	})
}
