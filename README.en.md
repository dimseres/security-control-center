# Berkut Solutions - Security Control Center



<p align="center">

  <img src="gui/static/logo.png" alt="Berkut SCC logo" width="220">

</p>



[Русская версия](README.md)



Berkut Solutions - Security Control Center (SCC) is a self-hosted platform for security operations and compliance management.



Current version: `1.0.9`



## Product Overview

Berkut SCC is a unified workspace for security teams where operational workflows, compliance artifacts, and control activities are managed in one system.



It replaces fragmented spreadsheets, chat approvals, and disconnected trackers with a consistent process model: documents, incidents, tasks, and monitoring signals are linked and governed by the same access model.



## Who It Is For

- Organizations that require a self-hosted environment with strict data ownership.

- Security teams combining compliance, incident response, and day-to-day operations.

- Companies that need auditable, repeatable workflows for internal and external assessments.

- Teams looking for practical deployment without SaaS lock-in.



## Business Value

- Improves security process visibility and ownership across teams.

- Reduces operational gaps: fewer lost tasks, missing document versions, or unmanaged changes.

- Speeds up audit and reporting preparation with structured data and event traceability.

- Keeps critical security workflows inside your infrastructure perimeter.



## Functional Capabilities

- Documents and approvals:

  versioning, collaboration, and integrated editing via OnlyOffice.

- Incidents:

  tracking, lifecycle management, and linkage to related artifacts.

- Monitoring and SLA:

  operational events, availability metrics, SLA policy, and maintenance windows.

- Monitoring automation:

  per-monitor rules for creating incidents/tasks on downtime and TLS risk conditions.

- Monitoring notifications center:

  quiet hours, message templates, and delivery history with acknowledgment.

- Settings cleanup:

  selective per-module cleanup with in-app confirmation and deleted-items feedback.

- Task management:

  spaces, boards, tasks, comments, links, and scheduling.

- Access control:

  users, roles, groups, and server-side authorization enforcement.

- Backups:

  encrypted `.bscc` backups, import/restore, scheduling, and retention policies.

- Deep-link UX:

  direct links for documents, incidents, and tasks are restored after browser refresh (F5).

- Incident workflow UX:

  stage status updates are reflected in the modal immediately after save.



## Security by Design

- Server-side zero-trust authorization checks on every endpoint.

- Audit trail for critical platform actions.

- Self-hosted data boundary for sensitive environments.

- Embedded UI assets with no external CDN dependency.

- Production-ready reverse-proxy/TLS integration.



## Technical Profile

- Backend: Go (`chi`, `cleanenv`, `goose`).

- Database: PostgreSQL.

- RBAC: Casbin.

- Deployment: Docker / Docker Compose.



## Documentation

- Docs index: [`docs/README.md`](docs/README.md)

- Russian docs: [`docs/ru/README.md`](docs/ru/README.md)

- English docs: [`docs/eng/README.md`](docs/eng/README.md)
- Monitoring notification message template: [`docs/eng/monitoring_notifications_message_template.md`](docs/eng/monitoring_notifications_message_template.md)



## Run and Deploy

- Quick start: [`QUICKSTART.en.md`](QUICKSTART.en.md)

- Deploy and CI/CD: [`docs/eng/deploy.md`](docs/eng/deploy.md)

- Runbook (start/recovery): [`docs/eng/runbook.md`](docs/eng/runbook.md)

- HTTPS + OnlyOffice: [`docs/eng/https_onlyoffice.md`](docs/eng/https_onlyoffice.md)

- Reverse-proxy compose example: [`docs/ru/docker-compose.https.yml`](docs/ru/docker-compose.https.yml)



## Security Notes

- Do not use default secrets outside development.

- Do not store secrets in git (`BACKUP_ENCRYPTION_KEY`, `DOCS_ENCRYPTION_KEY`, `PEPPER`, `CSRF_KEY`).

- Restrict `BERKUT_SECURITY_TRUSTED_PROXIES` to trusted addresses only.

- Set `TZ=Europe/Moscow` (or your operational timezone) in compose/.env to keep container and UI time aligned.



## Screenshots

![Screenshot 1](gui/static/screen1.png)

![Screenshot 2](gui/static/screen2.png)

![Screenshot 3](gui/static/screen3.png)

