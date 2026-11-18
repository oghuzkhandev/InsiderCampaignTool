package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/segment"
)

// SegmentGenerator, brief'ten doğrulanmış segment üretir (segment.Generator).
type SegmentGenerator interface {
	Generate(ctx context.Context, brief string) (segment.Segment, error)
}

// AudienceCounter, segmentin kitle sayılarını okur (audience.Counter).
type AudienceCounter interface {
	Count(ctx context.Context, seg segment.Segment, now time.Time) (audience.Counts, error)
}

// CampaignStore, kampanyaları saklar ve okur (campaign.Store).
type CampaignStore interface {
	Create(ctx context.Context, brief string, seg segment.Segment, counts audience.Counts) (campaign.Campaign, error)
	Get(ctx context.Context, id string) (campaign.Campaign, error)
}

// Deps, API'nin bağımlılıklarıdır.
type Deps struct {
	Segments  SegmentGenerator
	Audience  AudienceCounter
	Campaigns CampaignStore
	// Content ve Contents, içerik üretimi ve sürümlerinin saklanması içindir.
	Content  ContentGenerator
	Contents ContentStore
	// Approver, kampanyayı onaylar ve gönderim işlerini yazar.
	Approver CampaignApprover
	// SSEWriteTimeout, akışta her yazma için tanınan süredir; her yazmadan
	// önce yenilenir. Sıfırsa 10 saniye kullanılır.
	SSEWriteTimeout time.Duration
	// Now, referans zamandır (30 günlük pencere gibi). Boşsa time.Now.
	Now func() time.Time
	// Logger boşsa slog.Default kullanılır.
	Logger *slog.Logger
}

type server struct {
	Deps
}

// New, /api/ altındaki rotaları sunan bir handler döndürür. Bilinmeyen rota
// 404, yanlış yöntem 405 olarak JSON hata gövdesiyle döner.
func New(d Deps) http.Handler {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	s := &server{Deps: d}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/campaigns", s.createCampaign)
	mux.HandleFunc("/api/campaigns", methodNotAllowed(http.MethodPost))
	mux.HandleFunc("GET /api/campaigns/{id}", s.getCampaign)
	mux.HandleFunc("/api/campaigns/{id}", methodNotAllowed(http.MethodGet))
	mux.HandleFunc("POST /api/campaigns/{id}/content", s.generateContent)
	mux.HandleFunc("PUT /api/campaigns/{id}/content", s.putContent)
	mux.HandleFunc("/api/campaigns/{id}/content", methodNotAllowed(http.MethodPost+", "+http.MethodPut))
	mux.HandleFunc("POST /api/campaigns/{id}/approve", s.approveCampaign)
	mux.HandleFunc("/api/campaigns/{id}/approve", methodNotAllowed(http.MethodPost))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Böyle bir adres yok.")
	})
	return mux
}

func methodNotAllowed(allow string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", allow)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Bu adres için yöntem desteklenmiyor.")
	}
}
