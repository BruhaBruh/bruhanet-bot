# BruhaNet bot

## Установка / обновление

```bash
curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | bash
```

Сразу указать каталог (по умолчанию `/opt/bruhanet`):

```bash
curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | BRUHANET_DIR=/opt/bruhanet bash
```

С другой ветки:

```bash
curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | BRUHANET_REF=<branch> bash
```

Только конфиги и `.env`, без `pull` и `up`:

```bash
curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | BRUHANET_SKIP_UP=1 bash
```

Повторный запуск обновляет конфиги и перезапускает стек, существующий `.env` не трогает.

## Админка

Вход: команда `/login` в боте (только админ) присылает кнопку со ссылкой; сессия живёт сутки. Сервис `api` слушает `127.0.0.1:8090`, наружу его отдаёт nginx. Сертификат — выпускается так же, как для панели.

```nginx
server {
    listen 443 ssl;
    server_name admin.bruhanet.ru;

    # ssl_certificate / ssl_certificate_key — как у панели

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

В `.env` должен быть `ADMIN_BASE_URL=https://admin.bruhanet.ru` (`install.sh` допишет недостающие ключи).

Одноразовый токен из `/auth?token=…` попадает в access-лог nginx. Это безвредно (токен живёт 5 минут и используется один раз), но при желании можно отключить лог для этого пути: добавить `location = /auth { access_log off; proxy_pass http://127.0.0.1:8090; proxy_set_header Host $host; proxy_set_header X-Forwarded-Proto $scheme; }`.

## Postgres, бэкап и восстановление

Платежи и очередь уведомлений (outbox) лежат в Postgres (сервис `postgres`, том `postgres-data`). Пароль — `POSTGRES_PASSWORD` в `.env` (только латиница и цифры: значение идёт в URL; `install.sh` сгенерирует сам). Схему накатывает one-shot сервис `migrate` (`bot migrate`), бот стартует только после него. `install.sh` на существующем `.env` допишет недостающие ключи из `.env.example`, остальное не тронет.

Бэкап — сервис `backup` (`deploy/backup/backup.sh`): каждый день в 04:00 МСК делает `pg_dump -Fc` и шлёт файл админу (`TELEGRAM_ADMIN_ID`) в Telegram, затем пишет строку в таблицу `backups`. Лимит Telegram — 50 МБ; при ошибке на любом шаге админу придёт «⚠️ Бэкап не сделан: <шаг>: …». Ручной прогон: `docker compose exec backup /backup.sh --once`. Если бэкапа нет больше 26 часов, сработает алерт `backup-stale` (метрика `bot_backup_last_success_timestamp_seconds`); он же горит на свежем деплое до первого успешного бэкапа и при недоступной БД — сразу после деплоя запусти `docker compose exec backup /backup.sh --once`; `outbox-stuck` — очередь уведомлений не пустеет 15 минут.

Восстановление в чистый стек (скачай `bruhanet-YYYY-MM-DD.dump` из чата с ботом в `/opt/bruhanet/bruhanet.dump`):

```bash
cd /opt/bruhanet
docker compose -f docker-compose.yaml --env-file .env up -d postgres
docker compose -f docker-compose.yaml --env-file .env exec -T postgres \
  pg_restore -U bruhanet -d bruhanet --clean --if-exists < bruhanet.dump
docker compose -f docker-compose.yaml --env-file .env up -d
```
