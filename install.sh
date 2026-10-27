#!/usr/bin/env bash
# Установка/обновление прод-стека BruhaNet:
#   curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | bash
# Env: BRUHANET_DIR (каталог), BRUHANET_REF (ветка/тег), BRUHANET_TARBALL (локальный архив, для тестов),
#      BRUHANET_INPUT (откуда читать ответы, по умолчанию /dev/tty), BRUHANET_SKIP_UP=1 (без pull/up).
set -euo pipefail

die() { echo "Ошибка: $*" >&2; exit 1; }

REQUIRED=(TELEGRAM_TOKEN TELEGRAM_ADMIN_ID REMNAWAVE_PANEL_URL REMNAWAVE_API_TOKEN REMNAWAVE_INTERNAL_SQUAD_ID
  GRAFANA_ADMIN_PASSWORD ALERT_TELEGRAM_TOKEN ALERT_TELEGRAM_CHAT_ID)

for c in curl tar docker; do
  command -v "$c" >/dev/null || die "не найден '$c' — установи его и запусти снова"
done
docker compose version </dev/null >/dev/null 2>&1 || die "нужен docker compose v2 (плагин 'docker compose')"

# stdin занят скриптом при `curl | bash`, поэтому ответы — из отдельного дескриптора
exec 3<"${BRUHANET_INPUT:-/dev/tty}" || die "нет терминала для ввода; задай BRUHANET_INPUT=<файл>"

ask() { # ask <prompt> [-s] -> ответ в $REPLY
  local silent=()
  [[ ${2:-} == -s ]] && silent=(-s)
  read -r "${silent[@]}" -u 3 -p "$1" REPLY || die "ввод закончился (EOF)"
  [[ ${2:-} == -s ]] && echo >&2
  return 0
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
  echo ".env уже есть — не трогаю (режим обновления конфигов)."
  for line in "${example[@]}"; do
    [[ $line =~ ^([A-Z][A-Z0-9_]*)= ]] || continue
    grep -q "^${BASH_REMATCH[1]}=" .env || echo "  в .env нет ${BASH_REMATCH[1]} (см. .env.example)"
  done
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
    def="${line#*=}"
    def="${def#[\"\']}"
    def="${def%[\"\']}"
    [[ $key == BOT_IMAGE_TAG ]] && def=latest

    required=0
    [[ " ${REQUIRED[*]} " == *" $key "* ]] && required=1
    secret=0
    [[ $key =~ TOKEN|PASSWORD|COOKIE|SECRET ]] && secret=1

    while :; do
      if ((secret)); then
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
      if [[ -z $val ]] && ((required)); then
        echo "  $key обязателен." >&2
        continue
      fi
      break
    done
    printf "%s='%s'\n" "$key" "$val" >>"$envnew"
    n=$((n + 1))
  done
  ((n > 0)) || die "в .env.example нет ни одного ключа"
  mv "$envnew" .env
  envnew=""
  # env_file: .env в compose должен существовать, поэтому проверяем уже после mv
  if ! err="$(docker compose -f docker-compose.yaml --env-file .env config -q </dev/null 2>&1)"; then
    rm -f .env
    die "получился невалидный .env, ничего не сохранено: $err"
  fi
fi

if [[ ${BRUHANET_SKIP_UP:-} == 1 ]]; then
  echo "BRUHANET_SKIP_UP=1 — pull/up пропущены."
  exit 0
fi

dc=(docker compose -f docker-compose.yaml --env-file .env)
"${dc[@]}" pull </dev/null
"${dc[@]}" up -d </dev/null
"${dc[@]}" ps </dev/null

port="$(sed -n 's/^GRAFANA_PORT=["'"'"']\{0,1\}\([^"'"'"']*\).*/\1/p' .env)"
cat <<MSG

Готово. Grafana: http://127.0.0.1:${port:-3000}
Обновить:  curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | BRUHANET_DIR=$dir bash
Логи:      cd $dir && docker compose -f docker-compose.yaml --env-file .env logs -f bot
MSG
