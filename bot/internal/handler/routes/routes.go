package routes

import (
	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/router"
)

const (
	routeMenu = "menu"
	routeInfo = "info"
	routePay  = "pay"
)

type Deps struct {
	Config *config.Config
}

type handlers struct {
	cfg *config.Config
}

// Register — единственное место регистрации роутов бота.
func Register(r *router.Router, d Deps) {
	h := &handlers{cfg: d.Config}
	r.Callback(routeMenu, h.start)
	r.Default(h.start)
}
