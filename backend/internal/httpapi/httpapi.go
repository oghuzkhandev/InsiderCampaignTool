package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/audience"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/campaign"
	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/metrics"
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
	// Deliveries, gönderim durumunu okur (dashboard olay akışı).
	Deliveries DeliveryReader
	// DeliveryInterval, dashboard akışında iki durum görüntüsü arasındaki süredir.
	// Değişiklik olmasa da her aralıkta görüntü yollanır. Sıfırsa 1 saniye.
	DeliveryInterval time.Duration
	// DeliveryQueryTimeout, tek bir durum okumasının süre sınırıdır; aşılırsa akışa
	// `error` olayı yollanıp kapatılır. Sıfırsa 3 saniye.
	DeliveryQueryTimeout time.Duration
	// Shutdown, sunucu kapanmaya başlayınca kapanan kanaldır. Dashboard gibi uzun
	// ömürlü akışlar bunu izleyip kendiliğinden biter; aksi halde sunucunun düzgün
	// kapanışı bu akışları sonuna kadar beklerdi. Nil ise hiç kapanmaz.
	Shutdown <-chan struct{}
	// LLMValidationFailures, model çıktısı doğrulamadan geçemeyince artırılır
	// (`llm_validation_failures_total`). Nil ise sayılmaz.
	LLMValidationFailures *metrics.Counter
	// SSEWriteTimeout, akışta her yazma için tanınan süredir; her yazmadan
	// önce yenilenir. Sıfırsa 10 saniye kullanılır.
	SSEWriteTimeout time.Duration
	// Now, referans zamandır (30 günlük pencere gibi). Boşsa time.Now.
	Now func() time.Time
	// Logger boşsa slog.Default kullanılır.
	Logger *slog.Logger
}

// Validate, API'nin çalışması için zorunlu bağımlılıkların bağlı olduğunu
// doğrular. Eksik biri ilk istekte nil işaretçi paniğine dönüşürdü; üretimde
// başlangıçta (cmd/api) çağrılır ve süreç hiç başlamaz. New bilerek toleranslıdır:
// testler yalnızca ilgili rotanın bağımlılıklarını verir. Zorunlu olmayanlar
// (Now, Logger, süreler, Shutdown, LLMValidationFailures) boş kalabilir.
func (d Deps) Validate() error {
	missing := []string{}
	for _, dep := range []struct {
		name    string
		unbound bool
	}{
		{"Segments", d.Segments == nil},
		{"Audience", d.Audience == nil},
		{"Campaigns", d.Campaigns == nil},
		{"Content", d.Content == nil},
		{"Contents", d.Contents == nil},
		{"Approver", d.Approver == nil},
		{"Deliveries", d.Deliveries == nil},
	} {
		if dep.unbound {
			missing = append(missing, dep.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("httpapi: zorunlu bağımlılıklar bağlanmamış: %s", strings.Join(missing, ", "))
	}
	return nil
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
	mux.HandleFunc("GET /api/campaigns/{id}/events", s.deliveryEvents)
	mux.HandleFunc("/api/campaigns/{id}/events", methodNotAllowed(http.MethodGet))
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
