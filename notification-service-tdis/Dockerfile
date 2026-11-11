# ---- Etapa de compilación ----
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o notification-service ./notification-service-tdis

# ---- Etapa final ----
FROM alpine:latest

# Certificados raíz: necesarios para conectar a Neon y Redis por TLS.
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/notification-service .

EXPOSE 8083
CMD ["./notification-service"]
