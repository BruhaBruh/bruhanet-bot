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
	"github.com/bruhabruh/bruhanet/core/settings"
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
	// Ссылка подарка: активация вместо приветствия (успех сообщает уведомление из очереди).
	if code, ok := giftPayload(u); ok {
		h.redeemGift(ctx, b, u, code)
		return
	}
	// Ссылка со страницы входа админки (`?start=login`): админу — кнопка входа, как у /login.
	// Остальным — обычное приветствие, ничего не выдаём (и не отличаем от простого /start).
	if h.isAdmin(u) && isStartLogin(u) {
		h.login(ctx, b, u)
		return
	}
	h.leavePromo(ctx, tgupdate.UserID(u))
	show(ctx, b, u, h.mainKeyboard(ctx, tgupdate.UserID(u), "menu.greeting"), h.c.Text(ctx, "menu.greeting", nil))
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

func (h *handlers) mainKeyboard(ctx context.Context, telegramID int64, key string) *models.InlineKeyboardMarkup {
	return mainKeyboard(ctx, h.c, h.rw, telegramID, key, h.trialOffered)
}

// kb — построитель клавиатуры сообщения key для получателя из контекста; юзер панели читается лениво, только если
// условие кнопки этого просит.
func (h *handlers) kb(ctx context.Context, id, key string) *kbBuilder {
	return newKB(ctx, h.c, id, key).viaPanel(h.rw, h.trialOffered)
}

// trialOffer решает, показывать ли «Пробный период» (user — юзер панели, nil — юзера там нет);
// nil — не показывать (уведомления из очереди идут юзерам, у которых он уже не нужен).
type trialOffer func(ctx context.Context, telegramID int64, user *api.UserItemInfo) bool

// key — сообщение, под которым стоит клавиатура (её раскладка из конструктора).
func mainKeyboard(ctx context.Context, c *content.Provider, rw Remnawave, telegramID int64, key string, trial trialOffer) *models.InlineKeyboardMarkup {
	set := c.Settings(ctx)
	user, err := rw.GetUser(ctx, telegramID)
	switch {
	case errors.Is(err, remnawave.ErrUserNotFound):
		kb := newKB(ctx, c, "menu_new", key).known(telegramID, nil, trial).
			callback("button.buy", routePay, keyboard.StyleSuccess).
			callback("button.promo", routePromo, keyboard.StyleDefault).
			url("button.support", set.SupportURL, keyboard.StyleDanger)
		if trial != nil && trial(ctx, telegramID, nil) {
			kb.callback("button.trial", routeTrial, keyboard.StyleDefault)
		}
		menuExtras(kb, set)
		return kb.markup()
	case err != nil:
		stlog.Error(ctx, "failed to get remnawave user", "error", err)
		return keyboard.Inline(keyboard.Row(callbackButton(ctx, c, "button.service_down", routeMenu, keyboard.StyleDefault)))
	}
	kb := newKB(ctx, c, "menu", key).known(telegramID, user, trial).
		callback("button.info", routeInfo, keyboard.StylePrimary).
		callback("button.pay", routePay, keyboard.StyleDefault).
		copy("button.subscription", user.SubscriptionUrl, keyboard.StyleSuccess).
		url("button.support", set.SupportURL, keyboard.StyleDanger).
		callback("button.promo", routePromo, keyboard.StyleDefault)
	if trial != nil && trial(ctx, telegramID, user) {
		kb.callback("button.trial", routeTrial, keyboard.StyleDefault)
	}
	// «Устройства» — любому юзеру панели: очистка и MiniApp не зависят от продажи (докупка скрыта внутри экрана)
	kb.callback("button.devices", routeDevices, keyboard.StyleDefault)
	menuExtras(kb, set)
	return kb.markup()
}

// menuExtras — кнопки, зависящие только от настроек: «Подарить» (пока есть тарифы) и «Пригласить друга» (пока дни
// рефералки больше нуля).
func menuExtras(kb *kbBuilder, set settings.Settings) {
	if giftOffered(kb.ctx, kb.c) {
		kb.callback("button.gift", routeGift, keyboard.StyleDefault)
	}
	if set.ReferralEnabled() {
		kb.callback("button.referral", routeReferral, keyboard.StyleDefault)
	}
	if kb.env.eval("is_admin") { // «Админка» — только админу, web_app на страницу входа
		kb.webApp("button.admin", kb.c.AdminURL(), keyboard.StyleDefault)
	}
}
