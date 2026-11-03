package routes

import "github.com/bruhabruh/bruhanet/pkg/fsm"

// Стейты оплаты — только подсказка роутинга фото; сам платёж живёт в Postgres.
const (
	stateAwaitPhoto fsm.State = "await_photo"
	stateInReview   fsm.State = "in_review"
)

// Стейты промокодов: ожидание кода сообщением (при оплате — с data promoData, из меню — без)
// и «оплата промокодом в работе» — защита от двойного «Да».
const (
	stateAwaitPromo     fsm.State = "await_promo"
	stateAwaitPromoDays fsm.State = "await_promo_days"
	statePromoPaying    fsm.State = "promo_paying"
)
