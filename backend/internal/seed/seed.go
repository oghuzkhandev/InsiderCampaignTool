package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Result, bir Run çağrısının sonucudur.
type Result struct {
	// Inserted, bu çağrıda ilk kez oluşturulan kullanıcı sayısıdır.
	Inserted int
	// Skipped, zaten var olduğu için hiç dokunulmayan kullanıcı sayısıdır.
	Skipped int
}

// DB, Run'ın ihtiyaç duyduğu veritabanıdır.
type DB interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Run, demo kullanıcılarını ve izinlerini yazar. Tek transaction'da çalışır.
//
// Kullanıcı e-posta adresiyle tanınır. Yeni kullanıcı için tarihler now'a
// göre yazılır ve izinleri eklenir. Var olan kullanıcıya hiç dokunulmaz:
// tarihleri, bilgileri ve izinleri (silinmiş izin kayıtları dahil) olduğu gibi
// kalır. Bu yüzden tekrar çalıştırmak kayıt çoğaltmaz ve veriyi değiştirmez.
func Run(ctx context.Context, db DB, now time.Time) (Result, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("seed: transaction açılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	insertUser, err := tx.PrepareContext(ctx,
		`INSERT INTO users (email, first_name, phone, city, language, last_cart_add_at, last_purchase_at, order_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (email) DO NOTHING
		 RETURNING id`)
	if err != nil {
		return Result{}, fmt.Errorf("seed: kullanıcı sorgusu: %w", err)
	}
	defer insertUser.Close()

	insertConsent, err := tx.PrepareContext(ctx,
		`INSERT INTO user_consents (user_id, channel, granted) VALUES ($1, $2, $3)`)
	if err != nil {
		return Result{}, fmt.Errorf("seed: izin sorgusu: %w", err)
	}
	defer insertConsent.Close()

	var res Result
	for _, u := range Generate(now) {
		var id int64
		err := insertUser.QueryRowContext(ctx,
			u.Email, u.FirstName, u.Phone, u.City, u.Language, u.LastCartAddAt, u.LastPurchaseAt, u.OrderCount,
		).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			res.Skipped++ // zaten var: tarih ve izinlere dokunma
			continue
		}
		if err != nil {
			return Result{}, fmt.Errorf("seed: %s yazılamadı: %w", u.Email, err)
		}
		for _, c := range u.Consents {
			if _, err := insertConsent.ExecContext(ctx, id, c.Channel, c.Granted); err != nil {
				return Result{}, fmt.Errorf("seed: %s izni yazılamadı: %w", u.Email, err)
			}
		}
		res.Inserted++
	}

	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("seed: commit: %w", err)
	}
	return res, nil
}
