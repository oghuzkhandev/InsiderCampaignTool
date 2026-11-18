# SEG-001 — Segment üretimi ve kitle önizleme

**Durum:** Taslak

## Amaç

Pazarlamacı doğal dilde bir brief yazar ve kampanyanın kaç kişiye
ulaşabileceğini kanal bazında görür.

## Beklenen davranış

- Boş brief reddedilir ve LLM çağrılmaz. Yalnızca boşluk karakterlerinden
  oluşan brief de boş sayılır. API `422` `BRIEF_EMPTY` döner; sınırı aşan brief
  için `422` `BRIEF_TOO_LONG`.
- Kullanıcı brief'i gönderdiğinde Claude veya MockLLM yalnızca
  yapılandırılmış JSON biçiminde segment kuralı üretir.
- Backend tamamlanan JSON'u şema, desteklenen alan, operatör, değer tipi,
  aralık ve yapı sınırları açısından doğrular.
- Bilinmeyen alan, desteklenmeyen operatör, uyumsuz tip, geçersiz aralık,
  boş filtre veya yapı sınırını aşan kural reddedilir.
- Doğrulanmamış veya filtresiz kural için kitle sorgusu çalıştırılmaz.
- Model çıktısı doğrudan SQL olarak çalıştırılmaz. Backend sorgu yapısını
  sabit alan ve operatör eşlemeleriyle oluşturur.
- Filtre değerleri SQL metnine eklenmez; yalnızca sorgu parametresi olarak
  bağlanır. SQL benzeri metin içeren bir değer sorgunun yapısını değiştirmez.
- Segment JSON'u söz dizimi hatası içeriyorsa (bozuk JSON, şemaya uymayan
  yapı, yanlış değer tipi) doğrulama hatalarıyla birlikte bir otomatik
  düzeltme denemesi yapılır.
- Desteklenmeyen alan, desteklenmeyen operatör, desteklenmeyen yapı (ör.
  farklı alanlar arası OR, iç içe grup), geçersiz aralık, boş filtre ve dolu
  `unsupported` listesi için düzeltme denemesi yapılmaz. Model koşulu sessizce
  silip kitleyi genişletemez.
- Segment çıktısı zorunlu bir `unsupported` listesi taşır. Model, brief'in
  DSL ile ifade edemediği kısımları bu listeye yazar. Liste doluysa çıktı
  reddedilir.
- Düzeltme de geçersizse veya hata düzeltilemeyen türdeyse `SEGMENT_INVALID`
  döner. Kitle sorgusu çalıştırılmaz ve kampanya kaydı oluşturulmaz.
- `SEGMENT_INVALID` yanıtı HTTP `422` döner ve sebep kodlarının listesini
  taşır (ör. `UNSUPPORTED_FIELD`, `UNSUPPORTED_OPERATOR`,
  `UNSUPPORTED_STRUCTURE`, `UNSUPPORTED_PART`). Model çıktısı kullanıcıya ham
  olarak gösterilmez. `unsupported` öğeleri düz metin olarak, backend'in
  uyguladığı sayı ve uzunluk sınırıyla gösterilir.
- Arayüz sebepleri Türkçe açıklama olarak gösterir. Brief kutusu dolu ve
  düzenlenebilir kalır, kullanıcı brief'i düzenleyip yeniden gönderebilir.
  Hata durumunda kitle sayıları gösterilmez.
- Kampanya kaydı yalnızca segment doğrulandıktan ve kitle sayıları
  hesaplandıktan sonra `draft` olarak oluşturulur.
- Kampanyada modelin ham metni değil, doğrulanmış segment yapısı
  (`{"all":[{"field","op","value"}]}`) saklanır. Veritabanı aynı yapıyı
  constraint ile zorlar; boş `all`, 10'dan fazla koşul, bilinmeyen alan veya
  operatör, yanlış tip ve fazladan anahtar kaydedilemez.
- Geçerli segmentte arayüz, anlaşılan kuralları doğrulanmış segment
  yapısından üretilen okunur bir özetle kitle sayılarıyla birlikte gösterir.
- Brief güvenilmeyen girdidir. Brief içindeki talimatlar backend doğrulamasını
  atlatamaz. Model brief'teki talimata uyup geçersiz veya filtresiz bir kural
  üretse de aynı doğrulama onu reddeder.
- LLM çağrısı zaman aşımına uğrarsa veya `429` dönerse segment üretilmez ve
  hata döner. Kitle sorgusu çalıştırılmaz, kampanya onaya açılmaz. Arayüz
  yeniden deneme seçeneği sunar ve kullanıcı brief'i yeniden gönderebilir.
  Tamamlanmış bir çıktı olmadığı için otomatik düzeltme denemesi yapılmaz.
  API zaman aşımında `504` `LLM_TIMEOUT`, modelin `429` yanıtında `503`
  `LLM_RATE_LIMITED` (`Retry-After` ile) döner.
- Geçerli bir kural için toplam eşleşen kişi, SMS'e uygun kişi ve e-postaya
  uygun kişi sayıları ayrı gösterilir.
- Toplam kişi sayısı kanal izninden bağımsızdır. Kanal bazındaki uygun sayıya
  yalnızca ilgili kanal için açıkça izin verilmiş **ve** o kanalın adresi
  (SMS için telefon, e-posta için e-posta) bulunan kullanıcılar dahil edilir
  (ADR-023).
- İzin kaydı bulunmayan kullanıcı ilgili kanala uygun sayılmaz. İzinli ama
  telefonu olmayan kullanıcı toplamda sayılır, SMS'e uygun sayılmaz.
- İki kanala da izni olan kullanıcı her iki kanalın uygun sayısında yer
  alabilir. Kanal sayılarının toplamı benzersiz toplam kişi sayısı değildir.
- Geçerli bir kural sıfır kişiyle eşleşebilir. Önizleme `0` gösterir.
- Toplam eşleşen kişi olsa bile hiçbir kanalda uygun alıcı bulunmayan
  kampanya gönderime onaylanamaz.
- Desteklenen koşullar `all` listesinde AND ile birleşir: `city` (`eq`, `in`),
  `last_cart_add_at` ve `last_purchase_at` (`within_last_days`,
  `not_within_last_days`), `order_count` (`eq`, `gte`, `lte`).
- Sınırlar: en fazla 10 koşul. `in` en fazla 10 değer, her değer en fazla 64
  karakter. Gün sayısı 1-365, `order_count` değeri 0-100000 aralığında tam
  sayı. Brief en fazla 1000 karakter. `unsupported` için en fazla 5 öğe
  gösterilir, her öğe en fazla 200 karaktere kırpılır.
- `city` eşleşmesi tam ve büyük/küçük harfe duyarlıdır. Model şehri kayıtlı
  yazımıyla verir (ör. `İstanbul`).
- `within_last_days` n: alan değeri, referans zamandan n×24 saat önceki an
  (dahil) ile referans zaman arasında olmalıdır. `not_within_last_days` n:
  alan boş (hiç olmamış) veya bu sınırdan önce olmalıdır. Zamanlar UTC'dir,
  bir gün 24 saattir, referans zaman istek anıdır.
- Örnek brief şu koşullara dönüşür: `city eq İstanbul`, `last_cart_add_at
  within_last_days 30`, `last_purchase_at not_within_last_days 30`. Yani son
  30 günde sepete eklemiş ve aynı dönemde satın almamış İstanbul kullanıcıları.
- Sebep kodları: `MALFORMED_OUTPUT` ve `INVALID_VALUE_TYPE` (düzeltilebilir),
  `INVALID_RANGE`, `EMPTY_FILTER`, `UNSUPPORTED_FIELD`,
  `UNSUPPORTED_OPERATOR`, `UNSUPPORTED_STRUCTURE`, `UNSUPPORTED_PART`.

## Doğrulama

- Boş ve yalnızca boşluktan oluşan brief'in reddedildiğini ve MockLLM'in
  çağrılmadığını doğrula.
- Brief içine doğrulamayı atlatmaya çalışan talimatlar koy. MockLLM bu
  talimatlara uyan geçersiz veya filtresiz bir kural döndürdüğünde backend'in
  çıktıyı reddettiğini ve kitle sorgusunun çalışmadığını doğrula.
- MockLLM gecikmesiyle zaman aşımını ve `429` yanıtını test et. Hata
  dönmesini, kitle sorgusunun çalışmamasını ve düzeltme denemesi
  yapılmamasını doğrula. Aynı brief yeniden gönderilip MockLLM geçerli
  yanıt verdiğinde önizlemenin başarılı olduğunu doğrula.
- MockLLM bilinmeyen alan, desteklenmeyen operatör, yanlış değer tipi,
  geçersiz aralık ve aşırı iç içe yapı döndürdüğünde reddi doğrula.
- Geçersiz veya filtresiz kuralda kitle sorgusunun çağrılmadığını doğrula.
- SQL benzeri filtre değerlerinin sorgu metnine eklenmediğini ve yalnızca
  bağlı parametreleri değiştirdiğini test et.
- Bozuk JSON'un tek düzeltmeyle geçerli hâle gelmesini ve düzeltmenin de
  başarısız olması durumunda `SEGMENT_INVALID` dönmesini test et.
- Desteklenmeyen alan, operatör, yapı ve dolu `unsupported` için düzeltme
  çağrısı yapılmadığını (MockLLM tek çağrı), `422` ve ilgili sebep kodlarını,
  kitle sorgusunun çalışmadığını ve `campaigns` tablosunda satır
  oluşmadığını gerçek PostgreSQL ile doğrula.
- Arayüzde hata durumunda sebebin Türkçe gösterildiğini, brief'in dolu ve
  düzenlenebilir kaldığını ve kitle sayılarının gösterilmediğini doğrula.
  Brief düzenlenip MockLLM geçerli yanıt verdiğinde önizlemenin başarılı
  olduğunu doğrula.
- Geçerli segmentte kural özetinin doğrulanmış yapıdan üretildiğini ve
  kitle sayılarıyla birlikte gösterildiğini doğrula.
- Veritabanı constraint'ini yanlış tip, boş dizi, `null`, eksik veya fazla
  anahtar ve sınır değerleriyle test et. Aynı segmentler için Go
  doğrulayıcısı ile veritabanının aynı kararı verdiğini doğrula.
- PostgreSQL'de sonucu belli bir veri setiyle toplam ve iki kanal sayısını
  doğrula. İki kanala izinli, yalnızca tek kanala izinli, izni reddedilmiş
  ve izin kaydı olmayan kullanıcıları kapsa.
- Sıfır eşleşme ile eşleşmesi olup uygun alıcısı bulunmaması durumlarını
  ayrı doğrula.
- 20 gün önce satın alıp 5 gün önce sepete ekleyen kullanıcının mevcut
  taslak yoruma göre dışarıda kaldığını doğrula.
- Tarih sınırı testlerini sabit bir referans zamanıyla çalıştır.
- Testleri gerçek API anahtarı olmadan MockLLM ile çalıştır.

## Açık noktalar

- Şehir adı eşleşmesi büyük/küçük harfe duyarlıdır. `Istanbul` ile `İstanbul`
  farklı değerlerdir. Normalizasyon sonraya bırakıldı.
- Bilinen sınırlama: Go ve veritabanı doğrulayıcıları ayrı kodlardır. Aynı
  kararı verdikleri yalnızca testlerdeki senaryolar için doğrulanmıştır;
  başka girdilerde farklılaşabilirler (ör. NBSP gibi Unicode boşluklar yalnızca
  Go tarafında yakalanır).
- Bilinen sınırlama: Model brief'i geçerli ama farklı yorumlayabilir (ör. OR'u
  AND'e çevirebilir). Backend niyeti bilemediği için bunu tespit edemez.
  `unsupported` beyanı güvenilir değildir ve doğrulamanın yerini tutmaz. Kural
  özeti ve insan onayı bu riski azaltır.

## İlgili belgeler

- [Case brief'i](../CASE.md)
- [Teknik kararlar](../../DECISIONS.md)
- [Mimari](../ARCHITECTURE.md)
- [Hata senaryoları](../FAILURE-MODES.md)
- [İçerik ve onay](CNT-001-content-generation-and-approval.md)
