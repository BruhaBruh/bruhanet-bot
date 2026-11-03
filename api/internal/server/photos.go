package server

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const maxPhotoSize = 10 << 20 // лимит Telegram на фото

var photoTypes = []string{"image/jpeg", "image/png", "image/webp"}

// uploadPhoto принимает multipart-поле file, проверяет тип по содержимому и размер до Telegram,
// шлёт фото админу и отдаёт file_id — дальше рассылки ссылаются на него.
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
		name := part.FileName()
		if name == "" {
			name = "photo"
		}
		fileID, err := s.Telegram.SendPhoto(r.Context(), s.AdminID, bytes.NewReader(data), name, "Фото для рассылки")
		if err != nil {
			stlog.Warn(r.Context(), "upload photo to telegram", "error", err)
			writeError(w, http.StatusBadGateway, "telegram_unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"photo": fileID})
		return
	}
}
