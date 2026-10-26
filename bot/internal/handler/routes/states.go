package routes

import "github.com/bruhabruh/bruhanet/pkg/fsm"

const (
	stateAwaitPhoto fsm.State = "await_photo"
	stateInReview   fsm.State = "in_review"

	stateAwaitBroadcast   fsm.State = "await_broadcast"
	stateConfirmBroadcast fsm.State = "confirm_broadcast"
)
