-- 03_auditoria.sql
-- Bitácora de auditoría. Sin FK a users a propósito: el registro debe sobrevivir
-- aunque el usuario se borre, y el gateway puede auditar peticiones anónimas (login).
CREATE TABLE IF NOT EXISTS audit_log(
 id BIGSERIAL PRIMARY KEY,
 fecha TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 origen VARCHAR(20) NOT NULL CHECK (origen IN ('GATEWAY','DOMINIO','WORKER')),
 usuario_id UUID,
 rol VARCHAR(50),
 accion VARCHAR(120) NOT NULL,      -- 'POST /api/tdi/catalogo' o 'CATALOGO_CREADO'
 entidad VARCHAR(50),               -- 'catalogo_tdi', 'revisiones', 'registro_tdi'
 entidad_id TEXT,
 status INT,                        -- solo GATEWAY
 ip VARCHAR(64),                    -- solo GATEWAY
 antes JSONB,
 despues JSONB
);

CREATE INDEX IF NOT EXISTS idx_audit_fecha ON audit_log(fecha DESC);
CREATE INDEX IF NOT EXISTS idx_audit_usuario ON audit_log(usuario_id, fecha DESC);
CREATE INDEX IF NOT EXISTS idx_audit_entidad ON audit_log(entidad, entidad_id);

-- Las bandejas de notificaciones se consultan por usuario y fecha.
CREATE INDEX IF NOT EXISTS idx_notificaciones_usuario ON notificaciones(usuario_id, fecha DESC);
