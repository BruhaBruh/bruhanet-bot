package fsm

import "errors"

var (
	ErrNoFSM         = errors.New("fsm is not in context")
	ErrNoData        = errors.New("fsm data is empty")
	ErrStateMismatch = errors.New("fsm state mismatch")
	ErrEncode        = errors.New("failed to encode fsm data")
	ErrDecode        = errors.New("failed to decode fsm data")
	ErrStorage       = errors.New("fsm storage error")
)
