# PTI-UTEQ — Servicio User / Auth

Microservicio de usuarios y autenticación de la Plataforma de Trayectoria
Integral UTEQ. Emite los JWT que el Gateway verifica, valida credenciales y
persiste usuarios en Neon (PostgreSQL).

## Responsabilidades

- Registro de usuarios con contraseña hasheada (bcrypt).
- Login: valida credenciales y emite un JWT firmado con HS256.
- Perfil del usuario autenticado (`/me`), confiando en el header `X-User-Id`
  que el Gateway inyecta tras validar el token.

Este servicio **no toca Redis**: la denylist y el logout viven en el Gateway,
que es el dueño de los tokens. Aquí solo se emiten.

## Estructura

```
user-service/
├── main.go                      # Ensambla DB, emisor de JWT y rutas
├── internal/
│   ├── config/config.go         # Variables de entorno
│   ├── models/user.go           # Usuario y roles
│   ├── db/store.go              # Conexión a Neon (pgxpool) + queries
│   ├── auth/auth.go             # bcrypt + emisión de JWT
│   └── handlers/handlers.go     # register, login, me
├── Dockerfile
└── .env.example
```

## Endpoints

Las rutas son "limpias" (sin `/api/users`) porque el Gateway quita ese prefijo
antes de reenviar.

| Método | Ruta | Auth | Descripción |
|---|---|---|---|
| POST | `/register` | No | Crea usuario, devuelve token |
| POST | `/login` | No | Valida credenciales, devuelve token |
| GET | `/me` | Sí (vía Gateway) | Perfil del usuario autenticado |
| GET | `/health` | No | Healthcheck |

### Ejemplo de registro

```
curl -X POST http://localhost:8081/register \
  -H "Content-Type: application/json" \
  -d '{"email":"alumno@uteq.edu.mx","password":"contrasena123","full_name":"Ana López","role":"estudiante"}'
```

## Correr localmente

```
go mod download
go run .
```

(Necesitas un `DATABASE_URL` de Neon válido; la tabla `users` se crea sola al arrancar.)

## Notas de seguridad

- Las contraseñas se guardan con bcrypt (nunca en texto plano).
- El hash nunca se serializa a JSON (campo con tag `-`).
- En login, el mismo mensaje de error para "email no existe" y "contraseña
  incorrecta", para no permitir enumeración de usuarios.
- El secreto JWT debe ser idéntico al del Gateway.

## Importante para el despliegue

- El `JWT_SECRET` **debe ser el mismo** en este servicio y en el Gateway.
- Verifica el formato del `DATABASE_URL` contra el dashboard de Neon (suele
  requerir `?sslmode=require`).
