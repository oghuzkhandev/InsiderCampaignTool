# SEG-001 — Segment üretimi ve kitle önizleme

**Durum:** Taslak

## Amaç

Pazarlamacı doğal dilde bir brief yazar ve kampanyanın kaç kişiye
ulaşabileceğini kanal bazında görür.

## Beklenen davranış

- Boş brief reddedilir ve LLM çağrılmaz. Yalnızca boşluk karakterlerinden
  oluşan brief de boş sayılır.
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
- Tamamlanmış segment JSON'u geçersizse doğrulama hatalarıyla birlikte bir
  otomatik düzeltme denemesi yapılır.
- Düzeltme de geçersizse `SEGMENT_INVALID` döner ve kitle sorgusu çalıştırılmaz.
- Brief güvenilmeyen girdidir. Brief içindeki talimatlar backend doğrulamasını
  atlatamaz. Model brief'teki talimata uyup geçersiz veya filtresiz bir kural
  üretse de aynı doğrulama onu reddeder.
- LLM çağrısı zaman aşımına uğrarsa veya `429` dönerse segment üretilmez ve
  hata döner. Kitle sorgusu çalıştırılmaz, kampanya onaya açılmaz. Arayüz
  yeniden deneme seçeneği sunar ve kullanıcı brief'i yeniden gönderebilir.
  Tamamlanmış bir çıktı olmadığı için otomatik düzeltme denemesi yapılmaz.
- Geçerli bir kural için toplam eşleşen kişi, SMS'e uygun kişi ve e-postaya
  uygun kişi sayıları ayrı gösterilir.
- Toplam kişi sayısı kanal izninden bağımsızdır. Kanal bazındaki uygun sayıya
  yalnızca ilgili kanal için açıkça izin verilmiş kullanıcılar dahil edilir.
- İzin kaydı bulunmayan kullanıcı ilgili kanala uygun sayılmaz.
- İki kanala da izni olan kullanıcı her iki kanalın uygun sayısında yer
  alabilir. Kanal sayılarının toplamı benzersiz toplam kişi sayısı değildir.
- Geçerli bir kural sıfır kişiyle eşleşebilir. Önizleme `0` gösterir.
- Toplam eşleşen kişi olsa bile hiçbir kanalda uygun alıcı bulunmayan
  kampanya gönderime onaylanamaz.
- Örnek brief için taslak yorum, son 30 günde sepete eklemiş ve aynı
  30 günlük dönemde satın almamış İstanbul kullanıcılarını seçmektir.

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

- Desteklenen alan ve operatör listesi ile değer, filtre sayısı ve derinlik
  sınırları segment geliştirmesine başlanırken kesinleştirilmelidir.
- 30 günlük pencerenin sınır eşitlikleri ve saat dilimi davranışı aynı
  aşamada açıkça tanımlanmalıdır.
- Azami brief uzunluğu segment geliştirmesinde belirlenmelidir.

## İlgili belgeler

- [Case brief'i](../CASE.md)
- [Teknik kararlar](../../DECISIONS.md)
- [Mimari](../ARCHITECTURE.md)
- [Hata senaryoları](../FAILURE-MODES.md)
- [İçerik ve onay](CNT-001-content-generation-and-approval.md)
