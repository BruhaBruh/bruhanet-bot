package tgtest

import (
	"context"
	"sync"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/texts"
)

// Source — фейковый content.Source: правки текстов и настроек «из админки».
type Source struct {
	mu       sync.Mutex
	texts    map[string]string
	settings map[string][]byte
	contacts map[int64]model.UserContact
	styles   map[string]string
	layouts  map[string]texts.Layout
	msgKbs   map[string]texts.MsgLayout
}

// SetLayouts — раскладки клавиатур «из админки».
func (s *Source) SetLayouts(l map[string]texts.Layout) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.layouts = l
}

func (s *Source) KeyboardLayouts(context.Context) (map[string]texts.Layout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.layouts, nil
}

// SetMessageKeyboards — клавиатуры сообщений «из админки».
func (s *Source) SetMessageKeyboards(l map[string]texts.MsgLayout) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgKbs = l
}

func (s *Source) MessageKeyboards(context.Context) (map[string]texts.MsgLayout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.msgKbs, nil
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
