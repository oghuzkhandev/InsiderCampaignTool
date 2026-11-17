# CNT-001 — İçerik üretimi, düzenleme ve onay

**Durum:** Taslak

## Amaç

Pazarlamacı SMS ve e-posta taslaklarını canlı olarak görür, düzenler ve
doğrulanmış bir içerik sürümünü gönderim için onaylar.

## Beklenen davranış

- Claude veya MockLLM yalnızca yapılandırılmış JSON üretir. İçerik JSON'u
  `sms_text`, `email_subject` ve `email_body` alanlarını içerir.
- Backend JSON akışını birleştirir. Çözümlenen metin parçaları, alan adıyla
  birlikte JSON verisi taşıyan SSE `delta` olaylarıyla frontend'e aktarılır.
- Akış sırasında gösterilen içerik geçici önizlemedir. Tamamlanan JSON,
  backend doğrulamasından geçmeden içerik sürümü olarak kaydedilmez.
- SMS en fazla 160, e-posta konusu en fazla 80 Unicode kod noktası olabilir.
  Gerçek SMS kodlaması ve çok parçalı SMS hesabı kapsam dışındadır.
- `{{first_name}}` kişiselleştirmesi desteklenir. Desteklenmeyen değişken
  reddedilir. Adı bulunmayan alıcı için belirlenmiş fallback değeri kullanılır.
- Tamamlanmış JSON veya içerik doğrulanamazsa doğrulama hatalarıyla birlikte
  bir otomatik düzeltme denemesi yapılır.
- Düzeltme sırasında onay kapalıdır. Düzeltme JSON'u arka planda biriktirilir.
  Doğrulanıp kaydedildiğinde önizlemenin tamamını değiştirir; önceki metne
  eklenmez.
- Düzeltme de başarısızsa yeni sürüm kaydedilmez. Çözümlenebilmiş alanlar hata
  açıklamasıyla insan düzenlemesine açılır. Hiç metin çözümlenememişse
  kullanıcı yeniden üretebilir veya alanları doldurabilir.
- Akış yarıda kesilirse yeni sürüm kaydedilmez ve onay kapalı kalır.
  Otomatik düzeltme yapılmaz; yeniden üretme seçeneği sunulur.
- LLM çağrısı zaman aşımına uğrarsa veya `429` dönerse yeni sürüm kaydedilmez
  ve onay kapalı kalır. Arayüz hatayı gösterir ve kullanıcı içeriği yeniden
  üretebilir. Başarısız üretimde gösterilmiş kısmi metin onaya açılmaz.
  Tamamlanmış bir çıktı olmadığı için otomatik düzeltme denemesi yapılmaz.
- Brief içindeki talimatlar içerik doğrulamasını atlatamaz. Model brief'teki
  talimata uyup limiti aşan, bilinmeyen değişken içeren veya şemaya uymayan
  içerik üretse de aynı doğrulayıcı onu reddeder.
- İnsan düzenlemesi JSON gövdesiyle gönderilir ve aynı içerik doğrulayıcısından
  geçer. Geçerli her düzenleme yeni ve değişmez bir içerik sürümü oluşturur.
- LLM çağrısı boyunca veritabanı kilidi tutulmaz. Sonuç kaydedilirken kampanya
  durumu tekrar kontrol edilir; kampanya onaylanmışsa yeni sürüm kaydedilmez.
- SSE `validated` olayı yalnızca kayıt transaction'ı tamamlandıktan sonra
  gönderilir. Olay, kaydedilen içeriğin tamamını ve `content_version` değerini
  taşır.
- Onay isteği `content_version` taşır. Yalnızca son doğrulanmış sürüm ilk kez
  onaylanabilir. Eski sürümle onay `409` döner ve gönderim işi oluşturmaz.
- Doğrulanmış içerik sürümü olmayan kampanya için onay `409` döner ve gönderim
  işi oluşturmaz.
- İlk onayda kanal bazında uygun alıcı listesi bir kez seçilir. Kişiselleştirme
  ve mesaj kayıtları aynı alıcı listesi üzerinden oluşturulur.
- Her uygun alıcı için SMS ve e-posta konusu render edilerek tekrar doğrulanır.
  Bir alıcıda bile limit aşılırsa onay `422` döner; etkilenen alıcı sayısı ve
  örnek ad belirtilir. Kampanya `draft` kalır ve hiçbir gönderim işi oluşmaz.
- Filtresiz segment veya sıfır uygun alıcı için onay `422` döner ve gönderim
  işi oluşturulmaz.
- Başarılı onay, kampanya durumunu, onaylanan içerik sürümünü ve gönderim
  işlerini aynı transaction'da kaydeder.
- Onaylanan aynı sürümle yinelenen istek mevcut sonucu `200` ile döndürür.
  Alıcıları yeniden hesaplamaz ve yeni gönderim işi oluşturmaz.
- Kampanya başka bir sürümle onaylanmışsa farklı sürümle onay `409` döner.
- Onaydan sonra içerik düzenleme `409` döner. Worker yalnızca
  `approved_content_version` ile belirlenen içeriği gönderir.

## Doğrulama

- MockLLM ile geçerli JSON, bozuk JSON, eksik alan ve kesilen akışı test et.
- SMS için 160/161, e-posta konusu için 80/81 Unicode kod noktası sınırlarını
  test et. Türkçe karakter ve çok baytlı karakter örnekleri kullan.
- Bilinmeyen değişkeni, eksik adı ve kişiselleştirmeden sonra limit aşımını
  doğrula.
- Otomatik düzeltmenin bir kez yapıldığını; başarılı düzeltmenin eski metne
  eklenmeden önizlemeyi değiştirdiğini doğrula.
- Başarısız veya yarım üretimde sürüm kaydedilmediğini ve önizlemenin
  onaylanamadığını doğrula.
- MockLLM gecikmesiyle zaman aşımını ve `429` yanıtını test et. Sürüm
  kaydedilmediğini, onayın kapalı kaldığını, kısmi önizlemenin onaylanamadığını
  ve düzeltme denemesi yapılmadığını doğrula. Kullanıcı yeniden ürettiğinde
  MockLLM geçerli yanıt verirse yeni sürümün kaydedildiğini doğrula.
- Brief içine limiti veya değişken kurallarını atlatmaya çalışan talimatlar
  koy. MockLLM bu talimatlara uyan geçersiz içerik döndürdüğünde sürümün
  kaydedilmediğini doğrula.
- Geçerli insan düzenlemesinin yeni sürüm oluşturduğunu, geçersiz düzenlemenin
  kaydedilmediğini doğrula.
- İlk onayı, çift onayı, eski sürümle onayı, doğrulanmış sürümü olmayan
  kampanyada onayı ve onaydan sonra düzenlemeyi gerçek PostgreSQL ile test et.
- Alıcı bazlı render hatasında transaction'ın geri alındığını ve mesaj
  satırlarının oluşmadığını doğrula.
- İçerik üretimi veya insan düzenlemesi ile onay eşzamanlı olduğunda
  onaylanan sürümün sonradan değiştirilemediğini doğrula.
- Testleri gerçek API anahtarı olmadan MockLLM ile çalıştır.

## Açık noktalar

- Eksik `first_name` için kullanılacak fallback metni uygulama öncesinde
  belirlenmeli ve testte açıkça doğrulanmalıdır.

## İlgili belgeler

- [Case brief'i](../CASE.md)
- [Teknik kararlar](../../DECISIONS.md)
- [Mimari](../ARCHITECTURE.md)
- [Hata senaryoları](../FAILURE-MODES.md)
- [Segment önizleme](SEG-001-segment-preview.md)
- [Mesaj gönderimi](SND-001-message-delivery.md)
