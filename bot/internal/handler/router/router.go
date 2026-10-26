package router

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/pkg/callback"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

type callbackRoute struct {
	name string
	h    bot.HandlerFunc
	mws  []bot.Middleware
}

type messageRoute struct {
	name   string
	h      bot.HandlerFunc
	states []fsm.State
	kind   func(*models.Message) bool
}

type Option func(*messageRoute)

func InState(states ...fsm.State) Option {
	return func(m *messageRoute) { m.states = append(m.states, states...) }
}

func (m messageRoute) matches(msg *models.Message, st fsm.State) bool {
	return m.kind(msg) && (len(m.states) == 0 || slices.Contains(m.states, st))
}

type Router struct {
	callbacks []callbackRoute
	messages  []messageRoute
	fallback  bot.HandlerFunc
}

func New() *Router {
	return &Router{}
}

func (r *Router) Callback(name string, h bot.HandlerFunc, mws ...bot.Middleware) {
	if err := callback.ValidateName(name); err != nil {
		panic(err)
	}
	if slices.ContainsFunc(r.callbacks, func(c callbackRoute) bool { return c.name == name }) {
		panic(fmt.Sprintf("duplicate callback route %q", name))
	}
	r.callbacks = append(r.callbacks, callbackRoute{name: name, h: h, mws: mws})
}

func (r *Router) Message(name string, h bot.HandlerFunc, opts ...Option) {
	r.addMessage("message:"+name, h, func(*models.Message) bool { return true }, opts)
}

func (r *Router) Command(name string, h bot.HandlerFunc, opts ...Option) {
	cmd := "/" + name
	r.addMessage("command:"+name, h, func(m *models.Message) bool {
		return m.Text == cmd || strings.HasPrefix(m.Text, cmd+" ") || strings.HasPrefix(m.Text, cmd+"@")
	}, opts)
}

func (r *Router) Photo(name string, h bot.HandlerFunc, opts ...Option) {
	r.addMessage("photo:"+name, h, func(m *models.Message) bool { return len(m.Photo) > 0 }, opts)
}

func (r *Router) addMessage(name string, h bot.HandlerFunc, kind func(*models.Message) bool, opts []Option) {
	m := messageRoute{name: name, h: h, kind: kind}
	for _, opt := range opts {
		opt(&m)
	}
	r.messages = append(r.messages, m)
}

func (r *Router) Default(h bot.HandlerFunc) {
	r.fallback = h
}

func (r *Router) Install(b *bot.Bot) {
	for _, c := range r.callbacks {
		b.RegisterHandlerMatchFunc(
			matchCallback(c.name), c.h,
			slices.Concat([]bot.Middleware{routeName("callback:" + c.name)}, c.mws)...,
		)
	}
	if len(r.messages) > 0 {
		b.RegisterHandlerMatchFunc(isMessage, r.dispatchMessage)
	}
}

func (r *Router) HandleDefault(ctx context.Context, b *bot.Bot, u *models.Update) {
	if r.fallback == nil {
		return
	}
	routeName("default")(r.fallback)(ctx, b, u)
}

func (r *Router) dispatchMessage(ctx context.Context, b *bot.Bot, u *models.Update) {
	st, err := fsm.From(ctx).State(ctx)
	if err != nil && !errors.Is(err, fsm.ErrNoFSM) {
		stlog.Error(ctx, "failed to get fsm state", "error", err)
	}
	for _, m := range r.messages {
		if m.matches(u.Message, st) {
			routeName(m.name)(m.h)(ctx, b, u)
			return
		}
	}
	r.HandleDefault(ctx, b, u)
}

// matchCallback и isMessage выполняются синхронно в polling-горутине: только проверки полей.
func matchCallback(name string) bot.MatchFunc {
	prefix := name + ":"
	return func(u *models.Update) bool {
		if u.CallbackQuery == nil {
			return false
		}
		data := u.CallbackQuery.Data
		return data == name || strings.HasPrefix(data, prefix)
	}
}

func isMessage(u *models.Update) bool {
	return u.Message != nil
}

func routeName(name string) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, u *models.Update) {
			middleware.UpdateInfoFrom(ctx).SetRoute(name)
			next(stlog.With(ctx, "route", name), b, u)
		}
	}
}
