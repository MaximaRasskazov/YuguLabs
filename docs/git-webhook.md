# Лабораторная работа №6 — Webhook авто-деплоя по git

> Документ для защиты: что сделано, где лежит, как проверить и чем
> подтверждается каждый критерий из ТЗ.

## 1. Контекст и адаптация ТЗ

Исходное ТЗ написано под **Laravel/PHP**. Наш проект — **Go (backend) + Vue
(frontend)**, поэтому, как и в лабе по аудиту/логированию, сделана
**эквивалентная адаптация на наш стек с сохранением всех гарантий ТЗ**:
открытый webhook-маршрут, проверка секретного ключа, синхронный деплой
(`git checkout` → `git reset --hard` → `git pull`), логирование каждого
шага, блокировка параллельного запуска, обработка ошибок и корректные
HTTP-статусы.

Соответствие сущностей ТЗ → реализация:

| ТЗ (Laravel)                          | Наша реализация (Go)                         |
|---------------------------------------|----------------------------------------------|
| `GitWebhookController::__invoke`      | `handler.GitWebhookHandler.Handle`           |
| `DeployService`                       | `deploy.Service` (оркестратор)               |
| `exec()` / Symfony `Process`          | `deploy.Runner` → `GitRunner` (`os/exec`)    |
| Cache-lock `deploy_lock` с TTL        | `deploy.Locker` → `MemoryLock` (мьютекс+TTL) |
| `storage/logs/deployment.log`         | `deploy.Recorder` → `FileRecorder` (JSON Lines) |
| `config/git.php` + `.env`             | `internal/config` + `.env` (`GIT_*`)         |
| Form Request / `$request->input()`    | разбор тела в `extractSecret` (JSON и form)  |

## 2. Как это работает (поток запроса)

```
POST /api/hooks/git  { "secret_key": "..." }
        │
        ▼
 GitWebhookHandler.Handle
   ├─ secret не задан в .env?           → 503 webhook_disabled
   ├─ secret_key ≠ GIT_WEBHOOK_SECRET?  → 403 Invalid secret key   (constant-time)
   └─ ключ верный → deploy.Service.Deploy(ctx, clientIP)
            │
            ├─ лог "started" (время, IP)                 [4.1 ТЗ]
            ├─ TryLock(ttl) занято? → лог "locked" → 409 [4.2 ТЗ]
            ├─ .git нет в каталоге?  → лог "error" → 500
            ├─ git checkout <branch>   → лог command/...  [4.3 ТЗ]
            ├─ git reset --hard HEAD   → лог command/...
            ├─ git pull origin <branch>→ лог command/...
            │     любая команда упала? → лог error → Unlock → 500
            ├─ Unlock (defer)                             [4.4 ТЗ]
            └─ лог "finished/success" → 200 JSON со списком шагов
```

Деплой **синхронный**: HTTP-ответ возвращается только после завершения всех
команд (критерий ТЗ №10) — клиент получает финальный статус, а не «принято».

## 3. Архитектура и SOLID

Логика разнесена по слоям, зависимости — через интерфейсы:

```
handler.GitWebhookHandler   — HTTP: проверка ключа, маппинг ошибок в статусы
        │ (зависит от *deploy.Service)
        ▼
deploy.Service              — оркестрация деплоя (порядок шагов, лог, блокировка)
        │ зависит от интерфейсов:
        ├── Runner    — запуск git-команд        (бой: GitRunner / тест: fakeRunner)
        ├── Locker    — блокировка параллелизма   (бой: MemoryLock / тест: тот же)
        └── Recorder  — журнал деплоя             (бой: FileRecorder / тест: sliceRecorder)
```

- **Single Responsibility.** Контроллер не содержит логику деплоя; сервис не
  знает про HTTP; запуск команд, блокировка и журнал — три отдельные
  абстракции.
- **Dependency Inversion.** `deploy.Service` зависит от интерфейсов
  `Runner`/`Locker`/`Recorder`, а не от `os/exec` и файла напрямую — отсюда
  тесты без настоящего git.
- **Open/Closed.** Блокировку in-process (`MemoryLock`) можно заменить на
  распределённую (advisory-lock в Postgres, `flock` на общий том) без правок
  оркестратора — достаточно другой реализации `Locker`.
- **Секрет не доходит до сервиса.** `Service.Deploy` принимает только
  `clientIP`. Ключ остаётся в handler — он физически не может попасть в
  журнал деплоя.

## 4. Файлы

| Файл | Назначение |
|---|---|
| `backend/internal/service/deploy/deploy.go` | Оркестратор `Service`, `Config`, `Result`/`Step`, ошибки `ErrLocked`/`ErrNotGitRepo`, проверка `.git` |
| `backend/internal/service/deploy/runner.go` | `Runner` + `GitRunner` (`os/exec`), `Command`, `DeployCommands(branch)` |
| `backend/internal/service/deploy/lock.go` | `Locker` + `MemoryLock` (мьютекс + TTL + перехват протухшей блокировки) |
| `backend/internal/service/deploy/recorder.go` | `Recorder` + `FileRecorder` (JSON Lines + slog), стадии/статусы |
| `backend/internal/service/deploy/deploy_test.go` | Юнит-тесты сервиса (порядок команд, ветка из конфига, не-репо, ошибка, блокировка) |
| `backend/internal/service/deploy/lock_test.go` | Юнит-тесты блокировки |
| `backend/internal/transport/http/handler/deploy.go` | `GitWebhookHandler`: ключ (constant-time), `extractSecret`, маппинг статусов |
| `backend/internal/transport/http/handler/deploy_test.go` | Юнит-тесты handler (403/409/200/503, form-data, регистр) |
| `backend/internal/transport/http/dto/deploy.go` | `DeployResponse`/`DeployStep` + `FromDeployResult` |
| `backend/internal/transport/http/router_deploy.go` | `mountDeploy`: `POST /api/hooks/git` |
| `backend/internal/config/config.go` | Переменные `GIT_*` |
| `.env.example` | Документированные `GIT_*` |
| `docs/git-webhook.http` | REST Client-сценарий |

Подключение: `router.go` — `mountDeploy(r, d)` вне 30-сек таймаут-группы;
`cmd/server/main.go` — сборка `deploy.New(...)` и проброс в `Deps`.

## 5. Конфигурация (`.env`)

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `GIT_WEBHOOK_SECRET` | *(пусто)* | Секретный ключ (36 символов). **Пусто = хук выключен (503).** Хранится только в `.env`, в репозиторий не попадает. |
| `GIT_DEFAULT_BRANCH` | `main` | Ветка деплоя: `git checkout <branch>` и `git pull origin <branch>`. |
| `GIT_REPO_PATH` | `.` | Каталог git-репозитория, где выполняются команды (должен содержать `.git`). |
| `GIT_DEPLOY_LOG` | `storage/logs/deployment.log` | Файл журнала деплоя (каталог создаётся автоматически). |
| `GIT_DEPLOY_TIMEOUT` | `5m` | Потолок времени на весь деплой. |

Сгенерировать ключ: `openssl rand -hex 18` (даёт 36 hex-символов).

## 6. API

**`POST /api/hooks/git`** — открыт без авторизации. `secret_key` принимается
как из JSON-тела (`{"secret_key":"..."}`), так и из `form-data`
(`secret_key=...`). Метод **POST** выбран осознанно: при GET ключ попал бы в
URL, access-логи и историю — это утечка секрета.

| Код | Когда | Тело |
|---|---|---|
| `200 OK` | Деплой успешен | `{"status":"success","message":"Deployment completed","branch":"main","steps":[{"command":"git checkout main","output":"..."}, ...]}` |
| `403 Forbidden` | Ключ неверный / отсутствует / другой регистр | `{"error":"invalid_secret","message":"Invalid secret key"}` |
| `409 Conflict` | Деплой уже идёт | `{"error":"deploy_in_progress","message":"Deployment already in progress"}` |
| `500 Internal` | Не git-репозиторий или упала git-команда | `{"error":"deploy_failed","message":"git pull origin main: ..."}` |
| `503 Service Unavailable` | `GIT_WEBHOOK_SECRET` не задан | `{"error":"webhook_disabled","message":"git webhook is not configured"}` |

Все ответы — JSON (критерий ТЗ №11).

## 7. Безопасность

- **Секрет только в `.env`** (в `.gitignore`), в репозитории — лишь пустой
  плейсхолдер в `.env.example`.
- **Сравнение в постоянном времени** — `crypto/subtle.ConstantTimeCompare`:
  не подсказывает ключ по таймингу ответа. Регистрозависимо (разный регистр
  → 403).
- **Секрет не логируется и не возвращается.** Сервис деплоя его вообще не
  получает; в журнал пишутся только время, IP, команда, статус и вывод git.
- **Пустой ключ ≠ доступ.** Если `GIT_WEBHOOK_SECRET` не задан — отвечаем 503
  *до* сравнения, иначе пустой ключ совпал бы с пустым `secret_key`.
- **HTTPS** обеспечивает обратный прокси (в проде nginx/Caddy с TLS); ключ
  ходит в теле POST, а не в URL.

## 8. Блокировка параллельного запуска (критерий №9)

`MemoryLock.TryLock(ttl)` неблокирующий: первый запрос берёт блокировку,
второй параллельный получает `false` → handler отвечает **409**. После
завершения деплоя (успех или ошибка) блокировка снимается через `defer`.
`ttl` страхует от «зависшего» процесса: по истечении срока блокировка
считается протухшей и перехватывается следующим запросом — деплой не
застревает навсегда. Бэкенд — один процесс, поэтому in-process мьютекса
достаточно; для кластера интерфейс `Locker` позволяет подменить реализацию.

## 9. Логирование (критерий №5, отдельный сервис)

`FileRecorder` пишет в `GIT_DEPLOY_LOG` по одной JSON-строке на событие
(формат JSON Lines) и дублирует в `slog`. Если файл недоступен — деплой не
падает, остаётся `slog`. Пример журнала успешного деплоя:

```json
{"time":"2026-06-10T12:00:00Z","ip":"10.0.0.5","stage":"started","status":"started"}
{"time":"2026-06-10T12:00:00Z","ip":"10.0.0.5","stage":"command","status":"success","command":"git checkout main","detail":"Already on 'main'"}
{"time":"2026-06-10T12:00:01Z","ip":"10.0.0.5","stage":"command","status":"success","command":"git reset --hard HEAD","detail":"HEAD is now at ..."}
{"time":"2026-06-10T12:00:03Z","ip":"10.0.0.5","stage":"command","status":"success","command":"git pull origin main","detail":"Already up to date."}
{"time":"2026-06-10T12:00:03Z","ip":"10.0.0.5","stage":"finished","status":"success"}
```

Фиксируется: дата/время, IP инициатора, статус старта, каждая команда с
результатом и финал (критерии ТЗ 4.1–4.4). Секрета в журнале нет.

## 10. Обработка ошибок (критерий №2/№7)

- Каждая git-команда логируется (успех/ошибка + вывод).
- Ошибка любой команды → запись в журнал, снятие блокировки (`defer`), ответ
  **500** с описанием (вывод git, без секрета).
- Каталог не является git-репозиторием (нет `.git`) → **500**
  (`ErrNotGitRepo`), команды не запускаются.
- Таймаут/отмена контекста обрывает `git pull` (`exec.CommandContext`).

## 11. Тесты

Запуск: `cd backend && go test ./internal/service/deploy/... ./internal/transport/http/...`
(или `make test` — весь проект с race-детектором). Все тесты проходят.

| Тест | Что проверяет | TC из плана ТЗ |
|---|---|---|
| `TestDeploy_SuccessRunsCommandsInOrder` | Порядок `checkout → reset → pull`, журнал started/…/finished, 3 шага | TC-15, TC-19, TC-20 |
| `TestDeploy_BranchTakenFromConfig` | Ветка берётся из конфига (`develop`), не захардкожена | TC-16 |
| `TestDeploy_NotGitRepoFailsBeforeCommands` | Нет `.git` → `ErrNotGitRepo`, команды не выполняются | TC-17 |
| `TestDeploy_CommandFailureReleasesLock` | Падение `git pull` → ошибка + блокировка снята | TC-14, TC-18 |
| `TestDeploy_AlreadyLockedReturns409Marker` | Занятая блокировка → `ErrLocked`, команды не идут | TC-13 |
| `TestMemoryLock_SecondTryFailsWhileHeld` | Второй `TryLock` = false, после `Unlock` снова true | TC-12 |
| `TestMemoryLock_ExpiredLockIsStolen` | Протухшая блокировка перехватывается | TC-14 |
| `TestGitWebhook_ValidSecretReturns200JSON` | Верный ключ → 200, `Content-Type: application/json`, 3 шага | TC-07, TC-21 |
| `TestGitWebhook_WrongSecretReturns403` | Неверный ключ → 403 | TC-08 |
| `TestGitWebhook_DifferentCaseReturns403` | Другой регистр → 403 (регистрозависимо) | TC-09 |
| `TestGitWebhook_NoSecretConfiguredReturns503` | Ключ не задан → 503 | — |
| `TestGitWebhook_FormEncodedSecretReturns200` | `secret_key` из form-data | TC-07 |
| `TestGitWebhook_ConcurrentReturns409` | Параллельный запрос → 409 | TC-13, TC-22 |

## 12. Соответствие критериям оценки ТЗ (макс. 73)

| № | Критерий | Балл | Как закрыт |
|---|---|---|---|
| 1 | Все требования выполнены, всё работает | 20 | Эндпоинт + ключ + деплой + лог + блокировка + ошибки + статусы |
| 2 | Чистый код, SOLID, типизация, комментарии | 10 | Разделение handler/service/runner/locker/recorder; интерфейсы; docblocks; `go vet`/`gofmt -s` чисто |
| 3 | Маршруты под префиксом `hooks` | 1 | `r.Route("/api/hooks", …)` → `POST /api/hooks/git` |
| 4 | Проверка секретного ключа | 1 | `subtle.ConstantTimeCompare` с `GIT_WEBHOOK_SECRET` |
| 5 | Логирование (отдельный сервис) | 10 | `deploy.Recorder`/`FileRecorder` — отдельный слой |
| 6 | Form Requests (если нужны) | 1 | Разбор и валидация входа в `extractSecret` (JSON + form), Go-эквивалент |
| 7 | Корректные HTTP-статусы | 1 | 200 / 403 / 409 / 500 (+503) |
| 8 | Логи безопасны (нет секретов) | 5 | Секрет не доходит до сервиса/журнала; не возвращается в ответах |
| 9 | Блокировка одновременного выполнения | 5 | `MemoryLock` + 409 |
| 10 | Синхронное выполнение (ответ после завершения) | 5 | `Deploy` синхронный, ответ после всех команд |
| 11 | Ответ в формате JSON | 2 | `dto.DeployResponse` + `dto.ErrorResponse` |
| 12 | Покрытие тестами | 8 | 13 тестов (см. раздел 11) |
| 13 | Работа с Git (ветка, коммиты, слияние) | 4 | См. раздел 13 — выполняется вручную |

## 13. VCS-часть лабы (ветка `lb6`, коммиты, слияние)

Критерий №13 — это **workflow самой лабы в git** (создать ветку `lb6`,
коммитить в неё, в конце слить в `main` без удаления ветки). По текущей
договорённости в этом репозитории автоматические git-операции не делаются —
**эти шаги выполняются вручную**. Команды:

```bash
# 1. Ветка от основной
git checkout main          # или release — основная ветка репозитория
git checkout -b lb6
git push -u origin lb6

# 2. Зафиксировать изменения лабы
git add backend/internal/service/deploy \
        backend/internal/transport/http/handler/deploy.go \
        backend/internal/transport/http/handler/deploy_test.go \
        backend/internal/transport/http/dto/deploy.go \
        backend/internal/transport/http/router_deploy.go \
        backend/internal/transport/http/router.go \
        backend/internal/config/config.go \
        backend/cmd/server/main.go \
        .env.example docs/git-webhook.md docs/git-webhook.http
git commit -m "feat(deploy): git-webhook авто-деплоя (лаба №6)"
git push

# 3. Слияние в основную без удаления lb6
git checkout main
git merge --no-ff lb6 -m "merge: лаба №6 git-webhook"
git push
# ветку lb6 НЕ удаляем (нужна для истории)
```

## 14. Как проверить вручную

1. Задай `GIT_WEBHOOK_SECRET` в `.env` (`openssl rand -hex 18`).
2. Подними стек: `APP_PORT=8080 docker compose up -d --build`.
3. Открой `docs/git-webhook.http` в VS Code (расширение REST Client) и
   прогоняй запросы по порядку: 200 → 200(form) → 403 → 403(регистр) → 403(без ключа).
4. Параллельный 409 — двумя `curl` внахлёст (см. конец `.http`).
5. Журнал — `storage/logs/deployment.log` (внутри контейнера backend, путь
   относительно рабочего каталога процесса).

## 15. Ограничения адаптации

- В **Docker-образе backend** код вкомпилирован в бинарник, каталога `.git`
  там нет — `git pull` внутри контейнера не «передеплоит» Go-приложение
  (в отличие от интерпретируемого PHP). Для реального авто-деплоя
  `GIT_REPO_PATH` должен указывать на рабочий checkout (том), а пересборка
  образа — отдельный шаг CI. В рамках лабы важен сам механизм (хук, ключ,
  лог, блокировка, ошибки, статусы), он полностью реализован и покрыт
  тестами; при `GIT_REPO_PATH` без `.git` эндпоинт корректно отвечает 500.
- Локально (backend запущен на хосте, не в контейнере) с `GIT_REPO_PATH`,
  указывающим на этот репозиторий, деплой отрабатывает end-to-end по-настоящему.
