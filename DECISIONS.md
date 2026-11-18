# Teknik Kararlar

Bu dosya projedeki mimari kararları ve gerekçelerini kaydeder. **Taslak**
durumundaki kararlar proje sahibinin onayını bekler. Bileşenler ve uygulama
akışları [mimari belgede](docs/ARCHITECTURE.md) açıklanır.

## ADR-001 — Backend dili Go

- **Durum:** Taslak.
- **Bağlam:** Brief, backend için Go ve PHP'ye izin veriyor.
- **Karar:** Go kullanılır. API, worker, mock provider, seed ve migration
  aynı Go modülünde ayrı komutlardır. Ortak backend Docker imajını kullanırlar.
- **Gerekçe ve sonuç:** Go, HTTP streaming, iptal, zaman aşımı ve sınırlı
  eşzamanlılık için standart araçlar sunar. Ortak kod aynı modülde kalır.

## ADR-002 — PostgreSQL kuyruk olarak kullanılır

- **Durum:** Taslak.
- **Seçenekler:** PostgreSQL, RabbitMQ ve Redis Streams.
- **Karar:** Kampanya onayı ve gönderim işleri aynı PostgreSQL transaction'ında
  yazılır. Worker işleri `FOR UPDATE SKIP LOCKED` ile sahiplenir ve
  provider çağrısını transaction kapandıktan sonra yapar.
- **Gerekçe ve sonuç:** Onay ve kuyruk kaydı atomik olur. Dashboard aynı mesaj
  kayıtlarından beslenir. Bu yaklaşım polling gecikmesi getirir ve yüksek
  hacimde kuyruk ile diğer veritabanı işlemleri kaynak paylaşır.

## ADR-003 — Segment çıktısı doğrulanmış JSON DSL olur

- **Durum:** Taslak.
- **Karar:** Claude, mantıksal alan ve operatör içeren yapılandırılmış JSON
  üretir. Backend bunları sabit eşlemelerden SQL'e çevirir, değerleri SQL
  parametresi olarak bağlar.
- **Gerekçe ve sonuç:** Sorgunun yapısı backend'in kontrolündedir.
  Bilinmeyen alan, geçersiz operatör, uyumsuz tip, geçersiz aralık, boş
  filtre ve aşırı iç içe yapı reddedilir. Beklenen davranışlar `SEG-001`
  spec'inde tanımlanır.
- **Güncelleme (onaylandı):** DSL bu aşamada düz bir AND listesidir. Çıktı
  zorunlu bir `unsupported` listesi taşır ve doluysa reddedilir. Otomatik
  düzeltme yalnızca söz dizimi hatasında (bozuk JSON, şema, değer tipi)
  yapılır. Desteklenmeyen alan, operatör ve yapıda düzeltme yapılmaz.
  Geçersiz segment `422` `SEGMENT_INVALID` ve sebep kodlarıyla döner; kitle
  sorgusu çalışmaz, kampanya oluşmaz, arayüz sebebi gösterir ve brief
  düzenlenebilir kalır. Proje sahibinin gerekçesi: kullanıcı neyin
  desteklenmediğini görmeli ve brief'i düzenleyebilmeli. Düzeltme, koşulu
  sessizce silip kitleyi genişletmemeli. `unsupported` beyanı güvenilir
  değildir; geçerli ama farklı yorumlanan çıktıyı backend tespit edemez ve
  kural özeti ile insan onayı bu riski azaltır.

## ADR-004 — Kanal izni onayda ve gönderimde kontrol edilir

- **Durum:** Taslak.
- **Karar:** Onay sırasında yalnızca ilgili kanal için izni olan alıcılara
  iş oluşturulur. Worker provider çağrısından hemen önce izni tekrar kontrol
  eder. Bu kontrol hız sınırı beklemesinden sonra yapılır. İzin kaydı yoksa
  veya izin geri çekilmişse mesaj `skipped` olur. Yalnızca açıkça izin
  verilmiş kullanıcılar ilgili kanalın uygun alıcı sayısına ve iş kayıtlarına
  dahil edilir. Uygunluk ayrıca ilgili kanalın adresini (telefon/e-posta) ister
  (ADR-023).
- **Gerekçe ve sonuç:** İzin, onay ile gönderim arasında değişebilir.
  Önizleme toplam kitleyi, SMS'e uygun kişileri ve e-postaya uygun kişileri
  ayrı gösterir.

## ADR-005 — Tek worker süreci ve kanal başına hız sınırı

- **Durum:** Taslak.
- **Karar:** Compose demosunda tek worker süreci çalışır. SMS ve e-posta
  ayrı limiter kullanır. İlk ayar kanal başına 95 istek/sn ve burst 1.
  Retry dahil her provider çağrısı limite tabidir.
- **Gerekçe ve sonuç:** Tek süreçte tüm kanal denemeleri aynı limiter'dan
  geçer. 95 istek/sn yapılandırma hedefidir. Brief'teki 100 istek/sn sınırı,
  provider'ın aldığı istekler üzerinden kayan bir saniyelik pencereyle
  test edilir. Replikalar arasında ortak limit bu sürümün kapsamı dışındadır.

## ADR-006 — İş sahipliği, retry ve idempotency

- **Durum:** Taslak. Tekilleştirme kapsamı onaylandı.
- **Karar:** `attempts` işleme denemelerini sayar. `claim_token` her
  sahiplenmede yenilenir. Provider'a iletilen `Idempotency-Key` ise mesaj
  boyunca sabit kalır. Sonuç yalnızca güncel claim token ile yazılabilir.
- **Retry:** `429`, `5xx`, timeout ve geçici bağlantı hataları backoff ile
  yeniden denenir. `429` için geçerli `Retry-After` dikkate alınır.
  Diğer `4xx` yanıtları kalıcı hata sayılır.
- **Kurtarma:** En fazla beş claim yapılır. Lease'i dolan işin deneme hakkı
  varsa yeniden kuyruğa alınır. Hak bittiyse iş hata sebebiyle kapatılır.
- **Gerekçe ve sonuç:** `UNIQUE(campaign_id, user_id, channel)` iş kaydını
  tekilleştirir. Mock provider, anahtarı ve ilk kabul sonucunu PostgreSQL'e
  atomik kaydederek gönderimi tekilleştirir. Kabul edilmiş bir işin yanıtı
  tüm denemelerde kaybolursa mesaj `failed`, `failure_reason = exhausted`
  olur. Arayüz “Gönderim sonucu doğrulanamadı; deneme sınırı doldu”
  açıklamasını gösterir. Bu, provider’ın mesajı kesinlikle kabul etmediği
  anlamına gelmez.
- **İş sahiplenme:** Worker yalnızca boş gönderim kapasitesi kadar iş
  sahiplenir. Limiter beklemesi lease süresine dahildir; provider çağrısından
  önce güncel sahiplik ve lease geçerliliği kontrol edilir. Geçerliliğini
  kaybetmiş iş için yeni provider çağrısı başlatılmaz.
- **Doğrulama:** İlk kabulden sonra HTTP yanıtı düşürülür. Retry aynı
  anahtarı kullanır. İki HTTP isteği, bir kabul ve sonunda `sent` beklenir.
- **Tekilleştirme kapsamı (onaylandı):** Tekrar gönderim koruması kampanya,
  kullanıcı ve kanal bazındadır. Ayrı kampanyalar birbirinden bağımsızdır;
  brief veya metnin aynı olması kampanyalar arası gönderimi engellemez.
  Bu kapsam `UNIQUE (campaign_id, user_id, channel)` ve kampanya kapsamlı
  `Idempotency-Key` (`campaign:user:channel`) ile uygulanır. Kampanyalar
  arası tekrar gönderim koruması kapsam dışıdır.

## ADR-007 — İçerik ve dashboard için ayrı SSE akışları

- **Durum:** Taslak.
- **Seçenekler:** SSE, WebSocket ve polling.
- **Karar:** İçerik üretiminde `delta`, `validated` ve `error` olayları
  kullanılır. `validated` yalnızca doğrulanmış sürüm kaydedildikten sonra
  gönderilir. Dashboard her bağlantıda güncel snapshot ile başlar.
- **Gerekçe ve sonuç:** İki akış da sunucudan tarayıcıya veri taşır.
  Yarım kalan üretim yeniden başlatılabilir. Dashboard bağlantısı
  yenilendiğinde veritabanındaki güncel durumu alır.

## ADR-008 — Uzunluk kişiselleştirmeden sonra doğrulanır

- **Durum:** Taslak.
- **Karar:** Şema ve değişkenler taslak kaydında doğrulanır. Onayda, uygun
  her alıcı için oluşturulan SMS ve e-posta konusu tekrar kontrol edilir.
  Sınırlar SMS için 160, e-posta konusu için 80 Unicode kod noktasıdır.
- **Gerekçe ve sonuç:** `{{first_name}}` gibi değişkenler son metnin
  uzunluğunu değiştirir. Bir alıcının metni sınırı aşarsa onay `422` döner
  ve hiçbir iş oluşmaz. Gerçek SMS kodlama ve segment hesabı kapsam dışıdır.

## ADR-009 — LLM çıktısı JSON olur ve backend'de doğrulanır

- **Durum:** Onaylandı.
- **Karar:** CASE gereği model hem segment hem içerik için yalnızca
  yapılandırılmış JSON üretir. İçerik JSON'u `sms_text`, `email_subject` ve
  `email_body` alanlarını içerir. Backend JSON akışını birleştirirken
  çözümlenen metin parçalarını alan adıyla birlikte frontend'e JSON verisi
  taşıyan SSE olaylarıyla aktarır. Bunlar geçici önizlemedir. Tamamlanan
  çıktı JSON şeması ve içerik kurallarından geçmeden sürüm kaydedilmez.
- **Düzeltme:** Tamamlanmış segment veya içerik JSON'u doğrulanamazsa hata
  bilgileriyle bir otomatik düzeltme denemesi yapılır. Segment için düzeltme
  yalnızca söz dizimi hatasında yapılır (ADR-003). İçerik düzeltmesi
  arka planda biriktirilir; doğrulanıp kaydedilirse önizlemenin tamamını
  değiştirir, önceki metne eklenmez. `validated` olayı kaydedilen içeriğin
  tamamını ve sürümünü taşır. Düzeltme de başarısızsa yeni sürüm kaydedilmez;
  çözümlenebilmiş metinler hata açıklamasıyla düzenlemeye açılır. Hiç metin
  çözümlenememişse kullanıcı yeniden üretebilir veya alanları doldurabilir.
  Onay yalnızca doğrulanmış ve kaydedilmiş içerik üzerinden yapılabilir.
  Akış kesintisinde otomatik düzeltme yapılmaz; yeniden üretme sunulur.
- **Gerekçe ve sonuç:** CASE'in JSON doğrulama ve canlı içerik gereksinimleri
  birlikte karşılanır. Vue sağlayıcıya özel çıktı biçimini bilmez.
  `LLM_PROVIDER=mock` varsayılandır. Testler gerçek API anahtarı olmadan
  geçerli JSON, bozuk JSON, düzeltme ve kesilen akışı çalıştırabilir.

## ADR-010 — Alışveriş verisi kullanıcı düzeyinde tutulur

- **Durum:** Taslak.
- **Karar:** `last_cart_add_at`, `last_purchase_at` ve `order_count`
  kullanıcı üzerinde tutulur. Örnek brief için önerilen yorum, son 30 günde
  sepete ekleme olması ve aynı dönemde satın alma olmamasıdır.
- **Gerekçe ve sonuç:** Bu kural alan-değer karşılaştırmalarıyla ifade
  edilebilir. Ürün düzeyinde eşleştirme kapsam dışıdır. 20 gün önce satın
  alıp 5 gün önce sepete ekleyen kullanıcı, bu yorumda segment dışında kalır.

## ADR-011 — Onay içerik sürümüne bağlanır

- **Durum:** Onaylandı (proje sahibi, 2026-11-17).
- **Karar:** Onay isteği `content_version` taşır. Onaylanan aynı sürüm
  yeniden gönderilirse mevcut sonuç `200` ile döner. Sürüm çatışması veya
  doğrulanmış sürümün bulunmaması `409`, boş kitle veya render ihlali `422`
  döner.
- **Gerekçe ve sonuç:** Worker onay sırasında doğrulanan metni gönderir.
  Tüm mesajlar terminal duruma ulaştığında kampanya `completed` olur.
  Başarısız gönderimler ayrı sayılır. Onay transaction'ı geri alınırsa
  kampanya `draft` durumunda kalır.

## ADR-012 — Uygulama ve test için ayrı PostgreSQL database'leri

- **Durum:** Onaylandı.
- **Karar:** Docker ile çalışan tek PostgreSQL instance'ında iki ayrı
  database kullanılır. API ve worker uygulama/demo verileri için `campaigns`,
  entegrasyon testleri ise `campaigns_test` kullanır. İkisine de aynı
  migration'lar uygulanır; her database kendi `users`, `campaigns`,
  `messages` ve diğer uygulama tablolarını içerir.
- **Gerekçe ve sonuç:** Ayrı bir PostgreSQL instance'ı kurmadan test verileri
  uygulama ve demo verilerinden ayrılır. Testlerin veri oluşturma ve temizleme
  işlemleri yalnızca `campaigns_test` üzerinde çalışır; `campaigns` verilerini
  etkilemez. Testler ayrı bağlantı kullanır ve her yazma aşamasından önce
  bağlı database adının tam olarak `campaigns_test` olduğunu doğrular;
  farklıysa durur. Uygulama bağlantısına geri dönüş yapılmaz.

## ADR-013 — Yedi Compose servisi ve başlatma sırası

- **Durum:** Taslak. Servis listesi ve host portları onaylandı; başlatma sırası
  onay bekliyor.
- **Karar:** Compose yedi servis çalıştırır: `postgres`, `migrate`, `seed`,
  `api`, `worker`, `mock-provider` ve `web`. `migrate` ve `seed` tek seferlik
  servislerdir. Backend servisleri aynı imajı farklı komutlarla çalıştırır.
  Önerilen sıra: `postgres` (healthy) → `migrate` (tamamlanır) → `seed`
  (tamamlanır) → `api`, `worker`, `mock-provider` → `web`. Host portları
  yalnızca `127.0.0.1`'e açılır: web `8080`, api `8081`, mock-provider
  `8082`, postgres `5432`.
- **Gerekçe ve sonuç:** Servis listesi proje sahibinin kararıdır ve ileride
  değişmesi beklenmiyor. Migration ve seed ayrı servis olduğu için `api`,
  `worker` ve `mock-provider` şema ve veri hazır olmadan başlamaz. Portlar
  yalnızca yerel makineden erişilebilir. Detaylar
  [ARCHITECTURE.md §14](docs/ARCHITECTURE.md#14-compose-servisleri-ve-portlar)
  bölümündedir.

## ADR-014 — Web, nginx ile servis edilir ve proxy buffering kapalı olur

- **Durum:** Onaylandı.
- **Seçenekler:** Vite dev server veya derlenmiş statik dosyaları servis eden
  nginx.
- **Karar:** Vue uygulaması çok aşamalı build ile derlenir ve nginx ile
  servis edilir. `/api` istekleri aynı origin üzerinden API'ye proxy'lenir.
  Bu proxy'de buffering kapalıdır.
- **Gerekçe ve sonuç:** Proje sahibinin gerekçesi: yanıtların bekletilmeden
  iletilmesi gerekir. İçerik üretimi ve dashboard SSE akışları nginx
  tarafından biriktirilirse canlı olmaz. Aynı origin sayesinde CORS ayarı
  gerekmez ve `docker compose up` için yerelde `npm` çalıştırmak gerekmez.

## ADR-015 — `make test` yalnızca Postgres'i ayağa kaldırır

- **Durum:** Onaylandı.
- **Seçenekler:** Yalnızca Postgres'i Compose ile ayağa kaldırmak veya tam
  Compose stack'i kullanmak.
- **Karar:** `make test` yalnızca `postgres` servisini Compose ile başlatır.
  Unit ve entegrasyon testleri bu veritabanını kullanır. E2E testleri API,
  worker ve mock provider'ı Go testlerinden alt süreç olarak başlatır;
  worker-kill testi gerçek `SIGKILL` kullanır. Entegrasyon testi eklendiğinde
  test veritabanı PostgreSQL içinde hazırlanır ve ortak migration'lar
  uygulanır.
- **Gerekçe ve sonuç:** Proje sahibinin şartı: test bağlantısı açıkça
  `campaigns_test` database'ine karşılık gelmelidir; bu doğrulanamazsa işlem
  durdurulur ([ADR-012](#adr-012--uygulama-ve-test-için-ayrı-postgresql-databaseleri)).
  Tam stack gerekmediği için testler daha hızlı çalışır ve lease ile backoff
  süreleri testte kısaltılabilir.

## ADR-016 — Worker kendi metrics endpoint'ini sunar

- **Durum:** Onaylandı.
- **Seçenekler:** Worker'a ayrı bir metrics endpoint'i eklemek veya worker
  metriklerini veritabanından üretip API'nin `/metrics` ucunda sunmak.
- **Karar:** Worker counter'larını kendi `/metrics` endpoint'inde sunar.
  `queue_depth{channel}` bir gauge'dur: API, her okunuşta bunu PostgreSQL'deki
  `messages` tablosundan hesaplar ve kendi `/metrics` endpoint'inde sunar.
  API ve worker `/metrics` endpoint'lerine yalnızca Docker iç ağından erişilir;
  host'a yayımlanan portlardan sunulmaz.
- **Gerekçe ve sonuç:** Proje sahibinin gerekçesi: veritabanından metrik
  üretmek ekstra çabadır, yeni bir endpoint eklemek kolaydır ve iç ağdan
  erişilebilir. `queue_depth`'in asıl görevi PostgreSQL'den çekilen verilerle
  güncel bir gauge olmasıdır. Counter'lar süreç belleğinde tutulur; API ve
  worker ayrı süreç olduğu için API, worker counter'larını göremez. Worker
  yeniden başlarsa counter'ları sıfırlanır. `queue_depth` bellekte tutulmadığı
  için worker durduğunda da kuyruk birikimi görünür. API'nin host'a açılan
  portundan sunulmaması, `/metrics` için host'a yayımlanmayan ayrı bir iç
  dinleyici gerektirir.

## ADR-017 — Feature branch ve PR ile main akışı, CI GitHub Actions

- **Durum:** Onaylandı.
- **Seçenekler:** `development` branch'li çok katmanlı akış veya
  `main` + kısa ömürlü feature branch'ler. Merge için PR + squash, PR +
  merge commit veya doğrudan merge. CI için PR ve `main` push'u, yalnızca
  `main` push'u veya her branch push'u.
- **Karar:** `development` branch'i yoktur. `docs/PROGRESS.md`'deki her aşama
  `main`'den açılan ayrı bir feature branch'te geliştirilir ve PR ile
  `main`'e squash merge edilir. CI GitHub Actions ile çalışır ve PR'larda
  ile `main`'e push'ta tetiklenir. İlk kapsam: Compose doğrulaması, backend
  ile frontend lint, test ve build. PostgreSQL entegrasyon, e2e ve worker
  testleri ilgili aşamalarda eklenir.
- **Gerekçe ve sonuç:** Proje sahibinin gerekçesi: projede tek kişi
  çalışıyor, ekstra bir katman ve merge üstüne merge zaman kaybıdır.
  Aşama başına branch geçmişi düzenli tutar. Squash merge `main`'de aşama
  başına tek commit bırakır. CI'ın PR'larda da çalışması, hatayı merge
  öncesi gösterir; ayrı bir workflow gerekmez. `main` için "CI geçmeden
  merge edilmez" kuralı GitHub ayarıdır ve proje sahibi tarafından
  yapılır. Feature branch'e her push'ta CI çalışmaz, o durumda doğrulama
  yerelde yapılır.

## ADR-018 — Test klasör yapısı ve dosya adlandırması

- **Durum:** Onaylandı.
- **Karar:** Unit testler test ettikleri kodun yanında, ilgili klasörde durur.
  Integration testler `backend/tests/integration/`, e2e testler
  `backend/tests/e2e/` altındadır. Bu iki klasördeki dosya adı testin
  çözdüğü asıl konudur: bir veya en fazla iki küçük harfli kelime
  (`idempotency_test.go`, `rate-limit_test.go`). Backend altında
  `migrations/`, `tests/` ve `internal/` klasörleri açıldı.
- **Gerekçe ve sonuç:** Proje sahibinin gerekçesi: hızlı testlerin ayrı
  çalışması ve kritik davranışların gerçek PostgreSQL ile doğrulanması.
  Unit testler kodun yanında durduğu için PostgreSQL gerektirmeden hızlı
  çalışır. PostgreSQL gerektiren integration ve e2e testler `backend/tests/`
  altında ayrışır; bu yüzden ayrı çalıştırılabilirler. Dosya adı testin
  çözdüğü konuyu gösterir.

## ADR-019 — Segment önizleme aşamasının temel seçimleri

- **Durum:** Onaylandı.
- **Karar:**
  - PostgreSQL erişimi için `pgx v5`, migration için `goose` kullanılır.
    Migration SQL dosyaları `embed` ile binary'ye gömülür ve `migrate`
    servisinde çalışır.
  - Seed deterministiktir (sabit tohum, e-posta ile tanınan kullanıcılar).
    Alışveriş tarihleri (`last_cart_add_at`, `last_purchase_at`) yalnızca
    kullanıcı ilk oluşturulurken referans zamana göre yazılır. Tekrar
    çalıştırmada (`ON CONFLICT DO NOTHING`) var olan kullanıcının tarihleri,
    bilgileri ve izinleri değiştirilmez, silinmiş izin kayıtları geri
    eklenmez. (Güncelleme: ilk karar tarihleri her çalışmada yenilemekti;
    proje sahibi bunu yalnızca ilk oluşturmada yazılacak şekilde değiştirdi.)
  - Bu aşamada `draft` kampanya kaydedilir (brief, segment `jsonb`, durum,
    kitle sayıları). İlk migration `users`, `user_consents` ve `campaigns`
    tablolarını içerir. `campaigns.segment` yalnızca doğrulanmış yapıyı
    saklar ve veritabanı bu yapıyı `CHECK` constraint'iyle zorlar.
  - Segment DSL şekli ADR-003 güncellemesindedir (düz AND listesi).
- **Gerekçe ve sonuç:** Proje sahibi önerilen seçenekleri onayladı.
  `pgx`, `FOR UPDATE SKIP LOCKED` ve transaction'lar için uygun native
  sürücüdür; `goose` SQL migration'larını `embed` ile sade biçimde çalıştırır.
  Brief "son 30 gün" dediği için tarihler referans zamana göre üretilir;
  seed var olan veriye dokunmadığı için tekrar çalıştırmak kayıtları
  çoğaltmaz ve elle yapılan değişiklikleri ezmez. Bilinen sınırlama:
  kalıcı bir volume'da demo birkaç hafta sonra "son 30 gün" sayılarını
  azaltır; temiz sayılar için volume sıfırlanır (`docker compose down -v`).
  `campaigns` kaydı sonraki
  aşamalarda içerik ve onay alanlarıyla aynı satıra genişler. Bu bağımlılıklar
  ilk `go.sum` dosyasını oluşturur; CI'daki Go cache'i o zaman açılır.

## ADR-020 — HTTP katmanı `internal/httpapi` içinde, `cmd/api` yalnızca bağlama

- **Durum:** Onaylandı.
- **Seçenekler:** Handler'ları `cmd/api` içinde tutmak, ayrı bir HTTP paketine
  almak (`internal/httpapi`) veya buna ek olarak üretim akışı için ayrı bir
  servis paketi açmak.
- **Karar:** Handler'lar `backend/internal/httpapi` paketinde yaşar ve
  yalnızca HTTP işini yönetir: yönlendirme, istek ve yanıt dönüşümü, durum
  kodları, hata eşlemesi. Doğrulama `segment` gibi mevcut paketlerde, SQL ise
  `campaign`, `audience`, `seed` ve `database` paketlerinde kalır.
  `cmd/api` yalnızca bağlama yapar (yapılandırma, veritabanı, LLM istemcisi,
  sunucu); mevcut healthz kodu yerinde kalır. Handler'lar bağımlılıklara küçük
  arayüzlerle bağlanır. Bağımlılık yönü: `cmd/api` → `httpapi` → `segment`,
  `audience`, `campaign`, `llm`. Üretim akışı için ayrı servis paketi
  şimdilik açılmaz; akış büyürse ayrı pakete çıkarılır.
- **Gerekçe ve sonuç:** Proje sahibinin gerekçesi: her yeni endpoint
  `cmd/api`'yi şişirir ve temiz mimariye önem verilmeli. Ek sonuç: `package
  main` başka bir pakete import edilemediği için `cmd/api` içindeki
  handler'lara `backend/tests/` altındaki integration ve e2e testler
  (ADR-018) erişemezdi; ayrı pakette gerçek veritabanıyla `httptest` ile
  sınanabilirler. Hangi endpoint'lerin yazılacağı (kapsam) ayrı bir karardır
  ve bu ADR'nin dışındadır.

## ADR-021 — Arayüz bileşenleri için shadcn-vue (Vega stili)

- **Durum:** Onaylandı.
- **Karar:** Arayüz bileşenleri `shadcn-vue` ile kurulur: Tailwind CSS v4,
  Reka UI tabanı, Vega stili, Lucide ikonları. Tüm bileşenler tek komutla
  (`npx shadcn-vue@latest add --all`) `frontend/src/components/ui/` altına
  eklenir ve proje içinde kalır (bağımlılık değil, kaynak kod). Mevcut ekran
  şimdilik kendi bileşenleriyle çalışır; shadcn bileşenleri ileride
  kullanılacak.
- **Gerekçe ve sonuç:** Proje sahibinin gerekçesi: sade ve şık görünüm, süslü
  efektlere gerek yok, hazır bileşenler tek komutla gelsin. Kurulum sonuçları:
  - Tailwind'in `preflight` sıfırlaması mevcut ekranı bozuyordu; bileşenlere
    açık stiller eklendi (madde işareti, başlık kalınlığı, boşluklar, genişlik).
    Önce/sonra 3000 hesaplanmış stil değerinde fark yok.
  - Kendi CSS değişkenlerimiz `--app-*` olarak yeniden adlandırıldı (shadcn'ın
    `--border`, `--muted`, `--accent` değişkenleriyle çakışıyordu).
  - Üretilmiş `src/components/ui/**` eslint'ten hariç tutuldu (54 hata, hepsi
    üretilmiş kodda).
  - `chart` bileşeni için `@unovis/vue` ve `@unovis/ts` elle eklendi (CLI
    kurmuyor). `shadcn-vue` yalnızca derleme zamanı olduğu için
    `devDependency`.
  - CSS'e eklenen harici Google Fonts isteği kaldırıldı, sistem fontu kullanılır.
  - Bedeli: 13 yeni çalışma zamanı bağımlılığı, ~200 KB CSS (JS aynı), `npm
    audit` çıktısında ~6 yüksek seviyeli geçişli paket (derleme zamanı).
  - Koyu tema: shadcn `.dark` sınıfı bekler, mevcut ekran
    `prefers-color-scheme` kullanır. Bileşenler kullanılmaya başlanınca
    karara bağlanacak.

## ADR-022 — İçerik doğrulama: fallback metni ve içerik için ayrı LLM arayüzü

- **Durum:** Onaylandı (proje sahibi, 2026-11-17). Arayüz ayrıntısı proje
  sahibinin "mevcut tasarıma uygun genişlet" yetkisiyle uygulandı.
- **Karar:**
  - Adı bulunmayan alıcı için `{{first_name}}` yerine `Değerli müşterimiz`
    yazılır. Ad boşluktan ibaretse de fallback kullanılır.
  - İçerik üretimi için `llm.Client`'a dokunulmaz; ayrı bir `llm.ContentClient`
    arayüzü eklenir: `GenerateContent(ctx, req, onChunk) (string, error)`.
    Model çıktısını `onChunk` ile ham metin parçaları olarak iletir, tamamlanınca
    bütün JSON metnini döndürür. Akış yarıda kesilirse `llm.ErrStreamInterrupted`
    döner ve dönen metin boştur (iletilmiş parçalar yalnızca önizlemedir).
    `Mock` ve `Scripted` her iki arayüzü de uygular.
  - İçerik doğrulayıcısı (`internal/content`) segment doğrulayıcısı gibi
    kararlı sebep kodları döndürür. Segment'ten farkı: bütün içerik sorunları
    otomatik düzeltme denemesine uygundur (CNT-001).
  - Üretim sırasındaki uzunluk kontrolü, `{{first_name}}` yerine
    `max(en uzun gerçek ad, fallback)` uzunluğunu koyarak yapılır. Onayda her
    alıcı için gerçek render ile kesin kontrol yapılır. "En uzun gerçek ad"
    bilerek segmentten bağımsız olarak **tüm kullanıcılar** arasından alınır
    (`SELECT max(char_length(btrim(first_name))) FROM users`): basit ve
    tutucu bir üst sınırdır. Segmentin dışındaki çok uzun bir ad, geçerli bir
    içeriği gereksiz yere reddettirebilir; bu durumda onaydaki alıcı bazlı
    kontrol zaten kesin sonucu verir.
- **Gerekçe ve sonuç:** Fallback, "Merhaba Değerli müşterimiz" gibi sabit ve
  nötr bir ifadedir. Ayrı arayüz, segment için yazılmış `Client`'ı kullanan
  `segment.Generator`, `cmd/api` ve testleri değiştirmez; gerçek Claude
  adaptörü ileride iki arayüzü de uygulayacaktır. Bilinen sınırlama: kod
  noktası sayımında birleşik karakterler (ör. `e` + birleştirici aksan) iki
  sayılır; spec Unicode kod noktası sayımını istiyor.

## ADR-023 — Kanala uygunluk: açık izin ve adres; onay anında alıcı seçimi

- **Durum:** Onaylandı (proje sahibi, 2026-11-18): seçenek B.
- **Bağlam:** Önizleme sayıları yalnızca kanal iznine bakıyordu (SEG-001).
  Seed verisinde kullanıcıların yaklaşık %4'ünün telefonu yok, ama SMS izni
  telefondan bağımsız verilmiş. `messages.recipient` boş olamaz; izinli ama
  telefonsuz kullanıcıya SMS işi oluşturulamaz. Önizleme SMS 141 iken kuyruğa
  134 SMS giriyordu.
- **Karar (B):** Bir kullanıcı bir kanala uygundur, ancak o kanal için açıkça
  izin vermişse **ve** o kanalın adresi varsa (SMS için telefon, e-posta için
  e-posta; "ASCII boşluk dışında en az bir karakter"). Kitle sayıları
  (`segment.BuildCount`) ve onaydaki alıcı listesi (`segment.BuildRecipients`)
  aynı ifadeleri kullanır; önizleme ile kuyruk aynı kümeyi sayar. Toplam
  eşleşen kişi sayısı kanal uygunluğundan bağımsız kalır. Onay yanıtındaki
  `without_address` alanı kaldırıldı (artık adressiz izinli alıcı kuyruğa
  aday olmaz). Örnek brief ve seed için önizleme SMS 141 → 134 oldu
  (e-posta 151).
- **Reddedilen seçenekler:** A (adressizi onayda atla ve raporla; önizlemedeki
  "uygun" sayısı gönderilebilecekten büyük görünür), C (adressiz izinli alıcı
  varsa onayı reddet; seed verisinde örnek brief onaylanamazdı).
- **Onay anında alıcı seçimi:** Alıcılar onay transaction'ında, tek sorguyla ve
  o andaki izin, adres ve tarih penceresiyle seçilir. Önizleme ile onay arasında
  kitlenin kayması (izin geri çekilmesi, 30 günlük pencerenin kayması) bu yüzden
  kuyruk sayısına yansır ve yanıt gerçek kuyruğu bildirir. Önizleme sayısı yeniden
  hesaplanıp kaydedilmez (ADR-004 ile uyumlu).
- **Tekrar onay:** Kampanya draft değilse alıcılar yeniden hesaplanmaz; aynı
  sürümle `200` ve mevcut sonuç, başka sürümle `409` (ADR-011).

## ADR-024 — Frontend e2e testleri: izole Compose ortamı ve kontrollü veri

- **Durum:** Onaylandı (proje sahibi, 2026-11-18).
- **Karar:**
  - Playwright e2e testleri, geliştirme ortamından ayrı bir Compose projesinde
    (`compose.e2e.yaml`, proje adı `campaign-e2e`) koşar. Yerelde (`make e2e`) ve
    CI'da aynı tanım kullanılır. Ortam ayrı container, ağ ve volume'lardan oluşur;
    PostgreSQL host'a açılmaz ve veriyi diske yazmaz (tmpfs); API host'a açılmaz;
    web yalnızca `127.0.0.1:18080`'de yayınlanır. Testler bir kullanıcı gibi
    yalnızca web (aynı origin `/api` proxy'si) üzerinden konuşur. Geliştirme
    veritabanına ve portlarına (8080, 8081, 5173, 5432) dokunulmaz;
    `playwright.config.ts` adres verilmezse veya bu portlardan biri verilirse
    testleri başlatmaz.
  - Veri: rastgele demo verisi yerine küçük ve kontrollü bir veri kümesi
    (`SEED_DATASET=e2e`, `seed.GenerateE2E`): 10 kullanıcı, hiç rastgelelik yok,
    tüm tarihler 30 günlük sınırdan en az 15 gün uzak. CASE.md'deki örnek brief
    için sayılar kesindir (toplam 7, SMS 3, e-posta 4) ve testler önizleme ile
    kuyruk sayısını birbirine eşitlemek yerine bu kesin sayıları doğrular.
  - Her test kendi kampanyasını arayüzden oluşturur; testler birbirine bağımlı
    değildir ve paralel de koşabilir (CI'da kararlılık için tek worker).
  - Kapsam bilinçli olarak dar: ana akış, geçersiz içerik, kaydedilmemiş
    düzenleme ve eski sürümle onay (iki sekme). Hata enjeksiyonu, render sınırı
    ve eşzamanlılık ayrıntıları unit ve entegrasyon testlerinde kalır.
  - `make test` e2e'yi içermez (hızlı kalır, tarayıcı gerektirmez); CI'da ayrı
    `E2E` işi koşar.
- **Gerekçe ve sonuç:** Seed verisi tarihleri kuruluş anına göredir; önizleme ile
  onay arasında 30 günlük pencereyi geçen bir kullanıcı sayıyı değiştirebilir
  (geliştirme ortamında 141 → 140 görüldü). Kontrollü veri bu kararsızlığı
  ortadan kaldırır ve eşitlik yerine kesin değer doğrulamayı sağlar. İzole ortam,
  e2e'nin kampanya ve mesaj satırı yazmasının geliştirme verisini kirletmesini
  engeller. Bedel: CI'a Playwright tarayıcısı ve Compose kurulumu eklenir
  (Chromium yalnızca), işin süresi birkaç dakika uzar. `E2E_EXPECTED` sayıları
  backend (`seed.E2EExpected`) ve `frontend/e2e/helpers.ts` içinde ayrı tutulur;
  backend birim ve entegrasyon testleri backend tarafını sabitler.

