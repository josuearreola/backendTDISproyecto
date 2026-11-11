package db

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// Prueba de integración contra un Postgres desechable con scripts/BdScript.sql + 03_auditoria.sql.
// TEST_DATABASE_URL=postgres://... go test ./tdi-service-tdis/internal/db/
func TestReportesYAuditoria(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL no definida")
	}
	ctx := context.Background()
	s, err := New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var alumnoUser, adminUser, alumnoID, dimID, catID, regID, revID string
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, nombre, apellido_paterno) VALUES ('alumno@test.mx','x','José','Peña') RETURNING id`).Scan(&alumnoUser))
	must(s.pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, nombre) VALUES ('admin@test.mx','x','Admin') RETURNING id`).Scan(&adminUser))
	must(s.pool.QueryRow(ctx, `INSERT INTO alumnos (user_id, matricula, carrera, meta_horas) VALUES ($1,'202337153','TIDSM',10) RETURNING id`, alumnoUser).Scan(&alumnoID))
	_, err = s.pool.Exec(ctx, `INSERT INTO administrativos (user_id, cargo) VALUES ($1,'Director')`, adminUser)
	must(err)
	must(s.pool.QueryRow(ctx, `INSERT INTO dimensiones (nombre) VALUES ('Intelectual') RETURNING id`).Scan(&dimID))
	must(s.pool.QueryRow(ctx, `INSERT INTO catalogo_tdi (nombre, horas, puntaje, dimension_id, fecha_vencimiento) VALUES ('Congreso IA',10,5,$1,'2099-01-01') RETURNING id`, dimID).Scan(&catID))
	must(s.pool.QueryRow(ctx, `INSERT INTO registro_tdi (alumno_id, catalogo_tdi_id, estado) VALUES ($1,$2,'EN REVISION') RETURNING id`, alumnoID, catID).Scan(&regID))
	_, err = s.pool.Exec(ctx, `INSERT INTO validaciones (registro_tdi_id, resultado) VALUES ($1,'REVISION_MANUAL'), ($1,'APROBADA')`, regID)
	must(err)
	must(s.pool.QueryRow(ctx, `INSERT INTO revisiones (registro_tdi_id, decision, comentario) VALUES ($1,'PENDIENTE','CLIP 58%') RETURNING id`, regID).Scan(&revID))

	// Dictamen: la segunda vez no debe volver a sumar horas.
	det, err := s.GetStudentAndTDIInfoFromRevision(ctx, revID)
	must(err)
	must(s.ResolveRevision(ctx, revID, adminUser, "ADMINISTRATIVO", "APROBADA", "ok", det))
	if err := s.ResolveRevision(ctx, revID, adminUser, "ADMINISTRATIVO", "APROBADA", "ok", det); !errors.Is(err, ErrRevisionYaResuelta) {
		t.Fatalf("segundo dictamen: err = %v", err)
	}
	var horas int
	must(s.pool.QueryRow(ctx, `SELECT horas_acumuladas FROM alumnos WHERE id = $1`, alumnoID).Scan(&horas))
	if horas != 10 {
		t.Fatalf("horas_acumuladas = %d, se esperaban 10", horas)
	}

	m, err := s.GetMetricas(ctx, Rango{})
	must(err)
	if m.AlumnosMetaCumplida != 1 || m.TasaAutoAprobacion != 50 || m.RevisionesPendientes != 0 ||
		len(m.HorasPorDimension) != 1 || m.HorasPorDimension[0].Total != 10 {
		t.Fatalf("métricas inesperadas: %+v", m)
	}

	f := ReporteFiltro{Carrera: "TIDSM"}
	for nombre, fn := range map[string]func(context.Context, ReporteFiltro) (*Tabla, error){
		"registros": s.ReporteRegistros, "progreso": s.ReporteProgreso, "revisiones": s.ReporteRevisiones,
	} {
		tb, err := fn(ctx, f)
		if err != nil || len(tb.Filas) != 1 || len(tb.Filas[0]) != len(tb.Columnas) {
			t.Fatalf("%s: %v %+v", nombre, err, tb)
		}
	}

	// Auditoría: dominio (del dictamen) + gateway (de la cola).
	must(s.SaveGatewayAudit(ctx, []byte(`{"origen":"GATEWAY","accion":"POST /api/tdi/catalogo","status":201,"ip":"1.2.3.4","usuario_id":"`+adminUser+`","rol":"ADMINISTRATIVO","fecha":"2026-11-10T10:00:00Z"}`)))
	must(s.SaveGatewayAudit(ctx, []byte(`basura`))) // malformado: se descarta sin error
	entries, err := s.ListAudit(ctx, AuditFilter{UsuarioID: adminUser})
	must(err)
	if len(entries) != 2 {
		t.Fatalf("auditoría: %d entradas, se esperaban 2: %+v", len(entries), entries)
	}
	dominio, err := s.ListAudit(ctx, AuditFilter{Entidad: "revisiones", EntidadID: revID})
	must(err)
	if len(dominio) != 1 || dominio[0].Accion != "DICTAMEN_APROBADA" || dominio[0].Despues == nil {
		t.Fatalf("auditoría de dominio: %+v", dominio)
	}
	tb, err := s.ReporteAuditoria(ctx, AuditFilter{})
	must(err)
	if len(tb.Filas) != 2 || tb.Filas[0][2] == "" {
		t.Fatalf("reporte auditoría: %+v", tb.Filas)
	}

	// Una ruta enviada por el cliente más larga que VARCHAR(120) se recorta en vez de fallar
	// (antes el consumidor la reencolaba para siempre y bloqueaba audit_queue).
	largo := "POST /" + strings.Repeat("a", 300)
	must(s.SaveGatewayAudit(ctx, []byte(`{"accion":"`+largo+`","status":404,"ip":"`+strings.Repeat("9", 100)+`"}`)))

	// Un dato que la BD rechaza se clasifica como permanente; uno de conexión, no.
	_, err = s.ListAudit(ctx, AuditFilter{UsuarioID: "no-es-uuid"})
	if !ErrorPermanente(err) {
		t.Fatalf("UUID inválido debería ser error permanente: %v", err)
	}
	if ErrorPermanente(errors.New("connection refused")) {
		t.Fatal("un error de red no es permanente")
	}
}
