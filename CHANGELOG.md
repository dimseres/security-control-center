# Журнал изменений

## 1.0.9-1 — 19.02.2026

### Изменено
- `docker-compose.yaml`: nginx и Dockerfile для сборки перенесены в `deployments/*`.
- `docker-compose.dev.yaml`: убран устаревший ключ `version` (Compose v2 игнорирует его и выводит warning).
