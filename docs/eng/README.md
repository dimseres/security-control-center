# Berkut SCC Documentation (EN)



Documentation version baseline: `1.0.8`



## Sections

1. Architecture: `docs/eng/architecture.md`

2. API: `docs/eng/api.md`

3. Security: `docs/eng/security.md`

4. Deploy and CI/CD: `docs/eng/deploy.md`

5. Runbook (start and recovery): `docs/eng/runbook.md`

6. Tabs wiki: `docs/eng/wiki/tabs.md`

7. Features wiki: `docs/eng/wiki/features.md`

8. Current evolution plan: `docs/eng/roadmap.md`

9. Backups (.bscc): `docs/eng/backups.md`

10. HTTPS + OnlyOffice: `docs/eng/https_onlyoffice.md`

11. Reverse-proxy + OnlyOffice compose example: `docs/ru/docker-compose.https.yml`



## Context

Documentation is aligned with current runtime reality:

- PostgreSQL runtime

- goose migrations

- cleanenv configuration

- server-side zero-trust authorization

- `.bscc` backups module (create/import/download/restore/plan/scheduler/retention)

- monitoring SLA module (SLA tab, closed periods, background evaluator, incident policy)



## Included for 1.0.8

- Settings: dedicated Cleanup tab with selective per-module data cleanup.

- Monitoring: server-side flag to auto-close incidents when monitor recovers (`DOWN -> UP`).

- Localization and UX: fixes for logs/monitoring UI alignment and missing i18n labels.

- Backups: improved “New backup options” UX and hardened DB restore pipeline (`pg_restore`).

- Compose/runtime: unified container timezone via `TZ` (recommended `Europe/Moscow`).

- Monitoring notifications: hardened deliveries history handling for legacy/new records to avoid intermittent `500` in `/api/monitoring/notifications/deliveries`.

