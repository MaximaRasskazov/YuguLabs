# Лабораторная: логирование мутаций (история изменений + откат)

Полный отчёт по реализации логирования изменений сущностей **users / roles /
permissions** (аудит): что сделано, где, как устроено, чем отличается от
методички (Laravel → Go), как покрыто тестами и как соотносится с критериями
оценки и тест-кейсами из ТЗ.

> Аватарка — отдельная фича и описана в отдельном документе:
> [docs/avatar-feature.md](avatar-feature.md). Здесь — только логирование.

---

## 0. О лабораторной в двух словах

- **Что за лаба (ТЗ).** Добавить в систему **журнал изменений** сущностей
  users / roles / permissions с просмотром истории и **откатом** (undo). В
  методичке — на Laravel (Eloquent observers, таблица `change_logs`,
  story-роуты, restore). Смысл: любое изменение пользователя/ролей должно быть
  видно и обратимо, причём понятно неайтишнику.
- **Что надо было сделать.** Логировать создание/правку/смену ролей **в одной
  транзакции** с самой операцией (и не светить пароль), отдавать историю с
  диффом `old → new`, давать откат к состоянию из конкретной записи, закрыть
  всё правами, покрыть тестами.
- **Что сделали.** Транзакционное логирование прямо в сервисах (Go-аналог
  observers), API `…/story` + общий `/api/changelog`, `POST
  /api/changelog/{id}/restore`, права `changelog.view` / `changelog.restore`,
  админ-UI («История» у пользователя + «Журнал изменений»), тесты.
- **Что поменялось в последней итерации.** Смена роли теперь **одно атомарное
  событие** `role_changed` («Роль изменена: X → Y») с цельным откатом — вместо
  пары `assign`+`revoke`, из-за которой история была шумной, а откат половины
  свопа ломал логику. Выдача роли стала **идемпотентной** (повтор уже активной
  роли — не ошибка). Повторный откат той же записи — не 500, а **409
  `restore_noop`** + серый бейдж «Откат выполнен» в UI.

---

## 1. Краткое резюме

Реализована система автоматического логирования изменений: при создании,
правке и смене ролей пользователя в таблицу `change_logs` транзакционно
пишется полный срез «до/после». Историю можно посмотреть через API и в
админ-панели, а любую поддерживаемую запись — **откатить** (undo). Пароли в
историю не попадают; доступ закрыт правами `changelog.view` / `changelog.restore`.

Значимая часть уже существовала в проекте (таблица + сервис записи) — мы
достроили логирование для users/roles, HTTP-историю с диффом, откат, права,
тесты, админ-UI и эту документацию.

---

## 2. Архитектура и перевод методички (Laravel → Go)

| Требование ТЗ (Laravel) | Реализация у нас (Go) |
|---|---|
| Автологирование через **Eloquent observers** (`created/updated/deleted/restored`) | Явный вызов `changelog.*Tx` **внутри той же транзакции**, что и сама мутация. Это строго гарантирует требование транзакционности (observers её из коробки не дают). |
| Таблица `change_logs` (id, entity_type, entity_id, before/after JSON, created_at, created_by) | Уже есть — миграция [00009](../backend/sql/migrations/00009_create_change_logs.sql). JSONB, индексы, FK `created_by → users(id)`, поле `action`. |
| Модель `ChangeLog` + `$casts` для before/after | sqlc-структура `queries.ChangeLog` (before/after — JSONB → `[]byte`, разбираются в `map`). |
| `ChangeLogDTO` с `changed_fields` | `dto.ChangeLogEntry` + `changelog.Diff` — наружу отдаём только изменившиеся поля, в БД храним полный срез. |
| Роуты `/api/ref/.../story` | `GET /api/{users,roles,permissions}/{id}/story`. |
| Права `get-story-user/role/permission` | Один идиоматичный `changelog.view` (уже в сидах) + новый `changelog.restore` для undo. |
| Undo (`POST .../restore`) | `POST /api/changelog/{id}/restore` + сервис `RestoreFromLog`. |
| Транзакционность; не светить пароли | Лог пишется в `RunInTx` вместе с операцией; срез пользователя (`UserSnapshot`) **без** `password_hash`. |

**Почему так, а не observers:** в Go нет ORM-хуков уровня Eloquent. Логирование
прямо в транзакции сервиса честнее закрывает п. «лог пишется только при успешной
операции» — если запись лога упала, откатывается вся транзакция (см. тест-кейс TC-27).

---

## 3. Что и где добавлено

### Backend
| Файл | Что |
|---|---|
| `sql/migrations/00027_seed_changelog_restore_permission.sql` | Право `changelog.restore` + выдача администратору (`changelog.view` уже был в 00010) |
| `internal/service/changelog/snapshot.go` | `Diff` (changed_fields), `UserSnapshot` (без секретов), `ChangedFields`, константы сущностей |
| `internal/service/changelog/restore.go` | `Get`, `LogRoleChangeTx`, `LogRoleChangedTx`, `RestoreFromLog` (undo профиля и ролей в транзакции, в т.ч. `role_changed`); ошибка `ErrRestoreNoop` |
| `internal/service/changelog/service.go` | Константа действия `password_changed` |
| `internal/service/auth/service.go`, `profile.go` | Логирование `user`: `created` (регистрация), `updated` (профиль), `password_changed` (без пароля) |
| `internal/service/rbac/service.go` | Логирование выдачи/снятия роли (`user` + `role`); **атомарный `ChangeRole`** → одно событие `role_changed`; идемпотентная выдача (`HasRole`-гард) |
| `internal/transport/http/handler/rbac.go`, `router_rbac.go`, `dto/teacher_request.go` | Ручка `POST /api/users/{id}/roles/change` (`ChangeRoleRequest`) — атомарная смена роли |
| `internal/service/user/service.go`, `handler/users.go`, `router_rbac.go` | Ручка `PATCH /api/users/{id}` (право `users.update`) — админ правит ФИО/группу любого пользователя; пишет `updated` с `created_by` = администратор (в транзакции) |
| `internal/transport/http/handler/changelog.go` | Хендлеры story (user/role/permission) и restore |
| `internal/transport/http/dto/changelog.go` | DTO истории с `changed_fields` |
| `internal/transport/http/router_changelog.go`, `router.go`, `cmd/server/main.go` | Роуты, защита правами, проводка зависимостей |

### Frontend
| Файл | Что |
|---|---|
| `frontend/src/views/AdminPage.vue` | Кнопка «История» (модалка с диффом old→new и «Откатить») + «Журнал изменений» в шапке. Смена роли — один запрос `…/roles/change`. Запись `role_changed` показывается как «Роль изменена: X → Y». После отката кнопка → серый бейдж «Откат выполнен»; `restore_noop` → тост «Откат уже выполнен». Кнопка «Изменить» — модалка правки ФИО/группы (`PATCH /api/users/{id}`), изменение тут же видно в «Истории» |
| `frontend/src/views/ProfilePage.vue` (маршрут `/profile`) | Самостоятельная правка ФИО/группы пользователем (`PATCH /api/me`) — ещё один источник `updated`-логов для демо |

### Тесты
| Файл | Что |
|---|---|
| `internal/service/changelog/snapshot_test.go` | `Diff` (только изменившиеся поля), `UserSnapshot` (без пароля) — юнит, без БД |
| `internal/service/changelog/restore_test.go` | Undo профиля, undo выдачи роли, **undo `role_changed`** (teacher→dean → откат → снова teacher), повторный откат → `ErrRestoreNoop`, «откат не поддержан», «запись не найдена» — интеграционные |
| `internal/service/rbac/service_test.go` | **`ChangeRole`**: атомарный своп роли; при отклонении (privilege escalation) старая роль остаётся — интеграционные |
| `internal/service/user/service_test.go` | **`UpdateUser`**: админ правит чужой профиль → применяется + лог `updated` с `created_by` = админ; пустое ФИО → ошибка; несуществующий → not found — интеграционный |
| `internal/service/changelog/service_test.go` | Запись created/updated/soft_deleted, порядок, валидация — интеграционные (были) |
| `internal/service/auth/service_test.go` | `Register` пишет `created` в change_logs — интеграционный |
| `internal/transport/http/router_smoke_test.go` | Нет конфликтов chi-роутов |

---

## 4. API

| Метод | Путь | Право | Назначение |
|---|---|---|---|
| GET | `/api/changelog` | `changelog.view` | Общий журнал последних изменений по всем сущностям |
| GET | `/api/users/{id}/story` | `changelog.view` | История пользователя |
| GET | `/api/roles/{id}/story` | `changelog.view` | История роли |
| GET | `/api/permissions/{id}/story` | `changelog.view` | История права (обычно пуста) |
| POST | `/api/changelog/{id}/restore` | `changelog.restore` | Откат к `before` из записи лога |

Ответ story — массив записей; в каждой `changed_fields` = `{ поле: {old, new} }`
(только изменившиеся). Нет права → **403** (в теле — имя требуемого права:
`message` + поле `required_permission`), нет записи для restore → **404**,
откат не поддержан (`created`/`password_changed`) → **422**, повторный откат
(состояние уже целевое, менять нечего) → **409 `restore_noop`**. Все сообщения
об ошибках — человекочитаемые, объясняют причину (например, для `422` — что
именно у создания/смены пароля откатывать нечего).

> **Откат смены роли и эмулятор-sync.** Откат `role_changed` возвращает
> прежнюю роль корректно (см. тесты). Но фоновый sync из эмулятора раньше
> переутверждал роль на каждом цикле и «возвращал» её после отката. Исправлено:
> `AssignRoleFromSync` назначает роль эмулятора **только при первом импорте**
> (когда активной роли ещё нет) и не трогает роли, изменённые вручную —
> ручное управление авторитетно, откат держится. Регрессия закрыта тестом
> `TestAssignRoleFromSync_FirstImportOnly`.

> Сама смена роли выполняется атомарной ручкой RBAC
> `POST /api/users/{id}/roles/change` (`{from_slug, to_slug}`) — снять старую и
> выдать новую в одной транзакции. Именно она пишет событие `role_changed`.
>
> Правку ФИО/группы пользователя администратором выполняет
> `PATCH /api/users/{id}` (право `users.update`). Изменение пишется как
> `updated` с `created_by` = администратор (а не сам пользователь), поэтому в
> истории видно, кто правил, и правку можно откатить.

---

## 5. Что логируется и семантика undo

- **user**: `created` (регистрация), `updated` (правка профиля), `password_changed`
  (только факт — без пароля/хеша).
- **смена роли (админ-панель)**: `role_changed` — ОДНА запись со срезом
  «прежняя роль → новая» (`before`/`after`). Делается атомарным `ChangeRole`
  (снять старую + выдать новую в одной транзакции), поэтому промежуточного
  состояния «обе роли» не возникает, а история читается как «Роль изменена: X → Y».
- **точечная выдача/снятие роли** (standalone-эндпоинты, одобрение заявок на
  teacher): `role_assigned` / `role_revoked` — пишутся и в историю пользователя,
  и в историю роли. Выдача **идемпотентна**: повтор уже активной роли — no-op,
  не конфликт UNIQUE-индекса.
- **undo** (`RestoreFromLog`) — применяет `before` в той же транзакции и сам
  логируется как `restored_from_log`:
  - профиль (`updated`) → восстанавливает ФИО/группу/дату;
  - смена роли (`role_changed`) → возвращает прежнюю роль и снимает новую (цельно);
  - выдача роли (`role_assigned`) → снимает; снятие (`role_revoked`) → возвращает;
  - повторный откат той же записи (состояние уже целевое) → **409 `restore_noop`**
    («откат уже выполнен»), а не ошибка 500;
  - неподдержанные действия (`created`, `password_changed`) → **422**.

---

## 6. Безопасность

- В `change_logs` не пишется `password_hash` — срез строит `UserSnapshot`, где
  его нет (тест `TestUserSnapshot_NoSecrets`).
- Просмотр истории и откат закрыты правами; чужой пользователь получает 403.
- Ручки восстановления/смены пароля прикрыты rate-limit на уровне роутера.

---

## 7. Тесты: что покрыто и как запускать

**Юнит-тесты (без БД, выполняются всегда):**
- `Diff` — в `changed_fields` попадают только изменившиеся поля;
- `UserSnapshot` — в срезе нет пароля/хеша;
- smoke-тест роутера — нет конфликтов маршрутов.

**Интеграционные (нужен Postgres):**
- запись created/updated/soft_deleted, порядок, валидация полей;
- `Register` пишет `created` в change_logs;
- undo профиля возвращает прежние значения и пишет `restored_from_log`;
- undo выдачи роли снимает роль;
- откат `created` → `ErrRestoreUnsupported`; несуществующий лог → `ErrLogNotFound`.

**Как запускать:**
```bash
cd backend
go test ./...                 # юнит — зелёные; интеграционные скипаются без БД
make test-integration         # поднять Postgres и прогнать ВСЁ, включая интеграционные
```

**Статус прогона (на момент сдачи):** зелёные и юнит-, и **интеграционные**
тесты. Интеграционные прогнаны локально против поднятого Postgres
(`go run goose ... up` + `TEST_DATABASE_URL=…`): пакеты `rbac` и `changelog`
(включая `TestRestore_RoleChangedUndo`, `TestRestore_RoleDoubleIsNoop`,
`TestRBAC_ChangeRole_*`) — `ok`. Без `TEST_DATABASE_URL` интеграционные
корректно скипаются; полный прогон также идёт в CI.

---

## 8. Соответствие критериям оценки ТЗ

| Критерий | Баллы | Статус |
|---|---|---|
| Транзакционность (лог только при успехе) | 10 | ✅ лог в `RunInTx` вместе с операцией |
| Корректный сбор before/after | 10 | ✅ полные срезы в БД, дифф на выдаче |
| Undo (откат к состоянию из лога) | 30 | ✅ профиль + роли, `RestoreFromLog` |
| Права на просмотр/откат, 403 без прав | 1 | ✅ `changelog.view` / `changelog.restore` |
| 403 называет требуемое право | 1 | ✅ `message` + `required_permission` в теле ответа |
| JSON-ответы | 2 | ✅ |
| Не светить служебные поля/пароли (штраф −10) | — | ✅ `UserSnapshot` без `password_hash` |
| Покрытие тестами | до 8 | ✅ юнит + интеграционные |

---

## 9. Соответствие тест-кейсам ТЗ (TC-01…TC-40)

| TC | Проверка | Статус / как покрыто |
|---|---|---|
| 03–05 | Миграция, структура `change_logs`, FK `created_by` | ✅ миграция 00009 |
| 06–07 | Модель + DTO `changed_fields` | ✅ `queries.ChangeLog`, `dto.ChangeLogEntry` |
| 08 | Автологирование подключено | ✅ через сервис в auth/rbac (аналог observers) |
| 09 | Лог created | ✅ `TestAuth_Register_LogsUserCreated` |
| 10 | Лог updated (before/after) | ✅ `TestRestore_UserProfile`, `TestChangeLog_LogUpdated` |
| 11–13 | Soft/hard delete, restored | ◑ `LogSoftDeletedTx`/`restored` есть; users не удаляются (N/A для них) |
| 15 | Нет лишних логов при «пустом» апдейте | ◑ `Diff` даёт пустой `changed_fields` (запись пишется, дифф пуст) |
| 16–18 | Story user/role/permission | ✅ эндпоинты + права |
| 19 | Доступ без права → 403 | ✅ `RequirePermission(changelog.view)` |
| 20 | Несуществующая сущность | ◑ возвращаем 200 с пустым списком |
| 21–22 | Маршрут restore + откат к before | ✅ `RestoreFromLog`, `TestRestore_UserProfile` |
| 24 | Restore несуществующего лога → 404 | ✅ `TestRestore_NotFound` |
| 25 | Restore без прав → 403 | ✅ `changelog.restore` |
| 27–28 | Откат транзакции при ошибке / успех | ✅ лог в общей `RunInTx` |
| 30 | Пароль не в логе | ✅ `TestUserSnapshot_NoSecrets` |
| 31 | Пустая история → пустой 200 | ✅ |
| 32 | Restore лога не для той сущности | ✅ `RestoreFromLog` проверяет entity_type+action |
| 37–40 | Тесты есть/проходят, JSON, без секретов | ✅ |

Легенда: ✅ — сделано; ◑ — частично/с оговоркой (см. ниже).

---

## 10. Осознанные отклонения от методички

- **Observers → транзакционный лог в сервисе.** В Go нет Eloquent; выбранный
  подход надёжнее по транзакционности.
- **Единое право `changelog.view`** вместо гранулярных `get-story-*` (+ `changelog.restore`).
- **Несуществующая сущность в story → 200 с пустым списком** (а не 404): история
  не привязана жёстко к существованию строки.
- **`permission`-история обычно пуста**: в системе нет CRUD прав, они сидовые.
- **Без git-веток `lb4` (TC-01/02)**: по договорённости работаем строго локально.

---

## 11. Как проверить вживую

1. Поднять стек: `APP_PORT=8080 docker compose up -d --build` (миграция 00027 накатится на старте).
2. Войти администратором (сид-аккаунт).
3. Админ-панель → у пользователя нажать **«История»** → видно записи.
4. Сменить роль / поправить профиль → в истории появляется запись с `changed_fields`;
   кнопка **«Откатить»** возвращает прежнее состояние.
5. Кнопка **«Журнал изменений»** в шапке → общая лента последних действий по всем.
6. Под обычным пользователем те же запросы → **403**.

Для проверки **через API** (показать ответы преподавателю) — готовый сценарий
[docs/audit-logging.http](audit-logging.http) для расширения VS Code REST Client
(`humao.rest-client`): логин → смена роли → история → журнал → откат → 404/422.

---

## 12. Что не входит в этот этап

- Гранулярные `get-story-*` по сущностям (используем единый `changelog.view`).
- Отдельные UI-страницы истории ролей/прав (бэкенд-эндпоинты готовы, в админке —
  история пользователя).
- Обработка конфликтов при параллельном откате (TC-26): сейчас last-write-wins.
