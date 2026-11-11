package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrorPermanente indica que Postgres rechazó el dato en sí (clase 22: dato inválido,
// clase 23: restricción violada). Reintentar no sirve: el consumidor debe descartar el
// mensaje en vez de reencolarlo, o bloquearía la cola para siempre.
func ErrorPermanente(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && (strings.HasPrefix(pg.Code, "22") || strings.HasPrefix(pg.Code, "23"))
}

// recortar limita s a n caracteres (no bytes) para respetar los VARCHAR de audit_log.
func recortar(s *string, n int) *string {
	if s == nil {
		return nil
	}
	if r := []rune(*s); len(r) > n {
		v := string(r[:n])
		return &v
	}
	return s
}

// execer lo cumplen tanto *pgxpool.Pool como pgx.Tx, para auditar dentro de una transacción.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// AuditEntry es un renglón de audit_log. Antes/Despues se serializan a JSONB.
type AuditEntry struct {
	ID        int64     `json:"id"`
	Fecha     time.Time `json:"fecha"`
	Origen    string    `json:"origen"`
	UsuarioID *string   `json:"usuario_id"`
	Rol       *string   `json:"rol"`
	Accion    string    `json:"accion"`
	Entidad   *string   `json:"entidad"`
	EntidadID *string   `json:"entidad_id"`
	Status    *int      `json:"status"`
	IP        *string   `json:"ip"`
	Antes     any       `json:"antes"`
	Despues   any       `json:"despues"`
}

func insertAudit(ctx context.Context, x execer, e AuditEntry) error {
	antes, err := toJSONB(e.Antes)
	if err != nil {
		return err
	}
	despues, err := toJSONB(e.Despues)
	if err != nil {
		return err
	}
	if e.Fecha.IsZero() {
		e.Fecha = time.Now()
	}
	_, err = x.Exec(ctx, `
		INSERT INTO audit_log (fecha, origen, usuario_id, rol, accion, entidad, entidad_id, status, ip, antes, despues)
		VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, $9, $10, $11)
	`, e.Fecha, e.Origen, getString(e.UsuarioID), e.Rol, e.Accion, e.Entidad, e.EntidadID, e.Status, e.IP, antes, despues)
	return err
}

// toJSONB deja pasar nil como NULL y acepta tanto structs como json.RawMessage.
func toJSONB(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	if raw, ok := v.(json.RawMessage); ok {
		return raw, nil
	}
	return json.Marshal(v)
}

// Audit registra un evento de dominio fuera de transacción.
func (s *Store) Audit(ctx context.Context, e AuditEntry) error {
	return insertAudit(ctx, s.pool, e)
}

// AuditFilter son los filtros de GET /auditoria y de la exportación de auditoría.
type AuditFilter struct {
	Rango
	UsuarioID, Entidad, EntidadID, Origen string
	Limit, Offset                         int
}

func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, error) {
	q := &query{}
	q.where("fecha >= $%d", f.desde())
	q.where("fecha < $%d", f.hasta())
	q.whereIf(f.UsuarioID, "usuario_id = $%d::uuid")
	q.whereIf(f.Entidad, "entidad = $%d")
	q.whereIf(f.EntidadID, "entidad_id = $%d")
	q.whereIf(f.Origen, "origen = $%d")
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}

	sql := `SELECT id, fecha, origen, usuario_id::text, rol, accion, entidad, entidad_id, status, ip, antes, despues
		FROM audit_log` + q.sql() + fmt.Sprintf(" ORDER BY fecha DESC LIMIT %d OFFSET %d", f.Limit, max(f.Offset, 0))
	rows, err := s.pool.Query(ctx, sql, q.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var antes, despues []byte
		if err := rows.Scan(&e.ID, &e.Fecha, &e.Origen, &e.UsuarioID, &e.Rol, &e.Accion, &e.Entidad,
			&e.EntidadID, &e.Status, &e.IP, &antes, &despues); err != nil {
			return nil, err
		}
		if antes != nil {
			e.Antes = json.RawMessage(antes)
		}
		if despues != nil {
			e.Despues = json.RawMessage(despues)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SaveGatewayAudit persiste un mensaje crudo de audit_queue (formato de api-gateaway-tdis/internal/middleware/audit.go).
// Solo devuelve error si falla la BD (el llamador reencola); un mensaje malformado se descarta.
func (s *Store) SaveGatewayAudit(ctx context.Context, payload []byte) error {
	var e AuditEntry
	if err := json.Unmarshal(payload, &e); err != nil || e.Accion == "" {
		log.Printf("[TDI-SERVICE ] auditoría descartada (malformada): %s", payload)
		return nil
	}
	e.Origen = "GATEWAY"
	// La ruta y la IP vienen del cliente: se recortan a lo que admite audit_log.
	e.Accion = *recortar(&e.Accion, 120)
	e.IP = recortar(e.IP, 64)
	e.Rol = recortar(e.Rol, 50)
	return insertAudit(ctx, s.pool, e)
}
