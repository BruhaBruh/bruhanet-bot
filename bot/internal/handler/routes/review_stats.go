package routes

import (
	"fmt"
	"html"
	"strconv"

	"github.com/bruhabruh/bruhanet/core/model"
)

// formatStats — блок статистики юзера в сообщении проверки; строки уже экранированы для HTML.
func formatStats(s model.UserStats, advanceLimit int) []string {
	var blocked []string
	if s.Blocked {
		blocked = []string{"🚫 заблокирован: без авансов и промокодов"}
	}
	if !s.HasHistory {
		return append([]string{"🆕 первая оплата"}, blocked...)
	}
	paid := fmt.Sprintf("📊 Оплат: %d на %s ₽", s.Confirmed, groupThousands(s.ConfirmedSum))
	if s.Confirmed > 0 && s.LastConfirmedAt != nil {
		paid += " · последняя " + s.LastConfirmedAt.In(moscow).Format("02.01.2006")
	}
	advances := fmt.Sprintf("❌ Авансов без оплаты: %d из %d", s.UnpaidAdvances, advanceLimit)
	if s.UnpaidAdvances >= advanceLimit {
		advances = fmt.Sprintf("⚠️ Авансов без оплаты: %d — без аванса", s.UnpaidAdvances)
	}
	rows := []string{paid, advances}
	if s.FirstConfirmedAt != nil {
		rows = append(rows, "🕐 Клиент с "+s.FirstConfirmedAt.In(moscow).Format("01.2006"))
	}
	rows = append(rows, blocked...)
	for i, r := range rows {
		rows[i] = html.EscapeString(r)
	}
	return rows
}

// groupThousands: 1234567 → "1 234 567".
func groupThousands(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + groupThousands(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + " " + s[i:]
	}
	return s
}

// promoLine — строка о промокоде платежа для админа; уже экранирована.
func promoLine(p model.Promo) string {
	discount := fmt.Sprintf("−%d%%", p.Value)
	if p.Kind == model.PromoRub {
		discount = "−" + groupThousands(p.Value) + " ₽"
	}
	return html.EscapeString("🎟 промокод " + p.Code + " (" + discount + ")")
}
