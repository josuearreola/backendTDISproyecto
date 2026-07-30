# PTI-UTEQ — API Gateway

Punto único de entrada de la Plataforma de Trayectoria Integral UTEQ. Sigue el
patrón definido en el documento de requerimientos: el Gateway es el único que
verifica los JWT, consulta la denylist en Redis, aplica rate limiting y enruta
a los microservicios. Los servicios downstream confían en el Gateway y reciben
la identidad del usuario por los headers `X-User-Id` y `X-User-Role`.

## Estructura

```
gateway/
├── main.go                       # Ensambla router, Redis, proxy y middlewares
├── internal/
│   ├── config/config.go          # Lee y valida variables de entorno
│   ├── token/token.go            # JWT: verificación + denylist en Redis
│   ├── middleware/
│   │   ├── auth.go               # Exige JWT válido, inyecta identidad en headers
│   │   └── cors_ratelimit.go     # CORS y rate limiting con Redis
│   └── proxy/proxy.go            # Reverse proxy hacia cada servicio
├── Dockerfile                    # Imagen para Railway
└── .env.example                  # Variables necesarias
```

## Correr localmente

1. Levanta un Redis local (por ejemplo con Docker):
   ```
   docker run -p 6379:6379 redis
   ```
2. Copia `.env.example` a `.env` y ajusta los valores.
3. Carga las variables y arranca:
   ```
   go mod download
   go run .
   ```
4. Prueba el healthcheck:
   ```
   curl http://localhost:8080/health
   ```

## Variables de entorno

| Variable | Descripción |
|---|---|
| `PORT` | Puerto (Railway lo inyecta solo) |
| `JWT_SECRET` | Secreto HS256 para verificar tokens |
| `REDIS_URL` | URL de Redis (Railway: plugin Redis) |
| `USER_SERVICE_URL` | URL interna del servicio de usuarios |
| `ALLOW_ORIGIN` | Dominio del frontend para CORS |

## Notas de diseño

- **Fail-closed en auth:** si Redis no responde al consultar la denylist, se
  niega el acceso (no se asume que el token es válido).
- **Fail-open en rate limit:** si Redis falla, se deja pasar la petición para
  no tumbar el servicio; es una decisión que prioriza disponibilidad.
- **Anti-suplantación:** el middleware borra cualquier `X-User-Id`/`X-User-Role`
  que venga del cliente antes de inyectar los reales.

## Pendiente para el despliegue

Verificar contra la documentación vigente de Railway el formato exacto de
`REDIS_URL` y las URLs internas entre servicios, ya que pueden cambiar.
