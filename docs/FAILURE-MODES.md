# Hata senaryoları

Her satırda iddia ile kanıt yan yana: **ölçülebilir beklenen davranış** ve onu doğrulayan test. Test sütunundaki özellik kimlikleri ilgili spec'i belirtir; doğrulanacak davranış aynı satırda açıkça yazılır. Düz adlar planlanan testlere işaret eder; test yazıldıkça gerçek adlarla güncellenir.

## LLM ve segment

| Senaryo | Beklenen davranış | Test |
| --- | --- | --- |
| Segment çıktısı bozuk/JSON değil | Backend bir düzeltme denemesi yapar; çıktı hâlâ geçersizse `SEGMENT_INVALID` (422) döner, kampanya oluşmaz. | Unit · SEG-001 |
| MockLLM desteklenmeyen alan, operatör veya SQL benzeri değer üretir | Backend, desteklenmeyen alan veya operatör içeren geçersiz yapıyı kitle sorgusu çalıştırmadan reddeder. Kabul edilen değer SQL metnine girmez, yalnızca argüman olarak bağlanır; sorgu metni zararsız değerle aynıdır. Düzeltme denemesi yapılmaz; `SEGMENT_INVALID` (422) sebep kodlarıyla döner. | Unit + fuzz · SEG-001 |
| Model brief'in ifade edemediği kısmı bildirir (`unsupported` dolu) veya farklı alanlar arası OR gibi desteklenmeyen yapı üretir | Çıktı düzeltmeden reddedilir. `SEGMENT_INVALID` (422) sebep kodlarıyla döner, kitle sorgusu çalışmaz, kampanya oluşmaz. Arayüz sebebi Türkçe gösterir, brief dolu ve düzenlenebilir kalır, kitle sayısı gösterilmez. | Unit + Integration + Frontend · SEG-001 |
| Model brief'i geçerli ama farklı yorumlar (ör. OR'u AND'e çevirir) | Backend tespit edemez (bilinen sınırlama). Arayüz anlaşılan kuralları özet gösterir, insan onayı son kontroldür. | Frontend · SEG-001 (özetin gösterilmesi) |
| Boş brief veya yalnızca boşluk | `422 BRIEF_EMPTY`, LLM çağrılmaz | Unit + Integration · SEG-001 |
| Geçersiz istek gövdesi (bozuk JSON, bilinmeyen alan, yanlış içerik türü) | `400` veya `415`; segment üretimi, kitle sorgusu ve kayıt çağrılmaz | Unit · SEG-001 |
| İstek gövdesi 64 KiB sınırını aşar (tek büyük değer veya geçerli JSON sonrası fazla boşluk) | `413 REQUEST_TOO_LARGE`; tam sınırdaki gövde kabul edilir; bağımlılıklar çağrılmaz | Unit · SEG-001 |
| API içinde beklenmeyen hata (DB, bağlantı) | `500 INTERNAL`, ayrıntı yalnızca loga yazılır, istemciye sızmaz | Unit · SEG-001 |
| Brief içinde talimat/injection | Davranış prompt'a değil backend doğrulamasına dayanır; aynı reddetme testleri geçerli | Unit (MockLLM) · SEG-001 · CNT-001 |
| Filtresi olmayan kural (tüm kullanıcılar) | Filtresiz segment kuralı içeren kampanya onaya açılmaz. | Unit · SEG-001 |
| Geçerli kural ama sıfır uygun alıcı | Onay 422, mesaj oluşmaz | Integration |
| LLM zaman aşımı / 429 | **Segment:** zaman aşımı `504 LLM_TIMEOUT`, 429 `503 LLM_RATE_LIMITED`; arayüzde "yeniden dene"; kitle sorgusu çalışmaz. **İçerik:** akış zaten `200` ile başladığı için SSE `error` olayı (`LLM_TIMEOUT`, `LLM_RATE_LIMITED`); sürüm kaydedilmez, kısmi içerik onaya açılmaz. Otomatik düzeltme yapılmaz. | Unit (MockLLM gecikme ve 429) + Integration (içerik) · SEG-001 · CNT-001 |

## İçerik ve onay

| Senaryo | Beklenen davranış | Test |
| --- | --- | --- |
| İçerik JSON akışı birkaç parçadan sonra kesilir | SSE `error` olayı `STREAM_INTERRUPTED`; sürüm kaydedilmez, onay kapalıdır. Otomatik düzeltme yapılmaz, yeniden üretme sunulur. Arayüz hatayı gösterir, önceki kayıtlı sürümü korur ve "Yeniden üret" sunar. | Unit + Integration (API) + Frontend (akış kopması, hata gösterimi) |
| Doğrulama başarısız (limit aşımı, bozuk JSON) | Bir otomatik düzeltme JSON'u arka planda biriktirilir. Geçerliyse kaydedilen içerik önizlemenin tamamını değiştirir; eski metne eklenmez. Hâlâ geçersizse sürüm kaydedilmez, çözümlenmiş alanlar hata mesajıyla düzenlemeye açılır ve onay kapalıdır. Hiç metin çözümlenememişse yeniden üretme veya alanları doldurma sunulur. Backend SSE `error` olayında `CONTENT_INVALID`, sebep kodları ve doğrulanmamış `partial` metinleri döner. Arayüz sebepleri Türkçe ve alanın altında gösterir, `partial` metni düzeltmeye açar ve onayı kapalı tutar. | Unit + Integration + Frontend · CNT-001 |
| Üretim sürerken kampanya onaylanır | LLM çağrısı boyunca kilit tutulmadığı için onay engellenmez. Kayıt anında durum kilit altında yeniden kontrol edilir: SSE `error` (`CAMPAIGN_NOT_DRAFT`), yeni sürüm kaydedilmez, `validated` gönderilmez. | Integration |
| Üretim sırasında istemci ayrılır (sekme kapanır) veya SSE yazma/flush hata verir | Üretim hemen iptal olur, sonraki olay yazılmaz, hata olarak loglanmaz ve kimsenin görmediği sürüm kaydedilmez. Başlıklar bile yazılamıyorsa LLM çağrılmaz. `validated` yazılamazsa commit edilmiş sürüm kalır, yeniden yazma denenmez. | Unit |
| Yavaş (okumayan) SSE istemcisi | Her yazmadan önce yenilenen yazma süresi (varsayılan 10 sn) dolunca yazma hata verir; üretim iptal olur ve sürüm kaydedilmez. Süre toplamı değil tek yazmayı sınırlar. | Unit (gerçek sunucu, okumayan TCP bağlantısı) |
| `PUT /content` onaylı kampanyada veya geçersiz içerikle | Onaylıda `409 CAMPAIGN_NOT_DRAFT`; geçersizde `422 CONTENT_INVALID` sebep kodlarıyla, sürüm kaydedilmez. NUL karakteri 500 değil 422 olur. | Unit + Integration · CNT-001 |
| İçerik yazımı ile onay eşzamanlı | Onay kampanya satırını kilitliyken gelen yazım bekler ve onaydan sonra reddedilir; onaylanan sürüm son sürümdür, sürümler kesintisizdir. Uygulama kodu durumu kontrol etmese bile veritabanı tetikleyicileri aynı sonucu verir. | Integration (iki bağlantılı kilit testleri, kontrollü yarış) |
| Eşzamanlı çift onay, farklı sürümler | İkincisi kilit sonrası onaylanan sürümü değiştiremez (tetikleyici); aynı sürümle tekrar zararsızdır. | Integration |
| Migration `00002` geri alınır ve draft olmayan kampanya var | Down hiçbir veri silmeden reddedilir. Koruma kontrolden önce tabloları kilitler: uçuştaki onay varsa Down onu bekler, commit sonrası onaylı kampanyayı görüp reddeder (kilitsiz korumada Down başarılı olup onay verisini silerdi). | Integration |
| İlgili kanala izinli ama telefonu/e-postası olmayan kullanıcı | Kanala uygun sayılmaz: kitle sayısına ve kuyruğa girmez, toplamda sayılır. Önizleme ile kuyruk aynı kuralı kullanır (ADR-023). | Unit + Integration · SEG-001 |
| Onay işlemdeyken içerik düzenlenir veya üretilir | Düzenleme kampanya satırı kilidinde bekler, onay commit olunca `409 CAMPAIGN_NOT_DRAFT` alır; onaylanan sürüm son sürümdür. | Integration (kontrollü senkronizasyon) |
| Onay sırasında yeni sürüm yazılıyor | Onay kilitte bekler, yeni sürümü görür ve eski sürümle onayı `409 STALE_CONTENT_VERSION` ile reddeder; iş oluşmaz. | Integration |
| Mesaj satırları yazılırken hata | Onay da geri alınır: kampanya `draft`, onay alanları boş, hiçbir mesaj yok. | Integration |
| Onaydan önce herhangi bir yol | Önizleme, içerik üretimi/düzenleme ve reddedilen onaylar kuyruğa iş oluşturmaz. | Integration |
| Eşzamanlı çift onay (aynı sürüm) | Hepsi `200`, tek ilk onay, iş bir kez oluşur, `approved_at` aynı. | Integration |
| Doğrulanmış sürüm yokken onay | `409 NO_VALIDATED_CONTENT`, mesaj satırı yok | Integration · CNT-001 |
| Kullanıcı V3'ü görürken V4 kaydedildi, V3'ü onaylar | `409 STALE_CONTENT_VERSION`, `messages` satır sayısı 0; arayüz uyarıyı gösterir ve son sürümü yükler | Integration + E2E (iki sekme) |
| Çift tıklama, aynı sürümle onay | 200 ve aynı sonuç; `messages` sayısı değişmez | Integration |
| Kampanya başka sürümle onaylıyken farklı sürümle onay | `409 CAMPAIGN_ALREADY_APPROVED` | Integration |
| Onaydan sonra içerik düzenleme (`PUT`) | 409; worker hep `approved_content_version` metnini gönderir | Integration |
| Taslak `{{first_name}}` içeriyor, bir alıcı için render SMS 161 karakter | Onay 422 (kaç alıcı, örnek ad), kampanya `draft`, mesaj satırı yok | Integration |
| SMS > 160 veya konu > 80 (şablon düzeyinde) | Doğrulama reddeder | Unit · CNT-001 |
| Bilinmeyen `{{değişken}}` | Reddedilir | Unit · CNT-001 |
| Ad eksik | Fallback değer kullanılır | Unit · CNT-001 |
| İzinsiz kullanıcı veya izin kaydı olmayan kullanıcı | İlgili kanalın uygun alıcı sayısına dahil edilmez; o kanal için mesaj satırı oluşturulmaz. | Integration · SEG-001 |

## Gönderim

| Senaryo | Beklenen davranış | Test |
| --- | --- | --- |
| Onaydan sonra izin geri çekilir | Mesaj `sending → skipped`, **provider'a o mesaj için 0 istek** | Integration · SND-001 |
| Limiter beklerken kanal izni kaldırılır veya izin kaydı silinir | Son izin kontrolü beklemeden sonra yapılır; mesaj `skipped` olur, provider'a istek gitmez. | Integration |
| Limiter beklerken lease dolar veya iş yeniden sahiplenilir | Eski worker yeni provider çağrısı başlatmaz; eski sonuç güncel mesaj durumunu değiştirmez. | Integration |
| Gönderim kapasitesi dolu | Worker boş kapasitesinden fazla iş sahiplenmez. | Integration |
| Yanlışlıkla iki worker aynı satırı almaya çalışır | Yalnızca biri sahiplenir (`SKIP LOCKED`) | Integration (eşzamanlı claim) |
| Provider 500 / zaman aşımı | Backoff + jitter ile retry, **aynı** idempotency key, `attempts` artar | E2E |
| Provider 429 | `next_attempt_at ≥ şimdi + Retry-After` | Unit + E2E |
| **Yanıt kayboldu** (`accept_then_drop`) | 2 HTTP denemesi, aynı key, 1 provider kabul kaydı, mesaj `sent`, `attempts = 2`, `duplicate_deliveries = 0` | E2E · SND-001 (ana idempotency kanıtı) |
| Provider kabul eder, bütün yanıtlar kaybolur | Tek kabul kaydı, sıfır tekrar teslimat; deneme sınırında `failed`, `failure_reason = exhausted`. Arayüz gönderim sonucunun doğrulanamadığını belirtir. | E2E |
| Provider 4xx (geçersiz alıcı) | Retry yok, `failed` | Unit |
| Azami deneme doldu | `failed`, `failure_reason = exhausted` | Unit |
| Worker gönderim ortasında çöker | Lease dolunca reaper `queued` yapar, yeni claim aynı key ile yeniden dener, sonunda `sent`, `duplicate_deliveries = 0` | E2E (worker kill) |
| **Eski worker'ın sonucu geç gelir** (lease dolup iş yeniden sahiplenildi) | Token uyuşmaz, 0 satır güncellenir, `stale_result_total` artar, mesaj durumu bozulmaz | Integration · SND-001 |
| Hız sınırı | Provider'daki tüm deneme zaman damgalarına (**retry'lar dahil**) göre, kanal başına **kayan 1 saniyelik pencerede ≤ 100**. Test, limite yaklaşacak kadar iş verir | E2E (provider istatistiği) |
| 99 mesaj `sent`, 1 `failed` | Kampanya `completed`, dashboard 99/1 gösterir | Integration · SND-001 |
| 100'ü de `failed` | Kampanya yine `completed`, dashboard 100 başarısız gösterir | Integration |
| Retry bekleyen mesaj var (`next_attempt_at` gelecekte) | Kampanya `completed` olmaz | Integration |

## Altyapı ve arayüz

| Senaryo | Beklenen davranış | Test |
| --- | --- | --- |
| Test bağlantısı `campaigns_test` dışında bir database'e gider | Database kimliği kontrolünde test durur; migration, veri hazırlama veya temizleme çalışmaz. | Integration |
| DB erişilemez | `/readyz` 503, worker claim yapmaz ve backoff'ta bekler, DB dönünce devam eder | Integration · SND-001 · TRK-001 |
| SIGTERM | Yeni claim durur, devam eden çağrılara 15 sn; süre dolarsa kalan satırlar lease ile geri alınır | Integration · SND-001 |
| Dashboard SSE bağlantısı kopar | Yeniden bağlanınca ilk olay güncel snapshot | Frontend + Integration |
| Sayfa yenilenir | Kampanya ve son doğrulanmış içerik `GET /api/campaigns/{id}` ile yüklenir; kaydedilmemiş parçalar geri gelmez. Canlı takip SSE ile sürer. | Frontend/E2E |
| SSE istemcisi kopar | Sunucu tarafı goroutine temizlenir (sızıntı yok) | Unit |
| Worker durur, kuyruk birikir | `queue_depth{channel}` veritabanından hesaplandığı için artan değeri gösterir | Integration · TRK-001 |
| `/metrics` erişimi | Docker iç ağından erişilir, host'a yayımlanan portlardan sunulmaz | Integration · TRK-001 |
