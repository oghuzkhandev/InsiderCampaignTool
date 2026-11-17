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

**Branch:** `chore/project-setup` (PR #1 ile `main`'e merge edildi)

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

## Tamamlandı — Çalışma düzeni ve CI

**Branch:** `chore/project-setup` (Compose ve Go API başlangıcıyla aynı
PR #1)

**Hedef:** İlk kurulumu CI ile doğrulayıp aynı PR'da `main`'e almak.

- [x] `.github/workflows/ci.yml` eklendi. Tetikleyici: PR'lar ve `main`'e
  push. Üç iş paralel çalışır.
- [x] Backend işi: `go mod verify`, `go mod tidy -diff`, `gofmt` kontrolü,
  `go vet` ve kökten `make test`.
- [x] Frontend işi: `npm ci`, `oxlint`, `eslint`, `vitest run` ve
  `npm run build` (type-check dahil). Lint için `--fix` kullanan `lint`
  script'i yerine araçlar doğrudan çağrılıyor.
- [x] Compose işi: `docker compose config --quiet`. Çıkış kodu 0 ise
  geçer.
- [x] İlk Actions sonucu: PR (`pull_request`) ve `main`'e push çalışmalarında
  Backend, Frontend ve Compose yeşil. Oğuzhan doğruladı. Claude `gh` ile
  PR #1 Checks ve çalışma listesini kontrol etti.
- [x] PR #1 `main`'e squash merge edildi (7 commit tek commit, `c14f461`).
- [x] GitHub'da `main` için CI geçmeden merge edilmemesi kuralı ayarlandı
  (Oğuzhan).

**Çıkış koşulu:** Sağlandı.

**CI'a bilerek eklenmeyenler:** `npm audit`, ek Go lint aracı, Go bağımlılık
cache'i (henüz `go.sum` yok), `docker compose up` ile `/healthz` doğrulaması.
PostgreSQL entegrasyon, e2e ve worker testlerini ekleyen aşama CI'ı da
genişletir.

## Şu an — PostgreSQL, seed ve segment önizleme

**Branch:** `feat/segment-preview` (Oğuzhan güncel `main`'den açar)

- [ ] `CLAUDE.md` (branch ve CI kuralları), `DECISIONS.md` (ADR-017, ADR-018),
  `docs/specs/` altındaki dört spec, backend klasörleri ve Session-01 export
  güncellemesi PR #1'e girmedi. Oğuzhan bunları `chore/project-setup`
  üzerinden ikinci bir PR ile `main`'e alır. `feat/segment-preview` bu merge'den
  sonra güncel `main`'den açılır.
- [ ] PostgreSQL'i Compose'a ekle. Veritabanı hazır olduğunda migration ve
  seed otomatik çalışsın. Seed tekrar çalıştırılınca kayıtlar çoğalmasın.
- [ ] Birkaç bin sahte kullanıcı üret. Örnek brief için tarih sınırı,
  satın alma ve kanal izni senaryolarını içeren sabit örnekler ekle.
- [ ] `SEG-001` taslağı eklendi. Desteklenen filtreleri ve 30 günlük kuralın
  anlamını kısa, test edilebilir maddelerle netleştir.
- [ ] MockLLM segment çıktısını backend'de doğrula. SQL alanlarını sabit
  eşlemeden seç, filtre değerlerini parametre olarak bağla.
- [ ] Vue uygulamasını Compose'a ekle. Örnek brief için toplam, SMS'e uygun
  ve e-postaya uygun kitle sayılarını arayüzde göster. (Vue, Compose'a
  Vite dev server olarak eklendi; kitle sayıları henüz yok.)
- [ ] `.env.example` ekle. Gerçek entegrasyon `ANTHROPIC_API_KEY` değerini
  yerel `.env` üzerinden alsın. Demo ve testler MockLLM ile çalışsın.
  (`.env.example` ve Compose'ta `LLM_PROVIDER` ile `ANTHROPIC_API_KEY`
  geçişi hazır; Claude adaptörü ve MockLLM henüz yok.)
- [ ] PostgreSQL gerektiren test eklenince CI'a Postgres adımını da ekle
  (ADR-015, ADR-017).

**Çıkış koşulu:** Compose ile açılan arayüz, gerçek API anahtarı olmadan
seed verisi üzerinden beklenen kitle sayılarını gösteriyor. CI yeşil.

## Sonraki adımlar

Her aşama güncel `main`'den açılan ayrı bir branch'te ilerler. Branch
adları öneridir.

### 1. İçerik üretimi, düzenleme ve onay

**Branch:** `feat/content-approval`

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

### 2. Worker, mock provider ve dashboard

**Branch:** `feat/worker-dashboard`

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
- [ ] `README.md` kurulum, kısa mimari özeti, CI notu veya rozeti ve bilinen
  sınırlamaları içeriyor.
- [ ] `DECISIONS.md` kabul edilen kararları ve gerekçelerini içeriyor.
- [ ] Case için yapılan Claude oturumları `ai-sessions/` altında. Paylaşım
  öncesi gerçek API anahtarları ve kişisel bilgiler temizlendi.

## Açık konular

- ADR-013'teki Compose başlatma sırası onay bekliyor.
- Web şu an Vite dev server ile çalışıyor ve `/api` proxy'si yok. Teslimden
  önce nginx'e geçilecek (ADR-014).
- Test klasör yapısı ADR-018 ile onaylandı: unit testler kodun yanında,
  integration ve e2e `backend/tests/` altında. `backend/cmd/api` ve healthz
  kodu yerinde kalır. `backend/internal/` boş, paketler (worker, seed, queue,
  mock-provider gibi) aşamalar ilerledikçe açılacak.
- `make test` şu an yalnızca `go test ./...` çalıştırıyor. Postgres'i ayağa
  kaldırma ve `campaigns_test` hazırlığı (ADR-015) entegrasyon testi
  eklenince yapılacak.
- `SEG-001`, `CNT-001`, `SND-001` ve `TRK-001` spec dosyaları taslak olarak
  yazıldı ve [specs/README.md](specs/README.md) dizinine eklendi. Henüz
  commit'lenmedi.
- Karara bağlanmadı: 30 günlük kuralın yorumu (ADR-010) ve önizleme ile
  onay arasında kitlenin kayması.
- Azami brief uzunluğu, segment alan/operatör listesi ve sınırları ile eksik
  `first_name` için fallback metni ilgili geliştirme aşamasında
  netleştirilecek (SEG-001, CNT-001).
- `queue_depth` yalnızca `queued` mesajları sayacak. `ratelimit_wait_seconds`
  türü belirlenmedi.
- API'de graceful shutdown yok.
- CI'da ek Go lint aracı (`golangci-lint` gibi) yok. CI `gofmt` ve
  `go vet` ile başlıyor. Yeni araç bağımlılık kararıdır ve önce Oğuzhan'a
  sorulur.
- `npm audit` CI'a eklenmedi. Yerelde 5 moderate uyarı görüldü (minimatch,
  glob, editorconfig, js-beautify zinciri). Eklenirse yeni advisory'ler
  PR'ları kırabilir.
- Frontend lint komutları CI'da workflow içinde doğrudan çağrılıyor,
  `package.json` ve `Makefile` ile paylaşılmıyor. Yerelde `npm run lint`
  `--fix` ile dosya değiştirir.
- CI Compose için yalnızca `docker compose config --quiet` çalıştırıyor.
  İmajların build edilmesi ve `/healthz` kontrolü CI'da yok.

## Son doğrulama

- **Son çalışan adım:** PR #1 (`chore/project-setup`) CI'dan geçip `main`'e
  merge edildi. PR ve `main` push çalışmalarında Backend, Frontend ve
  Compose işleri yeşil. `gh run list` ve `gh pr view 1` çıktısı bunu
  doğruluyor.
- **Çalıştırılan komut ve sonuç (yerel):** `go mod verify`,
  `go mod tidy -diff`, `gofmt -l`, `go vet ./...` temiz. Kökten `make test`
  geçti. Frontend'de Node 24 ile `npm ci`, `oxlint .`, `eslint .`,
  `npm run test:unit -- --run` (1 test) ve `npm run build` geçti.
  `docker compose config --quiet` çıkış kodu 0.
- **Doğrulanamayan:** Claude `docker compose up` çalıştıramadı (izin
  reddedildi), bu yüzden Compose üzerinden çalıştırma yalnızca Oğuzhan'ın
  doğrulamasına dayanıyor. CI'ın Compose işi yalnızca yapılandırmayı
  doğrular.
- **Sıradaki iş:** Güncel `main`'den `feat/segment-preview` branch'ini aç
  (Oğuzhan), PostgreSQL, seed ve segment önizleme aşamasına başla.
  Migration dosyaları yalnızca Oğuzhan isteyince yazılır.
