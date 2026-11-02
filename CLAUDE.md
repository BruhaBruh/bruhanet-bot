# BruhaNet bot

Telegram-бот продажи VPN-подписок (панель Remnawave). Оплата — ручной перевод на карту, подтверждает админ.
Переписка старого бота (`tmp/*.go`, пакет `tgbot`, в git не попадает — `.gitignore: tmp/`). Старый код — эталон поведения и текстов, не архитектуры.

## Флоу оплаты

1. `/start` или любое сообщение → приветствие + главная клавиатура (Информация / Оплатить / Подписка (copy URL) / Поддержка; без юзера в панели — только «Купить»).
2. «Оплатить»/«Продлить» → клавиатура тарифов из `PAYMENT_PRICES` (месяцы → ₽, по 2 в ряд).
3. Выбор тарифа → «Верно?» Да/Нет.
4. «Да» → `CreateUser` в Remnawave (если нет; `expireAt=now`). Если у юзера уже есть `pending`-платёж — он становится `cancelled`, юзеру откатывается срок (`PrevExpireAt`), старое админ-сообщение закрывается. Новый платёж — строка `payments` со статусом `pending` (`telegram_id, username, months, price, prev_expire_at` = текущий срок юзера). Показать реквизиты (`PAYMENT_PHONE`, `PAYMENT_BANK`, комментарии «На кофе»/…).
5. Юзер шлёт фото при наличии pending → фото копируется админу + сообщение с кнопками Подтвердить/Отклонить (в callback `adm_ok`/`adm_no` — **id платежа**).
6. Аванс: `max(now, PrevExpireAt) + 24h`, платёж помечается `advance_granted`. Юзеру — «фото на проверке, доступ на 1 день» (при исчерпанном лимите — без строки про доступ).
7. Админ: **Подтвердить** → `max(PrevExpireAt, now) + Month*30d`; **Отклонить** → вернуть `PrevExpireAt`. Оба: статус `confirmed`/`declined`, убрать клавиатуру, реакция 👍/👎, уведомить юзера.

Инварианты: платёж живёт в Postgres (`payments`, не более одного `pending` на юзера — частичный unique index; статусы ровно `pending|confirmed|declined|cancelled`). Решение админа — `Store.ResolvePayment` под блокировкой строки: в той же транзакции вызывается `UpdateUser` (ошибка панели откатывает транзакцию, повтор считает ту же дату) и кладётся `payment_result` в outbox; повторное нажатие → «Уже обработано», нет платежа → «Платёж устарел». FSM (Redis) — лишь подсказка роутинга фото (`await_photo → in_review → none`). Базой для продления всегда служит `PrevExpireAt`, а не текущий срок в панели (там уже +1 день). Месяц = 30 дней. Время показывать в `Europe/Moscow`.

Правило аванса: неоплаченный аванс = платёж `declined|cancelled` с `advance_granted` после последнего `confirmed`. Таких ≥ `advanceLimit` (3) → аванс не выдаётся; сбрасывается подтверждённой оплатой. Админу в сообщении проверки показывается статистика юзера (`review_stats.go`).

Реализация — `bot/internal/handler/routes/`: `tariffs.go` (pay/price/buy), `photo.go`, `review.go` (adm_ok/adm_no), `review_stats.go`.

Outbox: все уведомления, не являющиеся ответом на действие юзера (результат проверки, напоминания, рассылка), кладутся в таблицу `outbox(id, event_type, payload)` (payload — jsonb; `payment_result`, `reminder`, `broadcast`, `broadcast_summary`) и доставляются воркером `internal/outbox`; обработанная строка удаляется. `payment_result` пишется в транзакции `ResolvePayment`. Напоминания (cron из `NOTIFICATION_CRON`, Europe/Moscow): «скоро закончится» (≤7д) и «в течение суток» (≤24ч), по одному разу на дату окончания; дедуп-ключ `notify:<id>:<expireAt unix>:<week|day>` остаётся в Redis. Код — `internal/reminder` (отбор/дедуп, кладёт в outbox), текст — `routes.ReminderMessage`.

## Структура

Go workspace (`go.work`), модуль-префикс `github.com/bruhabruh/bruhanet`.

- `pkg/` — общее: `app` (CompositeApp + `Run` с graceful shutdown), `lifecycle` (Start/Stop), `cfg` (env → struct + validator), `stlog` (slog + логгер в context), `remnawave` (обёртка `remnawave-api-go/v3`), `tgupdate` (user/chat/type апдейта), `fsm` (стейт + data юзера, `in_memory`/`redis`, CAS — единственное хранилище), `keyboard` (разметка inline-клавиатур, стили-константы), `callback` (имя роута + целый аргумент в callback data, без хранилища).
- `bot/` — `cmd/main.go` → `internal/app` (wiring), `internal/config` (по файлу на секцию), `internal/handler` (`New` — сборка бота; `handler/router` — роутер; `handler/routes` — все роуты с их хендлерами и клавиатурами, файл на роут; `handler/middleware` — цепочка middleware), `internal/model`, `internal/store` (Postgres: `pgxpool`, goose-миграции через `embed`, платежи/outbox/пользователи; `Migrate`, `CheckSchema`), `internal/outbox` (воркер доставки), `internal/reminder`, `internal/observability` (метрики `bot_*`, `/metrics` `/healthz` `/readyz` на `OBSERVABILITY_ADDR`, readiness, трейсинг OTLP → Tempo, подкоманда `bot healthcheck`), `internal/tgtest` (фейковый Telegram API для тестов).
- `deploy/` — прод-стек (`docker-compose.yaml`: bot, redis, loki, alloy, prometheus, tempo, grafana; конфиги `loki/`, `alloy/`, `prometheus/` (скрейп `bot:8081`, retention 30d), `tempo/` (OTLP HTTP 4318, retention 720h), `grafana/` (datasources loki/prometheus/tempo, лог → трейс по `trace_id`); `postgres` (том `postgres-data`) и one-shot `migrate`; `.env` не в git, шаблон `.env.example`; `deploy/backup` — суточный бэкап (этап 1, отдельная задача)).

## Деплой этапа 1 (Postgres)

Деплой этой ветки идёт без живых проверок: до мержа на VPS добавить `POSTGRES_PASSWORD` в `.env` (или прогнать `install.sh` — допишет недостающие ключи); CI-шаг `Check .env keys` упадёт, если ключа нет. Платежи, оставшиеся «на проверке» в Redis FSM, в Postgres не переезжают — бот при старте пишет warn `legacy fsm payment found` с `telegram_id`, разобрать вручную до мержа.

## Команды

`task` — список. Основное: `task setup`, `task up` (Redis в docker), `task dev SVC=bot` (air + jq), `task run SVC=bot`, `task test`, `task test:db` (SQL-тесты `store` на dev-Postgres, нужен `task up`), `task migrate` (`bot migrate`; `task run`/`task dev` делают это первым шагом), `task lint`, `task fmt`, `task tidy`.
Образ бота: `task docker:build` (TAG=local). Прод-стек: `task deploy:config|up|down|pull|ps`, `task deploy:logs -- <service>` (нужен `deploy/.env`, `BOT_IMAGE_TAG=local task deploy:up` для локального образа). Docker healthcheck бота — `/app/bot healthcheck` (liveness `/healthz`, readiness не влияет).
Установка/обновление на VPS: `curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | bash` (`install.sh` в корне: качает `deploy/`, спрашивает `.env` по `.env.example`, `pull` + `up -d`; существующий `.env` не трогает; env: `BRUHANET_DIR|REF|TARBALL|INPUT|SKIP_UP`).
CI/CD — `.github/workflows/ci.yaml`: lint + test (pkg, bot) + build образа на PR и push в `main`; на push в `main` — push в ghcr (`latest`, `sha-<short>`) и деплой по ssh (секреты environment `production`: `SSH_PRIVATE_KEY`, `SSH_KNOWN_HOSTS`, `SSH_HOST`, `SSH_USER`, `WORK_DIR` — абсолютный путь): копирует `deploy/` без `.env`, `pull` + `up -d` с `BOT_IMAGE_TAG=sha-<short>`, ждёт `State.Health.Status == healthy` до 90 с, иначе логи бота и fail, `docker image prune -f` (не `-a` — на VPS ещё Remnawave). Grafana по умолчанию на `127.0.0.1:3030` (3000 занят панелью Remnawave).
После CI-деплоя изменения в bind-mount конфигах `prometheus/`, `tempo/`, `loki/`, `alloy/` подхватываются только через `docker compose restart <service>`.
Новый модуль добавлять в `SERVICES` в `Taskfile.yaml` и в matrix в `ci.yaml`.
Redis-реализация FSM тестируется на miniredis, SQL-тесты `store` пропускаются без `TEST_DATABASE_URL` — docker для `task test` не нужен. Хендлеры тестируются через `tgtest` (`bot.WithSkipGetMe` + `WithServerURL` + `WithNotAsyncHandlers`, апдейты через `ProcessUpdate`).

## Конвенции и подводные камни

- Go **1.27**: есть generic-методы, stdlib `uuid`, `errors.AsType`. Не «чинить» их как ошибки. Но generic-метод не реализует метод интерфейса — поэтому `pkg/remnawave` с `int64`.
- UUID — везде `github.com/google/uuid`, не stdlib: его требуют типы `remnawave-api-go`.
- Ошибки: sentinel в `errors.go` пакета, оборачивать через `errors.Join(ErrX, err)`; проверять `errors.Is`.
- Конфиг: только env-теги `caarlos0/env` + `validate`. Новый env — сразу и в `.env.example`.
- Логи: `slog`, контекстный логгер через `stlog.With/L(ctx)`; поля snake_case.
- Линтер строгий (`.golangci.yml`: gosec, errorlint, exhaustive, gocritic…), формат gofumpt + goimports с local-prefix `github.com/bruhabruh/bruhanet`.
- Тексты юзеру на русском, MarkdownV2 (`models.ParseModeMarkdown`) — всё динамическое через `bot.EscapeMarkdown`. Плюрализация «месяц/месяца/месяцев».
- Имена файлов — нижний регистр через `_` (`in_memory.go`).
- `MatchFunc` go-telegram выполняется синхронно до всех middleware: только проверки полей, без I/O и паник.
- Роуты регистрируются только в `handler/routes/routes.go` (`Register`), хендлер роута — в своём файле `handler/routes/<route>.go`. Статичная кнопка — `keyboard.Callback(text, "pay")`; с аргументом — `var Price = callback.New[int]("price")`, кнопка `keyboard.Callback(text, Price.Data(6))`, в хендлере `Price.Parse(u.CallbackQuery.Data)`. Callback data подделывается клиентом — аргумент валидировать (тариф есть в `PAYMENT_PRICES`), админские роуты — с `middleware.AdminOnly`.
- Внешние вызовы — только через инструментированные клиенты (Remnawave-декоратор `InstrumentRemnawave`, `TelegramTransport`, Redis-хук); пробы зависимостей — через `observability.WithProbe` или необорачиваемый клиент. telegram_id/request_id/error в метки метрик не класть.
- SQL — только в `internal/store`. Миграции — отдельный one-shot `bot migrate` (в проде сервис `migrate`, бот стартует после него); `app.New` лишь делает `CheckSchema` (есть непримененные → `ErrSchemaOutdated`, бот не стартует). Уведомление, не являющееся ответом на действие (результат проверки, напоминание, рассылка), — только через outbox.
- Стили кнопок — только `keyboard.Style*` константы.
- На callback отвечать через `middleware.Answer`, или повесить на роут `middleware.AutoAnswer("текст")` (toast после хендлера); иначе глобальный `AutoAnswer()` ответит пустым.
- Служебное состояние апдейта (route, answered, panicked) — `middleware.UpdateInfoFrom(ctx)`; это не логирование.
- Дизайн каркаса хендлеров: `docs/superpowers/specs/2026-10-26-bot-handler-infra-design.md`.
- Callback data в старом боте — короткие коды `"0".."5"` + `:<arg>` (лимит Telegram 64 байта).
- Грабли старого кода, не повторять: обращение к `payment.*` до проверки `err` (nil-паника), повторное нажатие Подтвердить/Отклонить, игнор ошибок `UpdateUser` при выдаче 1 дня, admin-хендлеры без проверки `AdminID`.
- Экраны по callback — `show` (редактирование на месте, fallback — новое сообщение); ответы на фото/текст — `send`.
- Remnawave в хендлерах — только через `routes.Remnawave`; в тестах `tgtest.NewRemnawave`. Время — `h.now()` (поле `handlers.now`).
- Message-роуты роутера матчатся в порядке регистрации в `routes.Register` (= приоритет): `Message("broadcast", InState(await_broadcast))` и `Command("a")` — до `Photo("payment")`. Рассылка админа: `/a` → сообщение → превью + «Отправить» → в outbox N строк `broadcast` (по уникальному telegram ID из панели) + `broadcast_summary` админу; темп отправки задаёт воркер.
