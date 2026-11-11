package handlers

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luuboon/pti-uteq-backend/tdi-service-tdis/internal/db"
	"github.com/luuboon/pti-uteq-backend/tdi-service-tdis/internal/export"
)

// Roles con acceso a reportes, métricas y auditoría.
var rolesReportes = []string{"ADMINISTRATIVO", "COORDINADOR"}

func requireRole(w http.ResponseWriter, r *http.Request, roles []string) bool {
	if !slices.Contains(roles, r.Header.Get("X-User-Role")) {
		writeError(w, http.StatusForbidden, "acceso denegado: rol insuficiente")
		return false
	}
	return true
}

// parseRango lee ?desde=AAAA-MM-DD&hasta=AAAA-MM-DD (ambos opcionales e inclusivos).
func parseRango(r *http.Request) (db.Rango, error) {
	var rg db.Rango
	for _, p := range []struct {
		key string
		dst *time.Time
	}{{"desde", &rg.Desde}, {"hasta", &rg.Hasta}} {
		if v := r.URL.Query().Get(p.key); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				return rg, fmt.Errorf("parámetro '%s' inválido (formato AAAA-MM-DD)", p.key)
			}
			*p.dst = t
		}
	}
	if !rg.Desde.IsZero() && !rg.Hasta.IsZero() && rg.Hasta.Before(rg.Desde) {
		return rg, fmt.Errorf("'hasta' no puede ser anterior a 'desde'")
	}
	return rg, nil
}

func parseAuditFilter(r *http.Request) (db.AuditFilter, error) {
	rg, err := parseRango(r)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	return db.AuditFilter{
		Rango: rg, UsuarioID: q.Get("usuario_id"), Entidad: q.Get("entidad"),
		EntidadID: q.Get("entidad_id"), Origen: q.Get("origen"), Limit: limit, Offset: offset,
	}, err
}

// GetMetricasHandler devuelve los indicadores del dashboard administrativo.
// @Summary Métricas del sistema
// @Description Indicadores de negocio: registros por estado, validaciones de IA, revisiones, horas por dimensión/carrera, top actividades y alumnos con meta cumplida.
// @Tags reportes
// @Produce json
// @Security Bearer
// @Param desde query string false "Fecha inicial AAAA-MM-DD"
// @Param hasta query string false "Fecha final AAAA-MM-DD (inclusiva)"
// @Success 200 {object} db.Metricas
// @Failure 400 {object} map[string]string "Rango inválido"
// @Failure 403 {object} map[string]string "Acceso denegado"
// @Router /api/tdi/metricas [get]
func (h *Handler) GetMetricasHandler(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, rolesReportes) {
		return
	}
	rg, err := parseRango(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := h.Store.GetMetricas(r.Context(), rg)
	if err != nil {
		log.Printf("[TDI-SERVICE ] métricas: %v", err)
		writeError(w, http.StatusInternalServerError, "error al calcular métricas")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// ExportHandler genera un reporte descargable.
// @Summary Exportar reporte
// @Description Datasets: registros, progreso, revisiones, auditoria. Formatos: csv, xlsx, pdf.
// @Tags reportes
// @Produce octet-stream
// @Security Bearer
// @Param dataset path string true "registros | progreso | revisiones | auditoria"
// @Param formato query string false "csv (default) | xlsx | pdf"
// @Param desde query string false "Fecha inicial AAAA-MM-DD"
// @Param hasta query string false "Fecha final AAAA-MM-DD (inclusiva)"
// @Param carrera query string false "Filtrar por carrera (registros, progreso, revisiones)"
// @Param estado query string false "Estado del registro o decisión de la revisión"
// @Param usuario_id query string false "Solo auditoria"
// @Param entidad query string false "Solo auditoria"
// @Param origen query string false "Solo auditoria: GATEWAY | DOMINIO | WORKER"
// @Success 200 {file} file
// @Failure 400 {object} map[string]string "Parámetros inválidos"
// @Failure 403 {object} map[string]string "Acceso denegado"
// @Router /api/tdi/export/{dataset} [get]
func (h *Handler) ExportHandler(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, rolesReportes) {
		return
	}
	nombreFormato := r.URL.Query().Get("formato")
	if nombreFormato == "" {
		nombreFormato = "csv"
	}
	formato, ok := export.Formatos[nombreFormato]
	if !ok {
		writeError(w, http.StatusBadRequest, "formato inválido: usa csv, xlsx o pdf")
		return
	}

	dataset := chi.URLParam(r, "dataset")
	var tabla *db.Tabla
	var err error
	if dataset == "auditoria" {
		var f db.AuditFilter
		if f, err = parseAuditFilter(r); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		tabla, err = h.Store.ReporteAuditoria(r.Context(), f)
	} else {
		rg, perr := parseRango(r)
		if perr != nil {
			writeError(w, http.StatusBadRequest, perr.Error())
			return
		}
		f := db.ReporteFiltro{Rango: rg, Carrera: r.URL.Query().Get("carrera"), Estado: r.URL.Query().Get("estado")}
		switch dataset {
		case "registros":
			tabla, err = h.Store.ReporteRegistros(r.Context(), f)
		case "progreso":
			tabla, err = h.Store.ReporteProgreso(r.Context(), f)
		case "revisiones":
			tabla, err = h.Store.ReporteRevisiones(r.Context(), f)
		default:
			writeError(w, http.StatusNotFound, "dataset inválido: usa registros, progreso, revisiones o auditoria")
			return
		}
	}
	if err != nil {
		log.Printf("[TDI-SERVICE ] export %s: %v", dataset, err)
		writeError(w, http.StatusInternalServerError, "error al generar el reporte")
		return
	}

	// Se genera en memoria para poder responder 500 en JSON si el render falla a medias.
	var buf bytes.Buffer
	if err := formato.Write(&buf, tabla); err != nil {
		log.Printf("[TDI-SERVICE ] render %s/%s: %v", dataset, nombreFormato, err)
		writeError(w, http.StatusInternalServerError, "error al generar el archivo")
		return
	}
	nombre := fmt.Sprintf("%s_%s.%s", dataset, time.Now().Format("20060102_1504"), formato.Ext)
	w.Header().Set("Content-Type", formato.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
}

// GetAuditoriaHandler consulta la bitácora de auditoría.
// @Summary Consultar bitácora de auditoría
// @Description Lista paginada (más reciente primero) de eventos del gateway, dominio y worker.
// @Tags reportes
// @Produce json
// @Security Bearer
// @Param desde query string false "Fecha inicial AAAA-MM-DD"
// @Param hasta query string false "Fecha final AAAA-MM-DD (inclusiva)"
// @Param usuario_id query string false "ID del usuario"
// @Param entidad query string false "catalogo_tdi | revisiones | registro_tdi"
// @Param entidad_id query string false "ID de la entidad"
// @Param origen query string false "GATEWAY | DOMINIO | WORKER"
// @Param limit query int false "Máximo 1000 (default 100)"
// @Param offset query int false "Desplazamiento"
// @Success 200 {array} db.AuditEntry
// @Failure 403 {object} map[string]string "Acceso denegado"
// @Router /api/tdi/auditoria [get]
func (h *Handler) GetAuditoriaHandler(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, rolesReportes) {
		return
	}
	f, err := parseAuditFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := h.Store.ListAudit(r.Context(), f)
	if err != nil {
		log.Printf("[TDI-SERVICE ] auditoría: %v", err)
		if db.ErrorPermanente(err) { // p. ej. usuario_id que no es UUID
			writeError(w, http.StatusBadRequest, "filtros de auditoría inválidos")
			return
		}
		writeError(w, http.StatusInternalServerError, "error al consultar la auditoría")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
