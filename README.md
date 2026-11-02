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

## Postgres, бэкап и восстановление

Платежи и очередь уведомлений (outbox) лежат в Postgres (сервис `postgres`, том `postgres-data`). Пароль — `POSTGRES_PASSWORD` в `.env` (только латиница и цифры: значение идёт в URL; `install.sh` сгенерирует сам). Схему накатывает one-shot сервис `migrate` (`bot migrate`), бот стартует только после него. `install.sh` на существующем `.env` допишет недостающие ключи из `.env.example`, остальное не тронет.

Бэкап — сервис `backup` (`deploy/backup/backup.sh`): каждый день в 04:00 МСК делает `pg_dump -Fc` и шлёт файл админу (`TELEGRAM_ADMIN_ID`) в Telegram, затем пишет строку в таблицу `backups`. Лимит Telegram — 50 МБ; при ошибке на любом шаге админу придёт «⚠️ Бэкап не сделан: <шаг>: …». Ручной прогон: `docker compose exec backup /backup.sh --once`. Если бэкапа нет больше 26 часов, сработает алерт `backup-stale` (метрика `bot_backup_last_success_timestamp_seconds`); `outbox-stuck` — очередь уведомлений не пустеет 15 минут.

Восстановление в чистый стек (скачай `bruhanet-YYYY-MM-DD.dump` из чата с ботом в `/opt/bruhanet/bruhanet.dump`):

```bash
cd /opt/bruhanet
docker compose -f docker-compose.yaml --env-file .env up -d postgres
docker compose -f docker-compose.yaml --env-file .env exec -T postgres \
  pg_restore -U bruhanet -d bruhanet --clean --if-exists < bruhanet.dump
docker compose -f docker-compose.yaml --env-file .env up -d
```
