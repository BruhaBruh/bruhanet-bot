package fsm

import (
	"context"
	"encoding/json"
	"errors"
)

type State string

const StateNone State = ""

const setRetries = 3

type Record struct {
	State State
	Data  []byte
}

type Storage interface {
	Get(ctx context.Context, userID int64) (Record, error)
	Set(ctx context.Context, userID int64, r Record) error
	Delete(ctx context.Context, userID int64) error
	CompareAndSet(ctx context.Context, userID int64, from State, r Record) error
}

type FSM struct {
	store  Storage
	userID int64
}

func For(store Storage, userID int64) *FSM {
	return &FSM{store: store, userID: userID}
}

func (f *FSM) UserID() int64 {
	if f == nil {
		return 0
	}
	return f.userID
}

func (f *FSM) State(ctx context.Context) (State, error) {
	if f == nil {
		return StateNone, ErrNoFSM
	}
	r, err := f.store.Get(ctx, f.userID)
	if err != nil {
		return StateNone, err
	}
	return r.State, nil
}

// Set меняет состояние, сохраняя данные. CAS сравнивает только состояние, поэтому
// параллельная запись данных может быть перезатёрта; для переходов оплаты
// используйте Transition.
func (f *FSM) Set(ctx context.Context, st State) error {
	if f == nil {
		return ErrNoFSM
	}
	for range setRetries {
		r, err := f.store.Get(ctx, f.userID)
		if err != nil {
			return err
		}
		from := r.State
		r.State = st
		err = f.store.CompareAndSet(ctx, f.userID, from, r)
		if !errors.Is(err, ErrStateMismatch) {
			return err
		}
	}
	return ErrStateMismatch
}

func (f *FSM) Clear(ctx context.Context) error {
	if f == nil {
		return ErrNoFSM
	}
	return f.store.Delete(ctx, f.userID)
}

func Put(ctx context.Context, f *FSM, st State, v any) error {
	if f == nil {
		return ErrNoFSM
	}
	data, err := encode(v)
	if err != nil {
		return err
	}
	return f.store.Set(ctx, f.userID, Record{State: st, Data: data})
}

func Data[T any](ctx context.Context, f *FSM) (T, error) {
	var v T
	if f == nil {
		return v, ErrNoFSM
	}
	r, err := f.store.Get(ctx, f.userID)
	if err != nil {
		return v, err
	}
	if len(r.Data) == 0 {
		return v, ErrNoData
	}
	if err := json.Unmarshal(r.Data, &v); err != nil {
		return v, errors.Join(ErrDecode, err)
	}
	return v, nil
}

func Transition(ctx context.Context, f *FSM, from, to State, v any) error {
	if f == nil {
		return ErrNoFSM
	}
	data, err := encode(v)
	if err != nil {
		return err
	}
	return f.store.CompareAndSet(ctx, f.userID, from, Record{State: to, Data: data})
}

func encode(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, errors.Join(ErrEncode, err)
	}
	if string(data) == "null" { // типизированный nil, например (*T)(nil)
		return nil, nil
	}
	return data, nil
}
