package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/bruhabruh/bruhanet/core/store"
)

// directMessage — личное сообщение юзеру из таблицы users; доставка — через outbox.
func (s *server) directMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeBadRequest(w)
		return
	}
	var in message
	if !s.readMessage(w, r, &in, &in) {
		return
	}
	switch _, err := s.Store.UserByID(r.Context(), id); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
		return
	case err != nil:
		internal(w, r, "direct user", err)
		return
	}
	msgID, err := s.Store.CreateDirect(r.Context(), id, in.Text, in.Photo, in.PhotoID)
	if err != nil {
		internal(w, r, "create direct", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": msgID})
}
