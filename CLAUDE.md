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
7. Админ: **Подтвердить** → `max(SubscriptionExpireAt, now) + Month*30d`; **Отклонить** → вернуть `SubscriptionExpireAt`. Оба: удалить pending, убрать клавиатуру, реакция 👍/👎, уведомить юзера.

Инварианты: pending-платёж — `model.Payment` в FSM data юзера (`fsm:<telegramID>`, без TTL), смены стейта при подтверждении — через `fsm.Transition` (защита от двойного нажатия). Базой для продления всегда служит сохранённый `SubscriptionExpireAt`, а не текущий срок в панели (там уже +1 день). Месяц = 30 дней. Время показывать в `Europe/Moscow`.

Реализация — `bot/internal/handler/routes/`: `tariffs.go` (pay/price/buy), `photo.go`, `review.go` (adm_ok/adm_no), стейты `await_photo → in_review → none`. Админ-кнопка сверяется с `Payment.AdminMessageID` (устаревшая → «Платёж устарел»); в review сначала `fsm.Transition`, потом `UpdateUser`, при ошибке панели стейт возвращается для повтора. Повторная оплата во время проверки откатывает срок и закрывает старое админ-сообщение. Подтверждение считает новый срок от max(сохранённый срок, now), отклонение возвращает сохранённый срок.

Отдельный сервис `notification`: по `NOTIFICATION_CRON` напоминать юзерам о скором окончании подписки (в старом коде `Notify` с клавиатурой Продлить/Подписка).

## Структура

Go workspace (`go.work`), модуль-префикс `github.com/bruhabruh/bruhanet`.

- `pkg/` — общее: `app` (CompositeApp + `Run` с graceful shutdown), `lifecycle` (Start/Stop), `cfg` (env → struct + validator), `stlog` (slog + логгер в context), `remnawave` (обёртка `remnawave-api-go/v3`), `tgupdate` (user/chat/type апдейта), `fsm` (стейт + data юзера, `in_memory`/`redis`, CAS — единственное хранилище), `keyboard` (разметка inline-клавиатур, стили-константы), `callback` (имя роута + целый аргумент в callback data, без хранилища).
- `bot/` — `cmd/main.go` → `internal/app` (wiring), `internal/config` (по файлу на секцию), `internal/handler` (`New` — сборка бота; `handler/router` — роутер; `handler/routes` — все роуты с их хендлерами и клавиатурами, файл на роут; `handler/middleware` — цепочка middleware), `internal/model`, `internal/tgtest` (фейковый Telegram API для тестов).
- `notification/` — заготовка, пустой `main`.

## Команды

`task` — список. Основное: `task setup`, `task up` (Redis в docker), `task dev SVC=bot` (air + jq), `task run SVC=bot`, `task test`, `task lint`, `task fmt`, `task tidy`.
Новый модуль добавлять в `SERVICES` в `Taskfile.yaml`.
Redis-реализация FSM тестируется на miniredis, docker для `task test` не нужен. Хендлеры тестируются через `tgtest` (`bot.WithSkipGetMe` + `WithServerURL` + `WithNotAsyncHandlers`, апдейты через `ProcessUpdate`).

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
- Стили кнопок — только `keyboard.Style*` константы.
- На callback отвечать через `middleware.Answer`, или повесить на роут `middleware.AutoAnswer("текст")` (toast после хендлера); иначе глобальный `AutoAnswer()` ответит пустым.
- Служебное состояние апдейта (route, answered, panicked) — `middleware.UpdateInfoFrom(ctx)`; это не логирование.
- Дизайн каркаса хендлеров: `docs/superpowers/specs/2026-10-26-bot-handler-infra-design.md`.
- Callback data в старом боте — короткие коды `"0".."5"` + `:<arg>` (лимит Telegram 64 байта).
- Грабли старого кода, не повторять: обращение к `payment.*` до проверки `err` (nil-паника), повторное нажатие Подтвердить/Отклонить, игнор ошибок `UpdateUser` при выдаче 1 дня, admin-хендлеры без проверки `AdminID`.
- Экраны по callback — `show` (редактирование на месте, fallback — новое сообщение); ответы на фото/текст — `send`.
- Remnawave в хендлерах — только через `routes.Remnawave`; в тестах `tgtest.NewRemnawave`. Время — `h.now()` (поле `handlers.now`).
