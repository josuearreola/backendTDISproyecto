# ---- Etapa de compilación ----
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o user-service .

# ---- Etapa final ----
FROM alpine:latest

# Certificados raíz: necesarios para conectar a Neon por TLS.
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/user-service .

EXPOSE 8081
CMD ["./user-service"]
