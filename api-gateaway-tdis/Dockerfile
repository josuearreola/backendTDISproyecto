# ---- Etapa de compilación ----
FROM golang:1.25-alpine AS builder

WORKDIR /app

# go.mod/go.sum viven en la raíz del monorepo (un solo módulo Go para los 3 servicios).
# El "Docker Build Context" en Render debe ser la raíz del repo para que esto funcione.
COPY go.mod go.sum* ./
RUN go mod download

# Copiamos el resto del monorepo y compilamos solo el paquete de este servicio.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway ./api-gateaway-tdis

# ---- Etapa final (imagen mínima) ----
FROM alpine:latest

# Certificados raíz, necesarios para conexiones TLS salientes (Neon, Redis rediss://).
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/gateway .

# Render inyecta la variable PORT; el código la lee (ver internal/config).
EXPOSE 8080
CMD ["./gateway"]
