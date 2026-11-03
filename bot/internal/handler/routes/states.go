package routes

import "github.com/bruhabruh/bruhanet/pkg/fsm"

// Стейты оплаты — только подсказка роутинга фото; сам платёж живёт в Postgres.
const (
	stateAwaitPhoto fsm.State = "await_photo"
	stateInReview   fsm.State = "in_review"
)
