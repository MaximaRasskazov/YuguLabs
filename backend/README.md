# Backend (Go)

Backend-сервис системы учёта академических задолженностей.

Для **быстрого запуска всего стека** используй корневой [Quick Start](../README.md#-quick-start) — `make setup` поднимет всё одной командой.

Этот документ — для тех, кто работает с Go-кодом локально, без docker.

## Стек

- Go 1.26
- chi v5 — HTTP-роутер
- pgx v5 + pgxpool — драйвер Postgres
- sqlc — генерация типобезопасных репозиториев из SQL
- goose — миграции (`backend/sql/migrations`) и dev-сиды (`backend/sql/seeds`)
- golang-jwt/jwt/v5 — JWT access-токены
- bcrypt — хеш паролей
- slog (stdlib) — структурированное логирование

## Структура

```
backend/
├── cmd/server/                # main: config + pool + сборка сервисов + router + graceful shutdown
├── internal/
│   ├── config/                # загрузка .env, валидация JWT_SECRET
│   ├── pgutil/                # interop google/uuid ↔ pgtype.*
│   ├── repo/
│   │   ├── queries/           # sqlc-generated, НЕ редактировать вручную
│   │   ├── store.go           # *Store + RunInTx (UoW)
│   │   └── store_test.go
│   ├── service/
│   │   ├── token/             # JWT-генерация, валидация, refresh-replay-detection
│   │   ├── auth/              # Register / Login / Refresh / Logout / Me
│   │   ├── rbac/              # HasPermission, AssignRole с privilege-escalation guard
│   │   ├── audit/             # AuditService — события безопасности в audit_log
│   │   ├── changelog/         # ChangeLogService — before/after JSONB для истории и undo
│   │   ├── discipline/        # CRUD дисциплин + привязка teacher/student
│   │   ├── debt/              # академические долги (создание, оценка, отмена, выборки по ролям)
│   │   ├── retake/            # пересдачи + участники + переходы статусов + grade c закрытием debt
│   │   ├── report/            # сводные отчёты (долги по дисциплинам, пересдачи за период) + XLSX/CSV экспорт
│   │   ├── scheduler/         # фоновый тикер: scheduled→in_progress→completed по scheduled_at
│   │   └── notify/            # email + WebSocket + история уведомлений
│   └── transport/http/
│       ├── dto/               # API-схемы (auth, discipline, debt, retake, notifications)
│       ├── handler/           # /api/auth/*, /api/disciplines/*, /api/debts/*, /api/retakes/*, ...
│       ├── middleware/        # Auth, RequirePermission, CORS, SecurityHeaders
│       └── router.go          # сборка chi.Router + mount* для каждой доменной группы
├── sql/
│   ├── migrations/            # 00001-00027 — auth, RBAC, audit, changelog, аватары + доменные таблицы
│   └── seeds/                 # 00001_seed_dev_accounts — для SEED_DEV_ACCOUNTS=true
├── Dockerfile                 # multi-stage + goose CLI + entrypoint.sh
├── entrypoint.sh              # wait-for-postgres → goose up → seeds → exec server
├── Makefile                   # читает ../.env (единый источник конфига)
├── sqlc.yaml
└── go.mod
```

## API endpoints

Все защищённые endpoint'ы требуют `Authorization: Bearer <jwt>` и проходят RBAC-проверки на нужный permission. Полный список permission'ов — в миграции `00010_seed_rbac.sql`.

### Authentication
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| POST | `/api/auth/register` | — | Регистрация (выдаётся роль `student`) |
| POST | `/api/auth/login` | — | Логин по email + password |
| POST | `/api/auth/refresh` | — | Обмен refresh-cookie на новый access |
| POST | `/api/auth/logout` | Auth | Закрытие сессии |
| GET | `/api/auth/me` | Auth | Профиль + roles + permissions |
| PATCH | `/api/me` | Auth | Обновить профиль (PATCH-семантика: first_name/last_name/middle_name/group_name/birthday). Email и пароль НЕ через этот endpoint. |
| POST | `/api/me/password` | Auth | Сменить пароль: body `{current_password, new_password}`. Атомарно ревокует все access-токены пользователя — нужно перелогиниться. 401 при неверном current, 400 при new<8 или совпадающем с current. |
| POST | `/api/me/avatar` | Auth | Загрузить аватар (multipart, поле `avatar`). JPEG/PNG/WebP до 2 МБ; тип проверяется по сигнатуре файла. Ответ `{avatar_url}`. 413 если больше 2 МБ, 415 при неверном типе. |
| DELETE | `/api/me/avatar` | Auth | Удалить свой аватар |
| GET | `/api/users/:id/avatar` | — | Отдать картинку аватара (публично — для тега `<img>`). 404 если аватара нет. |

### Users (список — admin / dean)
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| GET | `/api/users?role=&search=&group_name=&limit=&offset=` | `users.view` | Список пользователей с фильтрами. role: student/teacher/dean/admin. search — ILIKE по email/first_name/last_name. group_name — точный матч. Возвращает `{items, total, limit, offset}` с прикреплёнными ролями. |

### Disciplines
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| GET | `/api/disciplines` | Auth | Список справочника (доступен всем авторизованным) |
| GET | `/api/disciplines/:id` | Auth | Одна дисциплина |
| GET | `/api/disciplines/:id/teachers` | Auth | Преподаватели дисциплины |
| GET | `/api/disciplines/:id/students` | Auth | Студенты дисциплины |
| POST | `/api/disciplines` | `disciplines.create` | Создать (admin/dean) |
| PATCH | `/api/disciplines/:id` | `disciplines.update` | Обновить (admin/dean) |
| DELETE | `/api/disciplines/:id` | `disciplines.delete` | Soft-delete (admin) |
| POST | `/api/disciplines/:id/restore` | `disciplines.delete` | Восстановить (admin) |
| POST | `/api/disciplines/:id/teachers` | `disciplines.update` | Привязать преподавателя |
| DELETE | `/api/disciplines/:id/teachers/:user_id` | `disciplines.update` | Снять |
| POST | `/api/disciplines/:id/students` | `disciplines.update` | Привязать студента (с academic_year+semester) |
| DELETE | `/api/disciplines/:id/students/:user_id` | `disciplines.update` | Снять |

### Дисциплины пользователя
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| GET | `/api/me/disciplines/student` | Auth | На каких дисциплинах я учусь |
| GET | `/api/me/disciplines/teacher` | Auth | Какие веду |
| GET | `/api/users/:id/disciplines/student` | `users.view` | Просмотр чужих (admin/dean) |
| GET | `/api/users/:id/disciplines/teacher` | `users.view` | Просмотр чужих (admin/dean) |

### Debts (академические долги)
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| GET | `/api/debts/my` | `debts.view.own` | Свои долги (студент) |
| GET | `/api/debts/by-discipline` | `debts.view.by_discipline` | Долги по моим дисциплинам (преподаватель) |
| GET | `/api/debts` | `debts.view.all` | Все долги (dean/admin) |
| GET | `/api/debts/summary` | `debts.view.all` | Сводка по дисциплинам (open/graded count) |
| GET | `/api/debts/:id` | `debts.view.all` | Один долг (dean/admin) |
| POST | `/api/debts` | `debts.create` | Создать долг (преподаватель, проверка teacher_disciplines + student_disciplines) |
| PATCH | `/api/debts/:id/grade` | `debts.update` | Выставить оценку 2..5 (open → graded) |
| PATCH | `/api/debts/:id/cancel` | `debts.delete` | Отменить долг (open → cancelled) |

### Retakes (пересдачи)
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| GET | `/api/retakes/my` | `retakes.view.own` | Свои пересдачи (студент/преподаватель — участники) |
| GET | `/api/retakes` | `retakes.view.all` | Все пересдачи (dean/admin), фильтр `?status=` опциональный |
| GET | `/api/retakes/:id` | `retakes.view.all` | Одна пересдача |
| GET | `/api/retakes/:id/participants` | `retakes.view.all` | Участники (студенты + преподаватели/комиссия) |
| POST | `/api/retakes` | `retakes.create` | Создать пересдачу (dean). kind = regular/commission |
| PATCH | `/api/retakes/:id` | `retakes.update` | Обновить расписание/место/notes (PATCH-семантика) |
| POST | `/api/retakes/:id/start` | `retakes.update` | scheduled → in_progress (commission проверяет ≥3 преподавателей) |
| POST | `/api/retakes/:id/complete` | `retakes.update` | scheduled/in_progress → completed |
| POST | `/api/retakes/:id/cancel` | `retakes.update` | scheduled/in_progress → cancelled |
| POST | `/api/retakes/:id/students` | `retakes.update` | Добавить студента с debt_id (с проверкой владельца долга) |
| DELETE | `/api/retakes/:id/students/:user_id` | `retakes.update` | Снять студента (только до выставления оценки) |
| POST | `/api/retakes/:id/teachers` | `retakes.update` | Добавить преподавателя (regular) / члена комиссии (commission) |
| DELETE | `/api/retakes/:id/teachers/:user_id` | `retakes.update` | Снять |
| PATCH | `/api/retakes/:id/students/:user_id/grade` | `retakes.assign_grade` | Выставить оценку 2..5 — атомарно закрывает связанный debt |

### Reports (сводные отчёты для деканата)
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| GET | `/api/reports/debts-summary` | `reports.export` | Сводка долгов по дисциплинам (open/graded count + название/код) |
| GET | `/api/reports/retakes?from=&to=&format=` | `reports.export` | Список проведённых пересдач за период. Без `format` — JSON. С `format=xlsx` или `format=csv` — бинарник с правильным Content-Type и Content-Disposition (для скачивания). Период в формате `YYYY-MM-DD` (или RFC3339), по умолчанию последний месяц. |

### Notifications (real-time + история)
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| GET | `/api/notifications` | Auth | История уведомлений пользователя |
| GET | `/api/notifications/unread-count` | Auth | Badge — количество непрочитанных |
| POST | `/api/notifications/:id/read` | Auth | Отметить прочитанным |
| GET | `/ws/notifications` | Auth (через query `token`) | WebSocket для push-уведомлений |

### История изменений (audit / undo)
Полные срезы before/after мутаций пишутся в `change_logs` транзакционно вместе с операцией. История наружу отдаётся диффом: `changed_fields` = `{ поле: {old, new} }` (только изменившиеся). См. [docs/lr-logging.md](../docs/lr-logging.md).
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| GET | `/api/changelog?limit=&offset=` | `changelog.view` | Общий журнал последних изменений по всем сущностям (новые сверху) |
| GET | `/api/users/:id/story` | `changelog.view` | История изменений пользователя (создание/правка профиля, выдача/снятие ролей) |
| GET | `/api/roles/:id/story` | `changelog.view` | История изменений роли (кому выдавали/снимали) |
| GET | `/api/permissions/:id/story` | `changelog.view` | История изменений права (обычно пуста — CRUD прав в системе нет) |
| POST | `/api/changelog/:id/restore` | `changelog.restore` | Откат (undo) к состоянию `before` из записи лога. Поддержаны профиль пользователя, выдача/снятие роли и атомарная смена роли (`role_changed`); сам откат тоже логируется (`restored_from_log`). 422 если откат не поддержан, 404 если записи нет, **409 `restore_noop`** если откат уже выполнен (состояние уже целевое). |
| POST | `/api/users/:id/roles/change` | `roles.assign` | Атомарная смена роли: `{from_slug, to_slug}` — снять старую и выдать новую в одной транзакции (одно событие `role_changed`). Идемпотентна, под privilege-escalation guard. |

### Деплой (git-webhook)
Авто-деплой по webhook'у: проверка секретного ключа → `git checkout/reset/pull` под блокировкой, с журналом и обработкой ошибок. Маршрут открыт (без auth), защищён только `GIT_WEBHOOK_SECRET`. См. [docs/git-webhook.md](../docs/git-webhook.md).
| Метод | Путь | Permission | Назначение |
|---|---|---|---|
| POST | `/api/hooks/git` | — (secret_key) | Запустить деплой. `secret_key` в JSON или form-data. 200 (JSON со списком шагов), 403 неверный ключ, 409 деплой уже идёт, 500 ошибка git, 503 если `GIT_WEBHOOK_SECRET` не задан. |

### Системные
| Метод | Путь | Назначение |
|---|---|---|
| GET | `/health` | Liveness/readiness check (проверяет pool.Ping) |

## Локальная разработка (без docker)

```bash
# Из корня репо: создай .env (это нужно сделать один раз)
make setup     # либо вручную: cp .env.example .env && подставить JWT_SECRET

# Подними только Postgres из docker, всё остальное локально:
docker compose up -d postgres mailpit

# Накати миграции с хоста (goose читает DB_HOST/DB_PORT из ../.env)
cd backend
make migrate-up

# Запусти сервер локально
make run
```

## Команды Makefile

```
make help                     # Список таргетов
make run                      # go run ./cmd/server
make build                    # сборка бинарника в bin/server
make test                     # unit-тесты (интеграционные скипаются)
make test-integration         # все тесты против поднятого postgres
make lint                     # gofmt + go vet
make tidy                     # go mod tidy
make migrate-up               # goose up на хостовой БД (DB_HOST из ../.env)
make migrate-down             # откатить последнюю миграцию
make migrate-status           # статус миграций
make migrate-create name=foo  # создать новую миграцию
make sqlc                     # перегенерировать internal/repo/queries из sql/queries
```

## Конфиг

См. корневой `.env.example`. Backend читает env-переменные через `internal/config`.

| Группа | Переменные |
|---|---|
| Application | `APP_ENV`, `APP_PORT`, `LOG_LEVEL` |
| Postgres | `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD` |
| JWT | `JWT_SECRET` (обязателен, ≥32 байт), `ACCESS_TOKEN_TTL`, `REFRESH_TOKEN_TTL` |
| Cookie | `COOKIE_SECURE`, `COOKIE_DOMAIN`, `COOKIE_PATH`, `COOKIE_SAMESITE` |
| CORS | `ALLOWED_ORIGINS` (через запятую) |
| Dev | `SEED_DEV_ACCOUNTS` — катить ли сиды демо-аккаунтов |
| Git-webhook | `GIT_WEBHOOK_SECRET` (пусто = выкл/503), `GIT_DEFAULT_BRANCH`, `GIT_REPO_PATH`, `GIT_DEPLOY_LOG`, `GIT_DEPLOY_TIMEOUT` |

## Демо-аккаунты для разработки

При `SEED_DEV_ACCOUNTS=true` (по умолчанию в `.env.example`) после `make setup` доступны 7 аккаунтов с паролем `password`:

| Email | Роль |
|---|---|
| `admin@academic.local` | admin |
| `dean@academic.local` | dean |
| `teacher1@academic.local` | teacher |
| `teacher2@academic.local` | teacher |
| `student1@academic.local` | student (группа БСБО-01-22) |
| `student2@academic.local` | student (БСБО-01-22) |
| `student3@academic.local` | student (БСБО-02-22) |

## Архитектурные инварианты

Несколько свойств, которые гарантируются на уровне БД и сервисов — стоит знать при работе с системой:

- **UoW во всех мутациях.** Любая мутация (создание/обновление/удаление) идёт через `repo.Store.RunInTx` с записью в `audit_log` и `change_logs` в той же транзакции. Лог не может оторваться от данных.
- **One-use refresh с replay-detection.** При попытке использовать refresh-токен второй раз `TokenService.Rotate` ревокует ВСЕ access-токены пользователя — разрывает сессию атакующего.
- **Privilege escalation guard.** `RBACService.AssignRole` не позволяет actor'у выдать роль с level больше своего. Декан (level 700) не может назначить admin (1000) даже с `roles.assign`.
- **UNIQUE-индексы как бизнес-правила.** `debts (student_id, discipline_id) WHERE status='open'` — нельзя 2 открытых долга на одну дисциплину. `teacher_role_requests (requested_by) WHERE status='pending'` — спам заявок запрещён.
- **Soft-delete везде где есть зависимые сущности.** `disciplines`, `roles`, `permissions` — soft-delete, потому что на них ссылаются `debts`/`role_user`/`permission_role`. Hard-delete сломал бы FK.
- **CHECK-constraints на статусах.** `debts.status='graded'` ⇒ `final_grade + graded_at + graded_by` обязательны. `retakes.kind='commission'` ⇒ `min_teachers ≥ 3`. БД гарантирует согласованность даже если сервис ошибётся.

## Текущий статус

### Готово (в `develop`)

- [x] **Auth-foundation** (PR #10): users, sessions, refresh, RBAC, audit, change_logs, миграции 00001-00010
- [x] **DX one-click setup** (PR #11): `make setup`, автомиграции, дев-сиды
- [x] **Доменная схема** (PR #12): миграции 00011-00019 + sqlc-репозитории для всех доменных таблиц + AuditService + ChangeLogService
- [x] **Disciplines** (PR #13): DisciplineService + handler'ы `/api/disciplines/*`
- [x] **Docker / Frontend infra** (PR #14): production compose, frontend Dockerfile + nginx
- [x] **BACK-10 CI + линтеры** (PR #15): GitHub Actions backend-ci, golangci-lint в Makefile
- [x] **BACK-06 Notify** (PR #16): email через SMTP + WebSocket-хаб + история уведомлений
- [x] **BACK-02 Debts** (PR #17): DebtService + handler'ы `/api/debts/*` + `/api/me/disciplines/*`
- [x] **BACK-03 Retakes** (PR #18): RetakeService + участники + grade с атомарным закрытием debt + handler'ы `/api/retakes/*`
- [x] **BACK-08 Reports** (PR #20): ReportService (DebtsSummary, RetakesForPeriod) + XLSX/CSV экспорт через excelize + handler'ы `/api/reports/*`
- [x] **BACK-09 Swagger** (PR #23): аннотации для всех эндпоинтов + `/swagger/*`
- [x] **BACK-04 RetakeChangeRequest** (PR #24): заявки на изменение пересдачи + handler'ы `/api/retake-change-requests/*`
- [x] **BACK-07 Scheduler** (PR #25): фоновый `time.Ticker` для автоперехода retake-статусов
- [x] **Discipline QA + Debt notify** (PR #27, Андрей): 409 на дубли, 400 на длинные имена, нотификации в debt.Service, helpers IsUniqueViolation/IsForeignKeyViolation

### Готово (в PR, ждёт мерджа)

- [ ] **BACK-06 Retake notifications** (`feat/backend-notify-retake-events`): подключение notify.Service к retake.Service — уведомления студентам о retake_scheduled / retake_updated / retake_cancelled / retake_grade_received. Шаблон email для оценки. Закрывает требование ТЗ "студент должен получать уведомления о пересдачах и оценках".

### В работе / следующие PR

| Задача | Содержание |
|---|---|
| BACK-05 | TeacherRoleRequest + RBAC HTTP API (у товарища, ветка `feat/back-05-teacher-requests`) |
| Скрыть регистрацию | Убрана ссылка на регистрацию со страницы входа (ветка `chore/hide-register-link`) |
| Восстановление пароля | Рабочий сброс по коду на email: notify + ручки `/api/auth/recover/*` (ветка `feat/password-reset`, миграция 00025) |
| Аватар профиля | Загрузка/удаление аватара (bytea), `/api/me/avatar` + публичная отдача (рабочее дерево, миграция 00026) |
| Логирование мутаций | История изменений users/roles + дифф `changed_fields` + undo: `/api/.../story`, `/api/changelog/:id/restore` (рабочее дерево, миграция 00027). См. [docs/lr-logging.md](../docs/lr-logging.md) |
| Git-webhook (лаба №6) | Авто-деплой `POST /api/hooks/git`: секретный ключ + `git checkout/reset/pull` под блокировкой + журнал + тесты (пакет `service/deploy`, рабочее дерево). См. [docs/git-webhook.md](../docs/git-webhook.md) |

### Покрытие тестами

Интеграционные тесты против реального Postgres (запускаются через `make test-integration`). Текущее покрытие по сервисам:

| Пакет | Coverage | Тестов |
|---|---|---|
| audit | 90.5% | 4 |
| repo | 84.6% | 4 |
| rbac | 83.9% | 6 |
| auth | 76.7% | 7 |
| token | 77.8% | 7 |
| changelog | 77.8% | 5 |
| debt | ~80% | 10 |
| discipline | 58.0% | 8 |
| notify | ~80% | 4 |
| retake | ~80% | 17 |
| report | ~75% | 7 |
| scheduler | ~80% | 6 |
| **Всего** | | **85 тестов** |
