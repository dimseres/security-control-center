# Berkut SCC - Документация (RU)

Актуальная версия документации: `1.0.7`

## Разделы
1. Архитектура: `docs/ru/architecture.md`
2. API: `docs/ru/api.md`
3. Безопасность: `docs/ru/security.md`
4. Деплой и CI/CD: `docs/ru/deploy.md`
5. Runbook (запуск и восстановление): `docs/ru/runbook.md`
6. Wiki по вкладкам: `docs/ru/wiki/tabs.md`
7. Wiki по функционалу: `docs/ru/wiki/features.md`
8. Актуальный план развития: `docs/ru/roadmap.md`
9. Бэкапы (.bscc): `docs/ru/backups.md`
10. HTTPS + OnlyOffice: `docs/ru/https_onlyoffice.md`
11. Пример compose для reverse-proxy + OnlyOffice: `docs/ru/docker-compose.https.yml`

## Контекст
Документация синхронизирована с текущей моделью:
- PostgreSQL runtime
- goose миграции
- cleanenv-конфигурация
- zero-trust проверки доступа на сервере
- модуль бэкапов `.bscc` (create/import/download/restore/plan/scheduler/retention)
- SLA-модуль мониторинга (вкладка SLA, закрытые периоды, background evaluator, policy инцидентов)

## Что учтено для 1.0.7
- Settings: выделена отдельная вкладка «Очистка» с выборочной очисткой данных по модулям.
- Monitoring: добавлен флаг авто-закрытия инцидента при восстановлении монитора (`DOWN -> UP`).
- Локализация и UX: выровнены проблемные элементы интерфейса логов/мониторинга и закрыты пропуски i18n.
- Backups: обновлён UX блока «Параметры нового бэкапа» и стабилизирован pipeline восстановления БД (`pg_restore`).
- Compose/runtime: добавлена единая таймзона контейнеров через `TZ` (рекомендуемое значение `Europe/Moscow`).
