# BruhaNet bot

Telegram-бот продажи VPN-подписок (панель Remnawave). Оплата — ручной перевод на карту, подтверждает админ.
Переписка старого бота (`tmp/*.go`, пакет `tgbot`, в git не попадает — `.gitignore: tmp/`). Старый код — эталон поведения и текстов, не архитектуры.

## Флоу оплаты

1. `/start` или любое сообщение → приветствие + главная клавиатура (Информация / Оплатить / Подписка (copy URL) / Поддержка; без юзера в панели — только «Купить»).
2. «Оплатить»/«Продлить» → клавиатура тарифов из `PAYMENT_PRICES` (месяцы → ₽, по 2 в ряд).
3. Выбор тарифа → «Верно?» Да/Нет.
4. «Да» → `CreateUser` в Remnawave (если нет; `expireAt=now`). Если уже есть pending-платёж — откатить юзеру `SubscriptionExpireAt` из него и удалить. Сохранить новый pending (`TelegramID, Username, Month, Price, SubscriptionExpireAt` = текущий срок юзера). Показать реквизиты (`PAYMENT_PHONE`, `PAYMENT_BANK`, комментарии «На кофе»/…).
5. Юзер шлёт фото при наличии pending → фото копируется админу + сообщение с кнопками Подтвердить/Отклонить (в callback — telegram ID юзера).
6. Сразу выдаётся 1 день: `max(now, SubscriptionExpireAt) + 24h`. Юзеру — «фото на проверке, доступ на 1 день».
7. Админ: **Подтвердить** → `SubscriptionExpireAt + Month*30d`; **Отклонить** → вернуть `SubscriptionExpireAt`. Оба: удалить pending, убрать клавиатуру, реакция 👍/👎, уведомить юзера.

Инварианты: pending — один на юзера, ключ `payment:<telegramID>` в Redis без TTL. Базой для продления всегда служит сохранённый `SubscriptionExpireAt`, а не текущий срок в панели (там уже +1 день). Месяц = 30 дней. Время показывать в `Europe/Moscow`.

Отдельный сервис `notification`: по `NOTIFICATION_CRON` напоминать юзерам о скором окончании подписки (в старом коде `Notify` с клавиатурой Продлить/Подписка).

## Структура

Go workspace (`go.work`), модуль-префикс `github.com/bruhabruh/bruhanet`.

- `pkg/` — общее: `app` (CompositeApp + `Run` с graceful shutdown), `lifecycle` (Start/Stop), `cfg` (env → struct + validator), `stlog` (slog + логгер в context), `remnawave` (обёртка `remnawave-api-go/v3`: Get/Create/UpdateUser, итератор `Users`).
- `bot/` — `cmd/main.go` → `internal/app` (сборка компонентов), `internal/config` (по файлу на секцию), `internal/handler` (Telegram, `go-telegram/bot`), `internal/model`, `internal/storage/payment` (Redis).
- `notification/` — заготовка, пустой `main`.

## Команды

`task` — список. Основное: `task setup`, `task up` (Redis в docker), `task dev SVC=bot` (air + jq), `task run SVC=bot`, `task test`, `task lint`, `task fmt`, `task tidy`.
Новый модуль добавлять в `SERVICES` в `Taskfile.yaml`.
Тесты `storage/payment` — интеграционные, нужен поднятый Redis и env (`REDIS_ADDR`, …).

## Конвенции и подводные камни

- Go **1.27**: есть generic-методы (`func (rw *remnawave) GetUser[T int|int64]`), stdlib `uuid`, `errors.AsType`. Не «чинить» их как ошибки.
- UUID — везде `github.com/google/uuid`, не stdlib: его требуют типы `remnawave-api-go`.
- Ошибки: sentinel в `errors.go` пакета, оборачивать через `errors.Join(ErrX, err)`; проверять `errors.Is`.
- Конфиг: только env-теги `caarlos0/env` + `validate`. Новый env — сразу и в `.env.example`.
- Логи: `slog`, контекстный логгер через `stlog.With/L(ctx)`; поля snake_case.
- Линтер строгий (`.golangci.yml`: gosec, errorlint, exhaustive, gocritic…), формат gofumpt + goimports с local-prefix `github.com/bruhabruh/bruhanet`.
- Тексты юзеру на русском, MarkdownV2 (`models.ParseModeMarkdown`) — всё динамическое через `bot.EscapeMarkdown`. Плюрализация «месяц/месяца/месяцев».
- Имена файлов — нижний регистр через `_` (`in_memory.go`).
- `MatchFunc` go-telegram выполняется синхронно до всех middleware: только проверки полей, без I/O и паник.
- Дизайн каркаса хендлеров: `docs/superpowers/specs/2026-10-26-bot-handler-infra-design.md`.
- Callback data в старом боте — короткие коды `"0".."5"` + `:<arg>` (лимит Telegram 64 байта).
- Грабли старого кода, не повторять: обращение к `payment.*` до проверки `err` (nil-паника), повторное нажатие Подтвердить/Отклонить, игнор ошибок `UpdateUser` при выдаче 1 дня, admin-хендлеры без проверки `AdminID`.
