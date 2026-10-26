package fsm

import (
	"bytes"
	"context"
	"sync"
)

type InMemory struct {
	mu      sync.Mutex
	records map[int64]Record
}

func NewInMemory() *InMemory {
	return &InMemory{records: make(map[int64]Record)}
}

func (s *InMemory) Get(_ context.Context, userID int64) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.records[userID]
	return Record{State: r.State, Data: bytes.Clone(r.Data)}, nil
}

func (s *InMemory) Set(_ context.Context, userID int64, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[userID] = Record{State: r.State, Data: bytes.Clone(r.Data)}
	return nil
}

func (s *InMemory) Delete(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, userID)
	return nil
}

func (s *InMemory) CompareAndSet(_ context.Context, userID int64, from State, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records[userID].State != from {
		return ErrStateMismatch
	}
	s.records[userID] = Record{State: r.State, Data: bytes.Clone(r.Data)}
	return nil
}
