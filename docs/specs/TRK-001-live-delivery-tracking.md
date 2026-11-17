# TRK-001 — Canlı gönderim takibi

**Durum:** Taslak

## Amaç

Pazarlamacı kampanyanın gönderim ilerlemesini, başarısızlıklarını ve
atlanan mesajlarını sayfayı yenilemeden takip eder. Servislerin sağlığı ve
temel metrikleri health ve metrics endpoint'lerinden okunabilir.

## Beklenen davranış

- Dashboard SSE bağlantısı açıldığında ilk olay kampanyanın veritabanındaki
  güncel durumunu ve mesaj sayılarını içeren snapshot olur.
- Bağlantı açıkken sonraki snapshot'lar saniyelik olarak gönderilir.
- Dashboard her snapshot ile gösterilen değerleri değiştirir; yeni sayıları
  önceki sayıların üzerine eklemez.
- Bekleyen, yeniden denenecek, gönderilmekte olan, gönderilen, başarısız ve
  atlanan işler ayrı gösterilir.
- Bekleyen sayı, `status = queued` ve `attempts = 0` olan işlerdir.
- Yeniden denenecek sayı, `status = queued` ve `attempts > 0` olan işlerdir.
  Bu işler bekleyen sayıya ikinci kez dahil edilmez.
- Diğer sayılar sırasıyla `sending`, `sent`, `failed` ve `skipped`
  durumundaki mesaj kayıtlarından hesaplanır.
- Bağlantı koptuğunda yeniden kurulur. Yeni bağlantı güncel snapshot ile
  başlar; arada kaçırılan olayların birikmesi gerekmez.
- Sayfa yenilendiğinde kampanya ve son doğrulanmış içerik
  `GET /api/campaigns/{id}` üzerinden geri yüklenir. Kaydedilmemiş içerik
  akışı parçaları geri yüklenmez.
- Bütün işler `sent`, `failed` veya `skipped` olduğunda kampanya `completed`
  olur. Başarısız veya atlanan işler bulunması tamamlanmayı engellemez.
- Retry zamanı gelecekte olan `queued` iş veya `sending` iş varsa kampanya
  `completed` gösterilmez.
- Bütün mesajlar başarısız olsa bile işler terminal duruma ulaştığında
  kampanya `completed` olur; dashboard gerçek başarısızlık sayılarını gösterir.
- Provider yanıtları alınamadığı için deneme sınırı dolan mesajlarda
  “Gönderim sonucu doğrulanamadı; deneme sınırı doldu” açıklaması gösterilir.
  Bu durum kesin olarak teslim edilmemiş mesaj şeklinde sunulmaz.
- Dashboard SSE istemcisi bağlantıyı kapattığında sunucu o bağlantıya ait
  kaynakları ve çalışan goroutine'i temizler.
- `GET /healthz` API süreci çalışırken `200` döner. `GET /readyz` veritabanına
  erişilebiliyorsa `200`, erişilemiyorsa `503` döner.
- API ve worker kendi `/metrics` endpoint'lerini Prometheus formatında sunar.
  Bu endpoint'lere yalnızca Docker iç ağından erişilir; host'a yayımlanan
  portlardan sunulmaz.
- Worker `messages_sent_total`, `send_attempts_total{outcome}`,
  `stale_result_total` ve `reaper_requeued_total` sayaçlarını kendi
  `/metrics` endpoint'inde sunar. API `llm_validation_failures_total`
  sayacını sunar ve worker sayaçlarını göstermez.
- Sayaçlar süreç belleğinde tutulur; süreç yeniden başlarsa sıfırlanır.
- `queue_depth{channel}` gauge'u bellekte tutulmaz. API `/metrics` her
  okunduğunda `messages` tablosundan kanal başına `status = queued` mesaj
  sayısını hesaplar. Worker durduğunda da kuyruk birikimi görünür.

## Doğrulama

- Bütün mesaj durumlarını içeren sabit bir veri setiyle dashboard sayılarını
  doğrula. Yeniden denenecek işlerin bekleyen sayıya dahil edilmediğini test et.
- İlk bağlantının güncel snapshot döndürdüğünü ve sonraki snapshot'ların
  önceki değerleri değiştirdiğini doğrula.
- SSE bağlantısını kesip yeniden kur; güncel sayıların geldiğini ve önceki
  sayıların ikinci kez eklenmediğini doğrula.
- Sayfayı yenile; kampanya ve son doğrulanmış içeriğin geri geldiğini doğrula.
- Gönderilen, başarısız ve atlanan işleri birlikte içeren kampanyanın bütün
  işler terminal olduğunda tamamlandığını doğrula.
- Bütün işleri başarısız kampanyanın da `completed` olduğunu ve başarısız
  sayısının doğru gösterildiğini doğrula.
- Gelecekte yeniden denenecek işi veya gönderilmekte olan işi bulunan
  kampanyanın tamamlanmadığını doğrula.
- Yanıt kaybıyla deneme sınırı dolan mesajın belirsiz sonucunun arayüzde
  açıklandığını doğrula.
- İstemci bağlantısı kapanınca sunucudaki ilgili akışın sonlandığını ve
  bağlantı kaynaklarının serbest bırakıldığını doğrula.
- Veritabanı erişilebilirken `/healthz` ve `/readyz` yanıtlarının `200`
  olduğunu doğrula. Erişim kesilince `/readyz` `503` dönmeli, `/healthz`
  `200` kalmalı ve erişim dönünce `/readyz` yeniden `200` olmalı.
- Sabit bir veri setiyle `queue_depth{channel}` değerlerinin kanal bazında
  `queued` mesaj sayısına eşit olduğunu doğrula. Worker durdurulmuşken
  kuyruk birikiminin değerde göründüğünü doğrula.
- Bilinen sayıda gönderim sonrasında worker sayaçlarının beklenen değerlere
  ulaştığını ve API `/metrics` çıktısında worker sayaçlarının bulunmadığını
  doğrula.
- `/metrics` endpoint'lerine Docker iç ağından erişilebildiğini, host'a
  yayımlanan portlardan erişilemediğini doğrula.

## Açık noktalar

- `ratelimit_wait_seconds` metriğinin türü uygulamada belirlenmelidir.
- API'nin host'a açılan portundan `/metrics` sunmaması, ADR-016'ya göre
  host'a yayımlanmayan ayrı bir iç dinleyici gerektirir. Uygulama ayrıntısı
  geliştirme aşamasında belirlenir.

## İlgili belgeler

- [Case brief'i](../CASE.md)
- [Teknik kararlar](../../DECISIONS.md)
- [Mimari](../ARCHITECTURE.md)
- [Hata senaryoları](../FAILURE-MODES.md)
- [İçerik ve onay](CNT-001-content-generation-and-approval.md)
- [Mesaj gönderimi](SND-001-message-delivery.md)
