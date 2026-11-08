#!/usr/bin/env bash
# Установка/обновление прод-стека BruhaNet:
#   curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | bash
# Минимальный стек (bot, api, Mini App, Postgres, Redis, xray, бэкапы; без Grafana/Loki/Prometheus/Tempo/Alloy):
#   curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | bash -s -- --minimal
# Env: BRUHANET_DIR (каталог), BRUHANET_REF (ветка/тег), BRUHANET_TARBALL (локальный архив, для тестов),
#      BRUHANET_INPUT (откуда читать ответы, по умолчанию /dev/tty), BRUHANET_SKIP_UP=1 (без pull/up),
#      BRUHANET_MINIMAL=1 (то же, что --minimal).
set -euo pipefail

die() { echo "Ошибка: $*" >&2; exit 1; }

MINIMAL="${BRUHANET_MINIMAL:-0}"
while (($#)); do
  case "$1" in
    --minimal) MINIMAL=1 ;;
    -h | --help)
      echo "Использование: install.sh [--minimal]"
      echo "  --minimal  без наблюдаемости (Grafana, Loki, Prometheus, Tempo, Alloy): bot, api, Mini App, Postgres, Redis, xray, бэкапы"
      exit 0
      ;;
    *) die "неизвестный аргумент '$1' (есть только --minimal)" ;;
  esac
  shift
done
[[ $MINIMAL == 1 ]] || MINIMAL=0

REQUIRED=(TELEGRAM_TOKEN TELEGRAM_ADMIN_ID REMNAWAVE_PANEL_URL REMNAWAVE_API_TOKEN REMNAWAVE_INTERNAL_SQUAD_ID
  XRAY_DOMAIN XRAY_UUID XRAY_PBK XRAY_SID)
# ключи только для наблюдаемости: в минимальной установке не спрашиваем, пишем значения по умолчанию (CI требует, чтобы ключи были)
OBS_KEYS=(GRAFANA_PORT GRAFANA_ADMIN_PASSWORD GRAFANA_ALLOWED_ORIGINS ALERT_TELEGRAM_TOKEN ALERT_TELEGRAM_CHAT_ID)
((MINIMAL)) || REQUIRED+=(GRAFANA_ADMIN_PASSWORD ALERT_TELEGRAM_TOKEN ALERT_TELEGRAM_CHAT_ID)

is_obs() { [[ " ${OBS_KEYS[*]} " == *" $1 "* ]]; }

# какие compose-файлы поднимает стек (читает docker compose из COMPOSE_FILE в .env)
compose_value() {
  if ((MINIMAL)); then echo "docker-compose.yaml"; else echo "docker-compose.yaml:docker-compose.observability.yaml"; fi
}

for c in curl tar docker; do
  command -v "$c" >/dev/null || die "не найден '$c' — установи его и запусти снова"
done
docker compose version </dev/null >/dev/null 2>&1 || die "нужен docker compose v2 (плагин 'docker compose')"

# stdin занят скриптом при `curl | bash`, поэтому ответы — из отдельного дескриптора
have_input=1
{ exec 3<"${BRUHANET_INPUT:-/dev/tty}"; } 2>/dev/null || have_input=0

ask() { # ask <prompt> [-s] -> ответ в $REPLY
  ((have_input)) || die "нет терминала для ввода; задай BRUHANET_INPUT=<файл>"
  local silent=()
  [[ ${2:-} == -s ]] && silent=(-s)
  read -r "${silent[@]}" -u 3 -p "$1" REPLY || die "ввод закончился (EOF)"
  [[ ${2:-} == -s ]] && echo >&2
  # мусор от вставки в терминал: \r и маркеры bracketed paste (ESC[200~ … ESC[201~)
  REPLY="${REPLY//$'\r'/}"
  REPLY="${REPLY//$'\e[200~'/}"
  REPLY="${REPLY//$'\e[201~'/}"
  return 0
}

gen_password() {
  if command -v openssl >/dev/null; then
    openssl rand -hex 24
  else
    head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n'
  fi
}

# default_of <строка KEY=VALUE из .env.example> -> значение без кавычек
default_of() {
  local d="${1#*=}"
  d="${d#[\"\']}"
  d="${d%[\"\']}"
  printf '%s' "$d"
}

# prompt_key <key> <default> -> значение в $val (спрашивает, пока не получит годное)
prompt_key() {
  local key="$1" def="$2" required=0 secret=0
  if [[ $key == COMPOSE_FILE ]]; then
    val="$(compose_value)"
    return 0
  fi
  if ((MINIMAL)) && is_obs "$key"; then
    val="$def"
    return 0
  fi
  [[ $key == BOT_IMAGE_TAG ]] && def=latest
  [[ $key == POSTGRES_PASSWORD ]] && def="$(gen_password)"
  [[ " ${REQUIRED[*]} " == *" $key "* ]] && required=1
  [[ $key =~ TOKEN|PASSWORD|COOKIE|SECRET|UUID ]] && secret=1
  while :; do
    if [[ $key == POSTGRES_PASSWORD ]]; then
      ask "$key (Enter — сгенерировать): " -s
      val="${REPLY:-$def}"
    elif ((secret)); then
      ask "$key: " -s
      val="$REPLY"
    elif ((required)); then
      ask "$key: "
      val="$REPLY"
    else
      ask "$key [$def]: "
      val="${REPLY:-$def}"
    fi
    if [[ $val == *"'"* ]]; then
      echo "  одинарная кавычка в значении не поддерживается." >&2
      continue
    fi
    if [[ $key == POSTGRES_PASSWORD && ! $val =~ ^[A-Za-z0-9]+$ ]]; then
      echo "  POSTGRES_PASSWORD: только латиница и цифры (значение идёт в URL)." >&2
      continue
    fi
    if [[ -z $val ]] && ((required)); then
      echo "  $key обязателен." >&2
      continue
    fi
    # значения XRAY_* попадают в JSON-конфиг через sed: пускаем только безопасные символы (как в deploy/xray/render.sh)
    if [[ $key == XRAY_PORT && ! $val =~ ^[0-9]+$ ]]; then
      echo "  $key: только число." >&2
      continue
    fi
    if [[ $key == XRAY_* && $key != XRAY_PORT && ! $val =~ ^[A-Za-z0-9._-]+$ ]]; then
      echo "  $key: допустимы латиница, цифры и . _ -" >&2
      continue
    fi
    break
  done
}

tmp="$(mktemp -d)"
envnew=""
trap 'rm -rf "$tmp"; [[ -z $envnew ]] || rm -f "$envnew"' EXIT

tgz="${BRUHANET_TARBALL:-}"
[[ -z $tgz ]] || tgz="$(realpath -- "$tgz")" || die "нет архива '$BRUHANET_TARBALL'"

dir="${BRUHANET_DIR:-}"
if [[ -z $dir ]]; then
  ask "Каталог установки [/opt/bruhanet]: "
  dir="${REPLY:-/opt/bruhanet}"
fi
[[ $dir == \~ || $dir == \~/* ]] && dir="$HOME${dir:1}"
mkdir -p "$dir" 2>/dev/null && [[ -w $dir ]] ||
  die "не могу создать/писать в '$dir' — запусти от root (sudo) или выбери другой каталог (BRUHANET_DIR=...)"
cd "$dir"
dir="$(pwd)"

if [[ -z $tgz ]]; then
  tgz="$tmp/src.tgz"
  echo "Скачиваю deploy/ (${BRUHANET_REF:-main})..."
  curl -fsSL -o "$tgz" "https://codeload.github.com/bruhabruh/bruhanet-bot/tar.gz/refs/heads/${BRUHANET_REF:-main}" ||
    die "не удалось скачать архив репозитория"
fi
# '*' не пересекает '/', поэтому совпадёт только <корень архива>/deploy
tar -xzf "$tgz" --strip-components=2 --wildcards --no-wildcards-match-slash '*/deploy' ||
  die "не удалось распаковать deploy/ из архива"
[[ -s .env.example ]] || die "в архиве нет deploy/.env.example"
mapfile -t example <.env.example

if [[ -f .env ]]; then
  echo ".env уже есть — существующие значения не трогаю (режим обновления конфигов)."
  if ((MINIMAL)) && grep -q "^COMPOSE_FILE=" .env; then
    echo "  COMPOSE_FILE уже задан в .env — не меняю. Для минимального стека поставь COMPOSE_FILE=docker-compose.yaml и перезапусти: docker compose up -d --remove-orphans."
  fi
  missing=()
  declare -A mdef=()
  for line in "${example[@]}"; do
    [[ $line =~ ^([A-Z_][A-Z0-9_]*)= ]] || continue
    k="${BASH_REMATCH[1]}"
    grep -q "^${k}=" .env && continue
    missing+=("$k")
    mdef[$k]="$(default_of "$line")"
  done
  if ((${#missing[@]} > 0)); then
    if ((!have_input)); then
      # без ввода: ключи с дефолтом берём как есть, остальные — ошибка со списком
      nodef=()
      for k in "${missing[@]}"; do
        [[ -n ${mdef[$k]} || $k == POSTGRES_PASSWORD ]] && continue
        ((MINIMAL)) && is_obs "$k" && continue
        nodef+=("$k")
      done
      ((${#nodef[@]} == 0)) || die "в .env нет ключей без значения по умолчанию: ${nodef[*]} — запусти с терминала или BRUHANET_INPUT=<файл>"
    fi
    echo "В .env не хватает ключей из .env.example: ${missing[*]}"
    cp -p .env .env.bak
    envnew="$dir/.env.new"
    cp -p .env "$envnew"
    for k in "${missing[@]}"; do
      if [[ $k == COMPOSE_FILE ]]; then
        val="$(compose_value)"
      elif ((have_input)); then
        prompt_key "$k" "${mdef[$k]}"
      elif [[ $k == POSTGRES_PASSWORD ]]; then
        val="$(gen_password)"
      else
        val="${mdef[$k]}"
      fi
      # дописываем с новой строки, даже если .env не оканчивается переводом строки
      [[ -z $(tail -c1 "$envnew") ]] || echo >>"$envnew"
      printf "%s='%s'\n" "$k" "$val" >>"$envnew"
    done
    mv "$envnew" .env
    chmod 600 .env
    envnew=""
    if ! err="$(docker compose --env-file .env config -q </dev/null 2>&1)"; then
      mv .env.bak .env
      die "получился невалидный .env, откатил: $err"
    fi
    rm -f .env.bak
  fi
else
  echo "Настраиваю .env (Enter — значение по умолчанию):"
  envnew="$dir/.env.new"
  (umask 077 && : >"$envnew")
  n=0
  for line in "${example[@]}"; do
    [[ -z ${line//[[:space:]]/} ]] && continue
    if [[ $line == '#'* ]]; then
      echo
      echo "${line#\# }"
      continue
    fi
    [[ $line == *=* ]] || continue
    key="${line%%=*}"
    prompt_key "$key" "$(default_of "$line")"
    printf "%s='%s'\n" "$key" "$val" >>"$envnew"
    n=$((n + 1))
  done
  ((n > 0)) || die "в .env.example нет ни одного ключа"
  mv "$envnew" .env
  envnew=""
  # env_file: .env в compose должен существовать, поэтому проверяем уже после mv
  if ! err="$(docker compose --env-file .env config -q </dev/null 2>&1)"; then
    rm -f .env
    die "получился невалидный .env, ничего не сохранено: $err"
  fi
fi

if [[ ${BRUHANET_SKIP_UP:-} == 1 ]]; then
  echo "BRUHANET_SKIP_UP=1 — pull/up пропущены."
  exit 0
fi

dc=(docker compose --env-file .env)
"${dc[@]}" pull </dev/null
"${dc[@]}" up -d --remove-orphans </dev/null
"${dc[@]}" ps </dev/null

port="$(sed -n 's/^GRAFANA_PORT=["'"'"']\{0,1\}\([^"'"'"']*\).*/\1/p' .env)"
cat <<MSG

Готово. Grafana: http://127.0.0.1:${port:-3030}
Обновить:  curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | BRUHANET_DIR=$dir bash
Логи:      cd $dir && docker compose --env-file .env logs -f bot
MSG
