package routes

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

func (h *handlers) start(ctx context.Context, b *bot.Bot, u *models.Update) {
	if t := tgupdate.Type(u); t == tgupdate.TypeEditedMessage || t == tgupdate.TypeOther || !isPrivate(u) {
		return
	}
	if tgupdate.ChatID(u) == 0 {
		return
	}
	h.recordReferral(ctx, u)
	// Ссылка со страницы входа админки (`?start=login`): админу — кнопка входа, как у /login.
	// Остальным — обычное приветствие, ничего не выдаём (и не отличаем от простого /start).
	if h.isAdmin(u) && isStartLogin(u) {
		h.login(ctx, b, u)
		return
	}
	h.leavePromo(ctx, tgupdate.UserID(u))
	show(ctx, b, u, h.mainKeyboard(ctx, tgupdate.UserID(u)), h.c.Text(ctx, "menu.greeting", nil))
}

// startPayload — аргумент deep link в «/start <payload>»; нет аргумента или это не /start — пусто.
func startPayload(u *models.Update) string {
	if u.Message == nil {
		return ""
	}
	f := strings.Fields(u.Message.Text)
	if len(f) != 2 || (f[0] != "/start" && !strings.HasPrefix(f[0], "/start@")) {
		return ""
	}
	return f[1]
}

// refPrefix — начало payload реферальной ссылки: ref_<telegram id пригласившего>.
const refPrefix = "ref_"

// recordReferral запоминает пригласившего по «/start ref_<id>», только если юзер здесь новый: строку юзера в БД ещё не
// завёл никто (LastSeen пишет её после хендлера, так что первое касание видно именно тут). Свой id, чужой несуществующий
// пригласивший, мусор и повторный старт ничего не записывают, и юзеру об этом ничего не показывается: приветствие одно.
// Запись не зависит от настройки дней: награду (если включена) решает первая оплата.
func (h *handlers) recordReferral(ctx context.Context, u *models.Update) {
	raw, ok := strings.CutPrefix(startPayload(u), refPrefix)
	if !ok || len(raw) > 19 {
		return
	}
	referrer, err := strconv.ParseInt(raw, 10, 64)
	from := tgupdate.User(u)
	if err != nil || referrer <= 0 || from == nil || from.IsBot || referrer == from.ID {
		return
	}
	// Таймаут: запись идёт до ответа юзеру; сбой не мешает приветствию.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err = h.pay.RecordReferral(ctx, model.UserSeen{
		TelegramID: from.ID, Username: from.Username, FirstName: from.FirstName, LastName: from.LastName, At: h.now(),
	}, referrer); err != nil {
		stlog.Warn(ctx, "failed to record referral", "error", err)
	}
}

// isStartLogin — сообщение ровно «/start login» (deep link payload).
func isStartLogin(u *models.Update) bool {
	if u.Message == nil {
		return false
	}
	f := strings.Fields(u.Message.Text)
	return len(f) == 2 && (f[0] == "/start" || strings.HasPrefix(f[0], "/start@")) && f[1] == "login"
}

// isPrivate: недоступное сообщение колбэка считаем приватным, ответ всё равно уйдёт по ChatID.
func isPrivate(u *models.Update) bool {
	switch {
	case u.Message != nil:
		return u.Message.Chat.Type == models.ChatTypePrivate
	case u.CallbackQuery != nil && u.CallbackQuery.Message.Message != nil:
		return u.CallbackQuery.Message.Message.Chat.Type == models.ChatTypePrivate
	}
	return true
}

func (h *handlers) mainKeyboard(ctx context.Context, telegramID int64) *models.InlineKeyboardMarkup {
	return mainKeyboard(ctx, h.c, h.rw, telegramID, h.trialOffered)
}

// trialOffer решает, показывать ли «Пробный период» (user — юзер панели, nil — юзера там нет);
// nil — не показывать (уведомления из очереди идут юзерам, у которых он уже не нужен).
type trialOffer func(ctx context.Context, telegramID int64, user *api.UserItemInfo) bool

func mainKeyboard(ctx context.Context, c *content.Provider, rw Remnawave, telegramID int64, trial trialOffer) *models.InlineKeyboardMarkup {
	support := urlButton(ctx, c, "button.support", c.Settings(ctx).SupportURL, keyboard.StyleDanger)
	promoBtn := callbackButton(ctx, c, "button.promo", routePromo, keyboard.StyleDefault)
	user, err := rw.GetUser(ctx, telegramID)
	switch {
	case errors.Is(err, remnawave.ErrUserNotFound):
		rows := [][]keyboard.Button{keyboard.Row(callbackButton(ctx, c, "button.buy", routePay, keyboard.StyleSuccess))}
		if trial != nil && trial(ctx, telegramID, nil) {
			rows = append(rows, keyboard.Row(callbackButton(ctx, c, "button.trial", routeTrial, keyboard.StyleDefault)))
		}
		rows = append(rows, keyboard.Row(promoBtn, support))
		return keyboard.Inline(append(rows, referralRows(ctx, c)...)...)
	case err != nil:
		stlog.Error(ctx, "failed to get remnawave user", "error", err)
		return keyboard.Inline(keyboard.Row(callbackButton(ctx, c, "button.service_down", routeMenu, keyboard.StyleDefault)))
	}
	rows := [][]keyboard.Button{
		keyboard.Row(
			callbackButton(ctx, c, "button.info", routeInfo, keyboard.StylePrimary),
			callbackButton(ctx, c, "button.pay", routePay, keyboard.StyleDefault),
		),
		keyboard.Row(
			copyButton(ctx, c, "button.subscription", user.SubscriptionUrl, keyboard.StyleSuccess),
			support,
		),
	}
	if trial != nil && trial(ctx, telegramID, user) {
		rows = append(rows, keyboard.Row(callbackButton(ctx, c, "button.trial", routeTrial, keyboard.StyleDefault)))
	}
	rows = append(rows, keyboard.Row(promoRow(ctx, c, promoBtn)...))
	return keyboard.Inline(append(rows, referralRows(ctx, c)...)...)
}

// referralRows — «Пригласить друга», пока в настройках дни рефералки больше нуля.
func referralRows(ctx context.Context, c *content.Provider) [][]keyboard.Button {
	if !c.Settings(ctx).ReferralEnabled() {
		return nil
	}
	return [][]keyboard.Button{keyboard.Row(callbackButton(ctx, c, "button.referral", routeReferral, keyboard.StyleDefault))}
}

// promoRow — «Промокод», а при включённой продаже устройств рядом «Устройства».
func promoRow(ctx context.Context, c *content.Provider, promoBtn keyboard.Button) []keyboard.Button {
	if !c.Settings(ctx).DevicesEnabled() {
		return []keyboard.Button{promoBtn}
	}
	return []keyboard.Button{promoBtn, callbackButton(ctx, c, "button.devices", routeDevices, keyboard.StyleDefault)}
}
