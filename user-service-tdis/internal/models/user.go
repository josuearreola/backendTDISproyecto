package models

import "time"

// Role define los roles del sistema, alineados con la base de datos.
type Role string

const (
	RoleAlumno         Role = "ALUMNO"
	RoleAdministrativo Role = "ADMINISTRATIVO"
	RoleCreadorTDI     Role = "CREADOR_TDI"
	RoleCoordinador    Role = "COORDINADOR"
)

// ValidRole verifica que un string sea un rol reconocido.
func ValidRole(r string) bool {
	switch Role(r) {
	case RoleAlumno, RoleAdministrativo, RoleCreadorTDI, RoleCoordinador:
		return true
	}
	return false
}

// PerfilAlumno contiene la información académica específica.
type PerfilAlumno struct {
	Matricula       string `json:"matricula"`
	Grupo           string `json:"grupo"`
	Carrera         string `json:"carrera"`
	Cuatrimestre    int    `json:"cuatrimestre"`
	Tutor           string `json:"tutor"`
	MetaHoras       int    `json:"meta_horas"`
	HorasAcumuladas int    `json:"horas_acumuladas"`
}

// PerfilAdministrativo contiene la información del cargo.
type PerfilAdministrativo struct {
	Cargo string `json:"cargo"`
}

// PerfilCreador contiene la información de institución o maestro.
type PerfilCreador struct {
	Institucion string `json:"institucion"`
	Tipo        string `json:"tipo"`
	Descripcion string `json:"descripcion"`
}

// User representa a un usuario en la base de datos.
type User struct {
	ID              string                `json:"id"`
	Email           string                `json:"email"`
	PasswordHash    string                `json:"-"`
	Nombre          string                `json:"nombre"`
	ApellidoPaterno string                `json:"apellido_paterno"`
	ApellidoMaterno string                `json:"apellido_materno"`
	Telefono        string                `json:"telefono"`
	Activo          bool                  `json:"activo"`
	Role            Role                  `json:"role"` 
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
	
	// Perfiles adicionales cargados dinámicamente según rol
	PerfilAlumno         *PerfilAlumno         `json:"perfil_alumno,omitempty"`
	PerfilAdministrativo *PerfilAdministrativo `json:"perfil_administrativo,omitempty"`
	PerfilCreador        *PerfilCreador        `json:"perfil_creador,omitempty"`
}

// Notification representa una alerta para el usuario.
type Notification struct {
	ID        string    `json:"id"`
	UserID    string    `json:"usuario_id"`
	Titulo    string    `json:"titulo"`
	Mensaje   string    `json:"mensaje"`
	Leida     bool      `json:"leida"`
	Fecha     time.Time `json:"fecha"`
}

