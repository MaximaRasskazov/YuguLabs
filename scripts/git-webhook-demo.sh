#!/usr/bin/env bash
###############################################################################
# Демонстрация РЕАЛЬНОГО git pull через webhook (лаба №6).
#
# Скрипт самодостаточный и безопасный: НЕ трогает твой рабочий репозиторий.
# Он создаёт во временной папке отдельный git-репозиторий с собственным
# "origin", в котором лежит маркер-коммит, поднимает throwaway Postgres +
# backend, дёргает webhook и показывает, что маркер РЕАЛЬНО подтянулся
# (git pull сработал) — плюс журнал деплоя и негативный кейс (403).
#
# Требуется: docker, go, git, curl. Запуск:  bash scripts/git-webhook-demo.sh
###############################################################################
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
PG="yugu-webhook-demo-pg"
PORT=8099
PGPORT=5440
# Секрет ровно 36 символов (как требует ТЗ).
SECRET="demo-secret-key-000000000000000000aa"
SRVPID=""

cleanup() {
  [ -n "$SRVPID" ] && kill "$SRVPID" >/dev/null 2>&1
  taskkill //F //IM webhook-demo-server.exe >/dev/null 2>&1 || true
  docker rm -f "$PG" >/dev/null 2>&1 || true
  rm -rf "$TMP" "$ROOT/backend/webhook-demo-server.exe" "$ROOT/backend/webhook-demo.log" 2>/dev/null || true
}
trap cleanup EXIT

echo "== 1. Throwaway git-репозиторий: origin с маркером, work отстаёт =="
git init -q --bare "$TMP/origin.git"
# Дефолтная ветка origin = main (надёжно на любой версии git, в т.ч. где
# init.defaultBranch=master) — иначе клоны окажутся на master и pull не сойдётся.
git -C "$TMP/origin.git" symbolic-ref HEAD refs/heads/main
git clone -q "$TMP/origin.git" "$TMP/work"
( cd "$TMP/work"
  git config user.email demo@local; git config user.name Demo
  echo "initial" > README.md
  git add README.md; git commit -q -m "init"; git branch -M main; git push -q origin main )
git clone -q "$TMP/origin.git" "$TMP/seed"
MARKER="DEPLOY_MARKER_$(date +%s).txt"
( cd "$TMP/seed"
  git config user.email demo@local; git config user.name Demo
  echo "Этот файл подтянут webhook-ом $(date -u +%FT%TZ)" > "$MARKER"
  git add "$MARKER"; git commit -q -m "add marker"; git push -q origin main )
if [ -f "$TMP/work/$MARKER" ]; then echo "  ОШИБКА: маркер уже в work"; else echo "  ✅ work отстаёт от origin на коммит с $MARKER"; fi

echo "== 2. Поднимаю Postgres + backend =="
docker rm -f "$PG" >/dev/null 2>&1 || true
docker run -d --name "$PG" -e POSTGRES_USER=academic -e POSTGRES_PASSWORD=academic \
  -e POSTGRES_DB=academic -p ${PGPORT}:5432 postgres:17-alpine >/dev/null
for i in $(seq 1 30); do docker exec "$PG" pg_isready -U academic -d academic >/dev/null 2>&1 && break; sleep 1; done
( cd "$ROOT/backend" && go build -o ./webhook-demo-server.exe ./cmd/server )
DB_HOST=127.0.0.1 DB_PORT=${PGPORT} DB_NAME=academic DB_USER=academic DB_PASSWORD=academic \
  JWT_SECRET=demo-jwt-secret-which-is-long-enough-32 APP_PORT=${PORT} EMULATOR_URL= ALLOWED_ORIGINS=x \
  GIT_WEBHOOK_SECRET="$SECRET" GIT_REPO_PATH="$TMP/work" GIT_DEFAULT_BRANCH=main \
  GIT_DEPLOY_LOG="$TMP/deployment.log" \
  "$ROOT/backend/webhook-demo-server.exe" > "$ROOT/backend/webhook-demo.log" 2>&1 &
SRVPID=$!
for i in $(seq 1 20); do [ "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:${PORT}/health 2>/dev/null)" = "200" ] && break; sleep 1; done

echo "== 3. POST /hooks/git с верным ключом =="
CODE=$(curl -s -o "$TMP/resp.json" -w "%{http_code}" -X POST "http://127.0.0.1:${PORT}/hooks/git" \
  -H 'Content-Type: application/json' -d "{\"secret_key\":\"$SECRET\"}")
echo "  HTTP $CODE"
echo "  ответ: $(cat "$TMP/resp.json")"

echo "== 4. Маркер реально подтянулся? =="
if [ -f "$TMP/work/$MARKER" ]; then
  echo "  ✅ git pull сработал: $MARKER появился в рабочем дереве — деплой настоящий"
else
  echo "  ❌ маркер не появился (см. backend/webhook-demo.log)"
fi

echo ""
echo "== Журнал деплоя =="
cat "$TMP/deployment.log" 2>/dev/null

echo ""
echo "== Негатив: неверный ключ → ждём 403 =="
echo "  HTTP $(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:${PORT}/hooks/git \
  -H 'Content-Type: application/json' -d '{"secret_key":"wrong"}')"
echo ""
echo "Готово. Временные ресурсы будут убраны автоматически."
