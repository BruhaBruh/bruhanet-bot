package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/store"
)

// CreateUser и GetUser панели для payflow.GrantDays; запись срока (UpdateUser) — panelWrite, с обновлением users.
func (p panelWrite) CreateUser(ctx context.Context, id int64, username string, squad uuid.UUID) (*api.UserItemInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, p.s.PanelTimeout)
	defer cancel()
	return p.s.Panel.CreateUser(ctx, id, username, squad)
}

func (p panelWrite) GetUser(ctx context.Context, id int64) (*api.UserItemInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, p.s.PanelTimeout)
	defer cancel()
	return p.s.Panel.GetUser(ctx, id)
}

// grantDays — «Выдать дни»: продление в панели, сдвиг pending-платежа, запись grants и уведомление юзеру
// в одной транзакции (payflow.GrantDays). Работает и для заблокированного вручную юзера: блокировка запрещает
// бесплатное, что юзер получает сам; решение админа ей не подчиняется.
func (s *server) grantDays(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeBadRequest(w)
		return
	}
	var in struct {
		Days int `json:"days"`
	}
	if !readJSONMax(w, r, &in, 1<<10, false) {
		return
	}
	if in.Days < 1 || in.Days > payflow.MaxGrantDays {
		writeInvalid(w, "дней — от 1 до "+strconv.Itoa(payflow.MaxGrantDays))
		return
	}
	u, err := s.Store.UserByID(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
		return
	case err != nil:
		internal(w, r, "grant user", err)
		return
	}
	// закрытая вкладка не должна рвать транзакцию после записи в панель
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), resolveTimeout)
	defer cancel()
	expireAt, err := payflow.GrantDays(ctx, s.Store, panelWrite{s}, payflow.GrantRequest{
		TelegramID: id, Username: u.Username, Days: in.Days, AdminID: s.AdminID, At: s.Now(), Squad: s.Squad,
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"expireAt": expireAt, "days": in.Days})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, payflow.ErrPanel):
		internalLog(r, "grant days: panel", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_unavailable", "message": "Панель не ответила, дни не выданы"})
	default:
		internal(w, r, "grant days", err)
	}
}

// blockUser / unblockUser — ручная блокировка: без авансов и промокодов, обычная оплата доступна. Повтор идемпотентен.
func (s *server) setBlocked(blocked bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeBadRequest(w)
			return
		}
		switch err = s.Store.SetUserBlocked(r.Context(), id, blocked, s.Now()); {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "not found")
		default:
			internal(w, r, "set blocked", err)
		}
	}
}
