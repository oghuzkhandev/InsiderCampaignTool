# Proje İlerlemesi

Son güncelleme: 2026-11-17

## Tamamlanan hazırlık

- [x] Repo klonlandı ve başlangıç klasörleri oluşturuldu.
- [x] Insider One brief'i `docs/CASE.md` dosyasına eklendi.
- [x] Claude Code talimatları, repo rehberi ve mimari taslak hazırlandı.
- [x] Araçlar WSL'de hazır: Go 1.27.4, Node 24 (repo özelinde `nvm use`
  ile), `make`, Docker ve Compose.
- [x] Compose servisleri, test altyapısı ve metrics kararları ADR-013 ile
  ADR-016 arasında yazıldı. ADR-013'teki başlatma sırası taslak.
- [x] `.gitignore`, `.env.example` ve backend ile frontend için
  `.dockerignore` hazır.

## Tamamlandı — Docker Compose ve Go API başlangıcı

**Hedef:** İlk Go endpoint'ini Docker Compose ile çalıştırmak ve test etmek.

- [x] WSL terminalinde `docker info` ve `docker compose version` ile
  Docker Engine ve Compose erişimini kontrol et.
- [x] `backend/` altında Go modülünü, `GET /healthz` endpoint'ini ve
  `200 OK` yanıtını doğrulayan testi ekle.
- [x] Go API için `backend/Dockerfile`, repo köküne `compose.yaml` ekle.
  `frontend/Dockerfile` da eklendi.
- [x] API'yi `docker compose up --build` ile başlat ve `/healthz` yanıtını
  host üzerinden doğrula.
- [x] Kök `Makefile` dosyasına ilk `make test` hedefini ekle ve çalıştır.

**Çıkış koşulu:** API Compose ile ayağa kalkıyor, `/healthz` `200` dönüyor
ve `make test` ilk Go testini çalıştırıyor. Sağlandı.

## Sonraki adımlar

### 1. PostgreSQL, seed ve segment önizleme

- [ ] PostgreSQL'i Compose'a ekle. Veritabanı hazır olduğunda migration ve
  seed otomatik çalışsın. Seed tekrar çalıştırılınca kayıtlar çoğalmasın.
- [ ] Birkaç bin sahte kullanıcı üret. Örnek brief için tarih sınırı,
  satın alma ve kanal izni senaryolarını içeren sabit örnekler ekle.
- [ ] `SEG-001` içinde desteklenen filtreleri ve 30 günlük kuralın anlamını
  kısa, test edilebilir maddelerle yaz.
- [ ] MockLLM segment çıktısını backend'de doğrula. SQL alanlarını sabit
  eşlemeden seç, filtre değerlerini parametre olarak bağla.
- [ ] Vue uygulamasını Compose'a ekle. Örnek brief için toplam, SMS'e uygun
  ve e-postaya uygun kitle sayılarını arayüzde göster. (Vue, Compose'a
  Vite dev server olarak eklendi; kitle sayıları henüz yok.)
- [ ] `.env.example` ekle. Gerçek entegrasyon `ANTHROPIC_API_KEY` değerini
  yerel `.env` üzerinden alsın. Demo ve testler MockLLM ile çalışsın.
  (`.env.example` ve Compose'ta `LLM_PROVIDER` ile `ANTHROPIC_API_KEY`
  geçişi hazır; Claude adaptörü ve MockLLM henüz yok.)

**Çıkış koşulu:** Compose ile açılan arayüz, gerçek API anahtarı olmadan
seed verisi üzerinden beklenen kitle sayılarını gösteriyor.

### 2. İçerik üretimi, düzenleme ve onay

- [ ] Claude adaptörünü ekle. Segment çıktısını ve tamamlanan içerik
  çıktısını backend'de doğrula.
- [ ] SMS ve e-posta taslaklarını SSE ile canlı göster. Tamamlanan geçerli
  taslağı kaydet. Kesilen stream'deki eksik taslağı onaya açma.
- [ ] İnsan düzenlemelerini doğrula ve yeni bir içerik sürümü olarak kaydet.
- [ ] Onayda uygun alıcıları kanal bazında bir kez seç. `{{first_name}}`
  gibi değişkenlerle render et. SMS için 160, e-posta konusu için 80
  karakter sınırını doğrula.
- [ ] Onayı ve `queued` mesaj işlerini aynı transaction'da yaz. Eski sürüm
  `409`, onaylanan aynı sürümün tekrarı `200` dönsün.

**Çıkış koşulu:** Kullanıcı taslağı canlı görüyor, düzenleyebiliyor ve
gördüğü sürümü onaylayabiliyor. Geçersiz onay kuyruk işi oluşturmuyor.

### 3. Worker, mock provider ve dashboard

- [ ] Worker ve mock provider'ı Compose'a ekle. Önce tek bir mesajı kuyruktan
  alıp provider'a gönder. Worker gönderimden hemen önce kanal iznini kontrol etsin.
- [ ] Mock provider gecikme, `429` ve `500` üretebilsin. Başarılı kabulü
  PostgreSQL'e idempotency anahtarıyla kaydetsin.
- [ ] Aynı anahtarla retry, azami deneme, lease kurtarma ve eski
  `claim_token` ile sonuç yazılmasının engellenmesini doğrula.
- [ ] Retry dahil SMS ve e-posta isteklerini ayrı sınırla. Her kanal için
  kayan bir saniyelik pencerede en fazla 100 istek olduğunu test et.
- [ ] Dashboard'da canlı gönderim durumunu göster. Health ve metrics
  endpoint'lerini doğrula.

**Çıkış koşulu:** İlk HTTP yanıtı kaybolduğunda retry yapılır, provider
yalnızca bir kabul kaydeder ve dashboard sonucu gösterir.

## Teslim doğrulaması

- [ ] Temiz bir klonda **`docker compose up`** ile Vue, Go API, PostgreSQL,
  migration, seed, worker ve mock provider çalışıyor.
- [ ] Örnek brief'ten canlı gönderim takibine kadar demo tamamlanıyor.
- [ ] **`make test`** gerçek API anahtarı olmadan geçiyor.
- [ ] Anahtar varsa gerçek Claude adaptörüyle örnek brief doğrulandı.
- [ ] `README.md` kurulum, kısa mimari özeti ve bilinen sınırlamaları içeriyor.
- [ ] `DECISIONS.md` kabul edilen kararları ve gerekçelerini içeriyor.
- [ ] Case için yapılan Claude oturumları `ai-sessions/` altında. Paylaşım
  öncesi gerçek API anahtarları ve kişisel bilgiler temizlendi.

## Açık konular

- ADR-013'teki Compose başlatma sırası onay bekliyor.
- Web şu an Vite dev server ile çalışıyor ve `/api` proxy'si yok. Teslimden
  önce nginx'e geçilecek (ADR-014).
- Test klasör yapısı henüz belirlenmedi. Testler şimdilik kodun yanında,
  handler'lar `backend/cmd/api` altında `package main` içinde.
- `make test` şu an yalnızca `go test ./...` çalıştırıyor. Postgres'i ayağa
  kaldırma ve `campaigns_test` hazırlığı (ADR-015) entegrasyon testi
  eklenince yapılacak.
- `CNT-001` ve `SND-001` spec'leri yazılmadı. [FAILURE-MODES.md](FAILURE-MODES.md)
  bu kimliklere referans veriyor, `SEG-001` dosyası da henüz yok.
- Karara bağlanmadı: kampanyalar arası tekrar gönderim kapsamı (ADR-006),
  30 günlük kuralın yorumu (ADR-010) ve önizleme ile onay arasında kitlenin
  kayması.
- `queue_depth` yalnızca `queued` mesajları sayacak. `ratelimit_wait_seconds`
  türü belirlenmedi.
- API'de graceful shutdown yok.

## Son doğrulama

- **Son çalışan adım:** Compose ile `api` (`127.0.0.1:8081`) ve `web`
  (`127.0.0.1:8080`) ayağa kalktı, `GET /healthz` `200` döndü. Compose
  doğrulamasını Oğuzhan yaptı.
- **Çalıştırılan komut ve sonuç:** `backend/` içinde `go test -v -count=1
  ./...` iki test geçti. `go vet` ve `gofmt -l` temiz. Kökten `make test`
  geçti. `docker compose build` ve `docker compose config -q` başarılı.
- **Doğrulanamayan:** Claude `docker compose up` çalıştıramadı (izin
  reddedildi), bu yüzden Compose üzerinden çalıştırma yalnızca Oğuzhan'ın
  doğrulamasına dayanıyor.
- **Sıradaki iş:** PostgreSQL'i Compose'a ekle (Sonraki adımlar, bölüm 1).
  Migration dosyaları yalnızca Oğuzhan isteyince yazılır.
