package routes

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/pkg/callback"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

const (
	routeGift = "gift"
	// giftPrefix — начало payload ссылки подарка: gift_<код>.
	giftPrefix = "gift_"
)

var (
	giftPriceRoute = callback.New[int]("gprice") // gprice:<месяцы> — подтверждение подарка
	giftBuyRoute   = callback.New[int]("gbuy")   // gbuy:<месяцы> — создать платёж за подарок
)

// giftCodeRe — допустимая форма кода из ссылки (crypto/rand, base64url): мусор отсекается без обращения к БД.
var giftCodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// giftLink — ссылка t.me/<бот>?start=gift_<код>.
func giftLink(botName, code string) string {
	return "https://t.me/" + botName + "?start=" + giftPrefix + code
}

// giftOffered — «Подарить» есть в меню, пока в настройках есть хотя бы один тариф.
func giftOffered(ctx context.Context, c *content.Provider) bool {
	return len(c.Settings(ctx).Prices) > 0
}

// gift — «Подарить»: выбор срока из тех же тарифов.
func (h *handlers) gift(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !giftOffered(ctx, h.c) {
		h.start(ctx, b, u)
		return
	}
	h.leavePromo(ctx, tgupdate.UserID(u))
	show(ctx, b, u, h.tariffKeyboardFor(ctx, "gift_tariffs", giftPriceRoute), h.c.Text(ctx, "gift.tariffs", nil))
}

// giftPrice — «Верно?» для подарка. Срок проверяется по текущим тарифам: callback подделывается клиентом.
func (h *handlers) giftPrice(ctx context.Context, b *bot.Bot, u *models.Update) {
	month, price, ok := h.tariff(ctx, giftPriceRoute, u)
	if !ok {
		h.gift(ctx, b, u)
		return
	}
	show(ctx, b, u, h.kb(ctx, "gift_confirm", "gift.confirm").
		callback("button.yes", giftBuyRoute.Data(month), keyboard.StyleSuccess).
		callback("button.no", routeGift, keyboard.StyleDanger).markup(),
		h.c.Text(ctx, "gift.confirm", tariffVars(month, price)))
}

// giftBuy — «Да»: платёж kind gift (без промокодов, устройств, аванса и без изменения срока покупателя) и реквизиты.
// Чужой открытый платёж не отменяется: пока он есть, отказ.
func (h *handlers) giftBuy(ctx context.Context, b *bot.Bot, u *models.Update) {
	month, price, ok := h.tariff(ctx, giftBuyRoute, u)
	if !ok {
		h.gift(ctx, b, u)
		return
	}
	userID := tgupdate.UserID(u)
	// двойное «Да»: тот же открытый платёж без чека показываем снова, второй не создаём
	if p, err := h.pay.PendingPayment(ctx, userID); err == nil && p.Kind == model.KindGift && p.Month == month && p.SubmittedAt == nil {
		h.showGiftRequisites(ctx, b, u, p)
		return
	}
	p, ok := h.createPayment(ctx, b, u, model.Payment{Kind: model.KindGift, Month: month, Price: price, PrevExpireAt: h.now()})
	if !ok {
		return
	}
	if err := fsm.Put(ctx, fsm.For(h.fsm, userID), stateAwaitPhoto, nil); err != nil {
		stlog.Error(ctx, "failed to set payment state", "error", err)
		h.showError(ctx, b, u)
		return
	}
	h.showGiftRequisites(ctx, b, u, p)
}

func (h *handlers) showGiftRequisites(ctx context.Context, b *bot.Bot, u *models.Update, p model.Payment) {
	vars := tariffVars(p.Month, p.Price)
	vars["comments"] = commentsHTML(h.c.Settings(ctx).Comments)
	show(ctx, b, u, h.kb(ctx, "", "gift.requisites").markup(), h.c.Text(ctx, "gift.requisites", vars))
}

func (h *handlers) giftBusy(ctx context.Context, b *bot.Bot, u *models.Update) {
	show(ctx, b, u, h.backKeyboard(ctx, "gift.busy", routeMenu), h.c.Text(ctx, "gift.busy", nil))
}

func (h *handlers) giftUnavailable(ctx context.Context, b *bot.Bot, u *models.Update) {
	send(ctx, b, tgupdate.ChatID(u), h.backKeyboard(ctx, "gift.unavailable", routeMenu), h.c.Text(ctx, "gift.unavailable", nil))
}

// redeemGift — «/start gift_<код>»: активация подарка. Любой отказ (мусор, нет такого, использован, отменён, не оплачен,
// юзер заблокирован) выглядит одинаково. Успех: срок продлён в транзакции хранилища вместе с панелью, уведомления
// активировавшему и покупателю лежат в outbox.
func (h *handlers) redeemGift(ctx context.Context, b *bot.Bot, u *models.Update, code string) {
	from := tgupdate.User(u)
	if from == nil || from.IsBot || !giftCodeRe.MatchString(code) {
		h.giftUnavailable(ctx, b, u)
		return
	}
	h.leavePromo(ctx, from.ID)
	_, _, err := payflow.RedeemGift(ctx, h.pay, h.rw, payflow.GiftRedeem{
		Code: code, TelegramID: from.ID, Username: from.Username, At: h.now(), Squad: h.cfg.Remnawave.InternalSquadId,
		// новый юзер заводится в панели с лимитом устройств из настроек; у пробного лимит остаётся как есть (подарок лишь продлевает срок)
		Prepare: func(ctx context.Context) error {
			_, err := h.ensureUser(ctx, from.ID, from.Username, 0)
			return err
		},
	})
	switch {
	case errors.Is(err, model.ErrGiftUnavailable):
		h.giftUnavailable(ctx, b, u)
	case err != nil:
		stlog.Error(ctx, "failed to redeem gift", "error", err)
		h.showError(ctx, b, u)
	}
}

// giftPayload — код из «/start gift_<код>»; ok=false — это не ссылка подарка.
func giftPayload(u *models.Update) (code string, ok bool) {
	return strings.CutPrefix(startPayload(u), giftPrefix)
}

// GiftNotice — текст и клавиатура уведомления о подарке из строки outbox (ключи gift.*). botName — username бота для
// ссылки покупателю.
func GiftNotice(ctx context.Context, c *content.Provider, rw Remnawave, botName string, m model.OutboxMessage) (string, *models.InlineKeyboardMarkup) {
	ctx = content.ForUser(ctx, m.ChatID)
	vars := map[string]string{
		"months": strconv.Itoa(m.Months) + " " + pluralMonths(m.Months), "monthsShort": strconv.Itoa(m.Months) + " мес.", "expireAt": formatTime(m.ExpireAt),
	}
	if m.TextKey == "gift.ready" {
		link := giftLink(botName, m.GiftCode)
		vars["link"] = link
		return c.Text(ctx, m.TextKey, vars), newKB(ctx, c, "gift_ready", m.TextKey).viaPanel(rw, nil).
			copy("button.copy_link", link, keyboard.StyleSuccess).
			callback("button.back", routeMenu, keyboard.StyleDefault).markup()
	}
	return c.Text(ctx, m.TextKey, vars), mainKeyboard(ctx, c, rw, m.ChatID, m.TextKey, nil)
}
