package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
)

// RecipientReader, kampanyanın mesaj alıcılarını okur (campaign.Store).
type RecipientReader interface {
	ListRecipients(ctx context.Context, id string, f campaign.RecipientFilter, page, limit int) (campaign.RecipientPage, error)
}

// maxRecipientSearchRunes: arama metni için makul bir üst sınır (ad, e-posta veya telefon).
const maxRecipientSearchRunes = 100

type recipientResponse struct {
	ID        int64   `json:"id"`
	UserID    int64   `json:"user_id"`
	FirstName *string `json:"first_name"`
	Channel   string  `json:"channel"`
	// Address, onay anındaki telefon veya e-postadır.
	Address       string     `json:"address"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	SentAt        *time.Time `json:"sent_at"`
	FailureReason *string    `json:"failure_reason"`
}

type recipientListResponse struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	// Total, süzgece uyan mesaj sayısıdır.
	Total int                 `json:"total"`
	Items []recipientResponse `json:"items"`
}

// listRecipients, kampanyanın mesaj alıcılarını (kim, hangi kanal ve adres, ne oldu) sayfalı
// döndürür. Salt okunurdur. Parametreler veritabanı okunmadan doğrulanır: page, limit (en çok 20),
// channel (sms|email), status (queued|sending|sent|failed|skipped) ve q (en çok 100 karakter).
func (s *server) listRecipients(w http.ResponseWriter, r *http.Request) {
	page, limit, ok := pagination(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := campaign.RecipientFilter{Channel: q.Get("channel"), Status: q.Get("status"), Search: q.Get("q")}
	switch f.Channel {
	case "", "sms", "email":
	default:
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Kanal sms veya email olmalı.")
		return
	}
	switch f.Status {
	case "", "queued", "sending", "sent", "failed", "skipped":
	default:
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Mesaj durumu geçersiz.")
		return
	}
	if !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > maxRecipientSearchRunes {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Arama metni en fazla 100 karakter olabilir.")
		return
	}
	if s.Recipients == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Alıcı listesi şu an kullanılamıyor.")
		return
	}

	result, err := s.Recipients.ListRecipients(r.Context(), r.PathValue("id"), f, page, limit)
	if err != nil {
		if errors.Is(err, campaign.ErrNotFound) {
			writeError(w, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", "Kampanya bulunamadı.")
			return
		}
		s.internalError(w, r, "alıcılar okunamadı", err)
		return
	}

	resp := recipientListResponse{Page: page, Limit: limit, Total: result.Total,
		Items: make([]recipientResponse, 0, len(result.Items))}
	for _, it := range result.Items {
		resp.Items = append(resp.Items, recipientResponse{
			ID: it.MessageID, UserID: it.UserID, FirstName: it.FirstName, Channel: it.Channel,
			Address: it.Address, Status: it.Status, Attempts: it.Attempts, SentAt: it.SentAt,
			FailureReason: it.FailureReason,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}
