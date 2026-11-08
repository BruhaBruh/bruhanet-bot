package payflow

import (
	"context"
	"errors"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
)

// DaysPromoStore — core/store: promo.Err* и promo.ErrNotFound проходят как есть.
type DaysPromoStore interface {
	ActivateDaysPromo(ctx context.Context, promoID, telegramID int64, now time.Time, apply func(p model.Promo) error) error
}

// DaysPromoRequest — активация кода на бонусные дни из меню бота или из Mini App.
type DaysPromoRequest struct {
	PromoID    int64
	TelegramID int64
	Username   string
	At         time.Time
	Squad      uuid.UUID
	// Ensure отдаёт юзера панели: бот при необходимости заводит его, Mini App только читает (нет юзера — ошибка наружу).
	Ensure func(ctx context.Context) (*api.UserItemInfo, error)
}

// DaysPromoResult — применённый код и срок до/после.
type DaysPromoResult struct {
	Promo                model.Promo
	OldExpire, NewExpire time.Time
}

// ActivateDaysPromo продлевает юзера на дни кода от later(срок в панели, At) в транзакции активации
// (Store.ActivateDaysPromo). Ошибка панели откатывает активацию (ErrPanel, код можно ввести снова). Если срок уже записан,
// а коммит не прошёл, прежний срок возвращается, только пока срок в панели всё ещё наш (ErrNotRestored в ошибке — расхождение).
func ActivateDaysPromo(ctx context.Context, st DaysPromoStore, panel TermPanel, r DaysPromoRequest) (DaysPromoResult, error) {
	var res DaysPromoResult
	err := st.ActivateDaysPromo(ctx, r.PromoID, r.TelegramID, r.At, func(p model.Promo) error {
		user, err := r.Ensure(ctx)
		if err != nil {
			return err
		}
		next := user.ExpireAt
		if r.At.After(next) {
			next = r.At
		}
		next = next.Add(time.Duration(p.Value) * 24 * time.Hour)
		if _, err = panel.UpdateUser(ctx, r.TelegramID, r.Username, next, r.Squad); err != nil {
			return errors.Join(ErrPanel, err)
		}
		res = DaysPromoResult{Promo: p, OldExpire: user.ExpireAt, NewExpire: next}
		return nil
	})
	if err != nil {
		if !res.NewExpire.IsZero() {
			err = errors.Join(err, RestoreTerm(ctx, panel, r.TelegramID, r.Username, res.NewExpire, res.OldExpire, r.Squad))
		}
		return DaysPromoResult{}, err
	}
	return res, nil
}
