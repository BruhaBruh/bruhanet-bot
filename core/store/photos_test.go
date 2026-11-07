package store

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
)

func TestPhotoRoundTripAndFileID(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	id, err := s.SavePhoto(ctx, []byte("IMG"), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if c, ct, err := s.Photo(ctx, id); err != nil || string(c) != "IMG" || ct != "image/png" {
		t.Fatalf("Photo = %q %q %v", c, ct, err)
	}
	if _, _, err := s.Photo(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown Photo err = %v", err)
	}
	if _, err := s.SavePhoto(ctx, []byte("x"), "image/gif"); err == nil {
		t.Fatal("content type outside the allow-list was accepted")
	}

	fid, content, _, err := s.PhotoForSend(ctx, id)
	if err != nil || fid != "" || string(content) != "IMG" {
		t.Fatalf("before file_id: %q %q %v", fid, content, err)
	}
	if err := s.SetPhotoFileID(ctx, id, "F1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPhotoFileID(ctx, id, "F2"); err != nil { // идемпотентно: первый file_id остаётся
		t.Fatal(err)
	}
	fid, content, _, err = s.PhotoForSend(ctx, id)
	if err != nil || fid != "F1" || content != nil {
		t.Fatalf("after file_id: %q %q %v", fid, content, err)
	}
	if _, _, _, err := s.PhotoForSend(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown PhotoForSend err = %v", err)
	}
}

func TestDeleteStalePhotos(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 10, nil)
	mk := func() uuid.UUID {
		id, err := s.SavePhoto(ctx, []byte("IMG"), "image/jpeg")
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	orphan, inDraft, inDirect := mk(), mk(), mk()
	d := draft("x")
	d.PhotoID = &inDraft
	if _, err := s.SaveDraft(ctx, uuid.New(), d); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDirect(ctx, 10, "hi", "", &inDirect); err != nil {
		t.Fatal(err)
	}
	// «старыми» считаем всё, что создано до границы; young создаём после неё
	time.Sleep(10 * time.Millisecond)
	cut := time.Now()
	time.Sleep(10 * time.Millisecond)
	young := mk()

	n, err := s.DeleteStalePhotos(ctx, cut)
	if err != nil || n != 1 {
		t.Fatalf("deleted %d, %v; want only the orphan", n, err)
	}
	for id, want := range map[uuid.UUID]bool{orphan: false, inDraft: true, inDirect: true, young: true} {
		if ok, err := s.PhotoExists(ctx, id); err != nil || ok != want {
			t.Errorf("exists(%s) = %v, want %v (%v)", id, ok, want, err)
		}
	}
}

// Картинка рассылки доходит до outbox: payload несёт photo_id, а Broadcast отдаёт photoId.
func TestBroadcastPhotoID(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 10, nil)
	pid, err := s.SavePhoto(ctx, []byte("IMG"), "image/webp")
	if err != nil {
		t.Fatal(err)
	}
	d := draft("hello")
	d.PhotoID = &pid
	b, _, err := s.SendBroadcast(ctx, d, 1, bt0)
	if err != nil {
		t.Fatal(err)
	}
	if b.PhotoID == nil || *b.PhotoID != pid {
		t.Fatalf("PhotoID = %v", b.PhotoID)
	}
	items, err := s.OutboxBatch(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	var got []model.OutboxItem
	for _, it := range items {
		if it.EventType == "broadcast" {
			got = append(got, it)
		}
	}
	if len(got) != 1 || got[0].PhotoID != pid.String() || got[0].Photo != "" {
		t.Fatalf("outbox = %+v", got)
	}
}
