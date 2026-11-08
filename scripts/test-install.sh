#!/usr/bin/env bash
# Проверка install.sh без pull/up: полный и минимальный стек, обновление существующего .env, отказ от опасных значений XRAY_*.
# Нужен docker compose (только `config`, контейнеры не запускаются). Запуск: scripts/test-install.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }

# архив с корнем <repo>/deploy, как у codeload
tar -C "$root" -czf "$work/src.tgz" --transform 's,^,repo/,' deploy

# значения для обязательных ключей, остальные — Enter (значение по умолчанию)
declare -A ans=(
  [TELEGRAM_TOKEN]=1:tok [TELEGRAM_ADMIN_ID]=1 [REMNAWAVE_PANEL_URL]=https://panel.example.test [REMNAWAVE_API_TOKEN]=tok
  [REMNAWAVE_INTERNAL_SQUAD_ID]=d15c35d9-b6a4-4f8a-ad36-3fdad933aa49 [XRAY_DOMAIN]=vpn.example.test
  [XRAY_UUID]=11111111-2222-4333-8444-555555555555 [XRAY_PBK]=qrBr4uIEhbLCrS7jd8ZHMb1vMzlpig-Uu5mqzoGhP34 [XRAY_SID]=bc77b7607214068a
  [GRAFANA_ADMIN_PASSWORD]=g1 [ALERT_TELEGRAM_TOKEN]=2:tok [ALERT_TELEGRAM_CHAT_ID]=1
)
obs=" GRAFANA_PORT GRAFANA_ADMIN_PASSWORD GRAFANA_ALLOWED_ORIGINS ALERT_TELEGRAM_TOKEN ALERT_TELEGRAM_CHAT_ID "

# ответы в порядке вопросов install.sh: ключи из .env.example по порядку, без COMPOSE_FILE и (в минимальном) без ключей наблюдаемости
answers() { # answers <minimal 0|1> [extra-first-line]
  [[ -z ${2:-} ]] || echo "$2"
  while IFS= read -r line; do
    [[ $line =~ ^([A-Z_][A-Z0-9_]*)= ]] || continue
    k="${BASH_REMATCH[1]}"
    [[ $k == COMPOSE_FILE ]] && continue
    [[ $1 == 1 && $obs == *" $k "* ]] && continue
    echo "${ans[$k]:-}"
  done <"$root/deploy/.env.example"
}

run() { # run <dir> <input> [args...]
  local d="$1" in="$2"
  shift 2
  BRUHANET_DIR="$d" BRUHANET_TARBALL="$work/src.tgz" BRUHANET_INPUT="$in" BRUHANET_SKIP_UP=1 bash "$root/install.sh" "$@" >"$work/out.log" 2>&1
}
services() { (cd "$1" && docker compose --env-file .env config --services | sort | tr '\n' ' '); }

# 1. полный стек
answers 0 >"$work/in-full"
run "$work/full" "$work/in-full" || { cat "$work/out.log"; fail "full install"; }
grep -q "^COMPOSE_FILE='docker-compose.yaml:docker-compose.observability.yaml'$" "$work/full/.env" || fail "full: COMPOSE_FILE"
s="$(services "$work/full")"
for svc in grafana loki prometheus tempo alloy bot api xray xray-config backup postgres redis migrate; do [[ $s == *"$svc "* ]] || fail "full: нет сервиса $svc ($s)"; done
ok "полный стек: $s"

# 2. минимальный стек (флаг и переменная)
answers 1 >"$work/in-min"
run "$work/min" "$work/in-min" --minimal || { cat "$work/out.log"; fail "minimal install"; }
grep -q "^COMPOSE_FILE='docker-compose.yaml'$" "$work/min/.env" || fail "min: COMPOSE_FILE"
grep -q "^GRAFANA_ADMIN_PASSWORD=''$" "$work/min/.env" || fail "min: ключ GRAFANA_ADMIN_PASSWORD должен быть (пустой) для CI-проверки ключей"
s="$(services "$work/min")"
for svc in grafana loki prometheus tempo alloy; do [[ $s != *"$svc "* ]] || fail "min: лишний сервис $svc ($s)"; done
for svc in bot api xray xray-config backup postgres redis migrate; do [[ $s == *"$svc "* ]] || fail "min: нет сервиса $svc ($s)"; done
ok "минимальный стек: $s"
if (cd "$work/min" && docker compose --env-file .env config | grep -q OTEL_EXPORTER_OTLP_ENDPOINT); then fail "min: трейсинг не должен быть включён"; fi
ok "минимальный стек без трейсинга"

# 3. обновление существующего .env без ввода: нет XRAY_* без значения по умолчанию — понятная ошибка
rm -rf "$work/upd" && mkdir "$work/upd"
grep -vE "^(XRAY_|COMPOSE_FILE)" "$work/min/.env" >"$work/upd/.env"
if run "$work/upd" /nonexistent --minimal; then fail "update без XRAY_* должен падать"; fi
grep -q "XRAY_DOMAIN" "$work/out.log" || { cat "$work/out.log"; fail "update: в ошибке нет XRAY_DOMAIN"; }
ok "обновление без XRAY_* отказывает со списком ключей"

# 4. обновление, не хватает только COMPOSE_FILE: добавляется по режиму
grep -vE "^COMPOSE_FILE" "$work/min/.env" >"$work/upd/.env"
run "$work/upd" /nonexistent --minimal || { cat "$work/out.log"; fail "update COMPOSE_FILE"; }
grep -q "^COMPOSE_FILE='docker-compose.yaml'$" "$work/upd/.env" || fail "update: COMPOSE_FILE не добавлен"
ok "обновление добавляет COMPOSE_FILE по режиму"

# 5. опасные значения XRAY_* не принимаются, повторный ввод проходит
rm -rf "$work/bad"
{ answers 1 | awk -v bad='a|b;rm' '{ if ($0 == "vpn.example.test" && !done) { print bad; done = 1 } print }'; } >"$work/in-bad"
run "$work/bad" "$work/in-bad" --minimal || { cat "$work/out.log"; fail "bad xray value flow"; }
grep -q "XRAY_DOMAIN: допустимы" "$work/out.log" || fail "bad: нет сообщения об недопустимых символах"
grep -q "^XRAY_DOMAIN='vpn.example.test'$" "$work/bad/.env" || fail "bad: в .env не то значение"
ok "небезопасное XRAY_DOMAIN отклонено"

echo "все проверки install.sh пройдены"
