# Как руками показать вебхук (лаба №6) — Linux и PowerShell

Этот документ — **пошаговая инструкция вручную**, без скриптов. В конце ты
**своими глазами увидишь файл**, который появился в папке, потому что сервер
выполнил `git pull` по вебхуку. Это и есть доказательство — не текст в
терминале, а реальный файл на диске.

> Почему ошибка `dial tcp 127.0.0.1:5432 ... actively refused`? Значит **не
> запущена база данных**. Сервер без БД не стартует. Сначала Postgres (Шаг 1),
> потом сервер (Шаг 2). Не наоборот.

---

## ЧАСТЬ A. Запуск сервера

### Шаг 1. Поднять Postgres (обязательно!)

Из **корня проекта** (где лежит `docker-compose.yml`):

```
docker compose up -d postgres
```

Проверь, что поднялся **и порт 5432 проброшен на хост** (это ключевой момент!):
```
docker compose ps
```
В колонке PORTS у postgres должно быть **`0.0.0.0:5432->5432/tcp`**.
Если там просто `5432/tcp` (без `0.0.0.0:5432->`) — порт НЕ проброшен, и
локальный backend не достучится до БД (та самая ошибка `5432 ... refused`).
В этом проекте проброс уже прописан в `docker-compose.yml` (`ports: 5432:5432`),
так что просто пересоздай контейнер: `docker compose up -d postgres`.

> Нет докера? Тогда нужен установленный локально PostgreSQL с базой
> `academic_debts`, пользователем `academic` и паролем `academic`. Проще docker.

### Шаг 2. Запустить backend

#### Linux / macOS (bash) — из папки `backend`:
```bash
cd backend
DB_HOST=127.0.0.1 DB_PORT=5432 DB_NAME=academic_debts DB_USER=academic \
DB_PASSWORD=academic \
JWT_SECRET=8f4b2a8d3e9f1c7a6b5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b \
APP_PORT=8080 EMULATOR_URL= ALLOWED_ORIGINS=* \
GIT_WEBHOOK_SECRET=mysecret123 \
GIT_REPO_PATH=.. GIT_DEFAULT_BRANCH=lb12 \
go run ./cmd/server
```

#### Windows (PowerShell) — из папки `backend`:
```powershell
cd backend
$env:DB_HOST="127.0.0.1"; $env:DB_PORT="5432"
$env:DB_NAME="academic_debts"; $env:DB_USER="academic"; $env:DB_PASSWORD="academic"
$env:JWT_SECRET="8f4b2a8d3e9f1c7a6b5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b"
$env:APP_PORT="8080"; $env:EMULATOR_URL=""; $env:ALLOWED_ORIGINS="*"
$env:GIT_WEBHOOK_SECRET="mysecret123"
$env:GIT_REPO_PATH=".."; $env:GIT_DEFAULT_BRANCH="lb12"
go run ./cmd/server
```

Успех — в логе строка `"server starting" ... ":8080"`. Этот терминал **не
закрывай**, сервер в нём работает. Дальше всё делаешь в **другом** терминале или
в REST Client.

### Шаг 3. Простейшая проверка кодов (REST Client)

Открой [git-webhook.http](git-webhook.http) и жми **Send Request**:
- верный ключ → **200** (JSON со списком шагов: `git checkout` → `reset` → `pull`);
- неверный ключ → **403**;
- без ключа → **403**.

Это показывает, что **проверка секрета и сам деплой-эндпоинт работают**. Но pull
здесь скажет «Already up to date» (репозиторий уже актуален) — поэтому для
**живого доказательства** делаем Часть B.

---

## ЧАСТЬ B. Живое доказательство: файл прилетает через вебхук

Идея: заведём отдельный «удалённый» репозиторий (просто папка) и рабочую копию,
настроим сервер на рабочую копию. Потом добавим файл в «удалённый» и push —
а сервер по вебхуку сделает pull и **притянет этот файл в рабочую копию**.

### Шаг B1. Создать игрушечный репозиторий

#### Linux / macOS (bash):
```bash
cd ~                                   # или любое место вне проекта
rm -rf wh-demo && mkdir wh-demo && cd wh-demo

git init --bare origin.git             # «GitHub» — удалённый репозиторий
git clone origin.git work              # рабочая копия (её будет деплоить сервер)
cd work
git config user.email d@d; git config user.name Demo
echo "start" > readme.txt
git add . && git commit -m "init" && git push origin master
cd ..
echo "Рабочая копия (work):" && ls work
```

#### Windows (PowerShell):
```powershell
cd $HOME
Remove-Item -Recurse -Force wh-demo -ErrorAction SilentlyContinue
mkdir wh-demo; cd wh-demo

git init --bare origin.git
git clone origin.git work
cd work
git config user.email d@d; git config user.name Demo
"start" | Out-File -Encoding utf8 readme.txt
git add .; git commit -m "init"; git push origin master
cd ..
"Рабочая копия (work):"; Get-ChildItem work
```

Сейчас в `work` есть только `readme.txt`.

### Шаг B2. Перезапустить сервер, нацелив на эту рабочую копию

Останови сервер из Части A (`Ctrl+C`) и запусти заново, поменяв **только**
`GIT_REPO_PATH` на путь к `work` и ветку на `master`:

#### Linux / macOS (bash) — из папки `backend`:
```bash
DB_HOST=127.0.0.1 DB_PORT=5432 DB_NAME=academic_debts DB_USER=academic \
DB_PASSWORD=academic \
JWT_SECRET=8f4b2a8d3e9f1c7a6b5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b \
APP_PORT=8080 EMULATOR_URL= ALLOWED_ORIGINS=* \
GIT_WEBHOOK_SECRET=mysecret123 \
GIT_REPO_PATH="$HOME/wh-demo/work" GIT_DEFAULT_BRANCH=master \
go run ./cmd/server
```

#### Windows (PowerShell) — из папки `backend`:
```powershell
$env:GIT_REPO_PATH="$HOME\wh-demo\work"; $env:GIT_DEFAULT_BRANCH="master"
go run ./cmd/server
```
(остальные `$env:` из Шага 2 уже заданы в этой сессии)

### Шаг B3. Добавить новый файл в «удалённый» и запушить

В **новом** терминале:

#### Linux / macOS (bash):
```bash
cd ~/wh-demo
git clone origin.git seed && cd seed
git config user.email d@d; git config user.name Demo
echo "Я ПОЯВИЛСЯ ЧЕРЕЗ ВЕБХУК" > deployed.txt
git add . && git commit -m "new file" && git push origin master
cd ..
echo "Сейчас в work ещё НЕТ deployed.txt:" && ls work
```

#### Windows (PowerShell):
```powershell
cd $HOME\wh-demo
git clone origin.git seed; cd seed
git config user.email d@d; git config user.name Demo
"Я ПОЯВИЛСЯ ЧЕРЕЗ ВЕБХУК" | Out-File -Encoding utf8 deployed.txt
git add .; git commit -m "new file"; git push origin master
cd ..
"Сейчас в work ещё НЕТ deployed.txt:"; Get-ChildItem work
```

Обрати внимание: `deployed.txt` есть в «удалённом», но в `work` его **ещё нет**.

### Шаг B4. Дёрнуть вебхук → сервер сделает pull

Любым способом:
- в [git-webhook.http](git-webhook.http) нажми **Send Request** на запросе #1, **или**
- curl:

#### Linux / macOS:
```bash
curl -X POST http://localhost:8080/api/hooks/git \
  -H 'Content-Type: application/json' -d '{"secret_key":"mysecret123"}'
```
#### Windows (PowerShell):
```powershell
curl.exe -X POST http://localhost:8080/api/hooks/git -H "Content-Type: application/json" -d '{\"secret_key\":\"mysecret123\"}'
```

### Шаг B5. ДОКАЗАТЕЛЬСТВО — файл появился

```bash
ls ~/wh-demo/work              # Linux/macOS
```
```powershell
Get-ChildItem $HOME\wh-demo\work    # Windows
```

Теперь в `work` есть **`deployed.txt`** — его туда притянул сервер по вебхуку.
Открой папку в проводнике/редакторе и покажи файл. **Это и есть доказательство:
файл реально на диске, его никто руками не писал — он прилетел через `git pull`,
запущенный вебхуком.**

Дополнительно покажи журнал деплоя (сервер пишет каждое событие):
```
backend/storage/logs/deployment.log
```

---

## Что говорить преподавателю

- «Вебхук — это POST `/api/hooks/git`. По нему сервер делает `git pull` в рабочем
  каталоге (`GIT_REPO_PATH`). Я завёл отдельный репозиторий, запушил в него
  новый файл, дёрнул вебхук — и файл **физически появился** в рабочей папке.»
- «Защита — секретным ключом (`GIT_WEBHOOK_SECRET`), сравнение constant-time.
  Неверный ключ → 403, пустой ключ на сервере → 503.»
- «Если в рабочей папке есть незакоммиченные изменения — деплой их обнаруживает,
  пишет warning в журнал и всё равно делает pull (`reset --hard` + `clean`),
  имитируя поведение прод-сервера.»

---

## Если что-то не так

| Симптом | Причина и решение |
|---|---|
| `dial tcp 127.0.0.1:5432 ... refused` (хотя контейнер запущен!) | Порт БД не проброшен на хост. Проверь `docker compose ps`: в PORTS должно быть `0.0.0.0:5432->5432`. Если нет — `docker compose up -d postgres` пересоздаст контейнер с пробросом (он уже прописан в compose). |
| Ответ **503** на вебхук | `GIT_WEBHOOK_SECRET` пуст в окружении сервера → задай его при запуске. |
| Ответ **403** на верный ключ | `@secret` в .http ≠ `GIT_WEBHOOK_SECRET` сервера. Сделай одинаковыми. |
| `pull` пишет `Already up to date` | В «удалённом» нет нового коммита → повтори Шаг B3 (push нового файла). |
| `deployed.txt` не появился | Сервер нацелен не на `work`. Проверь `GIT_REPO_PATH` и ветку (`master`). |
