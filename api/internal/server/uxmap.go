package server

import (
	"net/http"

	"github.com/bruhabruh/bruhanet/core/texts"
)

// uxMap отдаёт таблицы UX-карты бота (входы, переходы кнопок, связи без кнопок); узлы и клавиатуры панель берёт из GET /api/texts.
func (s *server) uxMap(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, texts.BuildUXMap())
}
