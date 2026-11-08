#!/bin/sh
# Одноразовый шаг перед xray: подставляет XRAY_* из .env в шаблон и кладёт готовый config.json в общий том.
# Значения идут в JSON через sed, поэтому допускаем только безопасные символы; пустое или странное — ошибка с именем ключа.
set -eu

fail() { echo "xray-config: $1" >&2; exit 1; }

check() { # check <KEY> <значение> <допустимые символы: word|num>
  [ -n "$2" ] || fail "XRAY_$1 не задан в .env"
  case "$3" in
    num) case "$2" in *[!0-9]*) fail "XRAY_$1 должен быть числом" ;; esac ;;
    *) case "$2" in *[!A-Za-z0-9._-]*) fail "XRAY_$1 содержит недопустимые символы (можно A-Z a-z 0-9 . _ -)" ;; esac ;;
  esac
}

check DOMAIN "${XRAY_DOMAIN:-}" word
check PORT "${XRAY_PORT:-}" num
check UUID "${XRAY_UUID:-}" word
check PBK "${XRAY_PBK:-}" word
check SID "${XRAY_SID:-}" word
check FINGERPRINT "${XRAY_FINGERPRINT:-}" word

sed -e "s|__XRAY_DOMAIN__|$XRAY_DOMAIN|g" \
    -e "s|__XRAY_PORT__|$XRAY_PORT|g" \
    -e "s|__XRAY_UUID__|$XRAY_UUID|g" \
    -e "s|__XRAY_PBK__|$XRAY_PBK|g" \
    -e "s|__XRAY_SID__|$XRAY_SID|g" \
    -e "s|__XRAY_FINGERPRINT__|$XRAY_FINGERPRINT|g" \
    /xray/config.json.tmpl >/out/config.json
chmod 644 /out/config.json
echo "xray-config: config.json записан (сервер $XRAY_DOMAIN:$XRAY_PORT)"
