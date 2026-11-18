## Repository Rehberi

Projeye başlamak için önce [case brief'ini](CASE.md) oku. Güncel işi ve açık
konuları [ilerleme notunda](PROGRESS.md) bulabilirsin.

## Uygulama klasörleri

- backend/ — Go API, Claude ve MockLLM entegrasyonu, segment doğrulama,
  PostgreSQL erişimi, kuyruk worker'ı ve mock SMS/e-posta sağlayıcısı.
  Migration,
  demo kullanıcı verisini oluşturan seed ve Go testleri de burada yer alır.
  - backend/cmd/ — Çalıştırılabilir komutlar: `api`, `migrate` (bekleyen
    migration'ları uygular) ve `seed` (demo kullanıcılarını yazar).
  - backend/internal/ — Uygulama paketleri. Unit testler (`*_test.go`)
    test ettikleri paketin yanında durur.
    - `httpapi` — API'nin HTTP katmanı (yönlendirme, handler'lar, durum
      kodları). İş kuralı ve SQL içermez.
    - `database` — PostgreSQL bağlantısı ve migration çalıştırma.
    - `seed` — Demo kullanıcıları ve kanal izinleri.
    - `campaign` — Draft kampanyayı kaydetme ve kimliğiyle okuma; doğrulanmış
      içerik sürümlerini kilit altında kaydetme, son sürümü ve en uzun adı
      okuma (`ContentStore`); onay (`Approver`): kilit altında sürüm ve
      alıcı doğrulaması, onay ve `queued` mesajların tek transaction'da yazımı.
    - `audience` — Segment sorgusunu çalıştırıp üç kitle sayısını ve onaydaki
      alıcı listesini okuma.
    - `segment` — Segment doğrulama ve parametreli SQL üretimi.
    - `content` — SMS ve e-posta içeriğinin doğrulanması (şema, değişken
      whitelist'i, uzunluk), alıcıya göre render, akışın artımlı çözümü
      (`delta`) ve bir kez düzeltmeli üretim (`Generator`).
    - `llm` — Model arayüzleri (segment için `Client`, akışlı içerik için
      `ContentClient`), MockLLM ve test istemcisi.
    - `testdb` — Entegrasyon testleri için `campaigns_test` hazırlığı.
  - backend/migrations/ — Veritabanı migration dosyaları. Yalnızca
    Oğuzhan isteyince eklenir.
  - backend/tests/integration/ — Gerçek PostgreSQL ile çalışan testler
    (kısıtlar ve tetikleyiciler, iki bağlantılı kilit ve eşzamanlılık
    testleri, migration Down/Up, içerik API'si).
  - backend/tests/e2e/ — API, worker ve mock provider ile uçtan uca testler.
  - Integration ve e2e dosya adı, testin çözdüğü konunun bir veya en fazla iki
    küçük harfli kelimesidir (`idempotency_test.go`, `rate-limit_test.go`).
- frontend/ — Vue ve TypeScript arayüzü. Brief girişi, kitle önizlemesi,
  canlı içerik, taslak düzenleme, insan onayı ve gönderim takibi burada gelişir.
  - frontend/src/api/ — API istemcisi (kampanya, içerik, onay), SSE ayrıştırıcı
    (`sse.ts`) ve hata mesajları.
  - frontend/src/segment/ — Segmentin okunur özeti ve sebep kodlarının Türkçe
    açıklamaları.
  - frontend/src/content/ — İçerik taslağı yardımcıları (sınırlar, uzunluk,
    akış parçalarının uygulanması) ve içerik/onay hata metinleri.
  - frontend/src/components/ ve views/ — Bileşenler ve ekranlar:
    `CampaignPreviewView` (`/`, brief ve kitle önizleme) ve `CampaignView`
    (`/campaigns/:id`, içerik üretimi, düzenleme ve onay). Testler yanındaki
    `__tests__` klasöründedir.
  - frontend/e2e/ — Playwright uçtan uca testleri ve yardımcıları. Her test kendi
    kampanyasını oluşturur. `make e2e` ile izole ortama karşı koşar (ADR-024).
  - frontend/src/components/ui/ — `shadcn-vue` ile üretilmiş bileşenler
    (ADR-021). Elle düzenlenmez, eslint dışındadır. `components.json` ve
    `src/lib/utils.ts` shadcn yapılandırmasıdır.

## Kök dosyalar

- [README.md](../README.md) — Reviewer için proje özeti, doğrulanmış kurulum
  ve test komutları, demo adımları ve bilinen sınırlamalar.

- [CLAUDE.md](../CLAUDE.md) — Claude Code'un proje talimatları. Brief,
  güncel görev, test komutları ve kritik çalışma kurallarına yönlendirir.

- `compose.e2e.yaml` — İzole e2e ortamı (ayrı Compose projesi, ayrı port,
  diske yazmayan veritabanı, kontrollü veri). `compose.yaml` ile birlikte
  kullanılır; `make e2e`, `make e2e-up` ve `make e2e-down` hedefleri bunu
  çalıştırır.

- [DECISIONS.md](../DECISIONS.md) — Oğuzhan'ın verdiği önemli teknik
  kararlar
  ve gerekçeleri. Claude seçenekleri ve trade-off'ları açıklar. Oğuzhan seçimi
  yaptıktan sonra Claude onaylanan kararı ve gerekçesini kaydedebilir.

## Proje dokümantasyonu

docs/, brief'i, teknik tasarımı ve geliştirme notlarını bir arada tutar.

- [CASE.md](CASE.md) — Insider One'ın orijinal brief'i ve teslim şartları.
  Gereksinimlerin ana kaynağı.

- [ARCHITECTURE.md](ARCHITECTURE.md) — Vue, Go API, Claude, PostgreSQL,
  worker ve mock provider arasındaki veri akışı ve sorumluluklar.

- [FAILURE-MODES.md](FAILURE-MODES.md) — Geçersiz model çıktısı, kesilen
  stream, HTTP 429 ve kaybolan sağlayıcı yanıtı gibi durumlarda beklenen
  davranış. Varsa ilgili teste veya spec'e bağlantı verir.

- [PROGRESS.md](PROGRESS.md) — Biten işler, sıradaki görev ve açık konular.
  GET /healthz gibi başlangıç görevleri burada takip edilir.

## Özellik şartnameleri

docs/specs/ gereksinimleri “Beklenen davranış” altında numarasız, kısa ve
test edilebilir maddelerle tarif eder. Özellik kimlikleri korunur. Örneğin
“İnsan onayı verilmeden gönderim işi oluşturulamaz.”

- [README.md](specs/README.md) — Özellik şartnamelerinin dizini.

- [TEMPLATE.md](specs/TEMPLATE.md) — Yeni spec için özellik kimliği, amaç,
  beklenen davranış ve doğrulama başlıklarını içeren şablon.
İnceleme notları
docs/reviews/ önemli değişikliklerin inceleme bulgularını ve doğrulama
sonuçlarını tutar. Örneğin provider yanıtı kaybolduğunda retry'ın çift gönderim
üretmediğini gösteren test burada referans verilebilir.

- [TEMPLATE.md](reviews/TEMPLATE.md) — İnceleme kapsamı, bulgular,
  çalıştırılan kontroller ve açık noktalar için şablon.
AI çalışma oturumları
ai-sessions/, case boyunca gerçekleştirilen Claude Code oturumlarının
export'larını veya Claude.ai paylaşım bağlantılarını içerir. Paylaşmadan önce
gerçek anahtarlar ve kişisel bilgiler temizlenir.

- [ai-sessions/README.md](../ai-sessions/README.md) — Oturumların tarihi,
  aracı, konusu ve ilgili export dosyası veya paylaşım bağlantısı.
  