package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rango de fechas inclusivo por día. Vacío = sin límite.
type Rango struct {
	Desde, Hasta time.Time
}

func (r Rango) desde() time.Time {
	if r.Desde.IsZero() {
		return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return r.Desde
}

// hasta es exclusivo: el día "hasta" completo queda incluido.
func (r Rango) hasta() time.Time {
	if r.Hasta.IsZero() {
		return time.Now().AddDate(0, 0, 1)
	}
	return r.Hasta.AddDate(0, 0, 1)
}

// query arma un WHERE con placeholders numerados ($1, $2...) sin concatenar valores.
type query struct {
	conds []string
	args  []any
}

// where agrega una condición con UN placeholder %d.
func (q *query) where(cond string, v any) {
	q.args = append(q.args, v)
	q.conds = append(q.conds, fmt.Sprintf(cond, len(q.args)))
}

func (q *query) whereIf(v, cond string) {
	if v != "" {
		q.where(cond, v)
	}
}

func (q *query) sql() string {
	if len(q.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(q.conds, " AND ")
}

// Tabla es el formato neutro que se exporta a CSV, XLSX o PDF.
type Tabla struct {
	Titulo   string
	Columnas []string
	Filas    [][]string
}

// ReporteFiltro son los filtros comunes de exportación.
type ReporteFiltro struct {
	Rango
	Carrera, Estado string
}

// ponytail: tope fijo de filas por exportación; paginar o hacer streaming si se rebasa.
const maxFilasExport = 50000

func (s *Store) tabla(ctx context.Context, titulo string, columnas []string, sql string, args ...any) (*Tabla, error) {
	rows, err := s.pool.Query(ctx, sql+fmt.Sprintf(" LIMIT %d", maxFilasExport), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	t := &Tabla{Titulo: titulo, Columnas: columnas, Filas: [][]string{}}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		fila := make([]string, len(vals))
		for i, v := range vals {
			fila[i] = celda(v)
		}
		t.Filas = append(t.Filas, fila)
	}
	return t, rows.Err()
}

func celda(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case time.Time:
		return x.Format("2006-01-02 15:04")
	case string:
		return x
	case [16]byte: // uuid
		return fmt.Sprintf("%x-%x-%x-%x-%x", x[0:4], x[4:6], x[6:8], x[8:10], x[10:])
	case float64:
		return strconv.FormatFloat(x, 'f', 2, 64)
	default:
		return fmt.Sprint(x)
	}
}

func (s *Store) ReporteRegistros(ctx context.Context, f ReporteFiltro) (*Tabla, error) {
	q := &query{}
	q.where("r.fecha_registro >= $%d", f.desde())
	q.where("r.fecha_registro < $%d", f.hasta())
	q.whereIf(f.Carrera, "a.carrera = $%d")
	q.whereIf(f.Estado, "r.estado = $%d")
	return s.tabla(ctx, "Registros de TDI",
		[]string{"Matrícula", "Alumno", "Carrera", "Grupo", "Actividad", "Dimensión", "Estado", "Horas", "Puntaje", "Fecha registro", "Fecha aprobación", "Motivo rechazo"}, `
		SELECT a.matricula,
		       CONCAT_WS(' ', u.nombre, u.apellido_paterno, u.apellido_materno),
		       a.carrera, a.grupo, c.nombre, d.nombre, r.estado,
		       r.horas_otorgadas, r.puntaje_obtenido, r.fecha_registro, r.fecha_aprobacion, r.motivo_rechazo
		FROM registro_tdi r
		JOIN alumnos a ON r.alumno_id = a.id
		JOIN users u ON a.user_id = u.id
		JOIN catalogo_tdi c ON r.catalogo_tdi_id = c.id
		LEFT JOIN dimensiones d ON c.dimension_id = d.id`+q.sql()+`
		ORDER BY r.fecha_registro DESC`, q.args...)
}

func (s *Store) ReporteProgreso(ctx context.Context, f ReporteFiltro) (*Tabla, error) {
	q := &query{}
	q.where("u.activo = $%d", true)
	q.whereIf(f.Carrera, "a.carrera = $%d")
	return s.tabla(ctx, "Progreso de alumnos",
		[]string{"Matrícula", "Alumno", "Carrera", "Grupo", "Cuatrimestre", "Meta (horas)", "Horas acumuladas", "Avance %", "Actividades aprobadas"}, `
		SELECT a.matricula,
		       CONCAT_WS(' ', u.nombre, u.apellido_paterno, u.apellido_materno),
		       a.carrera, a.grupo, a.cuatrimestre, a.meta_horas, a.horas_acumuladas,
		       ROUND(LEAST(100, a.horas_acumuladas * 100.0 / NULLIF(a.meta_horas, 0)), 2)::float8,
		       (SELECT COUNT(*) FROM registro_tdi r WHERE r.alumno_id = a.id AND r.estado = 'APROBADA')
		FROM alumnos a
		JOIN users u ON a.user_id = u.id`+q.sql()+`
		ORDER BY a.carrera, a.grupo, a.matricula`, q.args...)
}

func (s *Store) ReporteRevisiones(ctx context.Context, f ReporteFiltro) (*Tabla, error) {
	q := &query{}
	q.where("rv.fecha >= $%d", f.desde())
	q.where("rv.fecha < $%d", f.hasta())
	q.whereIf(f.Carrera, "a.carrera = $%d")
	q.whereIf(f.Estado, "rv.decision = $%d")
	return s.tabla(ctx, "Revisiones manuales",
		[]string{"Fecha", "Matrícula", "Alumno", "Actividad", "Decisión", "Comentario", "Revisor"}, `
		SELECT rv.fecha, a.matricula,
		       CONCAT_WS(' ', u.nombre, u.apellido_paterno, u.apellido_materno),
		       c.nombre, rv.decision, rv.comentario,
		       CONCAT_WS(' ', ua.nombre, ua.apellido_paterno)
		FROM revisiones rv
		JOIN registro_tdi r ON rv.registro_tdi_id = r.id
		JOIN alumnos a ON r.alumno_id = a.id
		JOIN users u ON a.user_id = u.id
		JOIN catalogo_tdi c ON r.catalogo_tdi_id = c.id
		LEFT JOIN administrativos ad ON rv.administrativo_id = ad.id
		LEFT JOIN users ua ON ad.user_id = ua.id`+q.sql()+`
		ORDER BY rv.fecha DESC`, q.args...)
}

func (s *Store) ReporteAuditoria(ctx context.Context, f AuditFilter) (*Tabla, error) {
	q := &query{}
	q.where("l.fecha >= $%d", f.desde())
	q.where("l.fecha < $%d", f.hasta())
	q.whereIf(f.UsuarioID, "l.usuario_id = $%d::uuid")
	q.whereIf(f.Entidad, "l.entidad = $%d")
	q.whereIf(f.Origen, "l.origen = $%d")
	return s.tabla(ctx, "Bitácora de auditoría",
		[]string{"Fecha", "Origen", "Usuario", "Rol", "Acción", "Entidad", "ID entidad", "Status", "IP"}, `
		SELECT l.fecha, l.origen, COALESCE(u.email, l.usuario_id::text), l.rol, l.accion,
		       l.entidad, l.entidad_id, l.status, l.ip
		FROM audit_log l
		LEFT JOIN users u ON l.usuario_id = u.id`+q.sql()+`
		ORDER BY l.fecha DESC`, q.args...)
}

// Conteo es un par etiqueta/valor para las métricas agrupadas.
type Conteo struct {
	Nombre string `json:"nombre"`
	Total  int64  `json:"total"`
}

type Metricas struct {
	Desde                 time.Time  `json:"desde"`
	Hasta                 time.Time  `json:"hasta"`
	RegistrosPorEstado    []Conteo   `json:"registros_por_estado"`
	ValidacionesIA        []Conteo   `json:"validaciones_ia"`
	TasaAutoAprobacion    float64    `json:"tasa_auto_aprobacion"` // % de validaciones de IA que aprobaron solas
	RevisionesPorDecision []Conteo   `json:"revisiones_por_decision"`
	RevisionesPendientes  int64      `json:"revisiones_pendientes"`
	PendienteMasAntigua   *time.Time `json:"pendiente_mas_antigua"`
	HorasPorDimension     []Conteo   `json:"horas_por_dimension"`
	HorasPorCarrera       []Conteo   `json:"horas_por_carrera"`
	TopActividades        []Conteo   `json:"top_actividades"`
	AlumnosActivos        int64      `json:"alumnos_activos"`
	AlumnosMetaCumplida   int64      `json:"alumnos_meta_cumplida"`
}

func (s *Store) conteos(ctx context.Context, sql string, args ...any) ([]Conteo, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conteo{}
	for rows.Next() {
		var c Conteo
		if err := rows.Scan(&c.Nombre, &c.Total); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetMetricas calcula los indicadores del dashboard administrativo para el rango.
// Pendientes y alumnos son foto del momento; lo demás respeta el rango.
func (s *Store) GetMetricas(ctx context.Context, r Rango) (*Metricas, error) {
	d, h := r.desde(), r.hasta()
	m := &Metricas{Desde: d, Hasta: h.AddDate(0, 0, -1)}

	var err error
	if m.RegistrosPorEstado, err = s.conteos(ctx, `
		SELECT COALESCE(estado, 'SIN ESTADO'), COUNT(*) FROM registro_tdi
		WHERE fecha_registro >= $1 AND fecha_registro < $2 GROUP BY 1 ORDER BY 2 DESC`, d, h); err != nil {
		return nil, fmt.Errorf("registros por estado: %w", err)
	}
	if m.ValidacionesIA, err = s.conteos(ctx, `
		SELECT COALESCE(resultado, 'SIN RESULTADO'), COUNT(*) FROM validaciones
		WHERE fecha >= $1 AND fecha < $2 GROUP BY 1 ORDER BY 2 DESC`, d, h); err != nil {
		return nil, fmt.Errorf("validaciones IA: %w", err)
	}
	var totalIA, aprobadasIA int64
	for _, c := range m.ValidacionesIA {
		totalIA += c.Total
		if c.Nombre == "APROBADA" {
			aprobadasIA = c.Total
		}
	}
	if totalIA > 0 {
		m.TasaAutoAprobacion = float64(aprobadasIA*10000/totalIA) / 100
	}

	if m.RevisionesPorDecision, err = s.conteos(ctx, `
		SELECT COALESCE(decision, 'SIN DECISION'), COUNT(*) FROM revisiones
		WHERE fecha >= $1 AND fecha < $2 GROUP BY 1 ORDER BY 2 DESC`, d, h); err != nil {
		return nil, fmt.Errorf("revisiones por decisión: %w", err)
	}
	if err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*), MIN(fecha) FROM revisiones WHERE decision = 'PENDIENTE'`).
		Scan(&m.RevisionesPendientes, &m.PendienteMasAntigua); err != nil {
		return nil, fmt.Errorf("revisiones pendientes: %w", err)
	}

	if m.HorasPorDimension, err = s.conteos(ctx, `
		SELECT COALESCE(dm.nombre, 'Sin dimensión'), COALESCE(SUM(r.horas_otorgadas), 0)
		FROM registro_tdi r
		JOIN catalogo_tdi c ON r.catalogo_tdi_id = c.id
		LEFT JOIN dimensiones dm ON c.dimension_id = dm.id
		WHERE r.estado = 'APROBADA' AND r.fecha_aprobacion >= $1 AND r.fecha_aprobacion < $2
		GROUP BY 1 ORDER BY 2 DESC`, d, h); err != nil {
		return nil, fmt.Errorf("horas por dimensión: %w", err)
	}
	if m.HorasPorCarrera, err = s.conteos(ctx, `
		SELECT COALESCE(a.carrera, 'Sin carrera'), COALESCE(SUM(r.horas_otorgadas), 0)
		FROM registro_tdi r
		JOIN alumnos a ON r.alumno_id = a.id
		WHERE r.estado = 'APROBADA' AND r.fecha_aprobacion >= $1 AND r.fecha_aprobacion < $2
		GROUP BY 1 ORDER BY 2 DESC`, d, h); err != nil {
		return nil, fmt.Errorf("horas por carrera: %w", err)
	}
	if m.TopActividades, err = s.conteos(ctx, `
		SELECT c.nombre, COUNT(*) FROM registro_tdi r
		JOIN catalogo_tdi c ON r.catalogo_tdi_id = c.id
		WHERE r.fecha_registro >= $1 AND r.fecha_registro < $2
		GROUP BY c.id, c.nombre ORDER BY 2 DESC LIMIT 5`, d, h); err != nil {
		return nil, fmt.Errorf("top actividades: %w", err)
	}

	if err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE a.horas_acumuladas >= a.meta_horas)
		FROM alumnos a JOIN users u ON a.user_id = u.id WHERE u.activo`).
		Scan(&m.AlumnosActivos, &m.AlumnosMetaCumplida); err != nil {
		return nil, fmt.Errorf("alumnos: %w", err)
	}
	return m, nil
}
