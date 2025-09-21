# =========================
# Stage 1: Сборка фронтенда
# =========================
FROM node:20-alpine AS frontend-builder

WORKDIR /frontend

# Скопируем package.json для установки зависимостей
COPY web/front/package*.json ./
RUN npm install --frozen-lockfile

# Скопируем исходники и соберем проект
COPY web/front/ .
RUN npm run build


# =========================
# Stage 2: Сборка Go backend
# =========================
FROM golang:1.24-alpine AS backend-builder

WORKDIR /app

# Скопируем go.mod и go.sum для кеширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Скопируем исходники
COPY . .

# Соберем бинарник
RUN go build -o dns-filter .


# =========================
# Stage 3: Финальный образ
# =========================
FROM alpine:latest

WORKDIR /app

# Установим сертификаты (если нужно делать запросы из Go)
RUN apk add  ca-certificates

# Копируем бинарник
COPY --from=backend-builder /app/dns-filter /app/

# Копируем собранный фронтенд
COPY --from=frontend-builder /frontend/dist /app/frontend

# Открываем порты
EXPOSE 53/udp
EXPOSE 53/tcp
EXPOSE 8080/tcp

# Запуск
CMD ["/app/dns-filter"]
