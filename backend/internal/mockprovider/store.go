package mockprovider

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrKeyReused, aynı Idempotency-Key farklı bir alıcı, kanal veya içerikle
// kullanılınca döner. Provider kayıtlı sonucu döndürmez, isteği reddeder
// (ADR-025).
var ErrKeyReused = errors.New("mockprovider: idempotency anahtarı farklı bir istekle kullanıldı")

// Request, bir kabul isteğinin tekilleştirme için gereken özetidir.
type Request struct {
	Key       string
	Channel   string
	Recipient string
	// ContentHash, içeriğin SHA-256 özetidir.
	ContentHash [32]byte
}

// Acceptance, kaydedilmiş ilk kabul sonucudur.
type Acceptance struct {
	MessageID string
	// Created, kabulün bu çağrıyla ilk kez kaydedildiğini gösterir. false ise
	// aynı istek daha önce kabul edilmişti ve kayıtlı sonuç döndü.
	Created bool
}

// Store, kabul kayıtlarının kalıcı deposudur.
type Store interface {
	// Accept, isteği atomik olarak kaydeder. Anahtar zaten kayıtlıysa ve istek
	// kayıtla aynıysa kayıtlı sonucu (Created=false) döndürür; kanal, alıcı
	// veya içerik farklıysa ErrKeyReused döner.
	Accept(ctx context.Context, r Request) (Acceptance, error)
}

// QueryRower, tek satır döndüren sorgu çalıştırabilen bir veritabanıdır.
type QueryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// PGStore, kabul kayıtlarını mock_provider_acceptances tablosunda tutar.
type PGStore struct {
	db QueryRower
}

// NewPGStore, verilen veritabanıyla bir PGStore oluşturur.
func NewPGStore(db QueryRower) *PGStore { return &PGStore{db: db} }

// Accept, Store arayüzünü uygular. Yazma INSERT ... ON CONFLICT DO NOTHING ile
// atomiktir: eşzamanlı aynı anahtarlı isteklerden yalnızca biri satırı yazar,
// diğerleri onun commit'ini bekler ve kayıtlı sonucu okur.
func (s *PGStore) Accept(ctx context.Context, r Request) (Acceptance, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO mock_provider_acceptances (idempotency_key, channel, recipient, content_hash)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (idempotency_key) DO NOTHING
		 RETURNING message_id::text`,
		r.Key, r.Channel, r.Recipient, r.ContentHash[:],
	).Scan(&id)
	if err == nil {
		return Acceptance{MessageID: id, Created: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Acceptance{}, fmt.Errorf("mockprovider: kabul kaydedilemedi: %w", err)
	}

	// Çakışma: kayıtlı kabulü oku ve istekle karşılaştır.
	var (
		channel, recipient string
		hash               []byte
	)
	err = s.db.QueryRowContext(ctx,
		`SELECT message_id::text, channel, recipient, content_hash
		 FROM mock_provider_acceptances WHERE idempotency_key = $1`, r.Key,
	).Scan(&id, &channel, &recipient, &hash)
	if err != nil {
		return Acceptance{}, fmt.Errorf("mockprovider: kayıtlı kabul okunamadı: %w", err)
	}
	if channel != r.Channel || recipient != r.Recipient || !bytes.Equal(hash, r.ContentHash[:]) {
		return Acceptance{}, ErrKeyReused
	}
	return Acceptance{MessageID: id}, nil
}
