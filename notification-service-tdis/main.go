// notification-service-tdis: dueño único de la tabla notificaciones.
// Los productores (tdi-service, validation-worker) hacen LPUSH a "notification_queue"
// con {"usuario_id","titulo","mensaje"}; este servicio las persiste y las sirve.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const queue = "notification_queue"

type notificacion struct {
	ID        string    `json:"id"`
	UsuarioID string    `json:"usuario_id"`
	Titulo    string    `json:"titulo"`
	Mensaje   string    `json:"mensaje"`
	Leida     bool      `json:"leida"`
	Fecha     time.Time `json:"fecha"`
}

type server struct {
	pool *pgxpool.Pool
}

func main() {
	dbURL, redisURL := os.Getenv("DATABASE_URL"), os.Getenv("REDIS_URL")
	if dbURL == "" || redisURL == "" {
		log.Fatal("faltan DATABASE_URL o REDIS_URL")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("no se pudo crear el pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("no se pudo conectar a la base de datos: %v", err)
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("REDIS_URL inválida: %v", err)
	}
	rdb := redis.NewClient(opt)

	s := &server{pool: pool}
	go s.consume(ctx, rdb)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/notificaciones", s.list)
	r.Get("/notificaciones/no-leidas", s.unreadCount)
	r.Put("/notificaciones/leer-todas", s.markAllRead)
	r.Put("/notificaciones/{id}/leer", s.markRead)

	srv := &http.Server{Addr: "0.0.0.0:" + port, Handler: r, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	log.Printf("[NOTIF-SERVICE] escuchando en :%s", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("el servidor se detuvo: %v", err)
	}
}

// consume drena la cola. Si el INSERT falla, el mensaje se devuelve a la cola
// para no perderlo (y se espera para no girar en caliente si la BD está caída).
func (s *server) consume(ctx context.Context, rdb *redis.Client) {
	log.Printf("[NOTIF-SERVICE] escuchando la cola '%s'", queue)
	for ctx.Err() == nil {
		res, err := rdb.BRPop(ctx, 10*time.Second, queue).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("[NOTIF-SERVICE] error leyendo Redis: %v", err)
				time.Sleep(5 * time.Second)
			}
			continue
		}

		var n notificacion
		if err := json.Unmarshal([]byte(res[1]), &n); err != nil || n.UsuarioID == "" || n.Titulo == "" {
			log.Printf("[NOTIF-SERVICE] mensaje descartado (inválido): %s", res[1])
			continue
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO notificaciones (usuario_id, titulo, mensaje) VALUES ($1, $2, $3)`,
			n.UsuarioID, n.Titulo, n.Mensaje); err != nil {
			// Dato rechazado por la BD (UUID inválido, usuario borrado, título largo):
			// reintentar no sirve y bloquearía la cola para todos, así que se descarta.
			var pg *pgconn.PgError
			if errors.As(err, &pg) && (strings.HasPrefix(pg.Code, "22") || strings.HasPrefix(pg.Code, "23")) {
				log.Printf("[NOTIF-SERVICE] notificación descartada (la BD rechazó el dato): %v — %s", err, res[1])
				continue
			}
			log.Printf("[NOTIF-SERVICE] error guardando notificación, se reencola: %v", err)
			rdb.RPush(context.Background(), queue, res[1]) // vuelve al frente de la cola
			time.Sleep(5 * time.Second)
		}
	}
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "no autorizado")
		return
	}
	// ?solo_no_leidas=true para la bandeja de pendientes.
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, usuario_id, titulo, COALESCE(mensaje, ''), leida, fecha
		FROM notificaciones
		WHERE usuario_id = $1 AND ($2 = FALSE OR leida = FALSE)
		ORDER BY fecha DESC
		LIMIT 200
	`, userID, r.URL.Query().Get("solo_no_leidas") == "true")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error al obtener notificaciones")
		return
	}
	defer rows.Close()

	out := []notificacion{}
	for rows.Next() {
		var n notificacion
		if err := rows.Scan(&n.ID, &n.UsuarioID, &n.Titulo, &n.Mensaje, &n.Leida, &n.Fecha); err != nil {
			writeError(w, http.StatusInternalServerError, "error al leer notificaciones")
			return
		}
		out = append(out, n)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) unreadCount(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "no autorizado")
		return
	}
	var total int
	if err := s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM notificaciones WHERE usuario_id = $1 AND leida = FALSE`, userID).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "error al contar notificaciones")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"no_leidas": total})
}

func (s *server) markRead(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "no autorizado")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE notificaciones SET leida = TRUE WHERE id = $1 AND usuario_id = $2`,
		chi.URLParam(r, "id"), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID de notificación inválido")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "notificación no encontrada")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "notificación marcada como leída con éxito"})
}

func (s *server) markAllRead(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "no autorizado")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE notificaciones SET leida = TRUE WHERE usuario_id = $1 AND leida = FALSE`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error al actualizar notificaciones")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "notificaciones marcadas como leídas", "actualizadas": tag.RowsAffected()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
