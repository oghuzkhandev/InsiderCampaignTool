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

**Branch:** `feat/segment-preview`

- [x] Kurallar, ADR'ler, spec'ler ve backend klasörleri PR #2 ile `main`'e
  alındı. Bu aşamanın seçimleri ADR-019'a ve ADR-003 güncellemesine yazıldı.
- [x] PostgreSQL Compose'a eklendi (`postgres` servisi).
- [ ] Veritabanı hazır olduğunda migration ve seed otomatik çalışsın
  (aşağıdaki `cmd/migrate` ve `cmd/seed` maddesi).
- [x] Üç bine yakın sahte kullanıcı ve örnek brief için sabit senaryo
  kullanıcıları (`internal/seed`). Seed tekrar çalışınca kayıtlar çoğalmaz.
- [x] `SEG-001` içinde desteklenen filtreler, sınırlar, 30 günlük pencere ve
  sebep kodları netleştirildi.
- [x] Segment çekirdeği (`backend/internal/segment`): doğrulayıcı, parametreli
  SQL üretimi, düzeltme akışı (yalnızca söz dizimi hatasında bir deneme),
  zaman aşımı ve `429` ayrımı. Unit ve fuzz testleri geçiyor.
- [x] `LLM` arayüzü, kural tabanlı MockLLM ve testler için `Scripted`
  istemci (`backend/internal/llm`).
- [x] İlk migration (`backend/migrations/00001_init_segment_preview.sql`,
  goose): `users`, `user_consents`, `campaigns`. `campaigns.segment` için
  veritabanı `CHECK` constraint'i. Yalnızca `campaigns_test`'te
  doğrulandı.
- [x] Test altyapısı (`internal/testdb`): `campaigns_test` oluşturma,
  migration, veritabanı adı doğrulaması. `make test` önce yalnızca
  `postgres` servisini başlatır.
- [x] Entegrasyon testleri (`backend/tests/integration`): tablo
  constraint'leri, segment constraint'i (yanlış tipler, boş diziler, sınırlar),
  Go doğrulayıcısı ile veritabanının tutarlılığı (yalnızca test edilen
  senaryolarda), saklama round-trip'i ve gerçek PostgreSQL'de kitle sayıları
  (izin, 30 gün sınırı, sıfır eşleşme).
- [x] `internal/database` (bağlantı ve migration), `internal/seed`,
  `internal/campaign` (draft kaydet ve kimlikle oku) ve `internal/audience`
  (üç kitle sayısı) paketleri. Seed deterministiktir: tarihler yalnızca
  kullanıcı ilk oluşturulurken referans zamana göre yazılır, tekrar
  çalışmada var olan tarihlere ve izinlere dokunulmaz (ADR-019
  güncellendi). Kitle sayıları Go'da bağımsız hesaplanan oracle ile
  karşılaştırıldı. Hepsi yalnızca `campaigns_test`'te doğrulandı.
- [x] `cmd/migrate` ve `cmd/seed` komutları ve Compose servisleri. Sıra
  ADR-013 taslağındaki gibi: `postgres` (healthy) → `migrate` → `seed` →
  `api`. Backend imajı bağımlılıkları artık imaj yapımında indirir.
  Konteyner içinden yalnızca `campaigns_test`'te doğrulandı: migrate
  tamamlandı, seed ilk çalıştırmada 3009 kullanıcı ekledi, ikinci
  çalıştırmada 0 ekledi ve 3009 atladı. Geliştirme veritabanında
  `docker compose up` ile migration ve seed'i Oğuzhan çalıştırır.
- [x] `internal/httpapi` paketi (ADR-020) ve kampanya önizleme endpoint'leri:
  `POST /api/campaigns` ve `GET /api/campaigns/{id}`. Handler'lar yalnızca
  HTTP işini yapar. Sözleşme ARCHITECTURE §5'te. `UPDATE` endpoint'i kapsam
  dışı bırakıldı. Handler unit testleri (`httptest`, sahte bağımlılıklar) ve
  gerçek PostgreSQL ile entegrasyon testleri (`tests/integration/api_test.go`)
  geçiyor. Gerçek `cmd/api` ikilisi `campaigns_test`'e yönelterek `curl` ile
  denendi.
- [x] Desteklenmeyen segment API tarafında: `422` `SEGMENT_INVALID` sebep
  kodlarıyla döner, kampanya oluşmaz (gerçek PostgreSQL ile doğrulandı), LLM
  zaman aşımı `504`, `429` `503`. Arayüzde sebep gösterimi ve brief
  düzenleme Vue adımında yapılacak.
- [x] `cmd/api` artık `SIGTERM`/Ctrl+C ile düzgün kapanıyor (devam eden
  isteklere en fazla 15 sn). Düzeltilen hata: sinyal yakalanıyor ama sunucu
  bağlamı izlemediği için süreç kapanmıyordu; regresyon testleri eklendi.
- [x] Düzeltilen hata: geçerli JSON'dan sonra 64 KiB'lik boşluk gelen gövde
  `413` yerine `400` dönüyordu. Gövde sonu kontrolü (`dec.Token()`)
  `MaxBytesError`'ı "fazladan veri"ye çeviriyordu. Önce başarısız olan
  regresyon testi (geçerli JSON + 64 KiB boşluk, 1 bayt fazla, parçalı gövde)
  ve tam sınırdaki gövdenin kabul edildiği test eklendi. Kapsam bilerek
  yalnızca bu kök nedenle sınırlı: `Content-Length` ön kontrolü, BOM toleransı
  ve gövde hatalarının sebebini ayırt edilebilir yapma (`reasons`) yapılmadı.
- [x] Konteynerde düzgün kapanış: `api` için `stop_grace_period: 20s`
  (Docker varsayılanı 10 sn, uygulamanın 15 sn'lik süresini kesiyordu) ve
  `go run` yerine derle + `exec` (PID 1 olan `go run` SIGTERM'i iletmiyor,
  `api durdu` logu hiç yazılmıyordu, çıkış kodu 143'tü). `campaigns_test`'e
  bağlı geçici konteynerde ölçüldü: önce 143 ve log yok, sonra çıkış kodu 0,
  `api durdu` logu, 0,4 sn. Otomatik test yok (Docker gerektirir). `worker`
  eklenince aynı iki ayar uygulanacak.
- [x] Vue kitle önizleme ekranı (`frontend/src`): brief girişi, toplam, SMS'e
  uygun ve e-postaya uygun sayılar, doğrulanmış yapıdan üretilen koşul özeti,
  `SEGMENT_INVALID` sebeplerinin Türkçe gösterimi (brief düzenlenebilir
  kalır, kitle sayısı gösterilmez), boş ve uzun brief için satır içi kontrol,
  yeniden denenebilir hatalarda "Yeniden dene", geç gelen yanıtın sonucu
  ezmemesi. Vite dev server `/api` isteklerini API'ye yönlendirir
  (`API_PROXY_TARGET`, Compose'ta `http://api:8081`). 73 birim testi, lint,
  tip kontrolü ve build geçiyor. Gerçek tarayıcıda (Chromium) gerçek API ile
  `campaigns_test` üzerinde denendi; `web` konteynerinin proxy'si `api`
  servisine ulaştığı doğrulandı.
- [x] shadcn-vue (Vega) kuruldu, tüm bileşenler `src/components/ui/`
  altında (ADR-021). Mevcut ekran bozulmadı (önce/sonra 3000 stil değerinde
  fark yok). Tip kontrolü, lint, 73 test ve build yeşil. Bileşenler henüz
  ekranda kullanılmıyor. Not: `web` konteynerinin `node_modules` named
  volume'u eski bağımlılıkları tutar; yeni bağımlılıklar için yalnızca o
  volume silinmeli (`docker compose down` + `docker volume rm
  campaign-tool_frontend_node_modules`). `down -v` postgres verisini de siler.
- [x] `.env.example` ve MockLLM hazır: demo ve testler gerçek API anahtarı
  olmadan MockLLM ile çalışıyor (`LLM_PROVIDER=mock` varsayılan, `cmd/api`
  bağlı). Gerçek Claude entegrasyonu ayrı bir iş olarak aşağıda
  ("3. Gerçek Claude entegrasyonu") duruyor ve bu aşamanın çıkış koşuluna
  dahil değil.
- [x] CI: Backend işi `make test` çalıştırdığı için `postgres` servisini
  Compose ile başlatır. Go bağımlılık cache'i açıldı. İlk Actions sonucu
  bekleniyor.

**Çıkış koşulu:** Compose ile açılan arayüz, gerçek API anahtarı olmadan
seed verisi üzerinden beklenen kitle sayılarını gösteriyor. CI yeşil.

## Sonraki adımlar

Her aşama güncel `main`'den açılan ayrı bir branch'te ilerler. Branch
adları öneridir.

### 1. İçerik üretimi, düzenleme ve onay

**Branch:** `feat/content-approval`

- [ ] Tamamlanan içerik çıktısını (JSON) backend'de doğrula. MockLLM ile
  çalışır. Segment çıktısı doğrulaması bitti. Gerçek Claude adaptörü ayrı iş
  (3. Gerçek Claude entegrasyonu).
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

### 3. Gerçek Claude entegrasyonu (isteğe bağlı, ayrı iş)

**Branch:** `feat/claude-adapter`

API anahtarı şu an eklenmeyecek. Demo ve testler MockLLM ile devam ediyor.
Anahtar eklenirse yapılacaklar:

- [ ] `llm.Client` arayüzünü Anthropic API ile uygulayan adaptör. `ANTHROPIC_API_KEY`
  yalnızca yerel `.env` üzerinden okunur, repoya veya çıktıya girmez.
- [ ] `LLM_PROVIDER=anthropic` seçilebilir olsun. Zaman aşımı ve `429`,
  mevcut `llm.ErrTimeout` ve `llm.ErrRateLimited` hatalarına eşlensin.
- [ ] Otomatik testler anahtar gerektirmeden sahte bir HTTP sunucusuyla
  çalışsın. Gerçek anahtarla örnek brief elle doğrulansın.

**Çıkış koşulu:** Anahtar varken örnek brief gerçek Claude ile aynı
doğrulayıcıdan geçerek kitle sayısı üretiyor. `make test` anahtarsız geçmeye
devam ediyor.

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
- Web Vite dev server ile çalışıyor; `/api` proxy'si yalnızca geliştirme
  içindir. Teslimden önce nginx'e geçilecek (ADR-014).
- `frontend/e2e/vue.spec.ts` yalnızca başlığı doğrulayacak şekilde
  güncellendi ama çalıştırılmadı: Playwright'ın beklediği tarayıcı sürümü
  (1255) kurulu değil, makinede 1200 var. CI Playwright çalıştırmıyor.
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
  `npm run test:unit -- --run` (73 test) ve `npm run build` geçti.
  `docker compose config --quiet` çıkış kodu 0.
- **Doğrulanamayan:** Claude `docker compose up` çalıştıramadı (izin
  reddedildi), bu yüzden Compose üzerinden çalıştırma yalnızca Oğuzhan'ın
  doğrulamasına dayanıyor. CI'ın Compose işi yalnızca yapılandırmayı
  doğrular.
- **Sıradaki iş:** Oğuzhan `docker compose up` ile (dev veritabanında
  migration ve seed çalışır) arayüzü `http://127.0.0.1:8080` üzerinde dener,
  commit ve push yapar, PR'daki Checks sonucuna bakar. Yeşilse PR squash
  merge edilir ve aşamanın çıkış koşulu sağlanmış olur. Ardından güncel
  `main`'den yeni branch açılıp içerik üretimi aşamasına geçilir.
  Migration dosyaları yalnızca Oğuzhan isteyince yazılır.
