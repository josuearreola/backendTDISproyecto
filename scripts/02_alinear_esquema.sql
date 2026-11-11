-- 02_alinear_esquema.sql
-- Agrega lo que el código usa y que no existía en la BD de producción (ProyectoTDIS)
-- ni en BdScript.sql. Solo agrega columnas/tablas y recalcula los contadores de horas
-- desde registro_tdi; no borra nada y se puede correr varias veces. Correr antes de 03_auditoria.sql.
BEGIN;

-- Horas totales del alumno (las suma el dictamen y el worker).
ALTER TABLE alumnos ADD COLUMN IF NOT EXISTS horas_acumuladas INT DEFAULT 0;

-- Usuario que creó la actividad del catálogo.
ALTER TABLE catalogo_tdi ADD COLUMN IF NOT EXISTS creador_user_id UUID REFERENCES users(id);

-- Fecha de subida de la evidencia (producción ya la tenía; BdScript.sql no).
ALTER TABLE evidencias ADD COLUMN IF NOT EXISTS fecha_subida TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP;

-- Progreso por dimensión. El UNIQUE lo exige el ON CONFLICT del dictamen y del worker.
CREATE TABLE IF NOT EXISTS progreso_alumno(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 alumno_id UUID REFERENCES alumnos(id),
 dimension_id UUID REFERENCES dimensiones(id),
 horas INT DEFAULT 0,
 porcentaje NUMERIC(5,2) DEFAULT 0,
 UNIQUE(alumno_id,dimension_id)
);

-- Descripción de las tablas maestras (producción ya la tenía; BdScript.sql no,
-- y sin ella fallan GET /categorias, /dimensiones, /trascendencias y /entornos).
ALTER TABLE categorias ADD COLUMN IF NOT EXISTS descripcion TEXT;
ALTER TABLE dimensiones ADD COLUMN IF NOT EXISTS descripcion TEXT;
ALTER TABLE trascendencias ADD COLUMN IF NOT EXISTS descripcion TEXT;
ALTER TABLE entornos ADD COLUMN IF NOT EXISTS descripcion TEXT;

-- Backfill desde los registros aprobados: los contadores nuevos arrancan en 0 aunque
-- ya existan aprobaciones. Recalcular desde la fuente es idempotente.
UPDATE alumnos a SET horas_acumuladas = COALESCE((
  SELECT SUM(r.horas_otorgadas) FROM registro_tdi r
  WHERE r.alumno_id = a.id AND r.estado = 'APROBADA'), 0);

INSERT INTO progreso_alumno (alumno_id, dimension_id, horas, porcentaje)
SELECT r.alumno_id, c.dimension_id, SUM(r.horas_otorgadas),
       LEAST(100, SUM(r.horas_otorgadas) * 100.0 / NULLIF(MAX(a.meta_horas), 0))
FROM registro_tdi r
JOIN catalogo_tdi c ON r.catalogo_tdi_id = c.id
JOIN alumnos a ON r.alumno_id = a.id
WHERE r.estado = 'APROBADA' AND c.dimension_id IS NOT NULL
GROUP BY r.alumno_id, c.dimension_id
ON CONFLICT (alumno_id, dimension_id) DO UPDATE SET
  horas = EXCLUDED.horas, porcentaje = EXCLUDED.porcentaje;

COMMIT;
