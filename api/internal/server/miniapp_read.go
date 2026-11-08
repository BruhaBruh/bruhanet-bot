package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const miniLinkLimit = 20 // выдач ссылки подписки в минуту на юзера

// miniPanelUser — юзер панели для текущего юзера Mini App: nil — его там нет; ok=false — ответ (502) уже записан.
func (s *server) miniPanelUser(w http.ResponseWriter, r *http.Request) (info *api.UserItemInfo, ok bool) {
	ctx, cancel := context.WithTimeout(r.Context(), s.PanelTimeout)
	defer cancel()
	info, err := s.Panel.GetUser(ctx, miniUser(r.Context()).ID)
	switch {
	case errors.Is(err, remnawave.ErrUserNotFound):
		return nil, true
	case err != nil:
		panelFailed(w, r, "panel get user for mini app", err)
		return nil, false
	}
	return info, true
}

type miniDevices struct {
	Used  *int `json:"used"`
	Limit *int `json:"limit"`
}

// miniSubscription — состояние подписки. Ссылки подписки здесь нет: её отдаёт только miniSubscriptionLink.
func (s *server) miniSubscription(w http.ResponseWriter, r *http.Request) {
	info, ok := s.miniPanelUser(w, r)
	if !ok {
		return
	}
	out := map[string]any{
		"state": "none", "expireAt": nil, "daysLeft": 0, "trafficUsedBytes": 0, "trafficLimitBytes": nil,
		"devices": miniDevices{},
	}
	if info != nil {
		now := s.Now()
		state := "disabled"
		switch {
		case info.Status == api.UserItemInfoStatusEXPIRED || !info.ExpireAt.After(now):
			state = "expired"
		case info.Status == api.UserItemInfoStatusACTIVE:
			state = "active"
		}
		left := 0
		if state == "active" {
			left = int((info.ExpireAt.Sub(now) + 24*time.Hour - 1) / (24 * time.Hour))
		}
		out["state"], out["expireAt"], out["daysLeft"] = state, info.ExpireAt.UTC(), left
		out["trafficUsedBytes"] = info.UserTraffic.UsedTrafficBytes
		if info.TrafficLimitBytes > 0 {
			out["trafficLimitBytes"] = info.TrafficLimitBytes
		}
		d := miniDevices{}
		if l, has := remnawave.DeviceLimit(*info); has {
			d.Limit = &l
		}
		ctx, cancel := context.WithTimeout(r.Context(), s.PanelTimeout)
		defer cancel()
		if list, err := s.Panel.UserDevices(ctx, info.ID); err != nil {
			stlog.L(r.Context()).Warn("mini app devices count failed", slog.Any("error", err))
		} else {
			n := len(list)
			d.Used = &n
		}
		out["devices"] = d
	}
	writeJSON(w, http.StatusOK, out)
}

// miniSubscriptionLink — ссылка-подписка (доступ к VPN): только POST по явному нажатию, не кэшируется, не логируется.
func (s *server) miniSubscriptionLink(w http.ResponseWriter, r *http.Request) {
	id := miniUser(r.Context()).ID
	if !s.mini.allow(limitKey("link", id), miniLinkLimit, time.Minute, s.Now()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	info, ok := s.miniPanelUser(w, r)
	if !ok {
		return
	}
	if info == nil || info.SubscriptionUrl == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	stlog.L(r.Context()).Info("mini app subscription link issued")
	writeJSON(w, http.StatusOK, map[string]string{"url": info.SubscriptionUrl})
}

func (s *server) miniReferral(w http.ResponseWriter, r *http.Request) {
	cur, err := s.stored(r.Context())
	if err != nil {
		internal(w, r, "mini app settings", err)
		return
	}
	if cur.ReferralDays <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	id := miniUser(r.Context()).ID
	ref, err := s.Store.UserReferral(r.Context(), id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		internal(w, r, "mini app referral", err)
		return
	}
	days, err := s.Store.ReferrerRewardDays(r.Context(), id)
	if err != nil {
		internal(w, r, "mini app referral days", err)
		return
	}
	link := ""
	if bot := s.botWho(r.Context()).Username; bot != "" {
		link = "https://t.me/" + bot + "?start=ref_" + strconv.FormatInt(id, 10)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true, "link": link, "days": cur.ReferralDays,
		"invited": ref.Invited, "paid": ref.Paid, "rewardDays": days,
	})
}

func (s *server) miniPromos(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.UserPromos(r.Context(), miniUser(r.Context()).ID)
	if err != nil {
		internal(w, r, "mini app promos", err)
		return
	}
	type item struct {
		Code        string    `json:"code"`
		Kind        string    `json:"kind"`
		Value       int       `json:"value"`
		ActivatedAt time.Time `json:"activatedAt"`
	}
	items := make([]item, 0, len(list))
	for _, p := range list {
		items = append(items, item{p.Code, string(p.Kind), p.Value, p.ActivatedAt.UTC()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
