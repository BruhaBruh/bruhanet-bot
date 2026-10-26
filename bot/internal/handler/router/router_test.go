package router

import (
	"context"
	"slices"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/callback"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

type recorder struct {
	calls  []string
	routes []string
}

func (rc *recorder) handler(name string) bot.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, _ *models.Update) {
		rc.calls = append(rc.calls, name)
		rc.routes = append(rc.routes, middleware.UpdateInfoFrom(ctx).Route())
	}
}

type routerEnv struct {
	b   *bot.Bot
	rec *recorder
	fsm *fsm.InMemory
}

func setupRouter(t *testing.T, register func(r *Router, rec *recorder)) routerEnv {
	t.Helper()
	env := routerEnv{rec: &recorder{}, fsm: fsm.NewInMemory()}
	r := New()
	register(r, env.rec)
	srv := tgtest.NewServer(t)
	env.b = srv.Bot(t,
		bot.WithDefaultHandler(r.HandleDefault),
		bot.WithMiddlewares(middleware.RequestID, fsm.Middleware(env.fsm), middleware.AutoAnswer()),
	)
	r.Install(env.b)
	return env
}

func TestRouterStaticCallback(t *testing.T) {
	env := setupRouter(t, func(r *Router, rec *recorder) {
		r.Callback("pay", rec.handler("pay"))
		r.Default(rec.handler("default"))
	})
	env.b.ProcessUpdate(t.Context(), tgtest.Callback(42, "pay"))
	if !slices.Equal(env.rec.calls, []string{"pay"}) || env.rec.routes[0] != "callback:pay" {
		t.Fatalf("calls = %v, routes = %v", env.rec.calls, env.rec.routes)
	}
}

func TestRouterArgCallback(t *testing.T) {
	price := callback.New[int]("price")
	var month int
	env := setupRouter(t, func(r *Router, _ *recorder) {
		r.Callback(price.Name(), func(_ context.Context, _ *bot.Bot, u *models.Update) {
			v, err := price.Parse(u.CallbackQuery.Data)
			if err != nil {
				t.Errorf("Parse: %v", err)
			}
			month = v
		})
	})
	env.b.ProcessUpdate(t.Context(), tgtest.Callback(42, price.Data(6)))
	if month != 6 {
		t.Fatalf("month = %d", month)
	}
}

func TestRouterArgCallbackBadDataStillRouted(t *testing.T) {
	env := setupRouter(t, func(r *Router, rec *recorder) {
		r.Callback("price", rec.handler("price"))
		r.Default(rec.handler("default"))
	})
	env.b.ProcessUpdate(t.Context(), tgtest.Callback(42, "price:abc"))
	if !slices.Equal(env.rec.calls, []string{"price"}) {
		t.Fatalf("calls = %v", env.rec.calls)
	}
}

func TestRouterUnknownCallbackFallsBack(t *testing.T) {
	env := setupRouter(t, func(r *Router, rec *recorder) {
		r.Callback("pay", rec.handler("pay"))
		r.Default(rec.handler("default"))
	})
	env.b.ProcessUpdate(t.Context(), tgtest.Callback(42, "payx"))
	env.b.ProcessUpdate(t.Context(), tgtest.Callback(42, "payx:1"))
	env.b.ProcessUpdate(t.Context(), tgtest.Message(42, "/start"))
	if !slices.Equal(env.rec.calls, []string{"default", "default", "default"}) || env.rec.routes[0] != "default" {
		t.Fatalf("calls = %v, routes = %v", env.rec.calls, env.rec.routes)
	}
}

func TestRouterPhotoByState(t *testing.T) {
	env := setupRouter(t, func(r *Router, rec *recorder) {
		r.Photo("payment", rec.handler("photo"), InState("await_photo"))
		r.Default(rec.handler("default"))
	})
	env.b.ProcessUpdate(t.Context(), tgtest.Photo(42))
	if err := fsm.For(env.fsm, 42).Set(t.Context(), "await_photo"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	env.b.ProcessUpdate(t.Context(), tgtest.Photo(42))
	if !slices.Equal(env.rec.calls, []string{"default", "photo"}) {
		t.Fatalf("calls = %v", env.rec.calls)
	}
	if env.rec.routes[1] != "photo:payment" {
		t.Fatalf("routes = %v", env.rec.routes)
	}
}

func TestRouterPhotoWithoutState(t *testing.T) {
	env := setupRouter(t, func(r *Router, rec *recorder) {
		r.Photo("any", rec.handler("photo"))
	})
	env.b.ProcessUpdate(t.Context(), tgtest.Photo(42))
	if !slices.Equal(env.rec.calls, []string{"photo"}) {
		t.Fatalf("calls = %v", env.rec.calls)
	}
}

func TestRouterRouteMiddlewares(t *testing.T) {
	env := setupRouter(t, func(r *Router, rec *recorder) {
		r.Callback("adm_ok", rec.handler("admin"), middleware.AdminOnly(1))
	})
	env.b.ProcessUpdate(t.Context(), tgtest.Callback(2, "adm_ok:5"))
	env.b.ProcessUpdate(t.Context(), tgtest.Callback(1, "adm_ok:5"))
	if !slices.Equal(env.rec.calls, []string{"admin"}) {
		t.Fatalf("calls = %v", env.rec.calls)
	}
}

func TestRouterPanicsOnBadRoutes(t *testing.T) {
	mustPanic := func(name string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: ожидалась паника", name)
			}
		}()
		f()
	}
	r := New()
	mustPanic("invalid", func() { r.Callback("Pay", nil) })
	r.Callback("pay", nil)
	mustPanic("duplicate", func() { r.Callback("pay", nil) })
}
