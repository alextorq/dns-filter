# Этап сборки
FROM golang:1.21-alpine AS builder

# Установка git и зависимостей
RUN apk add --no-cache git

WORKDIR /app

# Копируем модули
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники
COPY . .

# Собираем бинарник
RUN go build -o dns-filter ./cmd/dns-filter

# Минимальный образ для запуска
FROM alpine:latest
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Копируем бинарник из builder
COPY --from=builder /app/dns-filter .

# Порт, который слушает приложение
EXPOSE 53/udp
EXPOSE 53/tcp
EXPOSE 8080/tcp

# Команда запуска
CMD ["./dns-filter"]
