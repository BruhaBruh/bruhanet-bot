package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// static отдаёт файлы фронтенда; неизвестный путь — index.html (маршруты SPA).
func (s *server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if st, err := fs.Stat(s.Static, name); err != nil || st.IsDir() {
		name = "index.html"
	}
	http.ServeFileFS(w, r, s.Static, name) //nolint:gosec // G703 false positive: fs.FS не пускает ".." и абсолютные пути
}
