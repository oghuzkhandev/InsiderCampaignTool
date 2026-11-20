## Repository Rehberi

Projeye başlamak için önce [case brief'ini](CASE.md) oku. Güncel işi ve açık
konuları [ilerleme notunda](PROGRESS.md) bulabilirsin.

## Uygulama klasörleri

- backend/ — Go API, Claude ve MockLLM entegrasyonu, segment doğrulama,
  PostgreSQL erişimi, kuyruk worker'ı ve mock SMS/e-posta sağlayıcısı.
  Migration,
  demo kullanıcı verisini oluşturan seed ve Go testleri de burada yer alır.
  - backend/cmd/ — Çalıştırılabilir komutlar: `api`, `migrate` (bekleyen
    migration'ları uygular), `seed` (demo kullanıcılarını yazar), `worker`
    (kuyruk işleyici) ve `mock-provider` (sahte SMS/e-posta sağlayıcısı).
  - backend/internal/ — Uygulama paketleri. Unit testler (`*_test.go`)
    test ettikleri paketin yanında durur.
    - `httpapi` — API'nin HTTP katmanı (yönlendirme, handler'lar, durum
      kodları). İş kuralı ve SQL içermez.
    - `database` — PostgreSQL bağlantısı ve migration çalıştırma.
    - `seed` — Demo kullanıcıları ve kanal izinleri.
    - `campaign` — Draft kampanyayı kaydetme, kimliğiyle okuma ve son kampanyaları
      listeleme (`Store.List`, CMP-001); doğrulanmış
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
    - `worker` — Kuyruk işleyici: `FOR UPDATE SKIP LOCKED` ile claim (`PGRepo`),
      kanal başına hız sınırı (`IntervalLimiter`), provider istemcisi
      (`HTTPProvider`, sonuç sınıflandırması), retry kararı ve backoff
      (`Decide`, `Backoff`), reaper, token'lı sonuç yazımı, kampanya tamamlama,
      sayaçlar (`Metrics`, `ratelimit_wait_seconds` histogramı dahil, Prometheus
      çıktısı) ve ortam değişkenleriyle yapılandırma.
    - `metrics` — Bağımlılıksız Prometheus metin biçimi: sayaç, histogram,
      yazıcı, `/metrics` handler'ı ve ana porttan ayrı iç dinleyici (`Serve`).
      API ve worker aynı paketi kullanır (ADR-016, ADR-027).
    - `mockprovider` — Sahte SMS/e-posta sağlayıcısı: HTTP arayüzü, hata ve
      gecikme enjeksiyonu, `accept_then_drop` ve `drop_all_responses` modları,
      PostgreSQL'e atomik kabul kaydı (`PGStore`), bellekte istatistikler ve
      kayan pencere ölçümü (`MaxInWindow`).
    - `testdb` — Entegrasyon testleri için `campaigns_test` hazırlığı ve alt
      süreçlere verilen `campaigns_test` adresi (`URL`).
  - backend/migrations/ — Veritabanı migration dosyaları. Yalnızca
    Oğuzhan isteyince eklenir.
  - backend/tests/integration/ — Gerçek PostgreSQL ile çalışan testler
    (kısıtlar ve tetikleyiciler, iki bağlantılı kilit ve eşzamanlılık
    testleri, migration Down/Up, içerik API'si).
  - backend/tests/e2e/ — Gerçek `api`, `worker` ve `mock-provider` ikilileri alt
    süreç olarak (yalnızca `campaigns_test`'e bağlı): yanıt kaybı ve idempotency,
    provider yeniden başlatma, worker `SIGKILL`/`SIGTERM`, veritabanı
    kesintisi, kayan pencere hız sınırı ve gözlemlenebilirlik (`/readyz`
    kesinti ve toparlanma, `queue_depth`, worker sayaçları). `make test` içinde koşar.
  - Integration ve e2e dosya adı, testin çözdüğü konunun bir veya en fazla iki
    küçük harfli kelimesidir (`idempotency_test.go`, `rate-limit_test.go`).
- frontend/ — Vue ve TypeScript arayüzü. Brief girişi, kitle önizlemesi,
  canlı içerik, taslak düzenleme, insan onayı ve gönderim takibi burada gelişir.
  - frontend/src/api/ — API istemcisi (kampanya, içerik, onay, gönderim takibi
    `delivery.ts`), SSE ayrıştırıcı (`sse.ts`) ve hata mesajları. Dışarıdan gelen
    her yanıt `unknown` olarak alınıp burada doğrulanır.
  - frontend/src/segment/ — Segmentin okunur özeti ve sebep kodlarının Türkçe
    açıklamaları.
  - frontend/src/content/ — İçerik taslağı yardımcıları (sınırlar, uzunluk,
    akış parçalarının uygulanması, örnek adla önizleme metni) ve içerik/onay hata metinleri.
    `useUnsavedChangesGuard` kaydedilmemiş metin varken sayfadan ayrılırken uyarır
    (sekmeyi kapatma/yenileme için tarayıcı uyarısı, uygulama içi gezinme için onay).
  - frontend/src/delivery/ — Gönderim takibi (TRK-001) mantığı: mesaj sayılarından
    özetler (`summary.ts`), Türkçe metinler (`messages.ts`) ve canlı takip
    bağlantısının yaşam döngüsü (`useDeliveryStream.ts`: yeniden bağlanma, takılan
    bağlantıyı fark etme, kampanya değişince eski akışı kapatma).
  - frontend/src/campaign/ — Kampanya akışının dört adımı (Kitle, İçerik, Onay, Gönderim)
    ve bir kampanyanın hangi adımda olduğunu durumdan türeten saf mantık (`steps.ts`).
  - frontend/src/components/ ve views/ — Bileşenler ve ekranlar:
    `CampaignPreviewView` (`/`, brief ve kitle önizleme), `CampaignView`
    (`/campaigns/:id`, içerik üretimi, düzenleme ve onay; geniş ekranda iki sütun) ve
    `DeliveryDashboardView` (`/campaigns/:id/delivery`, canlı gönderim takibi;
    `DeliveryConnectionStatus` ve `DeliveryCounts` bileşenleriyle). Hepsi shadcn
    bileşenleriyle yazılır, scoped CSS yoktur (ADR-021). Üç ekranın ortak parçaları:
    `PageHeader` (başlık, durum rozeti, açıklama), `CampaignSteps` (adım göstergesi) ve
    `StepNav` (her aşamada Türkçe "Geri" / "İleri"; hedef açılamıyorsa kapalı düğme ve
    nedeni; kampanya ekranında dar ekranda alta yapışır). `MessagePreview` mesajın örnek
    adla ({{first_name}} yerine "Ayşe") nasıl görüneceğini gösterir ve düzenleyicide ile
    onay özetinde (`ApprovePanel`: sürüm, tahmini alıcılar, önizleme) kullanılır.
    Uygulama kabuğu `App.vue`'dadır: üst başlık (kenar çubuğu düğmesi ve ana sayfaya dönen
    "Campaign Tool" bağlantısı) ve `AppSidebar` (shadcn `Sidebar`: "Yeni kampanya" ve
    `RecentCampaigns`). `RecentCampaigns` "Son kampanyalar" listesidir (CMP-001; taslak
    kampanya kampanya ekranına, onaylı olan gönderim takibine açılır; hedefler
    `campaign/links.ts`'te). Liste, ekranların `campaign/recent.ts` üzerinden verdiği haberle
    ve sayfa değişince yenilenir. `StatusBadge` kampanya durumunu simge, metin ve tonla
    gösterir. `ErrorPanel` hata gösteren ortak `Alert`'tir. Testler yanındaki `__tests__`
    klasöründedir.
  - frontend/src/exampleBrief.ts ve `exampleBriefs.json` — Boş ekranda önerilen 2-3 örnek
    brief. Liste tek kaynaktır: backend entegrasyon testi
    (`backend/tests/integration/example-briefs_test.go`) aynı JSON dosyasını okuyup her
    örneğin desteklendiğini ve demo verisinde alıcı bulduğunu doğrular.
  - frontend/e2e/ — Playwright uçtan uca testleri ve yardımcıları. Her test kendi
    kampanyasını oluşturur. `make e2e` ile izole ortama karşı koşar (ADR-024).
    `visual-themes.spec.ts` ana ekranların açık ve koyu temada kontrastını ve imleç
    kuralını, `mobile-layout.spec.ts` 360 px'te yatay taşma olmadığını ve ana eylemlerin
    dokunma hedefi boyutunu (≥ 40 px) ve uzun brief/hata metinlerinin taşmadığını,
    `flow-navigation.spec.ts` örnek brief önerilerini, Geri/İleri gezintisini ve
    kaydedilmemiş değişiklik uyarısını, `campaign-list.spec.ts` "Son kampanyalar" listesini
    ve `GET /api/campaigns` sınırlarını, `app-shell.spec.ts` üst başlığı ("Campaign Tool"
    ana sayfaya döner) ve kenar çubuğunu (açma/kapama, liste güncellenmesi, uyarı) sınar.
    `delivery-dashboard.spec.ts` gönderim takibinin hata, yeniden bağlanma ve mobil
    senaryolarını sınar: yalnızca olay akışı taklit edilir, kampanya ve onay
    gerçektir. `delivery-real.spec.ts` (`@real`) hiçbir şeyi taklit etmez: gerçek
    worker, mock provider ve SSE ile gönderimin tamamlandığını gösterir
    (`make e2e-delivery`, `delivery` Compose profili).
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
  kullanılır; `make e2e` (üç döngü), `make e2e-up` ve `make e2e-down` hedefleri bunu
  çalıştırır. `worker` ve `mock-provider` bu ortamda yalnızca `delivery`
  profiliyle açılır: mevcut Playwright testleri onaydan sonra kampanyanın
  `approved` kaldığını doğrular. `prometheus` yalnızca `observability` profiliyle açılır.

- `observability/` — İsteğe bağlı Prometheus (ADR-028): `prometheus.yml` (scrape
  hedefleri `api:9090`, `worker:9090`) ve `verify.sh` (`make e2e-observability`:
  gerçek scrape'te iki hedef `up`, gerçek metrikler, saklama sınırı, worker durunca
  `up=0`). `make metrics-lint` `promtool` ile çıktıyı ve yapılandırmayı denetler.

- `frontend/Dockerfile` ve `frontend/nginx.conf` — Çok aşamalı imaj (derleme + nginx)
  ve proxy yapılandırması (ADR-014): tek sayfa yönlendirmesi, önbellek başlıkları,
  `/api` proxy'sinde buffering kapalı.

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
  