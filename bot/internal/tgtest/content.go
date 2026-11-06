package tgtest

import (
	"context"
	"sync"

	"github.com/bruhabruh/bruhanet/core/model"
)

// Source — фейковый content.Source: правки текстов и настроек «из админки».
type Source struct {
	mu       sync.Mutex
	texts    map[string]string
	settings map[string][]byte
	contacts map[int64]model.UserContact
	styles   map[string]string
}

func (s *Source) SetStyles(styles map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.styles = styles
}

func (s *Source) TextStyles(context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.styles, nil
}

func (s *Source) SetContact(id int64, c model.UserContact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.contacts == nil {
		s.contacts = map[int64]model.UserContact{}
	}
	s.contacts[id] = c
}

func (s *Source) UserContact(_ context.Context, id int64) (model.UserContact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contacts[id], nil
}

func (s *Source) SetTexts(texts map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texts = texts
}

func (s *Source) SetSettings(raw map[string][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = raw
}

func (s *Source) TextOverrides(context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.texts, nil
}

func (s *Source) SettingsRaw(context.Context) (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings, nil
}
