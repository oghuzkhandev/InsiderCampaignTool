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

## ADR-004 — Kanal izni onayda ve gönderimde kontrol edilir

- **Durum:** Taslak.
- **Karar:** Onay sırasında yalnızca ilgili kanal için izni olan alıcılara
  iş oluşturulur. Worker provider çağrısından hemen önce izni tekrar kontrol
  eder. Bu kontrol hız sınırı beklemesinden sonra yapılır. İzin kaydı yoksa
  veya izin geri çekilmişse mesaj `skipped` olur. Yalnızca açıkça izin
  verilmiş kullanıcılar ilgili kanalın uygun alıcı sayısına ve iş kayıtlarına
  dahil edilir.
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

- **Durum:** Taslak.
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
  bilgileriyle bir otomatik düzeltme denemesi yapılır. İçerik düzeltmesi
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

- **Durum:** Taslak.
- **Karar:** Onay isteği `content_version` taşır. Onaylanan aynı sürüm
  yeniden gönderilirse mevcut sonuç `200` ile döner. Sürüm çatışması `409`,
  boş kitle veya render ihlali `422` döner.
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
