# Hata senaryoları

Her satırda iddia ile kanıt yan yana: **ölçülebilir beklenen davranış** ve onu doğrulayan test. Test sütunundaki özellik kimlikleri ilgili spec'i belirtir; doğrulanacak davranış aynı satırda açıkça yazılır. Düz adlar planlanan testlere işaret eder; test yazıldıkça gerçek adlarla güncellenir.

## LLM ve segment

| Senaryo | Beklenen davranış | Test |
| --- | --- | --- |
| Segment çıktısı bozuk/JSON değil | Backend bir düzeltme denemesi yapar; çıktı hâlâ geçersizse `SEGMENT_INVALID` döner. | Unit · SEG-001 |
| MockLLM desteklenmeyen alan, operatör veya SQL benzeri değer üretir | Backend, desteklenmeyen alan veya operatör içeren geçersiz yapıyı kitle sorgusu çalıştırmadan reddeder. Kabul edilen değer SQL metnine girmez, yalnızca argüman olarak bağlanır; sorgu metni zararsız değerle aynıdır. | Unit + fuzz · SEG-001 |
| Brief içinde talimat/injection | Davranış prompt'a değil backend doğrulamasına dayanır; aynı reddetme testleri geçerli | Unit (MockLLM) |
| Filtresi olmayan kural (tüm kullanıcılar) | Filtresiz segment kuralı içeren kampanya onaya açılmaz. | Unit · SEG-001 |
| Geçerli kural ama sıfır uygun alıcı | Onay 422, mesaj oluşmaz | Integration |
| LLM zaman aşımı / 429 | Zaman aşımı hatası, arayüzde "yeniden dene", kısmi içerik onaya açılmaz | Unit (MockLLM gecikme) |

## İçerik ve onay

| Senaryo | Beklenen davranış | Test |
| --- | --- | --- |
| İçerik JSON akışı birkaç parçadan sonra kesilir | Arayüz hata gösterir; sürüm kaydedilmez, onay kapalıdır. Otomatik düzeltme yapılmaz, yeniden üretme sunulur. | Unit + E2E (MockLLM kesilen akış) |
| Doğrulama başarısız (limit aşımı, bozuk JSON) | Bir otomatik düzeltme JSON'u arka planda biriktirilir. Geçerliyse kaydedilen içerik önizlemenin tamamını değiştirir; eski metne eklenmez. Hâlâ geçersizse sürüm kaydedilmez, çözümlenmiş alanlar hata mesajıyla düzenlemeye açılır ve onay kapalıdır. Hiç metin çözümlenememişse yeniden üretme veya alanları doldurma sunulur. | Unit · CNT-001 |
| Doğrulanmış sürüm yokken onay | 409/422, mesaj satırı yok | Integration |
| Kullanıcı V3'ü görürken V4 kaydedildi, V3'ü onaylar | 409, `messages` satır sayısı 0 | Integration |
| Çift tıklama, aynı sürümle onay | 200 ve aynı sonuç; `messages` sayısı değişmez | Integration |
| Kampanya başka sürümle onaylıyken farklı sürümle onay | 409 | Integration |
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
| DB erişilemez | `/readyz` 503, worker claim yapmaz ve backoff'ta bekler, DB dönünce devam eder | Integration |
| SIGTERM | Yeni claim durur, devam eden çağrılara 15 sn; süre dolarsa kalan satırlar lease ile geri alınır | Integration |
| Dashboard SSE bağlantısı kopar | Yeniden bağlanınca ilk olay güncel snapshot | Frontend + Integration |
| Sayfa yenilenir | Kampanya ve son doğrulanmış içerik `GET /api/campaigns/{id}` ile yüklenir; kaydedilmemiş parçalar geri gelmez. Canlı takip SSE ile sürer. | Frontend/E2E |
| SSE istemcisi kopar | Sunucu tarafı goroutine temizlenir (sızıntı yok) | Unit |
