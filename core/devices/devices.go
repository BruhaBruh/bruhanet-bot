// Package devices — цены на дополнительные устройства подписки. Сам лимит живёт в панели (база + купленные).
package devices

import (
	"errors"
	"time"
)

// ErrNoTerm — подписка уже закончилась или не начиналась: докупить устройства «до конца срока» нельзя,
// они войдут в цену продления.
var ErrNoTerm = errors.New("devices: no active subscription term")

// MaxTotal — наибольший лимит устройств у юзера в сумме (база + купленные): верхняя граница, которую мы пишем в панель.
const MaxTotal = 100

// Room — сколько ещё устройств можно докупить при базе base и купленных extra; 0 — предел достигнут.
func Room(base, extra int) int { return max(0, MaxTotal-base-extra) }

const day = 24 * time.Hour

// RemainingDays — сколько суток осталось до expire, с округлением вверх; 0 — срок вышел.
func RemainingDays(expire, now time.Time) int {
	if !expire.After(now) {
		return 0
	}
	return int((expire.Sub(now) + day - 1) / day)
}

// ProratedPrice — цена count доп. устройств до конца подписки: ceil(price * count * дни / 30), не меньше 1 ₽.
// price — ₽ за устройство в месяц (30 дней). Срок вышел — ErrNoTerm.
func ProratedPrice(price, count int, expire, now time.Time) (int, error) {
	days := RemainingDays(expire, now)
	if days == 0 {
		return 0, ErrNoTerm
	}
	total := int64(price) * int64(count) * int64(days)
	return int(max(1, (total+29)/30)), nil
}

// TariffAddon — доплата за count доп. устройств в тарифе на months месяцев.
func TariffAddon(price, count, months int) int { return price * count * months }
