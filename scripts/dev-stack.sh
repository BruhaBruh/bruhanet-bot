#!/usr/bin/env bash
# Весь dev-стек в одном терминале: бот, api и vite (postgres, redis и миграции уже подняты task'ом stack).
# Порядок: бот первым (дождаться /healthz), затем api и vite: по одной компиляции за раз, машина рабочая.
# Ctrl+C останавливает всё. Логи с префиксами [bot] [api] [web].
set -u
cd "$(dirname "$0")/.."

# Те же значения, что у .env: проверка здоровья бота идёт на его адрес метрик.
bot_port="${OBSERVABILITY_ADDR:-:8081}"
bot_port="${bot_port##*:}"

trap 'trap - INT TERM EXIT; kill 0' INT TERM EXIT

prefix() { sed -u "s/^/[$1] /"; }

go -C bot run ./cmd 2>&1 | prefix bot &
for _ in $(seq 1 120); do
	curl -sf "http://localhost:${bot_port}/healthz" >/dev/null && break
	sleep 1
done
go -C api run ./cmd 2>&1 | prefix api &
pnpm -C web/admin dev 2>&1 | prefix web &

echo "dev-стек запущен: бот :${bot_port}, api :8090, панель http://localhost:5173 (войти: отправь боту /login). Ctrl+C — остановить всё."
wait
