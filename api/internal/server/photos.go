package server

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/store"
)

const maxPhotoSize = 10 << 20 // лимит Telegram на фото (sendPhoto)

var photoTypes = []string{"image/jpeg", "image/png", "image/webp"}

// uploadPhoto принимает multipart-поле file, проверяет тип по содержимому и размер, кладёт байты в БД и отдаёт photoId.
// В Telegram ничего не уходит: фото улетит с первым сообщением рассылки (воркер бота).
func (s *server) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "multipart/form-data" {
		writeError(w, http.StatusUnsupportedMediaType, "content type must be multipart/form-data")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoSize+1<<20) // +1 МБ на заголовки частей
	mr, err := r.MultipartReader()
	if err != nil {
		writeBadRequest(w)
		return
	}
	tooBig := func(err error) bool { return errors.As(err, new(*http.MaxBytesError)) }
	for {
		part, err := mr.NextPart()
		switch {
		case errors.Is(err, io.EOF):
			writeInvalid(w, "нет файла")
			return
		case tooBig(err):
			writeInvalid(w, "файл больше 10 МБ")
			return
		case err != nil:
			writeBadRequest(w)
			return
		}
		if part.FormName() != "file" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, maxPhotoSize+1))
		switch {
		case tooBig(err), err == nil && len(data) > maxPhotoSize:
			writeInvalid(w, "файл больше 10 МБ")
			return
		case err != nil:
			writeBadRequest(w)
			return
		}
		if !slices.Contains(photoTypes, http.DetectContentType(data)) {
			writeInvalid(w, "можно загрузить только JPEG, PNG или WebP")
			return
		}
		ct := http.DetectContentType(data)
		id, err := s.Store.SavePhoto(r.Context(), data, ct)
		if err != nil {
			internal(w, r, "save photo", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"photoId": id.String()})
		return
	}
}

// photo отдаёт загруженную картинку для превью в панели; id неизменяем, поэтому кэш браузера безопасен.
func (s *server) photo(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	data, ct, err := s.Store.Photo(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
		return
	case err != nil:
		internal(w, r, "get photo", err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data) //nolint:gosec // G705: тип — image/* из списка загрузки, nosniff
}
