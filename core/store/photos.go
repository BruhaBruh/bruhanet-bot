package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SavePhoto кладёт картинку рассылки в БД и отдаёт её id; тип и размер вызывающий уже проверил.
func (s *Store) SavePhoto(ctx context.Context, content []byte, contentType string) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.pool.Exec(ctx, `INSERT INTO broadcast_photos (id, content, content_type, size) VALUES ($1, $2, $3, $4)`,
		id, content, contentType, len(content))
	return id, err
}

// Photo — байты и тип картинки; нет такой — ErrNotFound.
func (s *Store) Photo(ctx context.Context, id uuid.UUID) ([]byte, string, error) {
	var (
		content []byte
		ct      string
	)
	err := s.pool.QueryRow(ctx, `SELECT content, content_type FROM broadcast_photos WHERE id = $1`, id).Scan(&content, &ct)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return content, ct, err
}

// PhotoExists — есть ли картинка (байты не читаются).
func (s *Store) PhotoExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM broadcast_photos WHERE id = $1)`, id).Scan(&ok)
	return ok, err
}

// PhotoForSend — что нужно для отправки: file_id, если Telegram уже принял картинку (байты тогда не читаются), иначе
// байты с типом. Нет такой картинки — ErrNotFound.
func (s *Store) PhotoForSend(ctx context.Context, id uuid.UUID) (fileID string, content []byte, contentType string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT COALESCE(file_id, ''), CASE WHEN file_id IS NULL THEN content END, content_type
		FROM broadcast_photos WHERE id = $1`, id).Scan(&fileID, &content, &contentType)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return fileID, content, contentType, err
}

// SetPhotoFileID запоминает file_id первой успешной отправки; уже записанный не трогает (идемпотентно).
func (s *Store) SetPhotoFileID(ctx context.Context, id uuid.UUID, fileID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE broadcast_photos SET file_id = $2 WHERE id = $1 AND file_id IS NULL`, id, fileID)
	return err
}

// DeleteStalePhotos удаляет картинки старше before, на которые не ссылается ни рассылка, ни личное сообщение.
func (s *Store) DeleteStalePhotos(ctx context.Context, before time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM broadcast_photos p WHERE p.created_at < $1
		AND NOT EXISTS (SELECT 1 FROM broadcasts b WHERE b.photo_id = p.id)
		AND NOT EXISTS (SELECT 1 FROM direct_messages d WHERE d.photo_id = p.id)`, before)
	return int(tag.RowsAffected()), err
}
