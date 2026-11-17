# SND-001 — Mesaj gönderimi

**Durum:** Taslak

## Amaç

Onaylanan kampanyanın mesajları arka planda, kanal izinleri uygulanarak
ve tekrar teslimat oluşturulmadan mock sağlayıcılara gönderilir.

## Beklenen davranış

- İnsan onayından önce gönderim işi oluşmaz. Başarılı onay kampanya durumunu
  ve gönderim işlerini aynı transaction'da kaydeder.
- Her kampanya, kullanıcı ve kanal için en fazla bir gönderim işi oluşur.
- Tekrar gönderim koruması kampanya, kullanıcı ve kanal kapsamındadır.
  Ayrı kampanyalar bağımsızdır; brief veya metnin aynı olması kampanyalar
  arası gönderimi engellemez.
- Gönderimler yalnızca mock SMS ve e-posta sağlayıcılarına yapılır.
- Worker yalnızca boş gönderim kapasitesi kadar iş sahiplenir.
- Her claim işleminde yeni `claim_token` oluşturulur ve `attempts` artırılır.
  Provider çağrısından önce çöken işleme denemeleri de deneme sınırına sayılır.
- Limiter beklemesi lease süresine dahildir. Worker, provider çağrısından
  önce güncel sahipliği ve lease geçerliliğini kontrol eder.
- Sahipliği veya lease geçerliliği kaybolmuş iş için yeni provider çağrısı
  başlatılmaz.
- Kanal izni limiter beklemesinden sonra, provider çağrısına mümkün olduğunca
  yakın kontrol edilir. İzin yoksa veya izin kaydı bulunmuyorsa mesaj
  `skipped` olur ve provider çağrılmaz.
- SMS ve e-posta ayrı hız sınırlarına tabidir. Her kanal için kayan herhangi
  bir 1 saniyelik pencerede en fazla 100 provider isteği yapılır.
  Retry istekleri de bu sınıra dahildir.
- `429`, `5xx`, timeout ve geçici bağlantı hataları, deneme hakkı varsa
  backoff ve jitter ile yeniden denenir.
- `429` yanıtındaki geçerli `Retry-After` değerinden önce yeniden deneme
  yapılmaz. Diğer `4xx` yanıtlarında retry yapılmaz ve mesaj `failed` olur.
- Varsayılan azami işleme denemesi beştir; yapılandırılabilir.
- Aynı işin bütün denemeleri sabit `campaign:user:channel` idempotency
  anahtarını kullanır.
- Mock provider ilk kabulü ve sonucunu PostgreSQL'e atomik kaydeder.
  Aynı anahtarla gelen istek ikinci teslimat oluşturmadan kayıtlı sonucu alır.
- Provider yeniden başlatılsa da kabul kaydı korunur.
- `accept_then_drop`, ilk kabul kaydı commit edildikten sonra yalnızca ilk
  kabul yanıtını düşürür. Tekrar isteği kayıtlı sonucu döndürür.
- Bütün yanıtların kaybolması durumunda deneme sınırında mesaj `failed`,
  `failure_reason = exhausted` olur. Bu sonuç provider'ın mesajı kesinlikle
  kabul etmediği anlamına gelmez.
- Yanıt alınamaması nedeniyle deneme sınırı dolduğunda arayüz
  “Gönderim sonucu doğrulanamadı; deneme sınırı doldu” açıklamasını gösterir.
- Worker çökerse lease'i dolan iş, kalan deneme hakkı varsa yeniden kuyruğa
  alınır; hakkı kalmadıysa başarısız olarak kapatılır.
- Veritabanına erişilemediğinde worker yeni iş sahiplenmez ve backoff ile
  bekler. Süreç sonlanmaz; erişim döndüğünde kuyruğu işlemeye kendiliğinden
  devam eder. Sonucu yazılamayan işler lease dolunca yukarıdaki lease
  kurtarma kuralıyla ele alınır.
- Worker `SIGTERM` aldığında yeni iş sahiplenmeyi durdurur. Devam eden
  provider çağrılarına 15 saniye tanır. Süre dolduğunda tamamlanmayan işler
  lease dolunca yukarıdaki lease kurtarma kuralıyla ele alınır.
- Sonuç yalnızca güncel claim token ve `sending` durumu eşleştiğinde yazılır.
  Eski worker'ın geç gelen sonucu güncel mesaj durumunu değiştirmez.
- Worker yalnızca kampanyanın `approved_content_version` ile belirlenen
  içerik sürümünü kullanır.
- Bütün işler `sent`, `failed` veya `skipped` olduğunda kampanya `completed`
  olur. Retry bekleyen veya gönderilmekte olan iş varsa tamamlanmaz.

## Doğrulama

- Onaysız kampanya ve çift onay senaryolarında iş sayısını doğrula.
- Aynı kullanıcıya ait iki ayrı kampanyanın bağımsız iş oluşturduğunu doğrula.
- Gönderim öncesinde izin iptalini ve izin kaydının silinmesini test et.
  Provider'a ilgili mesaj için hiç istek gitmediğini doğrula.
- Limiter beklemesi sırasında izin iptali, lease dolması ve işin başka
  worker tarafından sahiplenilmesi durumlarını test et.
- Eşzamanlı claim girişimlerinde aynı işin yalnızca bir sahibi olduğunu doğrula.
- Provider ilk isteği kabul edip yanıtı düşürdüğünde iki HTTP isteği, tek kabul
  kaydı, sıfır tekrar teslimat ve sonunda `sent` bekle.
- İlk kabulden sonra provider'ı yeniden başlat; aynı anahtarla yeniden
  denemenin ikinci teslimat oluşturmadığını doğrula.
- Bütün yanıtlar kaybolduğunda tek kabul kaydı, sıfır tekrar teslimat,
  sınırlı deneme ve sonucu açıklayan hata bilgisini doğrula.
- Geçici hata, kalıcı hata, `Retry-After` ve deneme sınırını test et.
- Worker'ı gönderim sırasında `SIGKILL` ile durdur; lease kurtarmasını ve
  aynı anahtarla güvenli yeniden denemeyi doğrula.
- Veritabanı erişimini kes ve geri getir. Erişim yokken worker'ın yeni iş
  sahiplenmediğini ve süreçte kaldığını, erişim dönünce kuyruğu işlediğini
  doğrula.
- Worker'a `SIGTERM` gönder. Yeni claim'in durduğunu, devam eden çağrının
  tamamlandığını ve süre dolarsa kalan işlerin lease ile geri alındığını
  doğrula. Test yapılandırmasında süreleri kısalt; varsayılan değerleri
  değiştirme.
- Eski claim sonucunun yeni sahibin durumunu değiştirmediğini doğrula.
- Provider'a ulaşan bütün isteklerin zaman damgalarıyla kanal başına kayan
  1 saniyelik pencere sınırını doğrula. Limite yaklaşacak kadar iş kullan.
- Gönderim testlerini gerçek API anahtarı ve gerçek mesaj sağlayıcısı
  kullanmadan çalıştır.

## İlgili belgeler

- [Case brief'i](../CASE.md)
- [Teknik kararlar](../../DECISIONS.md)
- [Mimari](../ARCHITECTURE.md)
- [Hata senaryoları](../FAILURE-MODES.md)
- [İçerik ve onay](CNT-001-content-generation-and-approval.md)
- [Canlı takip](TRK-001-live-delivery-tracking.md)
