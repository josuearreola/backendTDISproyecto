import os
import json
import time
import redis
import psycopg2
from psycopg2.extras import RealDictCursor
from dotenv import load_dotenv

import validator

# Cargar variables de entorno del archivo .env en la raíz
load_dotenv(dotenv_path="../.env")

REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379/0")
DATABASE_URL = os.getenv("DATABASE_URL")

def get_db_connection():
    """Establece conexión a la base de datos Postgres (Neon)."""
    return psycopg2.connect(DATABASE_URL, cursor_factory=RealDictCursor)

def process_validation_job(rdb, db_conn, job_data):
    registro_id = job_data.get("registro_tdi_id")
    nombre_archivo = job_data.get("nombre_archivo")
    url_archivo = job_data.get("url")
    hash_sha256 = job_data.get("hash_sha256")
    
    print(f"\n[Worker] Procesando validación para el registro: {registro_id}...")
    
    cursor = db_conn.cursor()
    try:
        # 1. Obtener los detalles del alumno y la actividad a partir del registro_tdi_id
        cursor.execute("""
            SELECT 
                u.nombre, u.apellido_paterno, u.apellido_materno,
                a.matricula,
                c.nombre AS tdi_nombre,
                c.horas AS tdi_horas,
                c.puntaje AS tdi_puntaje,
                a.id AS alumno_id,
                a.meta_horas,
                c.dimension_id
            FROM registro_tdi r
            JOIN alumnos a ON r.alumno_id = a.id
            JOIN users u ON a.user_id = u.id
            JOIN catalogo_tdi c ON r.catalogo_tdi_id = c.id
            WHERE r.id = %s
        """, (registro_id,))
        
        student_data = cursor.fetchone()
        if not student_data:
            print(f"[Worker] Error: No se encontró información del alumno para el registro {registro_id}")
            return
            
        student_name = f"{student_data['nombre']} {student_data['apellido_paterno'] or ''} {student_data['apellido_materno'] or ''}".strip()
        matricula = student_data["matricula"]
        tdi_nombre = student_data["tdi_nombre"]
        tdi_horas = student_data["tdi_horas"]
        tdi_puntaje = student_data["tdi_puntaje"]
        alumno_id = student_data["alumno_id"]
        meta_horas = student_data["meta_horas"] or 60
        dimension_id = student_data["dimension_id"]
        
        # 2. Control de Plagio (Verificar si el mismo hash ya fue subido en otra constancia de otro alumno)
        cursor.execute("""
            SELECT registro_tdi_id FROM evidencias 
            WHERE hash_sha256 = %s AND registro_tdi_id != %s
        """, (hash_sha256, registro_id))
        
        duplicate = cursor.fetchone()
        if duplicate:
            print(f"[Worker] ¡ALERTA DE PLAGIO! El hash {hash_sha256} ya existe en otro registro.")
            obs = f"Rechazo automático: Intento de plagio detectado. El archivo de evidencia es idéntico al del registro {duplicate['registro_tdi_id']}."
            
            # Guardar resultado en validaciones
            cursor.execute("""
                INSERT INTO validaciones (registro_tdi_id, ocr_exitoso, texto_extraido, coincidencia, resultado, observaciones)
                VALUES (%s, FALSE, '', 0.00, 'RECHAZADA', %s)
            """, (registro_id, obs))
            
            # Actualizar registro_tdi a RECHAZADA
            cursor.execute("""
                UPDATE registro_tdi
                SET estado = 'RECHAZADA', motivo_rechazo = %s
                WHERE id = %s
            """, (obs, registro_id))
            
            db_conn.commit()
            print(f"[Worker] Registro {registro_id} rechazado automáticamente por duplicidad.")
            return

        # 3. Ubicar archivo físico localmente
        file_path = os.path.join("../tdi-service-tdis/uploads", nombre_archivo)
        if not os.path.exists(file_path):
            # Fallback por si corre en raíz
            file_path = os.path.join("../uploads", nombre_archivo)
            if not os.path.exists(file_path):
                file_path = os.path.join("./uploads", nombre_archivo)
            
        print(f"[Worker] Leyendo archivo: {file_path}")
        
        # 4. Correr Pipeline de Validación de 4 Capas (OCR, CLIP, Desenfoque)
        aprobado_ia, coincidencia_score, observaciones = validator.process_file_pipeline(
            file_path, tdi_nombre, student_name, matricula
        )
        
        # 5. Guardar Resultados en base de datos
        if aprobado_ia:
            print(f"[Worker] IA APROBÓ automáticamente el registro: {registro_id}")
            
            # Insertar en validaciones
            cursor.execute("""
                INSERT INTO validaciones (registro_tdi_id, ocr_exitoso, texto_extraido, coincidencia, resultado, observaciones)
                VALUES (%s, TRUE, 'Texto procesado por IA', %s, 'APROBADA', %s)
            """, (registro_id, coincidencia_score, observaciones))
            
            # Cambiar estado a APROBADA y asignar puntos en registro_tdi
            cursor.execute("""
                UPDATE registro_tdi
                SET estado = 'APROBADA', horas_otorgadas = %s, puntaje_obtenido = %s, fecha_aprobacion = CURRENT_TIMESTAMP
                WHERE id = %s
            """, (tdi_horas, tdi_puntaje, registro_id))
            
            # Sumar puntos acumulados en alumnos
            cursor.execute("""
                UPDATE alumnos
                SET horas_acumuladas = horas_acumuladas + %s
                WHERE id = %s
            """, (tdi_horas, alumno_id))
            
            # Actualizar o insertar en progreso_alumno por dimensión
            cursor.execute("""
                SELECT horas FROM progreso_alumno 
                WHERE alumno_id = %s AND dimension_id = %s
            """, (alumno_id, dimension_id))
            
            prog = cursor.fetchone()
            new_horas = tdi_horas
            if prog:
                new_horas += prog["horas"]
                
            percentage = min(100.0, float(new_horas * 100.0 / meta_horas))
            
            cursor.execute("""
                INSERT INTO progreso_alumno (alumno_id, dimension_id, horas, porcentaje)
                VALUES (%s, %s, %s, %s)
                ON CONFLICT (alumno_id, dimension_id) DO UPDATE SET
                    horas = EXCLUDED.horas,
                    porcentaje = EXCLUDED.porcentaje
            """, (alumno_id, dimension_id, new_horas, percentage))
            
            db_conn.commit()
            print(f"[Worker] Puntos de actividad asignados automáticamente al alumno {matricula}.")
            
        else:
            print(f"[Worker] IA no pudo validar automáticamente. Derivando a revisión humana: {observaciones}")
            
            # Guardar en validaciones con el porcentaje obtenido por CLIP
            cursor.execute("""
                INSERT INTO validaciones (registro_tdi_id, ocr_exitoso, texto_extraido, coincidencia, resultado, observaciones)
                VALUES (%s, TRUE, 'Texto procesado por IA', %s, 'REVISION_MANUAL', %s)
            """, (registro_id, coincidencia_score, observaciones))
            
            # Insertar en la tabla de revisiones para asignación manual
            cursor.execute("""
                INSERT INTO revisiones (registro_tdi_id, decision, comentario)
                VALUES (%s, 'PENDIENTE', %s)
            """, (registro_id, observaciones))
            
            db_conn.commit()
            print(f"[Worker] Registro {registro_id} enviado a la tabla de revisiones para los administrativos.")
            
    except Exception as e:
        db_conn.rollback()
        print(f"[Worker] Error procesando la transacción de base de datos: {e}")
    finally:
        cursor.close()

def main():
    print("==================================================")
    print("  TDI VALIDATION WORKER DAEMON STARTING           ")
    print("==================================================")
    
       # Conectando a Redis
    try:
        rdb = redis.from_url(REDIS_URL, socket_timeout=60)
        rdb.ping()
        print("[Worker] Conectado a Redis con éxito.")

    except Exception as e:
        print(f"[Worker] Error crítico al conectar a Redis: {e}")
        return
        
    # Conectando a Postgres
    try:
        db_conn = get_db_connection()
        print("[Worker] Conectado a base de datos Postgres (Neon).")
    except Exception as e:
        print(f"[Worker] Error crítico al conectar a la base de datos: {e}")
        return

    print("[Worker] Escuchando la cola 'validation_queue'...")
    
    while True:
        try:
            # BRPOP bloquea la ejecución hasta que caiga un elemento en la cola
            job = rdb.brpop("validation_queue", timeout=10)
            if job:
                queue_name, payload_bytes = job
                try:
                    job_data = json.loads(payload_bytes.decode("utf-8"))
                    process_validation_job(rdb, db_conn, job_data)
                except Exception as ex:
                    print(f"[Worker] Error al decodificar la tarea: {ex}")
            else:
                db_conn.commit()  # Refrescar conexión a base de datos
        except redis.ConnectionError:
            print("[Worker] Conexión de Redis perdida. Reconectando en 5s...")
            time.sleep(5)
            try:
                rdb = redis.from_url(REDIS_URL, socket_timeout=60)
                rdb.ping()
            except:
                pass

        except psycopg2.InterfaceError:
            print("[Worker] Conexión de base de datos perdida. Reconectando en 5s...")
            time.sleep(5)
            try:
                db_conn = get_db_connection()
            except:
                pass
        except KeyboardInterrupt:
            print("\n[Worker] Deteniendo el demonio de validación.")
            break
        except Exception as e:
            print(f"[Worker] Excepción en el bucle principal: {e}")
            time.sleep(2)

    db_conn.close()

if __name__ == "__main__":
    main()
