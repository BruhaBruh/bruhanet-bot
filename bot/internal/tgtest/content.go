package tgtest

import (
	"context"
	"sync"
)

// Source — фейковый content.Source: правки текстов и настроек «из админки».
type Source struct {
	mu       sync.Mutex
	texts    map[string]string
	settings map[string][]byte
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
