package tgtest

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/store"
)

var errCommit = errors.New("tgtest: commit failed")

// Payments — in-memory фейк store для тестов хендлеров; правила UserStats как в SQL.
type Payments struct {
	mu             sync.Mutex
	payments       []model.Payment // индекс = ID-1
	outbox         []model.OutboxMessage
	failResolve    bool
	failSubmitOnce bool
}

func NewPayments() *Payments {
	return &Payments{}
}

// FailResolveCommitOnce: следующий ResolvePayment выполнит apply, затем вернёт ошибку без смены статуса.
func (p *Payments) FailResolveCommitOnce() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failResolve = true
}

// FailSubmitNotPendingOnce: следующий SubmitPayment вернёт store.ErrNotPending.
func (p *Payments) FailSubmitNotPendingOnce() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failSubmitOnce = true
}

func (p *Payments) All() []model.Payment {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.payments)
}

func (p *Payments) Outbox() []model.OutboxMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.outbox)
}

func (p *Payments) pendingIdx(telegramID int64) int {
	return slices.IndexFunc(p.payments, func(x model.Payment) bool {
		return x.TelegramID == telegramID && x.Status == model.PaymentPending
	})
}

func (p *Payments) byID(id int64) (*model.Payment, error) {
	if id < 1 || id > int64(len(p.payments)) {
		return nil, store.ErrNotFound
	}
	return &p.payments[id-1], nil
}

// CreatePayment — rollback вызывается под мьютексом фейка: из него нельзя звать Payments.
func (p *Payments) CreatePayment(
	_ context.Context, np model.Payment, rollback func(old model.Payment) error,
) (model.Payment, *model.Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var cancelled *model.Payment
	if i := p.pendingIdx(np.TelegramID); i >= 0 {
		old := &p.payments[i]
		if err := rollback(*old); err != nil {
			return model.Payment{}, nil, err
		}
		at := time.Now()
		old.Status, old.ResolvedAt = model.PaymentCancelled, &at
		c := *old
		cancelled = &c
		np.PrevExpireAt = old.PrevExpireAt
	}
	np.ID = int64(len(p.payments) + 1)
	np.Status = model.PaymentPending
	np.CreatedAt = time.Now()
	np.AdvanceGranted, np.AdminMessageID, np.SubmittedAt, np.ResolvedAt = false, 0, nil, nil
	p.payments = append(p.payments, np)
	return np, cancelled, nil
}

func (p *Payments) PendingPayment(_ context.Context, telegramID int64) (model.Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.pendingIdx(telegramID)
	if i < 0 {
		return model.Payment{}, store.ErrNotFound
	}
	return p.payments[i], nil
}

func (p *Payments) SubmitPayment(_ context.Context, id int64, adminMessageID int, at time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failSubmitOnce {
		p.failSubmitOnce = false
		return store.ErrNotPending
	}
	x, err := p.byID(id)
	if err != nil || x.Status != model.PaymentPending || x.SubmittedAt != nil {
		return store.ErrNotPending
	}
	x.AdminMessageID, x.SubmittedAt = adminMessageID, &at
	return nil
}

func (p *Payments) MarkAdvance(_ context.Context, id int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	x, err := p.byID(id)
	if err != nil {
		return err
	}
	x.AdvanceGranted = true
	return nil
}

func (p *Payments) UserStats(_ context.Context, telegramID, excludeID int64) (model.UserStats, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var (
		st   model.UserStats
		last int64
	)
	for _, x := range p.payments {
		if x.TelegramID != telegramID || x.ID == excludeID {
			continue
		}
		st.HasHistory = true
		if x.Status != model.PaymentConfirmed {
			continue
		}
		last = max(last, x.ID)
		st.Confirmed++
		st.ConfirmedSum += x.Price
		at := x.CreatedAt
		if x.ResolvedAt != nil {
			at = *x.ResolvedAt
		}
		if st.LastConfirmedAt == nil || at.After(*st.LastConfirmedAt) {
			st.LastConfirmedAt = &at
		}
		if st.FirstConfirmedAt == nil || at.Before(*st.FirstConfirmedAt) {
			st.FirstConfirmedAt = &at
		}
	}
	for _, x := range p.payments {
		if x.TelegramID == telegramID && x.ID != excludeID && x.ID > last && x.AdvanceGranted &&
			(x.Status == model.PaymentDeclined || x.Status == model.PaymentCancelled) {
			st.UnpaidAdvances++
		}
	}
	return st, nil
}

// ResolvePayment — apply вызывается под мьютексом фейка: из него нельзя звать Payments.
func (p *Payments) ResolvePayment(
	_ context.Context, id int64, status model.PaymentStatus, at time.Time, apply func(model.Payment) (model.OutboxMessage, error),
) (model.Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if status != model.PaymentConfirmed && status != model.PaymentDeclined {
		return model.Payment{}, store.ErrInvalidStatus
	}
	x, err := p.byID(id)
	if err != nil {
		return model.Payment{}, err
	}
	if x.Status != model.PaymentPending {
		return *x, store.ErrNotPending
	}
	msg, err := apply(*x)
	if err != nil {
		return model.Payment{}, err
	}
	if p.failResolve {
		p.failResolve = false
		return model.Payment{}, errCommit
	}
	x.Status, x.ResolvedAt = status, &at
	p.outbox = append(p.outbox, msg)
	return *x, nil
}
