-- 02_alinear_esquema.sql
-- Agrega lo que el código usa y que no existía en la BD de producción (ProyectoTDIS)
-- ni en BdScript.sql. Solo agrega: no borra ni modifica datos, y se puede correr
-- varias veces. Correr antes de 03_auditoria.sql.
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

COMMIT;
