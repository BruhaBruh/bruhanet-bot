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

Откат — только перезапуском старого деплоя в CI (он вернёт старый `deploy/`), не через `BOT_IMAGE_TAG=… up`: старый образ с ENTRYPOINT сломает команды api/migrate.

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

Тексты бота и настройки (тарифы, реквизиты, комментарии к переводу, ссылка поддержки) правятся в админке, разделы «Тексты» и «Настройки»; бот подхватывает изменения в течение ~10 секунд, перезапуск не нужен. Значения из `.env` (`PAYMENT_*`, `SUPPORT_URL`) — только дефолты, пока настройки не сохранены: после первого сохранения все настройки живут в БД, сбросить их нельзя, правки `.env` больше не применяются. «Сбросить к стандартному» возвращает к исходному только текст. «Отправить себе» присылает текст админу в Telegram. Форматирование — подмножество Telegram-HTML.

Промокоды — раздел «Промокоды»: скидка в процентах или рублях (вводится при оплате, цена 0 ₽ подтверждается без админа), бонусные дни (вводятся из кнопки «Промокод» в меню). Можно ограничить числом активаций, датой «действует до» (включительно, МСК), «только новым» (ни одной оплаты) и конкретным пользователем. Код активируется при подтверждении оплаты, не при вводе. Удалить код нельзя — только выключить. Пустой код при создании сгенерируется сам.

В `.env` должен быть `ADMIN_BASE_URL=https://admin.bruhanet.ru` (`install.sh` допишет недостающие ключи).

Одноразовый токен из `/auth?token=…` попадает в access-лог nginx. Это безвредно (токен живёт 5 минут и используется один раз), но при желании можно отключить лог для этого пути: добавить `location = /auth { access_log off; proxy_pass http://127.0.0.1:8090; proxy_set_header Host $host; proxy_set_header X-Forwarded-Proto $scheme; }`.

## Mini App «Кабинет»

Подписка, подключение, устройства, друзья и промокоды внутри Telegram. Тот же сервис `api` слушает второй порт `127.0.0.1:8092`, но только когда в `.env` задан `MINIAPP_ORIGIN=https://app.bruhanet.ru` (пусто — выключено). Адрес `MINIAPP_ORIGIN` и настройка «Ссылка на MiniApp» в админке должны совпадать. Вход — по подписи `initData` (токен бота), без cookie и сессий.

```nginx
limit_req_zone $binary_remote_addr zone=miniapp:10m rate=10r/s;

server {
    listen 443 ssl;
    server_name app.bruhanet.ru;

    # ssl_certificate / ssl_certificate_key — как у панели

    location / {
        limit_req zone=miniapp burst=30 nodelay;
        proxy_pass http://127.0.0.1:8092;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

В BotFather можно задать Menu Button на этот адрес. Кнопки внутри бота открываются как `web_app`: Mini App не открывается по обычной ссылке-кнопке (без неё нет `initData`). Локально Telegram принимает только https, поэтому для проверки в Telegram нужен туннель; вёрстку можно смотреть в `pnpm -C web/miniapp dev` на моках.

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

Рассылки и личные сообщения: раздел «Рассылки» (текст, опционально фото, «Отправить себе» для проверки, подтверждение, история со счётчиками доставки) и кнопка «Написать» в карточке пользователя. Команды `/a` в боте больше нет. Фото заливается через бота, для этого `api` использует тот же `TELEGRAM_TOKEN`; подпись к фото — до 1024 символов, без фото — до 4096.
