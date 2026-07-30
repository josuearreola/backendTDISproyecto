package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luuboon/pti-uteq-backend/user-service-tdis/internal/models"
)

// ErrNotFound se devuelve cuando no existe el usuario buscado.
var ErrNotFound = errors.New("usuario no encontrado")

// ErrEmailTaken se devuelve al registrar un email que ya existe.
var ErrEmailTaken = errors.New("el email ya está registrado")

// Store envuelve el pool de conexiones a Postgres (Neon).
type Store struct {
	pool *pgxpool.Pool
}

// New crea el pool de conexiones a Neon a partir del connection string.
// pgxpool maneja un pool de conexiones reutilizables, ideal para Neon.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el pool: %w", err)
	}

	// Verificamos la conexión al arrancar.
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctxPing); err != nil {
		return nil, fmt.Errorf("no se pudo conectar a Neon: %w", err)
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

// Migrate es un no-op en este caso ya que la base de datos está gestionada
// externamente mediante tu script de Neon. Retornamos nil directamente.
func (s *Store) Migrate(ctx context.Context) error {
	return nil
}

// CreateUser inserta un nuevo usuario en la tabla users y su rol en user_roles mediante una transacción.
func (s *Store) CreateUser(ctx context.Context, u *models.User) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Insertar el usuario en users
	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, nombre, apellido_paterno, apellido_materno, telefono, activo)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, u.ID, u.Email, u.PasswordHash, u.Nombre, u.ApellidoPaterno, u.ApellidoMaterno, u.Telefono, u.Activo)

	if err != nil {
		if isUniqueViolation(err) {
			return ErrEmailTaken
		}
		return err
	}

	// 2. Obtener el ID del rol por su nombre
	var roleID string
	err = tx.QueryRow(ctx, "SELECT id FROM roles WHERE nombre = $1", string(u.Role)).Scan(&roleID)
	if err != nil {
		return fmt.Errorf("no se encontró el rol %s en la base de datos: %w", u.Role, err)
	}

	// 3. Asociar el usuario con su rol en user_roles
	_, err = tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", u.ID, roleID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetByEmail busca un usuario por su email asociando su rol correspondiente.
func (s *Store) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	u := &models.User{}
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.nombre, u.apellido_paterno, u.apellido_materno, u.telefono, u.activo, COALESCE(r.nombre, ''), u.created_at, u.updated_at
		FROM users u
		LEFT JOIN user_roles ur ON u.id = ur.user_id
		LEFT JOIN roles r ON ur.role_id = r.id
		WHERE u.email = $1
	`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Nombre, &u.ApellidoPaterno, &u.ApellidoMaterno, &u.Telefono, &u.Activo, &role, &u.CreatedAt, &u.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Role = models.Role(role)
	s.LoadSubProfiles(ctx, u)
	return u, nil
}

// GetByID busca un usuario por su ID asociando su rol correspondiente.
func (s *Store) GetByID(ctx context.Context, id string) (*models.User, error) {
	u := &models.User{}
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.nombre, u.apellido_paterno, u.apellido_materno, u.telefono, u.activo, COALESCE(r.nombre, ''), u.created_at, u.updated_at
		FROM users u
		LEFT JOIN user_roles ur ON u.id = ur.user_id
		LEFT JOIN roles r ON ur.role_id = r.id
		WHERE u.id = $1
	`, id).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Nombre, &u.ApellidoPaterno, &u.ApellidoMaterno, &u.Telefono, &u.Activo, &role, &u.CreatedAt, &u.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Role = models.Role(role)
	s.LoadSubProfiles(ctx, u)

	return u, nil
}

// isUniqueViolation detecta el error de email duplicado de Postgres
// usando el código SQLSTATE 23505 (unique_violation) que expone pgx.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
// CreateOrUpdateAlumno guarda o actualiza la información académica de un alumno.
func (s *Store) CreateOrUpdateAlumno(ctx context.Context, userID string, matricula, grupo, carrera string, cuatrimestre int, tutor string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO alumnos (user_id, matricula, grupo, carrera, cuatrimestre, tutor)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE SET
			matricula = EXCLUDED.matricula,
			grupo = EXCLUDED.grupo,
			carrera = EXCLUDED.carrera,
			cuatrimestre = EXCLUDED.cuatrimestre,
			tutor = EXCLUDED.tutor
	`, userID, matricula, grupo, carrera, cuatrimestre, tutor)
	return err
}

// CreateOrUpdateAdministrativo guarda o actualiza el cargo de un administrativo.
func (s *Store) CreateOrUpdateAdministrativo(ctx context.Context, userID string, cargo string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO administrativos (user_id, cargo)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET cargo = EXCLUDED.cargo
	`, userID, cargo)
	return err
}

// CreateOrUpdateCreador guarda o actualiza los datos de un creador de TDI.
func (s *Store) CreateOrUpdateCreador(ctx context.Context, userID string, institucion, tipo, descripcion string) error {
	var id string
	// 1. Verificamos si ya existe el registro de creador para el usuario
	err := s.pool.QueryRow(ctx, "SELECT id FROM creadores_tdi WHERE user_id = $1", userID).Scan(&id)
	
	if err == nil {
		// Ya existe: hacemos UPDATE
		_, err = s.pool.Exec(ctx, `
			UPDATE creadores_tdi 
			SET institucion = $1, tipo = $2, descripcion = $3 
			WHERE user_id = $4
		`, institucion, tipo, descripcion, userID)
		return err
	}

	// No existe: hacemos INSERT
	_, err = s.pool.Exec(ctx, `
		INSERT INTO creadores_tdi (user_id, institucion, tipo, descripcion)
		VALUES ($1, $2, $3, $4)
	`, userID, institucion, tipo, descripcion)
	return err
}

// LoadSubProfiles busca y anexa perfiles adicionales en base al rol y user_id.
func (s *Store) LoadSubProfiles(ctx context.Context, u *models.User) error {
	// Intentar cargar perfil de Alumno
	var al models.PerfilAlumno
	err := s.pool.QueryRow(ctx, `
		SELECT matricula, grupo, carrera, cuatrimestre, tutor, meta_horas, horas_acumuladas 
		FROM alumnos WHERE user_id = $1
	`, u.ID).Scan(&al.Matricula, &al.Grupo, &al.Carrera, &al.Cuatrimestre, &al.Tutor, &al.MetaHoras, &al.HorasAcumuladas)
	if err == nil {
		u.PerfilAlumno = &al
	}

	// Intentar cargar perfil de Administrativo
	var adm models.PerfilAdministrativo
	err = s.pool.QueryRow(ctx, "SELECT cargo FROM administrativos WHERE user_id = $1", u.ID).Scan(&adm.Cargo)
	if err == nil {
		u.PerfilAdministrativo = &adm
	}

	// Intentar cargar perfil de Creador
	var cr models.PerfilCreador
	err = s.pool.QueryRow(ctx, `
		SELECT institucion, tipo, descripcion FROM creadores_tdi WHERE user_id = $1
	`, u.ID).Scan(&cr.Institucion, &cr.Tipo, &cr.Descripcion)
	if err == nil {
		u.PerfilCreador = &cr
	}

	return nil
}
// DeleteUser elimina un usuario y todas sus relaciones de perfil secundarias.
// DeactivateUser realiza una baja lógica pasando la columna 'activo' a false.
func (s *Store) DeactivateUser(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users 
		SET activo = false, updated_at = CURRENT_TIMESTAMP 
		WHERE id = $1
	`, userID)
	return err
}

// GetNotifications obtiene todas las notificaciones de un usuario ordenadas por fecha descendente.
func (s *Store) GetNotifications(ctx context.Context, userID string) ([]models.Notification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, usuario_id, titulo, mensaje, leida, fecha 
		FROM notificaciones 
		WHERE usuario_id = $1 
		ORDER BY fecha DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []models.Notification
	for rows.Next() {
		var n models.Notification
		err := rows.Scan(&n.ID, &n.UserID, &n.Titulo, &n.Mensaje, &n.Leida, &n.Fecha)
		if err != nil {
			return nil, err
		}
		notifications = append(notifications, n)
	}

	if notifications == nil {
		notifications = []models.Notification{}
	}

	return notifications, nil
}

// MarkNotificationRead marca una notificación específica de un usuario como leída.
func (s *Store) MarkNotificationRead(ctx context.Context, userID string, notificationID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notificaciones 
		SET leida = TRUE 
		WHERE id = $1 AND usuario_id = $2
	`, notificationID, userID)
	return err
}


