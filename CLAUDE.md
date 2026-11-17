# KURALLAR

## Proje bağlamı

- Gereksinimler ve teslim kriterleri `docs/CASE.md` dosyasında.
- Güncel durum ve sıradaki görev `docs/PROGRESS.md` dosyasında.
- Klasörlerin ve dosyaların görevleri `docs/REPO-STRUCTURE.md` dosyasında.
- Go uygulaması `backend/`, Vue ve TypeScript uygulaması `frontend/` altında.
- Özellik geliştirirken varsa ilgili `docs/specs/` dosyasını oku.

## Karar ve kapsam

- Kuyruk, veri modeli, retry ve idempotency gibi önemli mimari kararlarda
  seçenekleri, trade-off'ları ve önerini kısaca açıkla. Son kararı ben veririm.
- Kararı onayladıktan sonra seçimi ve verdiğim gerekçeyi `DECISIONS.md`
  dosyasına yaz. Bekleyen kararları taslak olarak tut.
- Verdiğim görevin kapsamı içinde ilerle. Yeni özellik, bağımlılık, şema
  değişikliği veya ürün davranışı öneriyorsan uygulamadan önce bana açıkla.
  Onaylı görevdeki rutin uygulama ayrıntılarını çözebilirsin.

## Dosyalar, Git ve veritabanı

- İncelediğin ilgili dosyaların adlarını ve bulgularını kısaca belirt.
- Değişiklikten sonra düzenlenen dosyaları listele ve ilgili `git diff`'i
  göster. Yeni dosyalar için içerik özeti veya diff eşdeğeri göster.
- Commit atma ve push yapman yasak.
- Migration dosyası oluşturmayı veya değiştirmeyi ancak ben istersem yap.
  Geliştirme veritabanında migration, seed veya başka bir yazma işlemini
  kendin çalıştırman yasak.
- Testlerin ihtiyaç duyduğu migration, seed ve yazma işlemleri yalnızca
  geçici ve izole test veritabanında çalışabilir.
- `docker compose up` temiz kurulumda migration ve seed'i otomatik
  çalıştırabilmeli. Geliştirme veritabanına yazacaksa komutu ben çalıştırırım.
- Gerçek `.env` değerlerini terminal çıktısına, diff'e, dokümanlara veya
  Git'e dahil etme.

## Uygulama kuralları

- Claude çıktısını backend'de doğrula. Model çıktısını doğrudan SQL olarak
  çalıştırma.
- Kampanyayı insan onayından önce kuyruğa alma. Gönderimde kanal izinlerini
  uygula ve yalnızca mock SMS/e-posta sağlayıcısını kullan.
- Otomatik testleri gerçek API anahtarı gerektirmeden MockLLM ile çalıştır.
- Go hakkında soru sorarsam gerektiğinde bildiğim Node.js veya Spring Boot
  üzerinden örnek ver.

## Doğrulama

- Uygulama davranışındaki bir hatayı düzeltirken hatayı yakalayan bir
  regresyon testi ekle veya mevcut testi genişlet. Mümkünse testin önce
  beklendiği şekilde başarısız olduğunu, düzeltmeden sonra geçtiğini göster.
  Otomatik test eklenemiyorsa nedenini ve nasıl doğruladığını belirt.
- İlgili Go testlerini çalıştır. Uygun olduğunda `backend/` içinde
  `go test ./...` ile kontrol et.
- Teslimde repo kökünden `make test` ve `docker compose up` çalışmalı.
- Sonunda çalıştırdığın komutları, sonuçlarını ve doğrulayamadığın noktaları
  belirt.
  