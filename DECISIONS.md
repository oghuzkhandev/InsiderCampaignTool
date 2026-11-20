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

- **Durum:** Onaylandı (proje sahibi, 2026-11-19).
- **Seçenekler:** PostgreSQL, RabbitMQ ve Redis Streams.
- **Karar:** Kampanya onayı ve gönderim işleri aynı PostgreSQL transaction'ında
  yazılır. Worker işleri `FOR UPDATE SKIP LOCKED` ile sahiplenir ve
  provider çağrısını transaction kapandıktan sonra yapar.
- **Gerekçe ve sonuç:** Onay ve kuyruk kaydı atomik olur. Dashboard aynı mesaj
  kayıtlarından beslenir. Bu yaklaşım polling gecikmesi getirir ve yüksek
  hacimde kuyruk ile diğer veritabanı işlemleri kaynak paylaşır.
- **Proje sahibinin gerekçesi:** PostgreSQL kullanılır çünkü kampanya onayı ve
  mesajların kuyruğa yazılması aynı transaction içinde olacaktır; ya ikisi
  birlikte kaydedilir ya da hiçbiri kaydedilmez. Ayrıca mevcut veritabanı
  kullanılır ve ek bir kuyruk servisi kurulmaz.

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

- **Durum:** Onaylandı (proje sahibi, 2026-11-19).
- **Karar:** Compose demosunda tek worker süreci çalışır. SMS ve e-posta
  ayrı limiter kullanır. İlk ayar kanal başına 95 istek/sn ve burst 1.
  Retry dahil her provider çağrısı limite tabidir.
- **Gerekçe ve sonuç:** Tek süreçte tüm kanal denemeleri aynı limiter'dan
  geçer. 95 istek/sn yapılandırma hedefidir. Brief'teki 100 istek/sn sınırı,
  provider'ın aldığı istekler üzerinden kayan bir saniyelik pencereyle
  test edilir. Replikalar arasında ortak limit bu sürümün kapsamı dışındadır.
- **Sıra ve kapasite (onaylandı):** Her provider çağrısı şu sırayı izler:
  claim → limiter beklemesi → kanal izni kontrolü → claim token ve lease
  kontrolü → çağrı. Limiter beklemesi lease'e dahil olduğundan kanal başına
  eşzamanlı sahiplenilen iş sayısı küçük tutulur (varsayılan 10, ayarlanabilir);
  büyük bir sahiplenilmiş kuyruk oluşturulmaz. Kuyruk boşken yoklama aralığı
  200 ms'dir, DB hatasında backoff uygulanır. Limiter süreç içindedir,
  replikalar arasında paylaşılmaz (bilinen sınırlama, README'de belirtilecek).
- **Gönderim anı kapısı (proje sahibinin talebiyle eklendi, 2026-11-19):**
  Limiter işleri ön kontrolden **önce** aralıklar. Ama limiter'dan geçen işler
  ön kontrolde (veritabanı beklemesi) veya bir süreç duraklamasında (GC, SIGSTOP,
  VM dondurma) takılıp birlikte serbest kalırsa, kayan 1 sn pencerede
  "sürekli hız + birikmiş iş sayısı" kadar istek çıkardı. Ölçülen: gerçek
  provider'da pencere başına **103–104** istek (brief sınırı 100). Düzeltme:
  ön kontrolden hemen sonra, HTTP çağrısından önce bir jeton kovası kapısı
  (`TokenGate`): hız `Rate` (95/s), burst `SendBurst` (varsayılan 2). Sınır
  matematikseldir: herhangi bir T süresinde en çok `burst + Rate·T` gönderim; 1 sn
  pencerede ≤ 97, yani brief'in 100'ünün altında, ağ ve zamanlayıcı kaymaları için
  pay kalır. Kapı bekletirse ön kontrol (sahiplik, lease, kanal izni) **yenilenir**;
  kapı, gönderimden önceki son adımdır. Limiter (burst 1 aralık) yerinde kalır:
  normal akışta kapı bekletmez, yalnızca birikimde devreye girir. Bilinen
  sınırlama: kapının izin verdiği an ile HTTP yazımı arasında süreç duraklarsa
  (mikrosaniyelik pencere) o anda kapıdan geçmiş en çok birkaç iş birlikte gider;
  bu pencereyi tamamen kapatmak kullanıcı alanı koduyla mümkün değildir.

## ADR-006 — İş sahipliği, retry ve idempotency

- **Durum:** Onaylandı (proje sahibi, 2026-11-19). Tekilleştirme kapsamı daha
  önce onaylanmıştı.
- **Karar:** `attempts` işleme denemelerini sayar. `claim_token` her
  sahiplenmede yenilenir. Provider'a iletilen `Idempotency-Key` ise mesaj
  boyunca sabit kalır. Sonuç yalnızca güncel claim token ile yazılabilir.
- **Retry:** `429`, `5xx`, timeout ve geçici bağlantı hataları backoff ile
  yeniden denenir. `429` için geçerli `Retry-After` dikkate alınır.
  `429` diğer `4xx` yanıtlarından ayrıdır ve her zaman geçici hata sayılır.
  `429` dışındaki `4xx` yanıtları kalıcı hata sayılır: retry yapılmaz, mesaj
  `failed` olur ve `failure_reason = rejected` yazılır.
- **`failure_reason` değerleri (onaylandı):** `exhausted` (deneme sınırı doldu;
  son deneme `429`, `5xx`, timeout, bağlantı hatası veya lease kaybıyla bitti,
  sonuç doğrulanamadı) ve `rejected` (`429` dışındaki bir `4xx` ile provider
  isteği kesin reddetti). Başka değer yoktur. `exhausted` için arayüz
  “Gönderim sonucu doğrulanamadı; deneme sınırı doldu” açıklamasını gösterir,
  `rejected` için provider'ın isteği reddettiği belirtilir.
- **Deneme sınırı kenar durumu:** Yeniden denenebilir bir hata geldiğinde
  `attempts` zaten azami değerdeyse mesaj `queued`'a dönmez, doğrudan
  `failed` (`exhausted`) olur. Reaper da aynı kuralı uygular.
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

> **Not (2026-11-19):** Varsayılan `docker compose up` hâlâ bu yedi servistir. İsteğe bağlı
> sekizinci servis `prometheus` yalnızca `observability` profiliyle açılır
> (ADR-028). API artık
> sağlık kontrolü taşır (`/readyz`) ve `web` ona bağlıdır: ilk açılışta API derlenirken
> `up --wait` ve arayüz hazır olana kadar bekler.

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
- **Uygulama (2026-11-19):** `frontend/Dockerfile` çok aşamalı (node derler, nginx
  1.28 servis eder), `frontend/nginx.conf`: tek sayfa yönlendirmesi, `index.html`
  `no-store`, karmalı `/assets/` kalıcı önbellek, `/api/` proxy'sinde `proxy_buffering
  off`, `gzip off`, `proxy_read_timeout 130s` (LLM çağrısı 60 sn'ye kadar sürebilir ve
  düzeltmeyle iki çağrı olabilir), Docker DNS'iyle ad çözümü (API yeniden oluşunca
  yeni IP bulunur) ve API kapalıyken HTML yerine `502 API_UNAVAILABLE` JSON hatası.
  Konteyner `80` dinler (host `8080`). Geliştirirken hızlı yenileme için `npm run dev`
  (Vite) kullanılır. **Ölçüm notu:** bu nginx sürümünde, küçük ve her biri flush
  edilen SSE olayları `proxy_buffering on` ve `gzip on` ile de zamanında iletildi;
  yani bu mutasyonlar testlerle yakalanamadı. Direktifler ADR'ye ve savunma amaçlı
  (büyük veya ani olay yığınları, başka sürümler) bu yüzden korunur; testler
  gözlemlenebilir gereksinimi (gecikmesiz teslim) doğrular.

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
  - **Koyu tema uygulaması (2026-11-19, onaylandı: proje sahibi):** shadcn
    token'ları ve `dark:` varyantı `.dark` sınıfı yerine `prefers-color-scheme: dark`'a
    bağlandı (`@custom-variant dark (@media (prefers-color-scheme: dark))`);
    zemin, yüzey, metin ve kenarlık token'ları uygulamanın `--app-*` paletinden
    gelir (tek kaynak), açık temadaki `destructive` `--app-danger`'a bağlandı.
    Neden: `.dark` sınıfı hiçbir yerde eklenmiyordu; işletim sistemi koyu temadayken
    beyaz kartlar ve koyu zeminde okunmayan koyu yazılar çıkıyordu. Alternatif:
    sınıfı JS ile ekleyip kullanıcıya tema anahtarı sunmak (kullanıcı tercihi
    saklanır); daha fazla kod ve bir ürün davranışıdır, bu yüzden yapılmadı. Bedel:
    kullanıcı işletim sistemi temasından bağımsız tema seçemez.
  - **Tek bileşen sistemi ve ortak düzen (2026-11-20, onaylandı: proje sahibi):** eski
    ekranlardaki elle yazılmış düğme, form alanı, kutu ve hata paneli (bileşen kapsamlı
    CSS) shadcn bileşenlerine taşındı (`Button`, `Card`, `Field`/`Label`, `Input`,
    `Textarea`, `Alert`, `Badge`, `Spinner`, `Skeleton`); uygulamada scoped CSS kalmadı ve
    kullanılmayan `--app-accent`, `--app-on-accent`, `--app-danger-bg`, `--app-warning-bg`
    token'ları silindi. **Birincil düğme nötr shadcn `default` varyantıdır** (mavi
    `--app-accent` yerine): dashboard zaten böyle ve token değişikliği gerekmedi; bedel,
    önceki mavi vurgunun gitmesi (**güncelleme:** aynı gün proje sahibi yumuşak renkler
    istedi; birincil renk pastel indigo oldu, aşağıdaki "Uygulama kabuğu" notuna bakın).
    Ortak kurallar: her ekranda tek `h1` (`PageHeader`: geri
    bağlantısı, başlık, durum rozeti, bir satır açıklama), her bölüm bir `Card` ve `h2`
    başlığı, kartta tek birincil eylem (`default`), ikincil eylem `outline`, ana eylemler
    `size="lg"` (40 px; telefonda tam genişlik). Kampanya ekranı geniş ekranda iki sütundur
    (düzenleyici solda, kompakt kitle ve onay özeti sağda), telefonda alt alta. Adım
    göstergesi (Kitle → İçerik → Onay → Gönderim) tıklanamaz bilgi gösterir; Reka `Stepper`
    etkileşimli bir sihirbaz için olduğundan (tetikleyici düğmeler, `h4` başlık) yerine
    `aria-current="step"` taşıyan düz bir `<ol>` kullanıldı. Yeni bağımlılık eklenmedi.
    Dashboard'da üstte gönderildi, başarısız ve tamamlanmamış (bekleyen + yeniden
    denenecek + gönderiliyor) özet sayıları, altında ilerleme ve ayrıntı vardır.
    **Kanal dağılımı** ilk sürümde eklenmemişti (snapshot kanal bazında sayı taşımıyordu,
    eklemek bir sözleşme değişikliğiydi); proje sahibi sonradan istedi ve eklendi: dashboard'da
    SMS ve e-posta ayrı gösterilir ([ADR-030](#adr-030--dashboard-snapshotı-kanal-dağılımı-ve-yeniden-denemeyle-toparlanan-sayısı)).
  - **Akışı kolaylaştıran eklemeler (2026-11-20, istek: proje sahibi):** yeni ekran, endpoint
    veya bağımlılık yok. (1) Boş ekranda 2-3 örnek brief (`exampleBriefs.json`, tek kaynak;
    backend testi her örneğin desteklendiğini doğrular). (2) Kaydedilmemiş metin varken
    sekme kapatma/yenileme tarayıcı uyarısı, uygulama içi gezinmede onay penceresi.
    (3) İşlem sonrası kısa geri bildirim ("Kaydedildi · sürüm 2"), işlem sürerken düğmeler
    kapalı ve "Kaydediliyor…"/"Onaylanıyor…". (4) Onay özetinde sürüm, tahmini alıcılar ve
    önizleme aynı kartta. (5) Kişiselleştirme önizlemesi örnek adla ("Ayşe") ve bunun
    örnek olduğu yazılı; gerçek mesaj onayda alıcının kendi adıyla üretilir (ADR-026).
    (6) Gönderim sonu özeti: tek cümlelik sonuç ("7 mesajın 5'i gönderildi, 1'i başarısız,
    1'i atlandı"; sayının okunuşuna göre doğru ek) ve gönderilen / başarısız / atlanan
    satırları kısa açıklama ve "Ne yapabilirsiniz?" önerileriyle. Öneriler uygulamanın gerçek
    davranışına dayanır: sonucu doğrulanamayan mesaj teslim edilmiş olabilir (sağlayıcıdan
    kontrol), reddedilenlerde adresler kontrol edilir, atlananlarda (izin yok) yapılacak bir
    şey yoktur, kampanya yeniden gönderilemez ve yeni bir kampanya brief'e uyan herkese
    (gönderilenler dahil) gider (kampanyalar arası tekilleştirme kapsam dışıdır, ADR-006).
    **Yeniden denemeyle toparlanan mesaj sayısı** ilk sürümde gösterilmemişti (snapshot'ta yoktu);
    sonradan backend verisi eklendi ve gösterilir (ADR-030). (7) Her aşamada Türkçe "Geri" / "İleri" (`StepNav`): "İleri" hedef
    açılana kadar kapalı ve nedeni yazılı (kitle hesaplanana, içerik onaylanana kadar);
    kampanya ekranındaki sol düğme yeni kampanya başlatır, çünkü önizleme ekranı var olan bir
    kampanyayı yüklemez ve brief güncelleme uç noktası kapsam dışıdır (**güncelleme:** bu
    düğme "Geri" değil kendi adıyla, "+ Yeni kampanya" diye anılır; kullanıcı mevcut
    kampanyaya döndüğünü sanmasın. Dashboard'da "İleri: Yeni kampanya" gönderimin
    tamamlanmasına bağlı değildir ve gönderim sürerken "Gönderim arka planda devam eder."
    notu görünür; kaydedilmemiş değişiklik uyarısı iki durumda da korunur). Kampanya ekranında
    "Geri/İleri" çubuğu dar ekranda alta yapışır ve onay kartı dar ekranda içerikten hemen
    sonra gelir.
  - **Uygulama kabuğu, simgeler ve pastel palet (2026-11-20, istek: proje sahibi):** yeni
    ekran, endpoint veya bağımlılık yok (mevcut shadcn `Sidebar` ve `@lucide/vue`).
    (1) **Kenar çubuğu:** "Yeni kampanya" düğmesi ve "Son kampanyalar" listesi (CMP-001) ana
    sayfadaki karttan her ekranda görünen kenar çubuğuna taşındı; böylece liste ve gezinti
    dağınık durmaz. Masaüstünde açıktır ve düğmeyle kapanır; telefonda çekmecedir ve bir
    bağlantıya gidilince kapanır. Liste, sayfa değişince ve bir ekran kampanyayı değiştirince
    (oluşturma, onay, dashboard'da durum değişimi) yeniden okunur (`campaign/recent.ts`; ekranlar
    listeyi tanımaz, yalnızca haber verir). Satırda brief, durum rozeti ve tarih vardır; satırın
    nereye açıldığı ("Kampanyayı aç" / "Gönderimi izle") görselde durum rozetinden anlaşılır,
    ekran okuyucuya ve ipucuna metin olarak verilir. Açık kampanyanın satırı `aria-current`
    ile işaretlidir. (2) **Üst başlık:** kenar çubuğu düğmesi ve ana sayfaya dönen "Campaign
    Tool" başlığı. **Sapma:** başlık `h1` değil bağlantıdır; her sayfada tek `h1` (sayfa başlığı)
    kalır, çünkü ekran okuyucu gezintisi, mevcut testler ve `PageHeader` buna dayanır; iki `h1`
    bu kuralı bozardı. Ekranların kendi `<main>` öğesi olduğundan `SidebarInset` (main)
    kullanılmadı. (3) **Simgeler:** kart başlıklarında, düğmelerde (`StepNav` okları metindeki
    "←/→" yerine simge), kitle/kanal satırlarında ve durum rozetlerinde `lucide`; hepsi
    `aria-hidden`, anlam metindedir. Durum rozeti ayrı bir bileşendir (`StatusBadge`: simge +
    metin + ton; durum yalnızca renkle anlatılmaz). (4) **Pastel palet:** önceki "nötr birincil"
    kararının yerine yumuşak mavi-gri zemin, beyaz kartlar ve pastel indigo vurgu
    (`styles.css` token'ları, açık ve koyu tema); durum tonları (taslak gri, onaylı amber,
    gönderiliyor mavi, tamamlandı yeşil). Metin kontrastı iki temada WCAG AA (4,5) üzerinde
    tutulur ve `visual-themes.spec.ts` ile ölçülür. Bedel: ana sayfadaki liste artık sayfa
    içeriğinin altında değil yanındadır; telefonda bir dokunuş uzaktadır.

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

## ADR-025 — Mock provider: kabul kaydı, anahtar çakışması ve hata modları

- **Durum:** Onaylandı (proje sahibi, 2026-11-19), aşağıdaki düzeltmelerle.
- **Karar:**
  - **Kabul kaydı:** `mock_provider_acceptances` (migration `00003`) yalnızca
    başarılı kabulleri saklar. `idempotency_key` birincil anahtardır; yazma
    `INSERT … ON CONFLICT DO NOTHING` ile atomiktir. `messages` tablosuna
    yabancı anahtar yoktur: provider harici bir sistemi temsil eder ve uygulama
    tablolarına bağımlı olmamalıdır. İlişki yalnızca anahtar metniyle kurulur;
    testler `messages` ile birleştirerek doğrular. Kayıtlı sonuç her zaman
    `200 {message_id}` olduğu için durum kodu saklanmaz; `429` ve `500`
    kabulden önce enjekte edilir, kaydedilmez.
  - **Anahtar çakışması (proje sahibinin düzeltmesi):** Aynı `Idempotency-Key`
    ile farklı alıcı, farklı kanal veya farklı içerik gelirse provider isteği
    reddeder (`422 IDEMPOTENCY_KEY_REUSED`) ve kayıtlı sonucu döndürmez. Bu
    yüzden kayıt kanalı, alıcıyı ve içeriğin SHA-256 özetini (`content_hash`)
    saklar. Worker için bu yanıt `429` dışındaki bir `4xx`'tir: retry yok,
    `failed`, `failure_reason = rejected` (ADR-006). Aynı anahtar ve aynı
    istek ise kayıtlı sonucu alır (`200`, aynı `message_id`).
  - **Hata modları:** `accept_then_drop` ana senaryoda **yalnızca ilk kabul
    yanıtını** düşürür (kayıt commit edildikten sonra); tekrar isteği kayıtlı
    sonucu alır. Sürekli yanıt kaybı ayrı bir moddur (`drop_all_responses`):
    kabul kaydedilir ama kabul edilen isteğin **her** yanıtı düşürülür; deneme
    sınırında mesaj `failed`, `exhausted` olur. İki mod ayrı testlerle
    doğrulanır. Gecikme, `429` (`Retry-After` ile) ve `500` ortam
    değişkenleriyle ayarlanır.
  - **İstatistikler bellekte (onaylandı):** Deneme zaman damgaları,
    `duplicate_requests` ve `duplicate_deliveries` provider belleğinde tutulur
    (`GET /admin/stats`). Yeniden başlatmada yalnızca kabul kayıtları kalıcıdır.
- **Gerekçe ve sonuç:** Atomik kabul kaydı ve içerik özeti, yanıt kaybında
  retry'ın ikinci teslimata yol açmadığını ve yanlış kullanılan anahtarın sessizce
  başka bir mesajın sonucunu döndürmediğini gösterir. Bellekte istatistik,
  saniyede ~95 istekte ek veritabanı yazmasından kaçınır; bedeli, provider
  yeniden başlayınca istatistiklerin sıfırlanmasıdır (yeniden başlatma testi
  yalnızca kabul kayıtlarına bakar). `duplicate_deliveries` benzersiz anahtar
  yüzünden yapısal olarak `0` kalır; testlerin kanıtı worker'ın aynı anahtarla
  birden fazla istek yapıp tek kabul görmesidir.

## ADR-026 — Alıcı adı onayda mesaja kopyalanır; worker retry dahil bu adı kullanır

- **Durum:** Onaylandı (proje sahibi, 2026-11-19): seçenek A.
- **Bağlam:** Worker adı her claim'de `users` tablosundan okuyordu. Ad iki deneme
  arasında değişirse aynı `Idempotency-Key` farklı içerikle giderdi ve provider
  isteği `422 IDEMPOTENCY_KEY_REUSED` ile reddederdi. En kötü durum: provider ilk
  isteği kabul edip yanıtı kaybetmişse, kabul edilmiş mesaj yanlışlıkla
  `failed/rejected` olurdu.
- **Karar:**
  - Migration `00004`: `messages.first_name text NOT NULL`. Onay transaction'ı,
    render için kullanılan **ham** adı (kullanıcının `first_name`'i, yoksa boş
    metin) her kanalın mesajına yazar. Boş veya yalnızca boşluktan oluşan ad
    render sırasında eskisi gibi fallback'e (`Değerli müşterimiz`) gider; kontrol
    karakterleri boşluğa çevrilir (`content.Render`, davranış değişmedi).
  - Worker adı `users`'tan değil `messages.first_name`'den okur. Retry dahil her
    deneme aynı adı, dolayısıyla aynı içeriği ve aynı anahtarı kullanır.
  - Şemada, onaylanan hâlden küçük bir sıkılaştırma: `DEFAULT ''` yalnızca mevcut
    satırların doldurulması içindir ve doldurmadan sonra kaldırılır. Sütunu bilmeyen
    eski bir API mesaj eklerse varsayılan sessizce adsız (fallback'li) mesaj
    üretirdi; şimdi onay `NOT NULL` hatasıyla gürültülü şekilde başarısız olur ve
    kampanya `draft` kalır. Geri almak tek satırdır (migration'daki `DROP DEFAULT`).
- **Eski mesajlar (açık sınırlama):** Mevcut mesajlar için onay anındaki ad
  bilinmez. Migration `users.first_name`'in **şimdiki** değerini yazar. Bu, daha
  önce **denenmiş** bir mesajda (`attempts > 0`) ilk denemede gönderilen içerikle
  aynı olmayabilir. Provider ilk isteği kabul edip yanıtı kaybetmiş olabileceği için
  bunu zararsız saymak da, eski worker'la boşaltmak da güvenli değildir (ad değişirse
  retry farklı içerikle gider). Bu yüzden:
  - `queued` veya `sending` durumunda `attempts > 0` olan **en az bir** mesaj varsa
    migration **hiçbir veri değiştirmeden durur** (`messages` tablosu önce kilitlenir,
    uçuştaki claim beklenir). Otomatik `failed` yapmaz, riskli mesajı sessizce güvenli
    saymaz.
  - Güvenli olanlar: `queued` ve `attempts = 0` (provider'a hiç istek gitmedi; ad
    migration anındaki değerle gider) ve terminal mesajlar (`sent`, `failed`,
    `skipped`; yalnızca bilgi).
  - Ne yapılacağına operatör karar verir. Dayanak yalnızca şudur: bu mesajların sonucu
    doğrulanamamıştır. Operatör isterse sonucu doğrulanamayan olarak açıkça kapatır
    (`failed` / `exhausted`; kendi SQL'iyle, migration yapmaz), sonra migration'ı
    yeniden çalıştırır.
- **Uygulama sırası (dev ve her ortam):** Migration sırasında **API ve worker
  kapalı** olmalıdır. Eski worker adı `users`'tan okumaya devam eder; eski API sütunu
  bilmediği için onayı başarısız olur. İkisi de yeni migration'dan **sonra güncel
  kodla** başlatılır:

  ```sh
  docker compose stop api worker                    # web ve mock-provider açık kalabilir
  docker compose up migrate                         # 00004; riskli mesaj varsa HATA verir, veri değişmez
  docker compose up -d --force-recreate api worker  # ikisi de güncel kodla (derleme başlangıçta yapılır)
  ```

  `migrate` hata verirse **api ve worker'ı başlatmayın**: güncel kod `first_name`
  sütununu bekler. Önce riskli mesajları çözüp migration'ı yeniden çalıştırın.
  Riskli mesajları görmek için (salt okunur):

  ```sql
  SELECT status, attempts, count(*) FROM messages
  WHERE status IN ('queued','sending') AND attempts > 0 GROUP BY 1, 2 ORDER BY 1, 2;
  ```

- **Gerekçe ve sonuç:** Proje sahibinin tercihi A: kopya onay anında yazılır, alıcı
  kümesinin onayda sabitlenmesiyle (ADR-023) aynı mantıktır ve en az kod
  değiştirir. Bedel: `messages` tablosunda ek bir sütun ve yukarıdaki geçiş
  prosedürü. Kopya satır olarak değişmez kabul edilir; şimdilik veritabanı bunu
  tetikleyiciyle zorlamıyor, yalnızca worker okuyor.

## ADR-027 — Metrik ve sağlık uçlarının uygulama ayrıntıları

- **Durum:** Onaylandı (proje sahibi, 2026-11-19), önerilen seçeneklerle. Kod bu
  önerilerle yazıldı ve testlerle doğrulandı. ADR-016 (iç dinleyici, `queue_depth`
  veritabanından) ve TRK-001 zaten karar verdiği için burada yalnızca TRK-001'in
  "uygulamada belirlenir" dediği ayrıntılar vardır. Prometheus **servisi** Compose'a
  şimdilik eklenmez (aşağıdaki "Sonradan" notuna bakın).
- **Seçenekler ve öneri:**
  1. **`ratelimit_wait_seconds` türü.** (a) *Histogram* (öneri): bekleme dağılımını
     verir; kova sınırları 1 ms–10 sn (100/sn hızın aralığı 10 ms). (b) İki sayaç
     (`..._total` toplam süre, `..._count` sayı): daha basit, ama dağılım ve yüzdelik
     yok. (c) Gauge: son değer; hızlı değişen bekleme için anlamsız. Öneri (a):
     "hız sınırı gerçekten ne kadar bekletiyor" sorusunu yalnızca dağılım yanıtlar.
     Bedel: kova sınırları sabit; yanlış seçilirse yeniden yapılandırılır.
  2. **Prometheus metin biçimi.** (a) *Küçük, bağımlılıksız yazıcı* (öneri,
     `internal/metrics`): 3 metrik türü, yaklaşık 250 satır, testli. (b)
     `prometheus/client_golang`: standart ve olgun, ama yeni bağımlılıktır ve
     varsayılan olarak Go çalışma zamanı metriklerini de ekler. Yeni bağımlılık
     CLAUDE.md gereği onayınıza bağlı; bu yüzden (a) ile ilerlendi. Metrik sayısı
     arttıkça (b)'ye geçmek kolaydır: sayaç ve histogram arayüzü küçüktür.
  3. **İç dinleyici.** (a) *`METRICS_ADDR` ile ayrı port* (öneri): boşsa kapalı,
     Compose'ta `:9090`, `ports` ile yayımlanmaz, yalnızca `/metrics` sunar, açılamazsa
     süreç başlamaz. (b) Ana portta kaynak IP kontrolü: Docker host portu NAT ile
     geçtiği için istemci IP'si güvenilir ayırt edici değildir. Öneri (a). Opt-in
     olması testlerin aynı makinede birden çok worker çalıştırabilmesi içindir.
  4. **Okunamayan değer.** `queue_depth` okunamazsa `/metrics` `503` döner, kısmi
     çıktı sunulmaz (öneri): sessiz `0`, "kuyruk boş" gibi okunur. Bedel: veritabanı
     kesintisinde API'nin sayaçları da o scrape'te görünmez.
  5. **`/readyz`.** Veritabanı `PingContext`, 2 sn süre sınırı; `503` gövdesi
     ayrıntı vermez; yalnızca durum değişince loglanır. `/healthz` veritabanına
     bakmaz (kesintide süreç yeniden başlatılmaz, yalnızca trafikten çıkar).
  6. **Nil bağımlılık.** `httpapi.Deps.Validate()` zorunlu yedi bağımlılığı denetler
     ve `cmd/api` başlangıçta çağırır (öneri). `httpapi.New` toleranslı kalır:
     mevcut testler yalnızca ilgili rotanın bağımlılığını verir; `New`'ü katılaştırmak
     onları yeniden yazmayı gerektirirdi.
  7. **`llm_validation_failures_total`.** Segment ve içerik üretiminde doğrulamadan
     geçemeyen model çıktısı sayılır; kullanıcının elle düzenlemesi sayılmaz.
- **Etki:** Worker ve API `/metrics` uçları açık, Compose'ta host'a yayımlanmıyor;
  `make e2e-metrics-internal` bunu çalışan ortamda denetler (CI dahil).
- **Sonradan:** Çıktı `promtool check metrics` ile doğrulanır (Prometheus sunucusu
  gerekmez). Prometheus sunucusunu Compose'a eklemek (scrape yapılandırması, depolama,
  uyarı kuralları) ayrı bir karardı; ADR-028 ile isteğe bağlı profil olarak eklendi.

## ADR-028 — İsteğe bağlı Prometheus profili ve promtool denetimi

- **Durum:** Onaylandı (proje sahibi, 2026-11-19).
- **Seçenekler:** (a) Prometheus'u hiç eklememek; (b) normal `docker compose up`'a
  eklemek; (c) yalnızca `observability` Compose profiliyle isteğe bağlı eklemek.
  Metrik biçimi için ayrıca: kendi küçük yazıcımız veya `prometheus/client_golang`.
- **Karar:** (c). `docker compose up` değişmez (yedi servis); `docker compose --profile
  observability up` Prometheus'u (`prom/prometheus:v3.5.0`) ekler. Prometheus API ve
  worker `/metrics` uçlarını Docker iç ağından okur (`api:9090`, `worker:9090`);
  bu uçlar host'a yayımlanmaz (ADR-016 aynen sürer). Prometheus arayüzü yalnızca
  `127.0.0.1:9091`'de açılır (e2e'de `127.0.0.1:19091`). Saklama sınırlıdır: 2 gün
  ve 256 MB. Grafana şimdilik yok; daha kapsamlı paneller için ileride
  değerlendirilecek. `client_golang`'a geçiş **şimdilik yapılmaz**, sonra karar
  verilecek (kendi yazıcımız ADR-027 ile kalır). `promtool` denetimi Makefile'a
  (`make metrics-lint`, `make e2e-observability`) ve CI'a eklenir.
- **Gerekçe ve sonuç:** Proje sahibinin tercihi: izleme isteğe bağlı olsun, varsayılan
  kurulum sade kalsın ve metrik uçları iç ağda kalsın. Bedel: sekizinci (isteğe
  bağlı) servis, depolama ve ek bir imaj; arayüz host'a açıldığı için yalnızca
  loopback'e bağlanır. `make e2e-observability` gerçek scrape'i doğrular: api ve worker
  `up`, gerçek metrikler geliyor, saklama sınırları doğru, worker durunca `up=0` olurken
  API'nin `queue_depth`'i görünmeye devam ediyor; ardından `promtool check metrics`
  ve `promtool check config`. CI'a `promtool` imajı (Docker) bu yüzden dahildir.
  Sonra karar verilecekler: Grafana, `client_golang`, uyarı kuralları. **Loki**
  (log toplama) bu projede gerekli görülmedi: loglar yapılandırılmış `slog` çıktısıyla
  `docker compose logs` üzerinden okunur.

## ADR-029 — Son kampanyalar listesi: `GET /api/campaigns`

- **Durum:** Onaylandı (proje sahibi, 2026-11-20).
- **Bağlam:** Ana sayfadan daha önce oluşturulan kampanyalara dönmenin yolu yoktu; kampanya
  kimliği yalnızca adres çubuğundaydı. Kampanya listesi yeni bir endpoint gerektirir.
- **Karar:**
  - `GET /api/campaigns`: en yeni başta, **varsayılan 10, en fazla 20**. Satırda yalnızca `id`,
    `brief`, `status`, `created_at` (kitle sayısı bilerek yok). Sayfalama, arama ve süzme yok.
  - Sıralama `created_at DESC, id DESC`: aynı anda oluşanlar kimliğe göre ayrılır, sıra kararlıdır.
  - `limit` 20'den büyükse **20'ye indirilir** (hata değil); tam sayı olmayan veya 1'den küçükse
    `400`. Sessizce düzeltmek yerine reddetmek, istemci hatasını görünür kılar; üst sınırı
    aşmak ise zararsız olduğu için indirilir.
  - Kullanıcı kavramı olmadığından tüm kampanyalar listelenir (`GET /api/campaigns/{id}` ile aynı).
  - **İndeks (güncelleme, proje sahibinin isteğiyle, 2026-11-20):** migration `00005`
    `campaigns_recent_idx (created_at DESC, id DESC)`. İlk uygulamada indeks yoktu (demo
    ölçeğinde sıralı tarama yeterli sayılmıştı); proje sahibi liste için iyi olacağından istedi.
    İndeks sorgunun sıralamasıyla birebir aynıdır. **Ne sağlar:** planlayıcı onu seçerse sorgu
    indeksi sırayla gezip ilk n kaydında durur; tam tablo taraması ve ayrı bir `Sort` adımı olmaz,
    maliyet tablo büyüklüğüyle değil kabaca O(log N + n) artar. **Ne sağlamaz:** seçilen
    sütunlar (`brief`, `status`) indekste olmadığı için n satır tablodan okunur (index-only
    değildir); planlayıcı maliyete göre karar verir, çok küçük tabloda tarama + sıralama daha
    ucuz görünüp indeksi seçmeyebilir (demo ölçeğinde bu olağandır). Test (`EXPLAIN`, sıralı
    tarama kapatılarak) indeksin bu sorgu için kullanılabilir ve sırasının doğru olduğunu gösterir,
    planlayıcının her zaman seçtiğini değil. İndeks yalnızca hızlandırır; sorgunun sonucunu
    değiştirmez (testle doğrulandı). **Maliyet:** her kampanya eklemesinde bir indeks girişi.
    Veri ve mevcut sütunlar değişmez; Down yalnızca indeksi kaldırır. **Kilit:** düz `CREATE
    INDEX` tabloda `SHARE` kilidi alır, oluşturma sürerken okumalar devam eder, kampanya
    yazmaları bitmesini bekler; `CONCURRENTLY` kullanılamaz (goose her migration'ı bir
    transaction'da çalıştırır) ve tablo küçük olduğundan bekleme kısadır. Dev veritabanına
    uygulamayı proje sahibi yapar.
  - Arayüz: taslak kampanya kampanya ekranına, `approved`, `sending` ve `completed` olan
    gönderim takibine açılır. Bu aşamaya (`feat/frontend-modernization`) proje sahibinin kararıyla
    dahil edildi.
- **Gerekçe ve sonuç:** Kullanıcı yarım bıraktığı işe dönebilir. Satırı hafif tutmak (kitle
  sayısı yok) sorguyu tek tabloda, birleştirmesiz ve ucuz bırakır; ayrıntı zaten kampanya ekranında.
  Bedel: listede kitle büyüklüğü görünmez, çok sayıda kampanyada eskilere ulaşılamaz (sayfalama
  yok). Spec: [CMP-001](docs/specs/CMP-001-campaign-list.md).

## ADR-030 — Dashboard snapshot'ı: kanal dağılımı ve yeniden denemeyle toparlanan sayısı

- **Durum:** Onaylandı (proje sahibi, 2026-11-20).
- **Bağlam:** Dashboard yalnızca toplam mesaj sayılarını gösteriyordu. İki şey eksikti: hangi
  kanalda (SMS, e-posta) ne olduğu ve yeniden denemenin (ADR-006, yanıt kaybı dahil) kaç mesajı
  kurtardığı. İkisi de SSE `snapshot` sözleşmesinde yoktu (ADR-021'de bu aşamaya alınmamıştı;
  proje sahibi sonradan istedi).
- **Karar:**
  - `snapshot`'a iki alan eklendi: **`channels`** (`sms` ve `email`, her biri `messages` ile aynı
    altı durum sayısı) ve **`recovered`** (`status = sent` ve `attempts > 1` mesajlar).
  - **Değişmez kural:** her durum için `channels.sms + channels.email = messages`; `recovered ≤
    messages.sent`. Sayımlar tek sorguda, `GROUP BY channel` ile yapılır ve `messages` kanal
    satırlarının toplamıdır; bu yüzden tutarsızlık oluşmaz. (Bu, `messages.channel` sütunundaki
    `CHECK (channel IN ('sms', 'email'))` kısıtına dayanır: üçüncü bir kanal eklenirse kırılım ve
    bu kural birlikte güncellenmelidir.) Arayüz kuralı bozan görüntüyü geçersiz sayar.
  - Sunucu alanları **her görüntüde** yollar (sıfır olsalar da): arayüz "alan yok" ile "sıfır"
    arasında ayrım yapmak zorunda kalmaz. Arayüz alanlar yoksa (eski sunucu) bu bölümleri
    göstermeden çalışır; yani değişiklik geriye dönük uyumludur.
  - **`recovered` tanımı:** `status = 'sent'` ve `attempts > 1` olan mesajlar. `attempts` her
    claim'de (işleme alınışta) artar (ARCHITECTURE §3); yani `> 1`, mesajın `sent` olmadan önce
    **birden fazla kez işlenmeye alındığı** demektir. İkinci claim üç durumda olur: (1) geçici
    provider hatası sonrası backoff (429, 5xx, zaman aşımı, bağlantı hatası, geçersiz 2xx);
    (2) kayıp yanıt: provider ilk isteği kabul etmiş olabilir, worker aynı `Idempotency-Key` ile
    yeniden deneyip kayıtlı sonucu alır (`accept_then_drop`); (3) lease'in dolması veya worker
    çökmesi sonrası reaper (önceki deneme provider'a hiç ulaşmamış olabilir).
  - **Ne anlama gelir, ne gelmez:** anlamı "ilk denemede tamamlanamadı, sonraki denemede `sent`
    olarak kaydedildi"dir. **"İlk istek başarısız oldu" demek değildir** (kayıp yanıtta provider
    ilk isteği kabul etmişti) ve arayüz bunu söylemez. Nedeni ayırmaz, provider çağrı sayısı da
    değildir. Provider tarafındaki idempotency (ADR-006) yüzünden toparlanan bir mesaj da en çok
    bir kez teslim edilmiştir; yani bu sayı mükerrer gönderim değil, yeniden denemenin işe
    yaradığı mesajlardır. Nedenleri ayırmak için mesaj başına provider çağrı sayısının veya son
    deneme nedeninin saklanması gerekirdi (bugün yalnızca süreç belleğindeki `send_attempts_total`
    sayacı var); bu yeni sütun ve worker değişikliği olurdu, bu karar onu istemez.
  - Migration yok: iki alan mevcut `messages` sütunlarından (`channel`, `status`, `attempts`)
    hesaplanır.
- **Gerekçe ve sonuç:** Pazarlamacı hangi kanalın sorunlu olduğunu ve yeniden denemenin ne kadar işe
  yaradığını görür; "kısmen başarısız" bir kampanyayı yorumlamak kolaylaşır. Bedel: sorgu iki
  satıra gruplanır (kanal başına) ve Go'da toplanır (üç satırlık işlem); snapshot gövdesi büyür
  (saniyede bir, birkaç yüz bayt). Spec: [TRK-001](docs/specs/TRK-001-live-delivery-tracking.md);
  sözleşme: [ARCHITECTURE §5](docs/ARCHITECTURE.md).

## ADR-031 — Segmente uyan kişiler listesi: `GET /api/campaigns/{id}/audience`

- **Durum:** Onaylandı (proje sahibi, 2026-11-20; üç seçenek de önerilen yönde seçildi).
- **Bağlam:** Kitle önizlemesi yalnızca üç sayı gösteriyordu (toplam, SMS'e uygun, e-postaya uygun).
  Pazarlamacı hangi kişilerin eşleştiğini, kimin neden bir kanala uygun olmadığını göremiyordu.
  Onayın alıcı sorgusu (`segment.BuildRecipients`) bu iş için kullanılamaz: yalnızca en az bir
  kanala uygun kişileri döndürür, yani izinsiz eşleşenleri eler.
- **Karar:**
  - **Endpoint:** `GET /api/campaigns/{id}/audience?page=N&limit=M`. Kampanyanın **kayıtlı,
    doğrulanmış** segmentini kullanır (model çağrılmaz, hiçbir şey yazılmaz). Kampanya yoksa `404
    CAMPAIGN_NOT_FOUND`. Kampanya durumundan bağımsızdır (salt okunur).
  - **Sayfalama: offset (`page` + `limit`).** Yanıt `page`, `limit`, `total` ve `items` taşır;
    arayüz "Sayfa N / M" ve Önceki/Sonraki gösterir. Seçenek olarak keyset (cursor) değerlendirildi
    ve seçilmedi: seed verisi sabit ve 3 bin kişi civarında olduğu için offset'in iki zayıflığı
    (çok büyük listede yavaşlama, sayfalar arasında kayıt kayması) bu demoda gerçekleşmez; keyset ise
    "Önceki" için ters cursor ve "Sayfa N / M" göstermeyi zorlaştırır. Bedel: ölçek büyürse bu karar
    yeniden ele alınmalıdır.
  - **Sıralama: `users.id ASC`.** Birincil anahtar ve benzersizdir, bu yüzden sıra her çağrıda
    aynıdır ve sayfalar çakışmaz. Ada göre sıralama NULL ve eşit adlarda belirsizlik getirirdi.
  - **Sınırlar:** `page` varsayılan 1, en az 1, en çok 100000 (daha büyüğü `400`; offset taşmasını
    önler). `limit` varsayılan 10, en çok 50. ADR-029 ile aynı kural: 50'den büyük `limit` 50'ye
    indirilir, tam sayı olmayan veya 1'den küçük `limit` ve `page` `400 INVALID_REQUEST` olur ve
    veritabanı okunmaz. Son sayfanın ötesi hata değil, boş `items` ve doğru `total` döner.
  - **Tek sorgu:** `total` ve sayfa **tek SQL ifadesinde** okunur; ikisi aynı anlık görüntüden
    gelir, sayfa ile toplam birbirini tutarsız göstermez.
  - **Referans zaman: istek anı** (`Deps.Now`). Onay da alıcıları onay anında seçtiği için liste,
    "şimdi onaylansaydı kimler eşleşirdi" sorusunu yanıtlar (ARCHITECTURE §6). Seçenek olarak
    kampanyanın oluşturulma zamanı değerlendirildi ve seçilmedi: onayda kullanılacak gerçek zamandan
    sapar ve izin değişikliklerini yine yansıtır, yani tutarlılığı garanti etmez. Bedel: kampanya
    günler sonra açılırsa listenin `total` değeri kayıtlı kitle sayısından farklı çıkabilir.
    Arayüz bunu "demo verisi; kesin alıcılar onay sırasında yeniden belirlenir" notuyla söyler.
  - **Kanal uygunluğu: durum + sebep.** Her kanal için `eligible`, `no_consent` veya `no_address`.
    Kural ADR-023 ile aynıdır (açık izin **ve** adres) ve kitle sayılarıyla aynı SQL ifadelerinden
    gelir. İzin yoksa (kayıt yok veya `granted = false`) adresin olup olmadığına bakılmadan
    `no_consent` döner; adres yalnızca izin varsa ve boşsa `no_address` olur. Böylece "SMS'e uygun"
    sayısı, listede `sms_status = eligible` olanların sayısıyla birebir eşleşir.
  - **Eksik bilgi açıkça `null`:** ad (`first_name`), telefon ve e-posta boş, NULL veya yalnızca
    boşluksa `null` döner; arayüz "Ad yok", "Telefon yok", "E-posta yok" yazar. Boş metin veya
    sahte değer yollanmaz. Ad için Go `strings.TrimSpace`, adresler için kanal uygunluğuyla aynı
    ASCII boşluk sınıfı kullanılır.
  - **Eşleşen ama izinsiz kişiler listelenir** (iki kanal da uygun değilse de). `total`, kitle
    önizlemesindeki "Toplam eşleşen kişi" ile aynı tanımdır (kanal iznine bakılmaz).
  - Migration yok, yeni bağımlılık yok; yalnızca mevcut `users` ve `user_consents` okunur.
  - **Kişisel veri:** liste ad, e-posta ve telefonu tam gösterir. Veriler sahte demo verisidir ve
    uygulamada kimlik doğrulama yoktur (ADR-029 ile aynı varsayım). Gerçek müşteri verisiyle
    kullanılacaksa yetkilendirme ve maskeleme gerekir; bu karar onları kapsamaz.
- **Gerekçe ve sonuç:** Pazarlamacı yalnızca "191 kişi" yerine kimlerin eşleştiğini ve izinsizlerin
  neden dışarıda kaldığını görür; sayılara güveni artar ve segment yanlış yorumlandıysa onaydan
  önce fark edilir. Bedel: kitle önizlemesine ikinci bir istek ve her sayfa için bir birleştirme
  sorgusu eklenir; offset sayfalama büyük ölçekte yeniden değerlendirilmelidir. Spec:
  [AUD-001](docs/specs/AUD-001-audience-member-list.md); sözleşme:
  [ARCHITECTURE §5](docs/ARCHITECTURE.md).

## ADR-032 — Çalışma alanı sayfaları ve kalıcı içerik şablonları

Zamanlanmış gönderim kararı ayrıca ADR-033'te kayıtlıdır.

- **Durum:** Onaylandı (Oğuzhan, 2026-11-20). Beş sayfa aynı `feat/audience-preview-list`
  branch'inde uygulanacak; şablon tablosu için migration yazılması açıkça onaylandı.
- **Karar:** Menü Genel bakış, Yeni kampanya, Kampanyalar, Kitleler ve İçerik şablonları
  bağlantılarından oluşur. Marka `Insider Campaign Tool`, yanında ikon yoktur.
- Genel bakış tek sorguyla güncel veritabanı sayılarını okur; tarihsel grafik değildir.
  Kampanya listesi durum filtresi ve offset sayfalama kullanır. Eski `GET /api/campaigns?limit=N`
  yanıtı korunur; `page` veya `status` ile istenen yanıta `total`, `page`, `limit` eklenir.
- Kitleler sayfası tüm sahte demo kullanıcılarını arar; kampanya segmentine uyan kişiler ayrı
  önizlemede kalır. Arama parametreli, literal alt metindir; kanal uygunluğu ADR-023 ile aynıdır.
- `00006` migration'ı `content_templates` tablosunu ekler: UUID, 1–100 karakter ad, SMS,
  e-posta konusu/gövdesi, oluşturulma ve güncelleme zamanı. Liste en son güncellenene göre sıralıdır.
  Şablonlar mevcut içerik doğrulayıcısını kullanır; kampanyaya kaydetmede gerçek ad uzunluklarıyla
  yeniden doğrulanır. Uygulamak kaydetmez/onaylamaz; yalnızca düzenleyiciyi doldurur.
- Şablon düzenlemek veya silmek kaydedilmiş kampanya sürümlerini etkilemez. Şablonlar sürümlenmez;
  eşzamanlı düzenlemelerde son yazan kazanır. Down yalnızca şablon tablosunu ve içindeki verileri
  siler; kampanya/mesaj verisini değiştirmez. Yeni bağımlılık yoktur.
- Sınırlar: uygulamada oturum/yetki modeli yoktur; şablonlar ve demo kullanıcıları ortaktır.
  Listeler varsayılan 10, en fazla 20 kayıt; sayfa en fazla 100000. Offset sayfaları ayrı
  isteklerde okunur, eşzamanlı veri değişimlerinde sayfalar arasında kayma olabilir.

## ADR-033 — PostgreSQL kuyruğuyla zamanlanmış gönderim

- **Durum:** Onaylandı (Oğuzhan, 2026-11-20): sunulan plan uygulanacak, mevcut
  `feat/audience-preview-list` branch'inde devam edilecek; odak zamanlanmış gönderimdir.
- Kampanyaya nullable `scheduled_at timestamptz` eklenir (00007). Yeni durum yerine mevcut
  approved/sending/completed kullanılır; onaylı planlı kampanya arayüzde Planlandı görünür.
- Yeni servis/cron yerine mesajların mevcut `next_attempt_at` alanı kullanılır. Onay ve
  mesajların zamanları aynı transaction'da yazılır. Alıcı seçimi onay anında sabitlenir;
  worker'ın güncel izin kontrolü, hız sınırı, retry/backoff ve idempotency davranışı korunur.
- API zamanı açık ofsetli RFC3339 olarak alır, UTC/mikro-saniye hassasiyetinde saklar. Geçmişlik
  PostgreSQL `clock_timestamp()` ile doğrulanır; worker da DB saatini kullanır. Yerel seçim
  frontend'de UTC'ye çevrilir; kullanıcıya saat dilimi/ofset onaydan önce gösterilir.
- Aynı sürüm + aynı zaman tekrarında önce mevcut onay bulunur, sonra yeni onay için gelecek
  zaman kontrolü yapılır. Böylece yanıt kaybından sonraki geç replay güvenlidir. Saat değiştirme
  409; mevcut planı sonradan değiştiren DB güncellemesi de trigger ile reddedilir.
- Bedel: iş, seçilen zamandan önce başlayamaz; worker kapalıysa veya kapasite doluysa geç başlar.
  Tamamlanma süresi hız sınırlarına bağlıdır. Onay ile gönderim arasında segmente yeni giren
  kişiler otomatik eklenmez. İptal/yeniden planlama ve tekrar eden görevler ayrı özelliklerdir.
- `queue_depth` planlı gelecekteki queued mesajları da kapsar. Dashboard saniyelik heartbeat
  göndermeye devam eder; beklemek bağlantı hatası sayılmaz.
- Migration Down, devam eden planlı kampanya varsa kilit altında reddedilir; terminal
  kampanyaların zaman geçmişi Down ile silinir. Dev DB'ye uygulama proje sahibindedir.
- Yeni bağımlılık yok. SCH-001 unit, PostgreSQL entegrasyon ve proje sahibinin son isteğiyle
  gerçek yığın Playwright testleriyle doğrulanır. `@real` testleri mevcut delivery/CI döngüsünde
  çalışır; tarayıcı saatini değiştirmek yerine gerçek dakika sınırı beklenir.
