# Captcha Service

Этот проект представляет собой сервис с CAPTCHA и drag-and-drop интерфейсом, реализованный с использованием Go, gRPC и простого фронтенда на HTML + Python HTTP-сервере.

## Быстрый старт

## Установка зависимостей

```bash
go mod tidy
```

## Локальный запуск


### Запуск balancer-mock

```bash
make balancer-run
```

### Запуск backend

```bash
make back-run
```

### Запуск frontend

```bash
make front-run
```

Откройте в браузере: [http://localhost:8000](http://localhost:8000)

## Запуск в Docker(если есть готовый балансер)

### Сборка Docker-образа

```bash
make docker-build
```

### Запуск с Docker Compose

```bash
make docker-run
```

Откройте в браузере: [http://localhost:8000](http://localhost:8000)

### Остановка контейнеров

```bash
make docker-down
```

## Конфигурация

Файл конфигурации backend:

```
backend/config/app.yaml
```

Можно использовать пример для локального запуска, но убрать .example:

```
backend/config/app.example.yaml
```
