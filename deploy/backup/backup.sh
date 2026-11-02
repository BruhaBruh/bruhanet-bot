#!/bin/sh
# Суточный pg_dump -Fc -> Telegram админу + строка в backups (по ней бот считает метрику и алерт).
# Env: PGHOST PGUSER PGPASSWORD PGDATABASE, TELEGRAM_TOKEN, TELEGRAM_ADMIN_ID, TZ, BACKUP_AT=HH:MM.
# `--once` — один прогон сразу (для ручной проверки), код выхода 1 при ошибке.
# TELEGRAM_API_URL — только для тестов (заглушка вместо api.telegram.org).
set -eu

BACKUP_AT="${BACKUP_AT:-04:00}"
API="${TELEGRAM_API_URL:-https://api.telegram.org}/bot${TELEGRAM_TOKEN}"
LIMIT=$((50 * 1024 * 1024))
ERR=/tmp/backup.err
STEPF=/tmp/backup.step

log() { echo "backup: $*" >&2; }

# вырезаем токен из текста: curl может печатать URL
scrub() { sed "s|${TELEGRAM_TOKEN}|***|g"; }

alert() {
	log "ошибка: $1"
	curl -sS --max-time 30 -o /dev/null --data-urlencode "chat_id=${TELEGRAM_ADMIN_ID}" \
		--data-urlencode "text=⚠️ Бэкап не сделан: $1" "${API}/sendMessage" 2>&1 | scrub >&2 || :
}

step() { echo "$1" >"$STEPF"; }

# 08 в $(( )) — невалидная восьмерица; срезаем ведущие нули
num() { echo $(($(echo "$1" | sed 's/^0*\(.\)/\1/'))); }

# set -e внутри `if (...)` не работает — везде явный || return 1
do_backup() {
	file="/tmp/bruhanet-$(date +%F).dump"
	trap 'rm -f "$file"' EXIT INT TERM
	step pg_dump
	pg_dump -Fc -f "$file" 2>"$ERR" || return 1
	size=$(wc -c <"$file")
	step размер
	if [ "$size" -gt "$LIMIT" ]; then
		echo "дамп $((size / 1048576)) МБ больше лимита Telegram 50 МБ" >"$ERR"
		return 1
	fi
	mb=$(awk "BEGIN{printf \"%.1f\", $size/1048576}")
	step sendDocument
	curl -fsS --max-time 300 -F "chat_id=${TELEGRAM_ADMIN_ID}" -F "document=@${file}" \
		-F "caption=Бэкап BruhaNet $(date +%d.%m.%Y), ${mb} МБ" "${API}/sendDocument" >/dev/null 2>"$ERR" || return 1
	step insert
	psql -qX -v ON_ERROR_STOP=1 -c "INSERT INTO backups(size_bytes) VALUES ($size)" >/dev/null 2>"$ERR" || return 1
	log "готово: ${mb} МБ"
}

# один прогон в подоболочке: trap удаляет файл, set -e не убивает цикл
run() {
	if (do_backup); then return 0; fi
	alert "$(cat "$STEPF"): $(tail -c 300 "$ERR" 2>/dev/null | scrub)"
	return 1
}

secs_to_next() {
	now=$(date +%H:%M:%S)
	cur=$(($(num "${now%%:*}") * 3600 + $(num "$(echo "$now" | cut -d: -f2)") * 60 + $(num "${now##*:}")))
	tgt=$(($(num "${BACKUP_AT%%:*}") * 3600 + $(num "${BACKUP_AT##*:}") * 60))
	d=$((tgt - cur))
	[ "$d" -le 0 ] && d=$((d + 86400))
	echo "$d"
}

if [ "${1:-}" = "--once" ]; then
	run
	exit $?
fi

while :; do
	s=$(secs_to_next)
	log "следующий бэкап через ${s} с (в ${BACKUP_AT} ${TZ:-UTC})"
	sleep "$s"
	run || :
done
