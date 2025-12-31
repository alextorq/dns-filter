#!/bin/bash
set -e

# Загрузка переменных окружения из .env (если файл существует)
if [ -f .env ]; then
    echo "Загружаем переменные окружения из файла .env..."
    set -o allexport
    source .env
    set +o allexport
fi

# Путь до папки проекта
PROJECT_DIR="/home/balamut/projects/dns-filter"

# Переход в директорию скрипта
cd "$PROJECT_DIR"

echo "Получаем последние изменения из репозитория..."
git pull origin main

# Сборка образа без использования кэша
echo "Собираем Docker-образ (без использования кэша)..."
docker compose build

# Остановка и удаление старых контейнеров
echo "Останавливаем предыдущие контейнеры (если есть)..."
docker compose down

# Запуск контейнеров в фоновом режиме
echo "Запускаем контейнеры в фоне..."
docker compose up -d

echo "Скрипт выполнен успешно."
