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

Бэкап — раз в сутки файлом админу в Telegram (сервис бэкапа добавляется отдельным коммитом этапа 1).

Восстановление в чистый стек (формат — дамп `pg_dump -Fc`, уточнить по сервису бэкапа):

```bash
cd /opt/bruhanet
docker compose -f docker-compose.yaml --env-file .env up -d postgres
docker compose -f docker-compose.yaml --env-file .env exec -T postgres \
  pg_restore -U bruhanet -d bruhanet --clean --if-exists < bruhanet.dump
docker compose -f docker-compose.yaml --env-file .env up -d
```
