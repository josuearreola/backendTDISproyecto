# ---- Etapa de compilación ----
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o tdi-service ./tdi-service-tdis

# ---- Etapa final ----
FROM alpine:latest

# Certificados raíz: necesarios para conectar a Neon y Redis por TLS.
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/tdi-service .

# Carpeta de evidencias subidas por los alumnos (en Render, sin Disk persistente,
# el contenido se pierde en cada redeploy/restart -- suficiente para pruebas).
RUN mkdir -p /app/uploads

EXPOSE 8082
CMD ["./tdi-service"]
