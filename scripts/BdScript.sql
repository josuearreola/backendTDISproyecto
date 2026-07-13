-- 01_schema_base.sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 email VARCHAR(120) UNIQUE NOT NULL,
 password_hash TEXT NOT NULL,
 nombre VARCHAR(80) NOT NULL,
 apellido_paterno VARCHAR(80),
 apellido_materno VARCHAR(80),
 telefono VARCHAR(20),
 activo BOOLEAN DEFAULT TRUE,
 created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
 updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE roles(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 nombre VARCHAR(50) UNIQUE NOT NULL
);

CREATE TABLE user_roles(
 user_id UUID REFERENCES users(id) ON DELETE CASCADE,
 role_id UUID REFERENCES roles(id) ON DELETE CASCADE,
 PRIMARY KEY(user_id,role_id)
);

CREATE TABLE alumnos(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID UNIQUE REFERENCES users(id),
 matricula VARCHAR(20) UNIQUE NOT NULL,
 grupo VARCHAR(20),
 carrera VARCHAR(100),
 cuatrimestre SMALLINT,
 tutor VARCHAR(120),
 meta_horas INT DEFAULT 60,
 horas_acumuladas INT DEFAULT 0
);

CREATE TABLE administrativos(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID UNIQUE REFERENCES users(id),
 cargo VARCHAR(80) NOT NULL
);

CREATE TABLE creadores_tdi(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID REFERENCES users(id) ON DELETE SET NULL,
 institucion VARCHAR(150) NOT NULL,
 tipo VARCHAR(40) NOT NULL,
 descripcion TEXT
);



CREATE TABLE dimensiones(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 nombre VARCHAR(80) UNIQUE NOT NULL
);

CREATE TABLE trascendencias(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 nombre VARCHAR(80) UNIQUE NOT NULL
);

CREATE TABLE entornos(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 nombre VARCHAR(80) UNIQUE NOT NULL
);

INSERT INTO roles(nombre) VALUES ('ALUMNO'),('ADMINISTRATIVO'),('CREADOR_TDI'),('COORDINADOR');

-- 02_schema_tdi.sql
CREATE TABLE categorias(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 nombre VARCHAR(80) UNIQUE NOT NULL
);

CREATE TABLE catalogo_tdi(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 nombre VARCHAR(150) NOT NULL,
 descripcion TEXT,
 evidencia_requerida TEXT,
 horas INT NOT NULL,
 puntaje INT NOT NULL,
 fecha_creacion DATE,
 fecha_vencimiento DATE,
 categoria_id UUID REFERENCES categorias(id),
 dimension_id UUID REFERENCES dimensiones(id),
 trascendencia_id UUID REFERENCES trascendencias(id),
 entorno_id UUID REFERENCES entornos(id),
 creador_id UUID REFERENCES creadores_tdi(id)
);

CREATE TABLE registro_tdi(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 alumno_id UUID REFERENCES alumnos(id),
 catalogo_tdi_id UUID REFERENCES catalogo_tdi(id),
 estado VARCHAR(20) CHECK (estado IN ('PENDIENTE','EN_REVISION','APROBADA','RECHAZADA')),
 semaforo VARCHAR(10) CHECK (semaforo IN ('VERDE','AMARILLO','ROJO')),
 horas_otorgadas INT,
 puntaje_obtenido INT,
 fecha_registro TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
 fecha_aprobacion TIMESTAMP,
 motivo_rechazo TEXT
);

CREATE TABLE evidencias(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 registro_tdi_id UUID REFERENCES registro_tdi(id) ON DELETE CASCADE,
 url TEXT NOT NULL,
 nombre_archivo VARCHAR(255),
 mime_type VARCHAR(100),
 hash_sha256 CHAR(64),
 tamano_bytes BIGINT
);

CREATE TABLE validaciones(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 registro_tdi_id UUID REFERENCES registro_tdi(id) ON DELETE CASCADE,
 ocr_exitoso BOOLEAN,
 texto_extraido TEXT,
 coincidencia NUMERIC(5,2),
 resultado VARCHAR(20),
 observaciones TEXT,
 fecha TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE revisiones(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 registro_tdi_id UUID REFERENCES registro_tdi(id),
 administrativo_id UUID REFERENCES administrativos(id),
 decision VARCHAR(20),
 comentario TEXT,
 fecha TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE historial_estados(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 registro_tdi_id UUID REFERENCES registro_tdi(id),
 estado_anterior VARCHAR(20),
 estado_nuevo VARCHAR(20),
 usuario_id UUID REFERENCES users(id),
 fecha TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE notificaciones(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 usuario_id UUID REFERENCES users(id),
 titulo VARCHAR(150),
 mensaje TEXT,
 leida BOOLEAN DEFAULT FALSE,
 fecha TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE progreso_alumno(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 alumno_id UUID REFERENCES alumnos(id),
 dimension_id UUID REFERENCES dimensiones(id),
 horas INT DEFAULT 0,
 porcentaje NUMERIC(5,2) DEFAULT 0,
 UNIQUE(alumno_id,dimension_id)
);

CREATE TABLE badges(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 nombre VARCHAR(100),
 descripcion TEXT,
 puntos_requeridos INT
);

CREATE TABLE alumno_badges(
 alumno_id UUID REFERENCES alumnos(id) ON DELETE CASCADE,
 badge_id UUID REFERENCES badges(id) ON DELETE CASCADE,
 fecha TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(alumno_id,badge_id)
);
