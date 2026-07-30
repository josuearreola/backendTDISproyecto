package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/luuboon/pti-uteq-backend/user-service-tdis/internal/auth"
	"github.com/luuboon/pti-uteq-backend/user-service-tdis/internal/db"
	"github.com/luuboon/pti-uteq-backend/user-service-tdis/internal/models"
)

// Handler agrupa las dependencias que necesitan los endpoints.
type Handler struct {
	Store  *db.Store
	Issuer *auth.Issuer
}


type registerReq struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	Nombre          string `json:"nombre"`
	ApellidoPaterno string `json:"apellido_paterno"`
	ApellidoMaterno string `json:"apellido_materno"`
	Telefono        string `json:"telefono"`
	Role            string `json:"role"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResp struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}
type completarPerfilReq struct {
	Matricula    string `json:"matricula"`
	Grupo        string `json:"grupo"`
	Carrera      string `json:"carrera"`
	Cuatrimestre int    `json:"cuatrimestre"`
	Tutor        string `json:"tutor"`
}
type completarAdministrativoReq struct {
	Cargo string `json:"cargo"`
}
type completarCreadorReq struct {
	Institucion string `json:"institucion"`
	Tipo        string `json:"tipo"` // Maestro, Tutor, Externa, etc.
	Descripcion string `json:"descripcion"`
}


// Register crea un usuario nuevo con la contraseña hasheada.
// @Summary Registrar un nuevo usuario
// @Description Crea un nuevo usuario y emite un token JWT
// @Tags users
// @Accept json
// @Produce json
// @Param request body registerReq true "Datos de registro"
// @Success 201 {object} tokenResp
// @Failure 400 {object} map[string]string "Cuerpo o datos inválidos"
// @Failure 409 {object} map[string]string "El email ya está registrado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/users/register [post]
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}

	// Validaciones básicas de entrada.
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" || req.Nombre == "" {
		writeError(w, http.StatusBadRequest, "email, password y nombre son obligatorios")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "la contraseña debe tener al menos 8 caracteres")
		return
	}
	// Si no se indica rol, por defecto es ALUMNO.
	if req.Role == "" {
		req.Role = string(models.RoleAlumno)
	}
	if !models.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "rol inválido")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo procesar la contraseña")
		return
	}

	user := &models.User{
		ID:              uuid.NewString(),
		Email:           req.Email,
		PasswordHash:    hash,
		Nombre:          req.Nombre,
		ApellidoPaterno: req.ApellidoPaterno,
		ApellidoMaterno: req.ApellidoMaterno,
		Telefono:        req.Telefono,
		Activo:          true,
		Role:            models.Role(req.Role),
	}

	if err := h.Store.CreateUser(r.Context(), user); err != nil {
		if errors.Is(err, db.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "el email ya está registrado")
			return
		}
		writeError(w, http.StatusInternalServerError, "no se pudo crear el usuario")
		return
	}

	// Emitimos un token para que el usuario quede logueado tras registrarse.
	token, err := h.Issuer.Issue(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo emitir el token")
		return
	}

	writeJSON(w, http.StatusCreated, tokenResp{Token: token, User: user})
}

// Login valida credenciales y emite un JWT.
// @Summary Iniciar sesión
// @Description Valida credenciales y emite un JWT
// @Tags users
// @Accept json
// @Produce json
// @Param request body loginReq true "Credenciales"
// @Success 200 {object} tokenResp
// @Failure 400 {object} map[string]string "Cuerpo inválido"
// @Failure 401 {object} map[string]string "Credenciales inválidas"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/users/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

		user, err := h.Store.GetByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "credenciales inválidas")
		return
	}

	// Impedir inicio de sesión si el usuario está inactivo (baja lógica)
	if !user.Activo {
		writeError(w, http.StatusUnauthorized, "Esta cuenta está desactivada. Contacta al administrador")
		return
	}


	if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "credenciales inválidas")
		return
	}

	token, err := h.Issuer.Issue(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo emitir el token")
		return
	}

	writeJSON(w, http.StatusOK, tokenResp{Token: token, User: user})
}

// CompletarPerfilAlumno guarda la información académica del estudiante.
// @Summary Completar perfil del alumno
// @Description Registra o actualiza la información académica de un estudiante
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID de usuario inyectado por el Gateway"
// @Param request body completarPerfilReq true "Datos académicos del alumno"
// @Success 200 {object} map[string]string "Perfil completado"
// @Failure 400 {object} map[string]string "Datos inválidos"
// @Failure 401 {object} map[string]string "No autorizado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/users/alumnos/completar-perfil [post]
func (h *Handler) CompletarPerfilAlumno(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "identidad no proporcionada")
		return
	}
	var req completarPerfilReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	// Validaciones mínimas
	req.Matricula = strings.TrimSpace(req.Matricula)
	req.Carrera = strings.TrimSpace(req.Carrera)
	if req.Matricula == "" || req.Carrera == "" || req.Cuatrimestre <= 0 {
		writeError(w, http.StatusBadRequest, "matrícula, carrera y cuatrimestre son obligatorios y válidos")
		return
	}
	err := h.Store.CreateOrUpdateAlumno(r.Context(), userID, req.Matricula, req.Grupo, req.Carrera, req.Cuatrimestre, req.Tutor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo guardar el perfil académico")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "perfil académico guardado con éxito"})
}


// Me devuelve el perfil del usuario autenticado. El Gateway ya validó el token
// e inyectó el ID en el header X-User-Id; aquí confiamos en ese header.
// @Summary Obtener perfil actual
// @Description Devuelve el perfil del usuario autenticado
// @Tags users
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID de usuario inyectado por el Gateway"
// @Success 200 {object} models.User
// @Failure 401 {object} map[string]string "Falta identidad"
// @Failure 404 {object} map[string]string "Usuario no encontrado"
// @Router /api/users/me [get]
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "falta identidad")
		return
	}

	user, err := h.Store.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// --- Utilidades de respuesta JSON ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
// CompletarPerfilAdministrativo guarda el cargo del administrativo.
// @Summary Completar perfil de administrativo
// @Description Registra o actualiza el cargo de un usuario administrativo
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID de usuario inyectado por el Gateway"
// @Param request body completarAdministrativoReq true "Datos del administrativo"
// @Success 200 {object} map[string]string "Perfil completado"
// @Failure 400 {object} map[string]string "Datos inválidos"
// @Failure 401 {object} map[string]string "No autorizado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/users/administrativos/completar-perfil [post]
func (h *Handler) CompletarPerfilAdministrativo(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "identidad no proporcionada")
		return
	}

	var req completarAdministrativoReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}

	req.Cargo = strings.TrimSpace(req.Cargo)
	if req.Cargo == "" {
		writeError(w, http.StatusBadRequest, "el cargo es obligatorio")
		return
	}

	err := h.Store.CreateOrUpdateAdministrativo(r.Context(), userID, req.Cargo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo guardar el cargo")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "perfil administrativo guardado con éxito"})
}

// CompletarPerfilCreador guarda los datos institucionales de un creador.
// @Summary Completar perfil de creador de TDI
// @Description Registra o actualiza los datos de institución del creador
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID de usuario inyectado por el Gateway"
// @Param request body completarCreadorReq true "Datos del creador"
// @Success 200 {object} map[string]string "Perfil completado"
// @Failure 400 {object} map[string]string "Datos inválidos"
// @Failure 401 {object} map[string]string "No autorizado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/users/creadores/completar-perfil [post]
func (h *Handler) CompletarPerfilCreador(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "identidad no proporcionada")
		return
	}

	var req completarCreadorReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}

	req.Institucion = strings.TrimSpace(req.Institucion)
	req.Tipo = strings.TrimSpace(req.Tipo)
	if req.Institucion == "" || req.Tipo == "" {
		writeError(w, http.StatusBadRequest, "institución y tipo son obligatorios")
		return
	}

	err := h.Store.CreateOrUpdateCreador(r.Context(), userID, req.Institucion, req.Tipo, req.Descripcion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo guardar el perfil de creador")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "perfil de creador guardado con éxito"})
}
// DeleteUserHandler realiza la baja lógica (desactivación) de un usuario.
// @Summary Desactivar un usuario (Baja Lógica)
// @Description Cambia el estado del usuario a inactivo (Solo Administrativo o Coordinador)
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Param X-User-Role header string true "Rol del usuario que ejecuta la acción"
// @Param id path string true "ID del usuario a desactivar"
// @Success 200 {object} map[string]string "Usuario desactivado"
// @Failure 403 {object} map[string]string "Acceso denegado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/users/users/{id} [delete]
func (h *Handler) DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Validar autorización por cabecera
	requesterRole := r.Header.Get("X-User-Role")
	if requesterRole != string(models.RoleAdministrativo) && requesterRole != string(models.RoleCoordinador) {
		writeError(w, http.StatusForbidden, "no tienes permisos para realizar esta acción")
		return
	}

	// 2. Obtener el ID de la ruta
	userID := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	if userID == "" {
		writeError(w, http.StatusBadRequest, "ID de usuario inválido")
		return
	}

	// 3. Ejecutar baja lógica
	err := h.Store.DeactivateUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo desactivar el usuario")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "usuario desactivado con éxito (baja lógica)"})
}
// Logout es un endpoint de simulación para documentación. El API Gateway intercepta esta ruta nativamente para invalidar el JWT.
// @Summary Cerrar sesión (Logout)
// @Description Invalida el token JWT actual agregándolo a la denylist para que no pueda volver a usarse.
// @Tags auth
// @Security Bearer
// @Success 200 {object} map[string]string "Sesión cerrada con éxito"
// @Failure 401 {object} map[string]string "Token inválido o no proporcionado"
// @Router /api/users/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// Esta función nunca se ejecutará realmente, el Gateway responde antes.
}

// GetNotificationsHandler obtiene el listado de notificaciones del usuario logueado.
// @Summary Obtener notificaciones del usuario
// @Description Retorna el listado de alertas y notificaciones del usuario autenticado.
// @Tags notificaciones
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID del usuario logueado"
// @Success 200 {array} models.Notification "Lista de notificaciones"
// @Failure 401 {object} map[string]string "No autorizado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/users/notificaciones [get]
func (h *Handler) GetNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "no autorizado")
		return
	}

	notifications, err := h.Store.GetNotifications(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error al obtener notificaciones: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, notifications)
}

// MarkNotificationReadHandler marca una notificación como leída.
// @Summary Marcar notificación como leída
// @Description Cambia el estado de leída a true para la notificación provista en la ruta.
// @Tags notificaciones
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID del usuario logueado"
// @Param id path string true "ID de la notificación"
// @Success 200 {object} map[string]string "Notificación marcada como leída"
// @Failure 401 {object} map[string]string "No autorizado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/users/notificaciones/{id}/leer [put]
func (h *Handler) MarkNotificationReadHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "no autorizado")
		return
	}

	// Obtener el ID de la notificación de la ruta
	trimmedPath := strings.TrimSuffix(r.URL.Path, "/leer")
	notificationID := trimmedPath[strings.LastIndex(trimmedPath, "/")+1:]
	if notificationID == "" {
		writeError(w, http.StatusBadRequest, "ID de notificación inválido")
		return
	}

	err := h.Store.MarkNotificationRead(r.Context(), userID, notificationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error al marcar notificación como leída: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "notificación marcada como leída con éxito"})
}

