package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/luuboon/pti-uteq-backend/tdi-service-tdis/internal/db"
)


type Handler struct {
	Store *db.Store
	RDB   *redis.Client
}

// ListCatalogHandler lista y filtra las actividades vigentes del catálogo.
// @Summary Listar y filtrar catálogo de TDIs
// @Description Obtiene todas las actividades vigentes con filtros opcionales
// @Tags tdi-catalog
// @Accept json
// @Produce json
// @Param categoria_id query string false "Filtrar por ID de Categoría"
// @Param dimension_id query string false "Filtrar por ID de Dimensión"
// @Param entorno_id query string false "Filtrar por ID de Entorno"
// @Param trascendencia_id query string false "Filtrar por ID de Trascendencia"
// @Param search query string false "Buscador por nombre o descripción"
// @Success 200 {array} db.CatalogItem
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/tdi/catalogo [get]
func (h *Handler) ListCatalogHandler(w http.ResponseWriter, r *http.Request) {
	categoryID := r.URL.Query().Get("categoria_id")
	dimensionID := r.URL.Query().Get("dimension_id")
	entornoID := r.URL.Query().Get("entorno_id")
	trascendenciaID := r.URL.Query().Get("trascendencia_id")
	search := r.URL.Query().Get("search")

	items, err := h.Store.ListCatalog(r.Context(), categoryID, dimensionID, entornoID, trascendenciaID, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo obtener el catálogo")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// GetCatalogItemHandler obtiene una única actividad por su ID.
// @Summary Obtener detalle de una TDI del catálogo
// @Description Obtiene los detalles de una actividad específica del catálogo
// @Tags tdi-catalog
// @Produce json
// @Param id path string true "ID de la TDI"
// @Success 200 {object} db.CatalogItem
// @Failure 404 {object} map[string]string "No encontrada"
// @Router /api/tdi/catalogo/{id} [get]
func (h *Handler) GetCatalogItemHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	item, err := h.Store.GetCatalogItem(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type saveTDIReq struct {
	Nombre             string `json:"nombre"`
	Descripcion        string `json:"descripcion"`
	EvidenciaRequerida string `json:"evidencia_requerida"`
	Horas              int    `json:"horas"`
	Puntaje            int    `json:"puntaje"`
	FechaVencimiento   string `json:"fecha_vencimiento"` // Formato AAAA-MM-DD
	CategoriaID        string `json:"categoria_id"`
	DimensionID        string `json:"dimension_id"`
	TrascendenciaID    string `json:"trascendencia_id"`
	EntornoID          string `json:"entorno_id"`
}
// CreateCatalogItemHandler crea una nueva actividad.
// @Summary Registrar nueva TDI en catálogo
// @Description Inserta una actividad al catálogo general verificando el rol del usuario en la base de datos
// @Tags tdi-catalog
// @Accept json
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID del usuario que ejecuta la acción"
// @Param request body saveTDIReq true "Datos de la actividad"
// @Success 201 {object} map[string]string "Actividad creada"
// @Failure 403 {object} map[string]string "No autorizado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/tdi/catalogo [post]
func (h *Handler) CreateCatalogItemHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "no autorizado: falta ID de usuario")
		return
	}

	// Verificar permisos y obtener el creador_id de forma segura en la base de datos
	creadorID, err := h.Store.VerifyCreatorAuthorization(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	var req saveTDIReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}

	vencimiento, err := time.Parse("2006-01-02", req.FechaVencimiento)
	if err != nil {
		writeError(w, http.StatusBadRequest, "fecha de vencimiento inválida (formato AAAA-MM-DD)")
		return
	}

	item := db.CatalogItem{
		Nombre:             req.Nombre,
		Descripcion:        req.Descripcion,
		EvidenciaRequerida: req.EvidenciaRequerida,
		Horas:              req.Horas,
		Puntaje:            req.Puntaje,
		FechaCreacion:      time.Now(),
		FechaVencimiento:   vencimiento,
		CategoriaID:        req.CategoriaID,
		DimensionID:        req.DimensionID,
		TrascendenciaID:    req.TrascendenciaID,
		EntornoID:          req.EntornoID,
		CreadorID:          creadorID,
	}

	if err := h.Store.CreateCatalogItem(r.Context(), item); err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo crear la actividad")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "actividad creada en catálogo con éxito"})
}

// UpdateCatalogItemHandler modifica una actividad existente.
// @Summary Editar una TDI del catálogo
// @Description Actualiza los campos de una actividad específica del catálogo verificando el rol del usuario en la BD
// @Tags tdi-catalog
// @Accept json
// @Produce json
// @Param X-User-Id header string true "ID del usuario que ejecuta la acción"
// @Param id path string true "ID de la TDI a editar"
// @Param request body saveTDIReq true "Datos actualizados"
// @Success 200 {object} map[string]string "Actividad actualizada"
// @Router /api/tdi/catalogo/{id} [put]
func (h *Handler) UpdateCatalogItemHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "no autorizado: falta ID de usuario")
		return
	}

	// Verificar permisos en la base de datos
	_, err := h.Store.VerifyCreatorAuthorization(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

	var req saveTDIReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}

	vencimiento, err := time.Parse("2006-01-02", req.FechaVencimiento)
	if err != nil {
		writeError(w, http.StatusBadRequest, "fecha de vencimiento inválida")
		return
	}

	item := db.CatalogItem{
		Nombre:             req.Nombre,
		Descripcion:        req.Descripcion,
		EvidenciaRequerida: req.EvidenciaRequerida,
		Horas:              req.Horas,
		Puntaje:            req.Puntaje,
		FechaVencimiento:   vencimiento,
		CategoriaID:        req.CategoriaID,
		DimensionID:        req.DimensionID,
		TrascendenciaID:    req.TrascendenciaID,
		EntornoID:          req.EntornoID,
	}

	if err := h.Store.UpdateCatalogItem(r.Context(), id, item); err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo actualizar la actividad")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "actividad actualizada con éxito"})
}


// DeleteCatalogItemHandler borra o expira una actividad del catálogo.
// @Summary Eliminar una TDI del catálogo
// @Description Elimina físicamente o expira lógicamente una actividad si ya tiene alumnos registrados
// @Tags tdi-catalog
// @Produce json
// @Param X-User-Role header string true "Rol de usuario"
// @Param id path string true "ID de la TDI a borrar"
// @Success 200 {object} map[string]string "Actividad eliminada"
// @Router /api/tdi/catalogo/{id} [delete]
func (h *Handler) DeleteCatalogItemHandler(w http.ResponseWriter, r *http.Request) {
	role := r.Header.Get("X-User-Role")
	if role != "CREADOR_TDI" && role != "COORDINADOR" && role != "ADMINISTRATIVO" {
		writeError(w, http.StatusForbidden, "no tienes permisos para borrar del catálogo")
		return
	}

	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	if err := h.Store.DeleteCatalogItem(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo eliminar la actividad")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "actividad eliminada/desactivada con éxito"})
}

type selectTDIReq struct {
	CatalogoTDIID string `json:"catalogo_tdi_id"`
}
// SeleccionarTDIHandler asocia un alumno a una actividad.
// @Summary Seleccionar una actividad
// @Description Asocia una actividad del catálogo a la cuenta de un alumno
// @Tags tdi-catalog
// @Accept json
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID del alumno que ejecuta la acción"
// @Param request body selectTDIReq true "ID de la actividad a seleccionar"
// @Success 200 {object} map[string]string "Actividad seleccionada"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/tdi/registro/seleccionar [post]
func (h *Handler) SeleccionarTDIHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "no autorizado")
		return
	}

	var req selectTDIReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}

	registroID, err := h.Store.SeleccionarTDI(r.Context(), userID, req.CatalogoTDIID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message":         "actividad seleccionada con éxito",
		"registro_tdi_id": registroID,
	})
}

// Utilidades JSON
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
// SubirEvidenciaHandler procesa la subida física de la evidencia al servidor local.
// @Summary Subir archivo de evidencia (Autohospedado)
// @Description Recibe un archivo (PDF, JPG, PNG, Word, Excel), lo renombra usando la matrícula del alumno, calcula su hash y lo guarda localmente.
// @Tags tdi-catalog
// @Accept multipart/form-data
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID del alumno que ejecuta la acción"
// @Param id path string true "ID del registro de participación (registro_tdi_id)"
// @Param archivo formData file true "Archivo de evidencia (Max 5MB)"
// @Success 200 {object} map[string]string "Evidencia subida y estado actualizado"
// @Failure 400 {object} map[string]string "Cuerpo o archivo inválido"
// @Failure 500 {object} map[string]string "Error interno al guardar"
// @Router /api/tdi/registro/{id}/subir-evidencia [post]
func (h *Handler) SubirEvidenciaHandler(w http.ResponseWriter, r *http.Request) {
	// Recortar primero la ruta para limpiar el sufijo
	trimmedPath := strings.TrimSuffix(r.URL.Path, "/subir-evidencia")
	registroID := trimmedPath[strings.LastIndex(trimmedPath, "/")+1:]

	if registroID == "" {
		writeError(w, http.StatusBadRequest, "ID de registro inválido")
		return
	}



	// Limitar subida a 5MB
	r.ParseMultipartForm(5 << 20)

	file, header, err := r.FormFile("archivo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "archivo no proporcionado o inválido")
		return
	}
	defer file.Close()

	// Validar extensión permitida
	ext := strings.ToLower(filepath.Ext(header.Filename))
	permitido := false
	extsPermitidas := []string{".pdf", ".png", ".jpg", ".jpeg", ".doc", ".docx", ".xls", ".xlsx"}
	for _, e := range extsPermitidas {
		if ext == e {
			permitido = true
			break
		}
	}

	if !permitido {
		writeError(w, http.StatusBadRequest, "formato no permitido. Solo se aceptan PDF, Imágenes (PNG/JPG), Word y Excel")
		return
	}

	// 1. Obtener matrícula del alumno
	matricula, _, err := h.Store.GetStudentAndTDIInfo(r.Context(), registroID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo obtener la matrícula del alumno: "+err.Error())
		return
	}

	// Nombre limitado únicamente a la matrícula del alumno más la extensión original
	nuevoNombre := fmt.Sprintf("%s%s", matricula, ext)

	// Crear carpeta local uploads/ si no existe
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		writeError(w, http.StatusInternalServerError, "error al preparar el directorio de subidas")
		return
	}

	// Ruta final del archivo local
	filePath := filepath.Join(uploadDir, nuevoNombre)
	out, err := os.Create(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo guardar el archivo en el servidor")
		return
	}
	defer out.Close()

	// 2. Calcular hash SHA-256 en caliente mientras copiamos el archivo a disco
	hashCalculator := sha256.New()
	writer := io.MultiWriter(out, hashCalculator)

	if _, err = io.Copy(writer, file); err != nil {
		writeError(w, http.StatusInternalServerError, "error al escribir el archivo")
		return
	}

	fileHash := hex.EncodeToString(hashCalculator.Sum(nil))
	// La URL pública del archivo en el servidor local
	urlArchivo := fmt.Sprintf("/uploads/%s", nuevoNombre)

	// Extraer el tipo MIME del header de subida
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

		// 3. Guardar metadatos y actualizar estado a EN_REVISION
	err = h.Store.SaveEvidenceMetadata(r.Context(), registroID, urlArchivo, nuevoNombre, mimeType, fileHash, header.Size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error al guardar metadatos de evidencia: "+err.Error())
		return
	}

	// 4. Encolar tarea de validación asíncrona en Redis (Paso 3)
	if h.RDB != nil {
		taskPayload := map[string]interface{}{
			"registro_tdi_id": registroID,
			"url":             urlArchivo,
			"nombre_archivo":  nuevoNombre,
			"mime_type":       mimeType,
			"hash_sha256":     fileHash,
			"tamano_bytes":    header.Size,
		}

		payloadBytes, err := json.Marshal(taskPayload)
		if err == nil {
			err = h.RDB.LPush(r.Context(), "validation_queue", payloadBytes).Err()
			if err != nil {
				fmt.Printf("[TDI-SERVICE ] Error al encolar tarea en Redis: %v\n", err)
			} else {
				fmt.Printf("[TDI-SERVICE ] Tarea de validación encolada para registro %s\n", registroID)
			}
		}
	} else {
		fmt.Println("[TDI-SERVICE ] Advertencia: Redis no configurado. La tarea no fue encolada en Redis.")
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message":         "evidencia subida con éxito. Estado del registro cambiado a EN_REVISION",
		"nombre_archivo":  nuevoNombre,
		"url_archivo":     urlArchivo,
		"hash":            fileHash,
		"mime_type":       mimeType,
		"tamano_bytes":    fmt.Sprintf("%d", header.Size),
	})
}

// GetAlumnoRegistrosHandler retorna las actividades en las que se ha inscrito el alumno.
// @Summary Listar registros/inscripciones del alumno
// @Description Retorna el historial de inscripciones a actividades del alumno logueado, con su respectivo estado.
// @Tags alumnos-info
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID del usuario (Alumno)"
// @Success 200 {array} map[string]interface{} "Lista de inscripciones"
// @Failure 401 {object} map[string]string "No autorizado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/tdi/registro/mis-registros [get]
func (h *Handler) GetAlumnoRegistrosHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "identidad del alumno no proporcionada")
		return
	}

	registros, err := h.Store.GetAlumnoRegistros(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error al consultar registros: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, registros)
}

// GetAlumnoProgresoHandler retorna el avance de puntos acumulados generales y por dimensión del alumno.
// @Summary Obtener progreso de puntos del alumno
// @Description Retorna la meta de puntos, puntos totales y el desglose de puntos por cada dimensión del alumno logueado.
// @Tags alumnos-info
// @Produce json
// @Security Bearer
// @Param X-User-Id header string true "ID del usuario (Alumno)"
// @Success 200 {object} map[string]interface{} "Progreso de puntos"
// @Failure 401 {object} map[string]string "No autorizado"
// @Failure 500 {object} map[string]string "Error interno"
// @Router /api/tdi/alumnos/progreso [get]
func (h *Handler) GetAlumnoProgresoHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "identidad del alumno no proporcionada")
		return
	}

	progreso, err := h.Store.GetAlumnoProgreso(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error al calcular el progreso: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, progreso)
}
