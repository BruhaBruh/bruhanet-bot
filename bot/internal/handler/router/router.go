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

type photoRoute struct {
	name   string
	h      bot.HandlerFunc
	states []fsm.State
}

type PhotoOption func(*photoRoute)

func InState(states ...fsm.State) PhotoOption {
	return func(p *photoRoute) { p.states = append(p.states, states...) }
}

func (p photoRoute) matches(st fsm.State) bool {
	return len(p.states) == 0 || slices.Contains(p.states, st)
}

type Router struct {
	callbacks []callbackRoute
	photos    []photoRoute
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

func (r *Router) Photo(name string, h bot.HandlerFunc, opts ...PhotoOption) {
	p := photoRoute{name: "photo:" + name, h: h}
	for _, opt := range opts {
		opt(&p)
	}
	r.photos = append(r.photos, p)
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
	if len(r.photos) > 0 {
		b.RegisterHandlerMatchFunc(isPhoto, r.dispatchPhoto)
	}
}

func (r *Router) HandleDefault(ctx context.Context, b *bot.Bot, u *models.Update) {
	if r.fallback == nil {
		return
	}
	routeName("default")(r.fallback)(ctx, b, u)
}

func (r *Router) dispatchPhoto(ctx context.Context, b *bot.Bot, u *models.Update) {
	st, err := fsm.From(ctx).State(ctx)
	if err != nil && !errors.Is(err, fsm.ErrNoFSM) {
		stlog.Error(ctx, "failed to get fsm state", "error", err)
	}
	for _, p := range r.photos {
		if p.matches(st) {
			routeName(p.name)(p.h)(ctx, b, u)
			return
		}
	}
	r.HandleDefault(ctx, b, u)
}

// matchCallback и isPhoto выполняются синхронно в polling-горутине: только проверки полей.
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

func isPhoto(u *models.Update) bool {
	return u.Message != nil && len(u.Message.Photo) > 0
}

func routeName(name string) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, u *models.Update) {
			middleware.UpdateInfoFrom(ctx).SetRoute(name)
			next(stlog.With(ctx, "route", name), b, u)
		}
	}
}
