# ---- Etapa de compilación ----
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copiamos primero go.mod/go.sum para aprovechar la caché de capas:
# si no cambian las dependencias, Docker no vuelve a descargarlas.
COPY go.mod go.sum* ./
RUN go mod download

# Copiamos el resto del código y compilamos un binario estático.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway .

# ---- Etapa final (imagen mínima) ----
FROM alpine:latest

# Certificados raíz, necesarios para conexiones TLS salientes (Neon, Redis rediss://).
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/gateway .

# Railway inyecta la variable PORT; el código la lee.
EXPOSE 8080
CMD ["./gateway"]
