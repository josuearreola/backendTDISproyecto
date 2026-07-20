package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("no se pudo conectar a Neon: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

// CatalogItem representa una actividad del catálogo general en base de datos.
type CatalogItem struct {
	ID                 string    `json:"id"`
	Nombre             string    `json:"nombre"`
	Descripcion        string    `json:"descripcion"`
	EvidenciaRequerida string    `json:"evidencia_requerida"`
	Horas              int       `json:"horas"`
	Puntaje            int       `json:"puntaje"`
	FechaCreacion      time.Time `json:"fecha_creacion"`
	FechaVencimiento   time.Time `json:"fecha_vencimiento"`
	CategoriaID        string    `json:"categoria_id"`
	DimensionID        string    `json:"dimension_id"`
	TrascendenciaID    string    `json:"trascendencia_id"`
	EntornoID          string    `json:"entorno_id"`
	CreadorID          *string   `json:"creador_id"`
}

// ListCatalog obtiene actividades filtradas por diversos parámetros.
func (s *Store) ListCatalog(ctx context.Context, categoryID, dimensionID, entornoID, trascendenciaID, search string) ([]CatalogItem, error) {
	query := `
		SELECT id, nombre, descripcion, evidencia_requerida, horas, puntaje, 
		       fecha_creacion, fecha_vencimiento, categoria_id, dimension_id, 
		       trascendencia_id, entorno_id, creador_id
		FROM catalogo_tdi
		WHERE fecha_vencimiento >= CURRENT_DATE
	`
	var args []interface{}
	argCount := 1

	if categoryID != "" {
		query += fmt.Sprintf(" AND categoria_id = $%d", argCount)
		args = append(args, categoryID)
		argCount++
	}
	if dimensionID != "" {
		query += fmt.Sprintf(" AND dimension_id = $%d", argCount)
		args = append(args, dimensionID)
		argCount++
	}
	if entornoID != "" {
		query += fmt.Sprintf(" AND entorno_id = $%d", argCount)
		args = append(args, entornoID)
		argCount++
	}
	if trascendenciaID != "" {
		query += fmt.Sprintf(" AND trascendencia_id = $%d", argCount)
		args = append(args, trascendenciaID)
		argCount++
	}
	if search != "" {
		query += fmt.Sprintf(" AND (nombre ILIKE $%d OR descripcion ILIKE $%d)", argCount, argCount)
		args = append(args, "%"+search+"%")
		argCount++
	}

	query += " ORDER BY fecha_creacion DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CatalogItem
	for rows.Next() {
		var item CatalogItem
		err := rows.Scan(
			&item.ID, &item.Nombre, &item.Descripcion, &item.EvidenciaRequerida,
			&item.Horas, &item.Puntaje, &item.FechaCreacion, &item.FechaVencimiento,
			&item.CategoriaID, &item.DimensionID, &item.TrascendenciaID, &item.EntornoID,
			&item.CreadorID,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// GetCatalogItem obtiene una actividad por su ID.
func (s *Store) GetCatalogItem(ctx context.Context, id string) (*CatalogItem, error) {
	item := &CatalogItem{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, nombre, descripcion, evidencia_requerida, horas, puntaje, 
		       fecha_creacion, fecha_vencimiento, categoria_id, dimension_id, 
		       trascendencia_id, entorno_id, creador_id
		FROM catalogo_tdi
		WHERE id = $1
	`, id).Scan(
		&item.ID, &item.Nombre, &item.Descripcion, &item.EvidenciaRequerida,
		&item.Horas, &item.Puntaje, &item.FechaCreacion, &item.FechaVencimiento,
		&item.CategoriaID, &item.DimensionID, &item.TrascendenciaID, &item.EntornoID,
		&item.CreadorID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("actividad no encontrada")
		}
		return nil, err
	}
	return item, nil
}

// CreateCatalogItem inserta una nueva actividad.
func (s *Store) CreateCatalogItem(ctx context.Context, item CatalogItem) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO catalogo_tdi (nombre, descripcion, evidencia_requerida, horas, puntaje, 
		                          fecha_creacion, fecha_vencimiento, categoria_id, dimension_id, 
		                          trascendencia_id, entorno_id, creador_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, item.Nombre, item.Descripcion, item.EvidenciaRequerida, item.Horas, item.Puntaje,
		item.FechaCreacion, item.FechaVencimiento, item.CategoriaID, item.DimensionID,
		item.TrascendenciaID, item.EntornoID, item.CreadorID)
	return err
}

// UpdateCatalogItem modifica los campos de una actividad.
func (s *Store) UpdateCatalogItem(ctx context.Context, id string, item CatalogItem) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE catalogo_tdi 
		SET nombre = $1, descripcion = $2, evidencia_requerida = $3, horas = $4, 
		    puntaje = $5, fecha_vencimiento = $6, categoria_id = $7, dimension_id = $8, 
		    trascendencia_id = $9, entorno_id = $10
		WHERE id = $11
	`, item.Nombre, item.Descripcion, item.EvidenciaRequerida, item.Horas, item.Puntaje,
		item.FechaVencimiento, item.CategoriaID, item.DimensionID, item.TrascendenciaID,
		item.EntornoID, id)
	return err
}

// DeleteCatalogItem intenta borrar físicamente una actividad.
func (s *Store) DeleteCatalogItem(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM catalogo_tdi WHERE id = $1", id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			_, err = s.pool.Exec(ctx, `
				UPDATE catalogo_tdi 
				SET fecha_vencimiento = CURRENT_DATE - INTERVAL '1 day' 
				WHERE id = $1
			`, id)
			return err
		}
		return err
	}
	return nil
}

// SeleccionarTDI asocia un alumno a una actividad.
func (s *Store) SeleccionarTDI(ctx context.Context, userID string, catalogoTDIID string) (string, error) {
	var alumnoID string
	err := s.pool.QueryRow(ctx, "SELECT id FROM alumnos WHERE user_id = $1", userID).Scan(&alumnoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("debes completar tu perfil de alumno primero")
		}
		return "", err
	}

	var registroID string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO registro_tdi (alumno_id, catalogo_tdi_id, estado)
		VALUES ($1, $2, 'PENDIENTE')
		RETURNING id
	`, alumnoID, catalogoTDIID).Scan(&registroID)

	return registroID, err
}
// VerifyCreatorAuthorization verifica si el usuario tiene un rol que le permita crear actividades.
// Devuelve el ID de creador_tdi correspondiente si su rol es CREADOR_TDI.
func (s *Store) VerifyCreatorAuthorization(ctx context.Context, userID string) (*string, error) {
	var roleName string
	// Consulta en las tablas de roles para validar si tiene el permiso adecuado
	err := s.pool.QueryRow(ctx, `
		SELECT r.nombre 
		FROM user_roles ur
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND r.nombre IN ('CREADOR_TDI', 'ADMINISTRATIVO', 'COORDINADOR')
		LIMIT 1
	`, userID).Scan(&roleName)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no tienes permisos para agregar o modificar actividades en el catálogo")
		}
		return nil, err
	}

	// Si su rol es CREADOR_TDI, necesitamos su ID de la tabla creadores_tdi
	if roleName == "CREADOR_TDI" {
		var creadorID string
		err = s.pool.QueryRow(ctx, "SELECT id FROM creadores_tdi WHERE user_id = $1", userID).Scan(&creadorID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("debes completar primero tu perfil de Creador de TDI antes de publicar actividades")
			}
			return nil, err
		}
		return &creadorID, nil
	}

	// Si es Administrativo o Coordinador, la actividad se publica con creador_id NULL (creada por la institución)
	return nil, nil
}

// GetStudentAndTDIInfo obtiene la matrícula del alumno y el ID del catálogo a partir del ID de registro.
func (s *Store) GetStudentAndTDIInfo(ctx context.Context, registroID string) (matricula string, tdiID string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT a.matricula, r.catalogo_tdi_id 
		FROM registro_tdi r
		JOIN alumnos a ON r.alumno_id = a.id
		WHERE r.id = $1
	`, registroID).Scan(&matricula, &tdiID)
	return matricula, tdiID, err
}

// SaveEvidenceMetadata guarda la evidencia en base de datos (haciendo UPDATE o INSERT) y actualiza el estado a EN_VALIDACION.
func (s *Store) SaveEvidenceMetadata(ctx context.Context, registroID, url, nombreArchivo, mimeType, hash string, tamanoBytes int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Verificar si ya existe un registro de evidencia para este registro_tdi_id
	var evidenciaID string
	err = tx.QueryRow(ctx, "SELECT id FROM evidencias WHERE registro_tdi_id = $1", registroID).Scan(&evidenciaID)

	if err == nil {
		// Ya existe: hacemos UPDATE
		_, err = tx.Exec(ctx, `
			UPDATE evidencias 
			SET url = $1, nombre_archivo = $2, mime_type = $3, hash_sha256 = $4, tamano_bytes = $5, fecha_subida = CURRENT_TIMESTAMP
			WHERE registro_tdi_id = $6
		`, url, nombreArchivo, mimeType, hash, tamanoBytes, registroID)
		if err != nil {
			return fmt.Errorf("error al actualizar metadatos de evidencia: %w", err)
		}
	} else {
		// No existe: hacemos INSERT
		_, err = tx.Exec(ctx, `
			INSERT INTO evidencias (registro_tdi_id, url, nombre_archivo, mime_type, hash_sha256, tamano_bytes)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, registroID, url, nombreArchivo, mimeType, hash, tamanoBytes)
		if err != nil {
			return fmt.Errorf("error al insertar metadatos de evidencia: %w", err)
		}
	}

	// 2. Actualizar el estado del registro a EN_VALIDACION
	_, err = tx.Exec(ctx, `
		UPDATE registro_tdi 
		SET estado = 'EN_REVISION' 
		WHERE id = $1
	`, registroID)
	if err != nil {
		return fmt.Errorf("error al actualizar estado de registro: %w", err)
	}

	return tx.Commit(ctx)
}

// GetAlumnoRegistros obtiene el historial de actividades seleccionadas por el alumno.
func (s *Store) GetAlumnoRegistros(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT 
			r.id AS registro_id, 
			r.estado, 
			r.horas_otorgadas AS puntos_otorgados, 
			r.fecha_registro, 
			r.fecha_aprobacion, 
			r.motivo_rechazo,
			c.nombre AS tdi_nombre, 
			c.descripcion AS tdi_descripcion, 
			c.puntaje AS tdi_puntos
		FROM registro_tdi r
		JOIN alumnos a ON r.alumno_id = a.id
		JOIN catalogo_tdi c ON r.catalogo_tdi_id = c.id
		WHERE a.user_id = $1
		ORDER BY r.fecha_registro DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var registros []map[string]interface{}
	for rows.Next() {
		var id, estado, tdiNombre, tdiDescripcion string
		var puntosOtorgados *int
		var fechaRegistro time.Time
		var fechaAprobacion *time.Time
		var motivoRechazo *string
		var tdiPuntos int

		err := rows.Scan(
			&id, &estado, &puntosOtorgados, &fechaRegistro, 
			&fechaAprobacion, &motivoRechazo, &tdiNombre, &tdiDescripcion, &tdiPuntos,
		)
		if err != nil {
			return nil, err
		}

		reg := map[string]interface{}{
			"registro_id":      id,
			"estado":           estado,
			"puntos_otorgados": puntosOtorgados,
			"fecha_registro":   fechaRegistro,
			"fecha_aprobacion": fechaAprobacion,
			"motivo_rechazo":   motivoRechazo,
			"tdi_nombre":       tdiNombre,
			"tdi_descripcion":  tdiDescripcion,
			"tdi_puntos_base":  tdiPuntos,
		}
		registros = append(registros, reg)
	}

	if registros == nil {
		registros = []map[string]interface{}{}
	}

	return registros, nil
}

// GetAlumnoProgreso obtiene el total de puntos acumulados y el desglose por dimensiones.
func (s *Store) GetAlumnoProgreso(ctx context.Context, userID string) (map[string]interface{}, error) {
	// 1. Obtener los totales generales del alumno
	var metaPuntos, puntosAcumulados int
	err := s.pool.QueryRow(ctx, `
		SELECT meta_horas, horas_acumuladas 
		FROM alumnos 
		WHERE user_id = $1
	`, userID).Scan(&metaPuntos, &puntosAcumulados)
	if err != nil {
		return nil, fmt.Errorf("error al obtener totales del alumno: %w", err)
	}

	// 2. Obtener el progreso por dimensión (hacemos LEFT JOIN con dimensiones para que devuelva todas)
	rows, err := s.pool.Query(ctx, `
		SELECT 
			d.nombre AS dimension_nombre,
			COALESCE(p.horas, 0) AS puntos_acumulados,
			COALESCE(p.porcentaje, 0) AS porcentaje
		FROM dimensiones d
		LEFT JOIN progreso_alumno p ON p.dimension_id = d.id AND p.alumno_id = (
			SELECT id FROM alumnos WHERE user_id = $1
		)
		ORDER BY d.nombre ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("error al obtener progreso por dimensiones: %w", err)
	}
	defer rows.Close()

	var dimensionesProgreso []map[string]interface{}
	for rows.Next() {
		var dimNombre string
		var puntos int
		var porcentaje float64

		if err := rows.Scan(&dimNombre, &puntos, &porcentaje); err != nil {
			return nil, err
		}

		dimensionesProgreso = append(dimensionesProgreso, map[string]interface{}{
			"dimension":          dimNombre,
			"puntos_acumulados":  puntos,
			"porcentaje":         porcentaje,
		})
	}

	if dimensionesProgreso == nil {
		dimensionesProgreso = []map[string]interface{}{}
	}

	return map[string]interface{}{
		"meta_puntos":        metaPuntos,
		"puntos_acumulados":  puntosAcumulados,
		"dimensiones":        dimensionesProgreso,
	}, nil
}

// PendingRevision representa la información detallada de una revisión de evidencia pendiente.
type PendingRevision struct {
	RevisionID       string    `json:"revision_id"`
	RegistroTdiID    string    `json:"registro_tdi_id"`
	Nombre           string    `json:"nombre"`
	ApellidoPaterno  string    `json:"apellido_paterno"`
	ApellidoMaterno  string    `json:"apellido_materno"`
	Matricula        string    `json:"matricula"`
	TdiNombre        string    `json:"tdi_nombre"`
	TdiHoras         int       `json:"tdi_horas"`
	TdiPuntaje       int       `json:"tdi_puntaje"`
	EvidenciaURL     *string   `json:"evidencia_url"`
	EvidenciaNombre  *string   `json:"evidencia_nombre"`
	OcrObservaciones string    `json:"ocr_observaciones"`
	FechaSolicitud   time.Time `json:"fecha_solicitud"`
}

// RevisionDetails contiene la información del alumno e interés de la TDI para procesar el dictamen.
type RevisionDetails struct {
	UserID      string
	AlumnoID    string
	TdiNombre   string
	TdiHoras    int
	TdiPuntaje  int
	MetaHoras   int
	DimensionID string
	RegistroID  string
}

// GetPendingRevisions obtiene el listado de evidencias pendientes por revisar del sistema.
func (s *Store) GetPendingRevisions(ctx context.Context) ([]PendingRevision, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT 
			r.id AS revision_id,
			reg.id AS registro_tdi_id,
			u.nombre, u.apellido_paterno, u.apellido_materno,
			a.matricula,
			c.nombre AS tdi_nombre,
			c.horas AS tdi_horas,
			c.puntaje AS tdi_puntaje,
			e.url AS evidencia_url,
			e.nombre_archivo AS evidencia_nombre,
			r.comentario AS ocr_observaciones,
			r.fecha AS fecha_solicitud
		FROM revisiones r
		JOIN registro_tdi reg ON r.registro_tdi_id = reg.id
		JOIN alumnos a ON reg.alumno_id = a.id
		JOIN users u ON a.user_id = u.id
		JOIN catalogo_tdi c ON reg.catalogo_tdi_id = c.id
		LEFT JOIN evidencias e ON reg.id = e.registro_tdi_id
		WHERE r.decision = 'PENDIENTE'
		ORDER BY r.fecha ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("error al obtener revisiones pendientes: %w", err)
	}
	defer rows.Close()

	var revisions []PendingRevision
	for rows.Next() {
		var r PendingRevision
		err := rows.Scan(
			&r.RevisionID, &r.RegistroTdiID, &r.Nombre, &r.ApellidoPaterno, &r.ApellidoMaterno,
			&r.Matricula, &r.TdiNombre, &r.TdiHoras, &r.TdiPuntaje, &r.EvidenciaURL,
			&r.EvidenciaNombre, &r.OcrObservaciones, &r.FechaSolicitud,
		)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, r)
	}

	if revisions == nil {
		revisions = []PendingRevision{}
	}

	return revisions, nil
}

// GetStudentAndTDIInfoFromRevision obtiene la información detallada para resolver la revisión.
func (s *Store) GetStudentAndTDIInfoFromRevision(ctx context.Context, revisionID string) (*RevisionDetails, error) {
	var d RevisionDetails
	err := s.pool.QueryRow(ctx, `
		SELECT 
			a.user_id,
			a.id AS alumno_id,
			c.nombre AS tdi_nombre,
			c.horas AS tdi_horas,
			c.puntaje AS tdi_puntaje,
			COALESCE(a.meta_horas, 60) AS meta_horas,
			c.dimension_id,
			reg.id AS registro_id
		FROM revisiones r
		JOIN registro_tdi reg ON r.registro_tdi_id = reg.id
		JOIN alumnos a ON reg.alumno_id = a.id
		JOIN catalogo_tdi c ON reg.catalogo_tdi_id = c.id
		WHERE r.id = $1
	`, revisionID).Scan(
		&d.UserID, &d.AlumnoID, &d.TdiNombre, &d.TdiHoras, &d.TdiPuntaje,
		&d.MetaHoras, &d.DimensionID, &d.RegistroID,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ResolveRevision resuelve una revisión de forma atómica en base de datos.
func (s *Store) ResolveRevision(ctx context.Context, revisionID string, requesterUserID string, decision string, comentario string, details *RevisionDetails) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error iniciando transaccion: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Intentar buscar el id de administrativos del usuario que realiza la acción
	var adminID *string
	var aid string
	err = tx.QueryRow(ctx, "SELECT id FROM administrativos WHERE user_id = $1", requesterUserID).Scan(&aid)
	if err == nil {
		adminID = &aid
	}

	// 2. Actualizar tabla revisiones
	_, err = tx.Exec(ctx, `
		UPDATE revisiones
		SET decision = $1, comentario = $2, administrativo_id = $3, fecha = CURRENT_TIMESTAMP
		WHERE id = $4
	`, decision, comentario, adminID, revisionID)
	if err != nil {
		return fmt.Errorf("no se pudo actualizar la revision: %w", err)
	}

	// 3. Actualizar registro_tdi e historial
	if decision == "APROBADA" {
		_, err = tx.Exec(ctx, `
			UPDATE registro_tdi
			SET estado = 'APROBADA', horas_otorgadas = $1, puntaje_obtenido = $2, fecha_aprobacion = CURRENT_TIMESTAMP
			WHERE id = $3
		`, details.TdiHoras, details.TdiPuntaje, details.RegistroID)
		if err != nil {
			return fmt.Errorf("no se pudo aprobar el registro: %w", err)
		}

		// Sumar horas al alumno
		_, err = tx.Exec(ctx, `
			UPDATE alumnos
			SET horas_acumuladas = horas_acumuladas + $1
			WHERE id = $2
		`, details.TdiHoras, details.AlumnoID)
		if err != nil {
			return fmt.Errorf("no se se pudo sumas horas al alumno: %w", err)
		}

		// Recalcular y actualizar progreso por dimensión
		var currentHoras int
		err = tx.QueryRow(ctx, `
			SELECT horas FROM progreso_alumno 
			WHERE alumno_id = $1 AND dimension_id = $2
		`, details.AlumnoID, details.DimensionID).Scan(&currentHoras)
		
		newHoras := details.TdiHoras
		if err == nil {
			newHoras += currentHoras
		}

		percentage := float64(newHoras*100) / float64(details.MetaHoras)
		if percentage > 100.0 {
			percentage = 100.0
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO progreso_alumno (alumno_id, dimension_id, horas, porcentaje)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (alumno_id, dimension_id) DO UPDATE SET
				horas = EXCLUDED.horas,
				porcentaje = EXCLUDED.porcentaje
		`, details.AlumnoID, details.DimensionID, newHoras, percentage)
		if err != nil {
			return fmt.Errorf("no se pudo actualizar progreso: %w", err)
		}

		// Insertar notificación de aprobación
		_, err = tx.Exec(ctx, `
			INSERT INTO notificaciones (usuario_id, titulo, mensaje)
			VALUES ($1, 'Evidencia Aprobada por Administrador', $2)
		`, details.UserID, fmt.Sprintf("Tu evidencia para la actividad '%s' ha sido aprobada manualmente. Se te han otorgado %d horas.", details.TdiNombre, details.TdiHoras))
		if err != nil {
			return fmt.Errorf("no se pudo insertar notificacion: %w", err)
		}

	} else {
		// Rechazar registro
		_, err = tx.Exec(ctx, `
			UPDATE registro_tdi
			SET estado = 'RECHAZADA', motivo_rechazo = $1
			WHERE id = $2
		`, comentario, details.RegistroID)
		if err != nil {
			return fmt.Errorf("no se pudo rechazar el registro: %w", err)
		}

		// Insertar notificación de rechazo
		_, err = tx.Exec(ctx, `
			INSERT INTO notificaciones (usuario_id, titulo, mensaje)
			VALUES ($1, 'Evidencia Rechazada por Administrador', $2)
		`, details.UserID, fmt.Sprintf("Tu evidencia para la actividad '%s' ha sido rechazada. Motivo: %s", details.TdiNombre, comentario))
		if err != nil {
			return fmt.Errorf("no se pudo insertar notificacion: %w", err)
		}
	}

	return tx.Commit(ctx)
}

