# Proje İlerlemesi

Son güncelleme: 2026-11-20

## Şu an — Zamanlanmış gönderim

**Branch:** `feat/audience-preview-list`; Oğuzhan aynı branch'te devam edilmesini onayladı.
Kapsam ADR-033 ve SCH-001; Oğuzhan'ın son isteğiyle zamanlayıcı için gerçek yığın e2e de eklendi.

- Onay kartında hemen gönderim veya cihazın yerel saatine göre tarih/saat seçimi var.
  Son onayda saat dilimi/ofset görünür; geçersiz/geçmiş zaman seçimi reddedilir.
- `00007_scheduled_delivery.sql`, campaigns.scheduled_at ve onay sonrası değişmezlik
  korumasını ekler. Dev veritabanına uygulanmadı. Dev'e migration önce, API/web build sonra.
- Onay, zaman ve mesajların next_attempt_at değerleri tek transaction'da yazılır. Alıcılar
  onay anında sabitlenir. Mevcut worker sorgusu zamanı gelmemiş işleri almaz; yeni servis yok.
- Aynı sürüm ve aynı anla tekrar 200; zaman geçmiş olsa da replay yeni iş oluşturmaz.
  Farklı zaman 409; yeni onayda geçmiş zaman 422. PostgreSQL saati ve UTC/mikro-saniye
  normalizasyonu kullanılır. İçerik sürümüne bağlı onay ve gönderimde izin kontrolü korunur.
- GET, sayfalı kampanya listesi ve SSE scheduled_at taşır. Kampanya, liste ve dashboard'da
  plan görünür; durum sözlüğüne yeni değer eklenmedi. Worker kapalıysa işler geç başlayabilir.
- Yerel doğrulama: zamanlayıcının handler/gerçek PostgreSQL testleri geçti. Erken claim yok,
  zamanı gelince gerçek worker/provider gönderiyor; gecikmiş replay ve ofset eşdeğerliği,
  paralel onay, rollback ve migration Down koruması sınandı. Frontend 726 test geçti;
  lint/oxlint, build ve e2e tip kontrolü geçti. Son `make test`, Go vet/tidy/gofmt,
  Prettier ve `git diff --check` temiz. İstanbul saat dilimiyle zaman/form testleri de 17/17 geçti.
  Eski onay kilit testi SELECT metnini birebir arıyordu; scheduled_at alanı eklenince
  sabiti güncellendi. Önceki başarısız koşudan sonra ilgili test ve tam make test geçti.
- `scheduled-delivery-real.spec.ts`: iki `@real` Playwright testi eklendi. Gerçek API/DB/worker/
  provider/SSE, İstanbul saat dilimi; yenilemede planın korunması, erken gönderilmeme ve vadesinde
  7 mesajın tamamlanması, aynı/farklı saatle tekrar onay ve boş/geçmiş saat reddi sınanıyor.
  Yerel `e2e-delivery` koşusu 3/3 geçti (mevcut anlık gönderim + iki yeni zamanlayıcı testi).
  Planlı gerçek gönderim 1,1 dakikada, hata senaryosu yaklaşık 1 saniyede geçti; 7/7 sent,
  sıfır failed ve aynı planla replay doğrulandı. Test saati/fetch/SSE taklit edilmedi.
  `campaign-schedule-e2e` projesi, 18086 portu ve ayrı artifact klasörü kullanıldı; ortam silindi.
  Type-check:e2e, eslint, oxlint, Prettier ve git diff kontrolü temiz. Mevcut CI E2E işinin
  `--grep @real` adımı yeni dosyayı otomatik keşfeder; adım adı zamanlanmış gönderimi de
  belirtiyor. Yeni araç, servis veya job gerekmedi. Gerçek GitHub Actions doğrulanmadı.
  Commit/push yok. Normal `make test` mevcut backend süreç e2e testlerini de içerir.

## Şu an — Kişi listesi ve çalışma alanı sayfaları

**Branch:** `feat/audience-preview-list`. Oğuzhan beş sayfanın aynı branch'e eklenmesini
ve kalıcı şablon tablosu için migration yazılmasını açıkça onayladı (ADR-032).

- Menü: Genel bakış, Yeni kampanya, Kampanyalar, Kitleler, İçerik şablonları.
- Genel bakış gerçek veritabanı özeti; kampanyalar durum filtreli ve sayfalı.
- Kitleler tüm demo kullanıcılarında arama ve sayfalama yapar; kampanyanın segmentine
  uyan kişiler önizlemede ayrı listelenir. Kanal uygunluk sebepleri korunur.
- `00006_content_templates.sql`: kalıcı şablon tablosu. Mevcut doğrulayıcı kullanılır.
  Şablonu kampanyaya uygulamak düzenleyiciyi doldurur; sürüm kaydetme/onay ayrıca gerekir.
  Şablonu düzenleme/silme kaydedilmiş kampanya sürümlerini değiştirmez.
- Yeni API'ler: GET overview, GET audience, GET/POST templates, PUT/DELETE templates/{id};
  kampanya listesinin sayfalı/filtreli çağrısı. Ayrıntılı sözleşme ARCHITECTURE ve WRK-001'de.
- Son yerel doğrulama: frontend 638 test (37 dosya), eslint/oxlint, build ve e2e tip kontrolü
  geçti. `make test`, `go test -count=1 ./...`, go vet, go mod verify/tidy ve gofmt temiz.
  Workspace ve migration Down/Up testleri `-race` ile de geçti. Standart Playwright 40/40,
  gerçek worker/provider/SSE döngüsü 1/1; iç metrics erişimi/port yalıtımı kontrolü geçti.
  Sonuçlar `campaign-workspace-e2e` projesi ve 18084 portunda alındı; test ortamı silindi.
  İsteğe bağlı observability döngüsü bu değişiklikte yeniden çalıştırılmadı; gerçek CI bekliyor.
- Doğrulamada görülenler: başka bir agent aynı `campaign-e2e` projesini kullanınca ilk toplu
  deneme bağlantı hataları aldı; ayrı proje/port ve `E2E_OUTPUT_DIR` ile çakışma giderildi.
  Eski menü/listenin konumuna bağlı testler yeni sayfalara uyarlandı. Mobil taşma testi,
  görsel olarak kırpılmış ekran okuyucu başlıklarını görünür sayıyordu; gerçek computed clip
  kontrolü eklendi. Son tarayıcı koşusu temiz.
- Mevcut `TestDeliveryCompletesCampaignAndRecordsEveryAcceptanceOnce` bir `make test`
  koşusunda bir mesaj için fazladan deneme gördü. Nedeni doğrulanmadı; test değiştirilmedi.
  Aynı test 5 tekrarda ve sonraki tam `make test` koşusunda geçti. CI'da tekrar ederse
  mesajın `last_error`/attempt verisi korunarak incelenmeli; bu kayıt gizlenmedi.
- Dev veritabanına migration uygulanmadı. Commit/push yapılmadı; gerçek GitHub Actions
  sonucu henüz doğrulanmadı. `00006` Down şablon verilerini siler, kampanyaları silmez.

## Tamamlanan hazırlık

- [x] Repo klonlandı ve başlangıç klasörleri oluşturuldu.
- [x] Insider One brief'i `docs/CASE.md` dosyasına eklendi.
- [x] Claude Code talimatları, repo rehberi ve mimari taslak hazırlandı.
- [x] Araçlar WSL'de hazır: Go 1.27.4, Node 24 (repo özelinde `nvm use`
  ile), `make`, Docker ve Compose.
- [x] Compose servisleri, test altyapısı ve metrics kararları ADR-013 ile
  ADR-016 arasında yazıldı. ADR-013 hâlâ taslak: servis listesi ve portlar
  onaylı, başlatma sırası onay bekliyor (sıra segment-preview aşamasında
  uygulandı ve Compose'ta çalıştı).
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

**CI'a bilerek eklenmeyenler:** `npm audit`, ek Go lint aracı,
`docker compose up` ile `/healthz` doğrulaması. (Go bağımlılık cache'i
başlangıçta `go.sum` olmadığı için kapalıydı; segment-preview aşamasında
`go.sum` oluşunca açıldı.) PostgreSQL entegrasyon testleri segment-preview
aşamasında `make test` üzerinden CI'a girdi. e2e ve worker testlerini ekleyen
aşama CI'ı da genişletir.

## Tamamlandı — PostgreSQL, seed ve segment önizleme

**Branch:** `feat/segment-preview` (PR #3 ile `main`'e squash merge edildi,
`170ecb5`). PR #3'te Backend, Frontend ve Compose kontrolleri yeşil (`gh pr
view 3` ile doğrulandı).

- [x] Kurallar, ADR'ler, spec'ler ve backend klasörleri PR #2 ile `main`'e
  alındı. Bu aşamanın seçimleri ADR-019'a ve ADR-003 güncellemesine yazıldı.
- [x] PostgreSQL Compose'a eklendi (`postgres` servisi).
- [x] Veritabanı hazır olduğunda migration ve seed otomatik çalışıyor
  (aşağıdaki `cmd/migrate` ve `cmd/seed` maddesi). Oğuzhan `docker compose
  up` ile temiz kurulumda doğruladı: Postgres, migrate ve seed başarılı.
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
  Compose'ta karşılaşılan Tailwind paket hatası bu yüzdendi. Oğuzhan `web`
  konteynerinde `npm ci` çalıştırarak çözdü.
- [x] `.env.example` ve MockLLM hazır: demo ve testler gerçek API anahtarı
  olmadan MockLLM ile çalışıyor (`LLM_PROVIDER=mock` varsayılan, `cmd/api`
  bağlı). Gerçek Claude entegrasyonu ayrı bir iş olarak aşağıda
  ("2. Gerçek Claude entegrasyonu") duruyor ve bu aşamanın çıkış koşuluna
  dahil değil.
- [x] CI: Backend işi `make test` çalıştırdığı için `postgres` servisini
  Compose ile başlatır. Go bağımlılık cache'i açıldı. PR #3'te Backend,
  Frontend ve Compose işleri yeşil.
- [x] Compose ile uçtan uca doğrulama (Oğuzhan): arayüzde örnek brief için
  toplam 191, SMS'e uygun 141, e-postaya uygun 151 kullanıcı göründü ve draft
  kampanya kaydedildi. Gerçek API anahtarı kullanılmadı (MockLLM).
- [x] Playwright kuruldu ve `frontend/e2e/vue.spec.ts` Chromium'da geçti
  (Oğuzhan yerelde çalıştırdı ve bildirdi; Claude çalıştırmadı). Test yalnızca
  `h1` başlığını ("Kampanya önizleme") doğrular. CI Playwright çalıştırmıyor. (Sonradan: bu test kaldırıldı ve e2e CI'a eklendi, bkz. ADR-024.)

**Çıkış koşulu:** Compose ile açılan arayüz, gerçek API anahtarı olmadan
seed verisi üzerinden beklenen kitle sayılarını gösteriyor. CI yeşil.
Sağlandı.

## Tamamlandı — İçerik üretimi, düzenleme ve onay

**Branch:** `feat/content-approval`: PR #5 ile `main`'e squash merge edildi
(`b5cb799`, 90 dosya, +15.564 / −295). PR #5'te **Backend, Frontend, Compose ve
E2E** işlerinin hepsi yeşil. Her işteki `CANCELLED` kaydı sorun değildir: PR
kapatılıp yeniden açıldığında (Oğuzhan bildirdi) ilk çalışma iptal olmuştur.
Merge sonrası `main` push CI'ı da başarıyla tamamlandı (`success`, doğrulandı).

**Spec:** [CNT-001](specs/CNT-001-content-generation-and-approval.md). Kararlar:
ADR-011 (onay sürüme bağlı, onaylandı), ADR-022 (fallback metni, ayrı LLM
arayüzü, en uzun ad hesabı, onaylandı) ve ADR-023 (kanala uygunluk: izin ve adres,
onaylandı → B).

### Yapılanlar

**1. Doğrulama ve üretim (backend paketleri)**

- [x] `internal/content`: içerik doğrulayıcı (JSON şeması, `{{first_name}}`
  whitelist'i, en uzun ad ile SMS 160 / konu 80 kod noktası sınırı, NUL ve konuda
  satır sonu reddi), `Render`, kanal bazlı `CheckSMS` / `CheckSubject`.
  Fallback metni `Değerli müşterimiz` (ADR-022).
- [x] Akış sırasında JSON parçalarının çözümlenmesi: parça sınırından bağımsız
  artımlı çözücü (`delta`, `Replace` ile); kaçışlar, `\uXXXX`, vekil çiftleri ve
  bölünmüş UTF-8 `encoding/json` ile aynı sonucu verir (fuzz testli).
- [x] `content.Generator`: geçersiz çıktıda tek düzeltme denemesi (düzeltme
  çıktısı önizlemeye akmaz); zaman aşımı, 429 ve kesilen akışta düzeltme yok;
  başarısızlıkta doğrulanmamış `partial` metinler düzenleme için sunulur.
- [x] `llm.ContentClient` (ayrı arayüz) ve `llm.Provider`; MockLLM ve `Scripted`
  istemcisi içerik akışını destekler. Testler gerçek API anahtarı gerektirmez.

**2. Veritabanı**

- [x] Migration `00002_content_versions_and_messages.sql`: `campaign_contents`,
  `campaigns` onay alanları, tam `messages` tablosu ve üç indeks, üç
  tetikleyici (sürüm satırı değişmez, onaylı kampanyaya sürüm eklenemez,
  onaylanan sürüm değişmez). İçerik metni kısıtı (`content_text_valid`) Go
  doğrulayıcısının tam aynası değil, temel DB korumasıdır; uygulamadaki kilit ve
  durum kontrolleri kalır.
- [x] `Down` koruması: draft olmayan kampanya varken hiçbir veri silmeden
  reddedilir. Koruma kontrolden önce `campaigns`, `campaign_contents` ve
  `messages` tablolarını kilitler; kilitsiz korumada, commit edilmemiş bir onay
  varken Down başarılı olup onay verisini siliyordu (tek kullanımlık testle
  doğrulandı). Artık uçuştaki onayı bekleyip reddeder.

**3. İçerik API'si**

- [x] İçerik sürümü store'u (`campaign.ContentStore`): kampanya satırı
  `FOR UPDATE`, durum kilit altında kontrol, `max(version)+1` (kesintisiz
  1..N), onaydan sonra `ErrNotDraft`; son sürümü ve en uzun gerçek adı okur.
- [x] `POST /api/campaigns/{id}/content`: SSE (`delta`, `validated`, `error`).
  `validated` yalnızca commit sonrası gelir; kesilen/zaman aşımı/429/geçersiz
  üretimde sürüm yoktur; üretim sırasında onay gelirse `CAMPAIGN_NOT_DRAFT`;
  LLM çağrısı boyunca kilit tutulmaz.
- [x] `PUT /api/campaigns/{id}/content`: aynı doğrulayıcı, yeni `human` sürümü,
  geçersizde `422`, onaydan sonra `409`.
- [x] `GET /api/campaigns/{id}`: son doğrulanmış sürümü `content` alanıyla, onaylı
  kampanyada `approved_content_version` ve `approved_at` ile döndürür.
- [x] SSE güvenilirliği: yazma veya flush hatasında üretim iptal edilir ve sürüm
  kaydedilmez (başlıklar yazılamıyorsa LLM hiç çağrılmaz); her yazmadan önce
  yenilenen yazma süresi (10 sn, `Deps.SSEWriteTimeout`) yavaş istemciyi keser.
- [x] `cmd/api` Mock ile bağlandı. Sözleşme ARCHITECTURE §5'te.

**4. Onay (`POST /api/campaigns/{id}/approve`)**

- [x] Alıcı seçimi: `segment.BuildRecipients` (kitle sayısıyla aynı koşullar,
  tek sorgu, onay anındaki izinler); kanal bazında yalnızca ilgili kanalın metni
  alıcıya göre render edilir ve sınır doğrulanır.
- [x] `campaign.Approver`: onay, onaylanan sürüm ve `queued` mesajlar aynı
  transaction'da yazılır. `409`: sürümsüz, eski sürüm, başka sürümle onaylı;
  `422`: render sınırı (alan, etkilenen alıcı sayısı, örnek ad), uygun alıcı
  yok, filtresiz segment; ilk onay ve aynı sürümle tekrar `200` (tekrar
  alıcıları yeniden hesaplamaz). Onaydan önce hiçbir yolla kuyruğa iş girmez.
- [x] Kanala uygunluk açık izin **ve** adres ister (ADR-023 → B, onaylandı):
  `segment.BuildCount` ve `segment.BuildRecipients` aynı ifadeyi kullanır;
  önizleme ile kuyruk aynı kümeyi sayar, onay yanıtındaki `without_address`
  kaldırıldı. Seed'li örnek brief için önizleme SMS 141 → 134 (e-posta 151);
  önizleme ve kuyruk birebir eşit. Test: SQL ve alıcı sorgusu birim testleri,
  fixture'lı entegrasyon testi (izinli ama telefonsuz/boş telefonlu/boş
  e-postalı kullanıcılar), seed verisi oracle'ı ve önizleme = kuyruk testi;
  mutasyonla (telefon şartını yalnızca sayımdan çıkarma) yakalandığı görüldü.

**5. Vue ekranı (`/campaigns/:id`)**

- [x] `CampaignView`: kampanyayı yükler (`GET`), sayfa yenilenince son
  doğrulanmış içeriği ve onay durumunu geri yükler. Önizleme ekranından
  "İçeriği hazırla" bağlantısıyla açılır.
- [x] İçerik üretimi SSE ile canlı gösterilir: geçici önizleme salt okunurdur,
  `validated` gelince önizleme doğrulanmış sürümle tümüyle değiştirilir (eski
  metne eklenmez). Üretim sürerken tekrar üretim ve onay kapalıdır.
- [x] Hata durumları: doğrulanamayan üretimde sebepler Türkçe ve alanın altında
  gösterilir, doğrulanmamış `partial` metin düzeltmeye açılır (onay yoktur);
  zaman aşımı, 429 ve yarıda kesilen akışta önceki kayıtlı sürüm korunur ve
  "Yeniden üret" sunulur; üretmeden "İçeriği elle yaz" seçeneği vardır.
- [x] Düzenleme: SMS (160) ve konu (80) için kod noktası sayacı ({{first_name}}
  için fallback uzunluğu varsayan bir tahmin; kesin kontrol sunucuda),
  kaydedilmemiş değişiklik uyarısı, "Yeni sürüm olarak kaydet" (`PUT`),
  `422` sebepleri alan altında.
- [x] Onay: iki adımlı (kutu açılır, "Onayla ve kuyruğa al"), yalnızca
  kaydedilmiş son sürümle ve kaydedilmemiş değişiklik yokken; çift tıklama tek
  istek yapar. Başarıda kuyruğa alınan SMS/e-posta sayıları gösterilir ve içerik
  kilitlenir. `STALE_CONTENT_VERSION` ve `CAMPAIGN_ALREADY_APPROVED` ile son
  durum yeniden yüklenir; render sınırı aşımında etkilenen alıcı sayısı ve örnek
  ad gösterilir. Metinler `v-html` ile değil metin olarak basılır.
- [x] Ekran kapanınca veya başka kampanyaya geçilince (`/campaigns/A` →
  `/campaigns/B`, aynı bileşen yeniden kullanılır) süren üretim, kaydetme ve
  onay istekleri iptal edilir, işlem bayrakları ve önceki kampanyanın taslağı
  sıfırlanır, A'dan geç gelen yanıtlar B'yi değiştirmez. Bu geçişte düğmelerin
  "Üretiliyor…/Kaydediliyor…" durumunda kilitli kalması hatası bulundu ve
  düzeltildi (regresyon testleri önce başarısız oldu).
  **İptal, tamamlanmış (commit edilmiş) bir kaydı geri almaz.** İstemci iptali
  yalnızca isteği keser: sunucuda işlem commit edilmişse (yanıt istemciye
  ulaşmamış olsa bile) sürüm veya onay kalır; commit'ten önce bağlam iptal
  olursa işlem geri alınır. İstemci hangisinin olduğunu bilemez, bu yüzden
  A'ya dönülünce durum sunucudan yeniden okunur. Onay için ikisi de testle
  doğrulandı (commit öncesi iptalde kampanya draft ve mesajsız kalır;
  tamamlanmış onay sonradan gelen iptalle geri alınmaz; mutasyon yakalandı).
  İçerik üretimi için commit sonrası iptalde sürüm kalır, commit öncesi iptalde
  kaydedilmez (birim ve entegrasyon testli). `PUT` kaydetme için aynı bağlam
  mekanizması geçerli, ama commit öncesi iptal ayrıca test edilmedi.
- [x] Ekran kapanınca süren istekler ve akış iptal edilir. API istemcisi
  (`api/content.ts`, `api/sse.ts`): artımlı SSE ayrıştırıcı (parça sınırı, CRLF,
  bölünmüş UTF-8), akış kopması `STREAM_INTERRUPTED`.
- [x] Kitle sayıları açıklaması ADR-023'e göre güncellendi (izin ve adres).

**6. Frontend e2e testleri (Playwright, ADR-024)**

- [x] İzole ortam: `compose.e2e.yaml` (proje `campaign-e2e`): ayrı container, ağ
  ve volume'lar, postgres host'a açılmaz ve tmpfs'te çalışır, api host'a açılmaz,
  web yalnızca `127.0.0.1:18080`. Yerelde `make e2e` (kur, koştur, sil; çıkış
  kodu testlerin kodu, başarısızlıkta servis logları) ve CI'da aynı tanım.
  Geliştirme veritabanına ve portlarına (8080, 8081, 5173, 5432) dokunmaz;
  `playwright.config.ts` adres verilmezse veya bu portlar verilirse testleri
  başlatmaz (doğrulandı).
- [x] Kontrollü veri: `SEED_DATASET=e2e` (`seed.GenerateE2E`, `cmd/seed`): 10
  kullanıcı, rastgelelik yok, tarihler 30 günlük sınırdan en az 15 gün uzak.
  CASE.md örnek brief'i için sayılar kesin: toplam 7, SMS 3, e-posta 4 (birim
  testleri, referans zamanı kaydırma, SQL entegrasyon testi).
- [x] Dört test, her biri kendi kampanyasını oluşturur: (1) brief → kitle
  (kesin sayılar) → içerik → düzenle → kaydet → onayla → yenileyince onaylı durum
  korunur (sunucu durumu da doğrulanır); (2) desteklenmeyen değişken: kaydetme
  reddedilir, sürüm artmaz, onay kapalı; (3) kaydedilmemiş düzenleme: onaylanamaz;
  (4) iki sekme: eski sürümü onaylamak reddedilir, son sürüm yüklenir, devam
  edilebilir. Tek tarayıcı (Chromium). Önceki tek basit test
  (`e2e/vue.spec.ts`, başlık kontrolü) ana akış testi tarafından kapsandığı için
  kaldırıldı.
- [x] CI: `E2E` işi eklendi (Playwright Chromium kurulumu, `make e2e-up`, testler,
  başarısızlıkta servis logları ve rapor/trace artifact'ı, her durumda ortamın
  silinmesi); Compose işi e2e yapılandırmasını da doğrular. CI henüz çalışmadı.
- [x] Doğrulama (yerelde, izole ortamda): 4 test geçti; her test 5 kez (20 koşu),
  4 worker paralel ve her biri tek başına geçti; mutasyonlar (onay kapısı, eski
  sürüm yenilemesi, salt okunur kilit, kitle sayıları) beklenen testlerce
  yakalandı; `make e2e` baştan sona (çıkış kodu 0) ve kasıtlı kırılmış hâlinde
  (çıkış kodu 2, loglar yazıldı, ortam silindi) denendi.

**7. Testler ve doğrulama**

- [x] Frontend (Node 24, CI'ın frontend işiyle aynı komutlar): `oxlint`,
  `eslint`, `vitest run` ve `npm run build` (tip kontrolü dahil) temiz; 186 birim
  testi geçiyor (73 mevcut + 113 yeni: SSE ayrıştırıcı, API istemcisi, taslak ve
  mesaj yardımcıları, `ContentEditor`, `ApprovePanel`, `CampaignView`, route ve
  bağlantı). Mutasyon kontrolleriyle (önceki sürümü geri yükleme, kod noktası
  sayacı, iki adımlı onay, kopan akışın başarı sayılması) testlerin yakaladığı
  görüldü. `docker compose config --quiet` temiz.
- [x] Gerçek tarayıcıda (Chromium, Playwright betiği; depoda değil) uçtan uca
  denendi: Vue ekranı → Vite proxy → gerçek `cmd/api` (MockLLM) →
  `campaigns_test`. Önizleme, içerik üretimi (SSE), sayfa yenileme, geçersiz
  değişken (422, alan altında), yeni sürüm (insan), iki adımlı onay, onaylı
  kampanyanın salt okunur yüklenmesi; önizleme SMS 132 / e-posta 150 = kuyruk
  132 / 150; konsolda hata yok.
- [x] Vite dev proxy'sinin SSE'yi tamponlamadığı, 0,4 sn aralıklı olay yollayan
  deneme sunucusuyla ölçüldü (olaylar tek tek geldi). Compose'taki `web`
  konteyneri (aynı Vite proxy'si) ve nginx'e geçiş sonrası ayrıca doğrulanacak.
- [x] Birim testleri: doğrulayıcı, çözücü (fuzz), `Generator`, HTTP
  handler'ları (sahte bağımlılıklarla), alıcı sorgusu, kanal bazlı render.
- [x] Gerçek PostgreSQL (`campaigns_test`) üzerinde store ve endpoint
  entegrasyon testleri: üretim/yeniden üretim/düzenleme akışı, başarısız
  üretimde sürüm yok, onay, alıcı/kanal/izin/adres matrisi, seed verisiyle
  önizleme-kuyruk uyumu (oracle), atomiklik, onay öncesi kuyruğa iş girmemesi.
- [x] Veritabanı constraint, tetikleyici ve eşzamanlılık testleri: iki
  bağlantılı kilit testleri, eşzamanlı yazar/onay senaryoları (kilit beklemesi
  `pg_stat_activity` ile gözlenir, rastgele gecikme yok), eşzamanlı çift onay,
  onay/içerik düzenleme yarışı, migration Down/Up gidiş-dönüşü, Down koruması,
  dolu veritabanına uygulama.
- [x] Hataları yakalayan regresyon testleri, önce mevcut kodda başarısız olduğu
  görülerek eklendi: SSE yazma/flush hatası, başlık flush hatası, yazma süresi
  yenileme, yavaş istemci (gerçek sunucu), Down yarışı. Ayrıca
  mutasyon kontrolleri (tetikleyiciler, `FOR UPDATE`, kayıt sırası, onay
  mantığı) testlerce yakalandı.
- [x] Race kontrolleri: `go test -race` ile içerik, HTTP ve entegrasyon
  eşzamanlılık testleri, tekrarlı koşturmalarla.
- [x] Gerçek `cmd/api` ikilisiyle `curl` duman testleri (yalnızca
  `campaigns_test`, MockLLM): içerik, düzenleme, onay ve SIGTERM ile temiz
  kapanış (çıkış kodu 0). API'nin kontrollü kapanması PR #3'te yapılmıştı; bu
  branch'te yalnızca yeni uç noktalarla birlikte yeniden doğrulandı.
- [x] Oğuzhan Compose üzerinde, **doğrudan `localhost:8081` (api) üzerinden**
  `curl` ile uçtan uca elle doğruladı (içerikler kayda alınmadı):
  - `POST /api/campaigns` → `201`, draft kampanya; toplam 191, SMS 141,
    e-posta 151.
  - `POST /api/campaigns/{id}/content` → SSE `delta` olayları, ardından
    `validated`; içerik sürüm 1 (`llm`) kaydedildi.
  - `GET /api/campaigns/{id}` → kampanya ve içerik sürüm 1 geri okundu.
  - `PUT /api/campaigns/{id}/content` → `200`; düzenlenen içerik `human`
    kaynaklı sürüm 2 olarak kaydedildi, `GET` sürüm 2'yi döndürdü.
  - `POST /api/campaigns/{id}/approve` sürüm 2 → `200`, kampanya `approved`;
    133 SMS ve 150 e-posta, toplam 283 mesaj işi; 7 telefonsuz alıcı
    `without_address` olarak raporlandı (o sırada ADR-023 seçenek A
    uygulanıyordu; B sonrası bu alan kalktı ve önizleme ile kuyruk eşit).
  - Aynı sürümle tekrar onay → `200`; onay zamanı ve mesaj sayıları aynı kaldı,
    `without_address` tekrar yanıtta yoktu (tasarım gereği: yalnızca ilk onayda).
  - Sürüm 1 ile onay → `409 CAMPAIGN_ALREADY_APPROVED`.
  - Not: önizleme 141 SMS / 151 e-posta iken kuyruğa 133 + 7 telefonsuz = 140
    SMS ve 150 e-posta girdi; birer kişilik fark var. Olası neden önizleme ile
    onay arasında 30 günlük pencerenin kayması (alıcılar onay anında seçilir,
    ADR-023); bu açıklama doğrulanmadı.
  - Not: ilk içerik üretiminde dev veritabanında `campaign_contents` tablosu
    yoktu (migration `00002` uygulanmamıştı) ve istek hata verdi. Migration
    önce `campaigns_test`'te doğrulandı, sonra Oğuzhan dev veritabanına
    uyguladı ve aynı uç nokta başarılı çalıştı.
  - Bu denemeler `web` (8080) proxy'si üzerinden değil, api'ye doğrudan
    yapıldı.
- [x] Dokümanlar: ARCHITECTURE §4-§6, FAILURE-MODES, REPO-STRUCTURE, CNT-001
  açık noktaları, DECISIONS (ADR-011, ADR-022, ADR-023).

### Açık kalanlar (sonraki aşamalara taşınanlar)

Aşağıdaki `[ ]` maddelerin hiçbiri bu aşamanın çıkış koşulunu engellemedi; ilgili
aşamada veya ayrı küçük işler olarak ele alınır, sıra Oğuzhan'la belirlenir.

- [x] **Karar (Oğuzhan): ADR-023 → B uygulandı** (yukarıda, "Onay" bölümü).
- [x] Vue ekranı ve frontend lint, test, build kontrolleri (yukarıda).
- [x] Onay uç noktasının Compose üzerinde elle doğrulanması (Oğuzhan, `8081`
  üzerinden; yukarıda ayrıntılı).
- [x] CI: PR #5'te Backend, Frontend, Compose ve E2E işleri yeşil (gerçek runner'da
  Chromium ve izole Compose ortamıyla dahil); merge sonrası `main` push CI'ı da
  `success`. İptal görünen ilk çalışma PR'ın kapatılıp yeniden açılmasından
  kaynaklanır.
- [ ] `llm_calls` tablosu (kampanya, tür: segment veya içerik, deneme,
  doğrulama sonucu, kısa hata kodu, gecikme, token sayıları; ham yanıt
  saklanmaz) ve `llm_validation_failures_total` metriği (ARCHITECTURE §4, §12).
  Kapsamı ve zamanı Oğuzhan'la netleştirilecek.
- [ ] SSE keepalive: düzeltme denemesi sırasında ve ilk parçaya kadar hiç olay
  gitmez (en kötü durumda iki çağrı × 60 sn). nginx'in varsayılan
  `proxy_read_timeout` değeri 60 sn; ADR-014'te nginx'e geçildiğinde bağlantı
  kopabilir. Öneri: periyodik `: keepalive` yorum satırı.
- [x] Compose'taki `web` (8080) konteyneri üzerinden SSE ve Vue ekranı elle
  denendi (Oğuzhan, "OK" olarak bildirdi; hangi adımların denendiği ayrıntılı
  kayda alınmadı). Vite dev proxy'si ayrıca yerelde ölçüldü (tamponlama yok).
  nginx'e geçişte (ADR-014) `proxy_buffering off` ve `proxy_read_timeout`
  yeniden doğrulanmalı.
- [x] Aktif bir SSE akışı sırasında SIGTERM davranışı: **çözüldü ve test edildi**.
  Dashboard akışı `cmd/api/shutdown_stream_test.go`, içerik üretimi akışı
  `cmd/api/shutdown_content_test.go` (ayrıntı: D3 sonrası maddeler).
- [x] `httpapi.New` nil bağımlılık doğrulaması ve `cmd/api` bağlama testi: **çözüldü**
  (`Deps.Validate()` başlangıçta çağrılır; `newHandler` gerçek veritabanıyla test
  edilir). `New` bilerek toleranslı kaldı (ADR-027 seçenek 6).
- [ ] Maliyet ve kötüye kullanım (ürün kararı): `POST /content` yetkisiz ve hız
  sınırsız, aynı kampanya için paralel üretim paralel sürüm üretir, `PUT` ile
  sürüm sayısı sınırsız büyür. Seçenekler: kampanya başına tek eşzamanlı
  üretim, sürüm üst sınırı.
- [ ] E-posta gövdesi için uzunluk sınırı: CASE.md yalnızca SMS 160 ve e-posta
  **konusu** 80 karakter der (satır 32); gövde için sınır CASE.md'de ve CNT-001'de
  tanımlı değil. Şu an yalnızca boş olmama ve değişken kuralları var.
- [ ] En uzun ad hesabı şimdilik tüm kullanıcılardan alınır (ADR-022); segmentle
  sınırlamak istenirse ayrıca karar verilir. `MaxFirstNameRunes` her istekte
  `users` tablosunu tarar (3 bin kullanıcıda sorun değil).
- [x] Playwright e2e akışı (içerik ve onay) ve CI'a eklenmesi: yapıldı (yukarıda,
  "6. Frontend e2e testleri"); CI'daki `E2E` işi PR #5'te yeşil geçti.

**Çıkış koşulu:** Kullanıcı taslağı canlı görüyor, düzenleyebiliyor ve
gördüğü sürümü onaylayabiliyor. Geçersiz onay kuyruk işi oluşturmuyor.
**Sağlandı:** backend ve arayüz hazır; yerelde, Compose'ta (`web` dahil) ve CI'da
(Backend, Frontend, Compose, E2E) doğrulandı.

## Sonraki adımlar

Her aşama güncel `main`'den açılan ayrı bir branch'te ilerler. Branch
adları öneridir. 1. Worker, mock provider ve dashboard aşaması tamamlandı ve
`main`'e alındı. Şu an çalışılan aşama "Frontend iyileştirmeleri" (aşağıda,
`feat/frontend-modernization`). Yukarıdaki "Açık kalanlar" ve aşağıdaki "Açık konular"
bölümündeki işler (SSE keepalive, `llm_calls`, maliyet sınırları vb.) ayrı bir
kararla ele alınır.

### 1. Worker, mock provider ve dashboard (tamamlandı)

**Branch:** `feat/worker-dashboard`: PR #7 ile `main`'e squash merge edildi
(`fffdded`, 2026-11-19 20:43 UTC). PR #7'de **Backend, Frontend, Compose ve E2E**
işlerinin hepsi `SUCCESS` (`gh pr view 7` ile doğrulandı); merge sonrası `main` push
CI'ı da `success`. Aşağıdaki maddeler aşamanın o günkü kaydıdır.

**Specler:** [SND-001](specs/SND-001-message-delivery.md),
[TRK-001](specs/TRK-001-live-delivery-tracking.md). **Kararlar:** ADR-002, 005 ve
006 onaylandı (2026-11-19); ADR-025 (mock provider) eklendi. `failure_reason`
yalnızca `exhausted` ve `rejected`. `429` kalıcı hata değildir (retry), `429`
dışındaki `4xx` `rejected`'tır.

- [x] Worker ve mock provider yazıldı ve Compose'a eklendi (`worker`,
  `mock-provider`; `cmd/worker`, `cmd/mock-provider`). Worker kanal iznini limiter
  beklemesinden sonra, provider çağrısından hemen önce (sahiplik ve lease ile
  aynı sorguda) kontrol ediyor; izin yoksa `skipped`, provider'a istek gitmiyor.
- [x] Mock provider gecikme, `429` (`Retry-After` ile) ve `500` üretiyor.
  Başarılı kabulü migration `00003` (`mock_provider_acceptances`) ile
  PostgreSQL'e idempotency anahtarıyla atomik kaydediyor. Aynı anahtarla farklı
  alıcı, kanal veya içerik `422` ile reddediliyor. `accept_then_drop` yalnızca
  ilk kabul yanıtını düşürüyor; sürekli yanıt kaybı ayrı mod
  (`drop_all_responses`).
- [x] Aynı anahtarla retry, azami deneme, lease kurtarma ve eski `claim_token`
  ile sonuç yazılmasının engellenmesi doğrulandı (aşağıda).
- [x] SMS ve e-posta ayrı limiter'larla sınırlı (retry dahil). Provider'a
  ulaşan isteklerin zaman damgalarıyla kayan 1 sn pencere ölçüldü: kanal başına
  en çok 96 (CPU yükü altında 97), sınır 100. Limitsiz kontrol koşusunda 300.
- [x] **Dashboard frontend'i** (TRK-001): `/campaigns/:id/delivery` ekranı. Ayrıntı
  aşağıda ("Dashboard frontend'i").
- [x] **Dashboard SSE uç noktası (backend, D3):** `GET /api/campaigns/{id}/events`
  ([ARCHITECTURE §5](ARCHITECTURE.md) sözleşmesi). Tek sorguluk sayım
  (`campaign.Store.DeliverySnapshot`), `internal/httpapi/delivery.go` işleyicisi.
  - **Heartbeat:** kuyrukta değişiklik olmasa da her aralıkta (1 sn) görüntü
    yollanır; arayüz sessiz bağlantıyı takılmış sayıp yenilediği için gerekliydi.
  - **Hata ve temizlik:** okuma başına 3 sn süre sınırı → `error` olayı ve kapanış;
    `completed` görüntüsünden, istemci ayrılınca ve sunucu kapanırken (`Deps.Shutdown`)
    akış biter. `cmd/api` `newServer` bu kapanış kanalını `RegisterOnShutdown` ile bağlar.
  - **Testler:** `tests/integration/delivery-snapshot_test.go` (7, gerçek PostgreSQL
    sayımı), `delivery-stream_test.go` (4, gerçek handler + gerçek veritabanı:
    değişiklik sonraki görüntüde, `completed` akışı kapatır, boşta aynı görüntüler,
    404, draft), `internal/httpapi/delivery_test.go` (15, `-race` temiz),
    `cmd/api/shutdown_stream_test.go` (2, açık akışla kapanış). Mutasyonlar
    (heartbeat'i kaldırma, okuma süre sınırını kaldırma, `Shutdown` izlemeyi
    kaldırma, `RegisterOnShutdown` bağını kaldırma) testlerce yakalandı.
  - **Gerçek yığın e2e:** `frontend/e2e/delivery-real.spec.ts` (`@real`, hiçbir şey
    taklit edilmez): kullanıcı gibi oluştur, içerik üret, onayla → dashboard → gerçek
    worker + mock provider (`accept_then_drop`, her mesajın ilk yanıtı düşer) + gerçek
    SSE ile `Tamamlandı`, 7/7 gönderildi, 0 başarısız. `make e2e-delivery` (3,2 sn)
    geçti. Worker ve provider olmayan ortamda test beklenen şekilde başarısız oldu
    ("Onaylandı, kuyrukta"). Taklitli testler hata, kopma + yeniden bağlanma,
    takılma ve mobil için kaldı.
  - **Make/CI:** `e2e-standard` (`--grep-invert @real`, `delivery` profili kapalı),
    `e2e-delivery` (profil açık, `--grep @real`), `e2e` ikisini koşar; `e2e-down` ve
    `e2e-logs` profili de kapsar. CI E2E işi ikinci döngüyle genişletildi (yeni araç
    yok).
- [x] **Sağlık ve metrik uçları (D1, D2):** ADR-016 ve TRK-001'e göre; uygulama
  ayrıntıları [ADR-027](../DECISIONS.md#adr-027--metrik-ve-sağlık-uçlarının-uygulama-ayrıntıları)
  (onaylandı, 2026-11-19).
  - **`/healthz`** (önceden vardı, veritabanına bakmaz) ve **`/readyz`** (veritabanı
    ping'i, 2 sn süre sınırı; kesintide `503`, dönünce `200`; hata ayrıntısı yanıta
    sızmaz, yalnızca durum değişince loglanır). `httpapi.Deps.Validate()` zorunlu
    bağımlılıklar eksikse API'nin başlamasını engeller (`cmd/api` çağırır).
  - **`/metrics`**: yeni `internal/metrics` paketi (bağımlılıksız Prometheus metin
    yazıcısı: sayaç, histogram, `Serve`). API ve worker ana porttan **ayrı iç
    dinleyicide** (`METRICS_ADDR`, Compose'ta `:9090`, `expose`, `ports` yok) sunar;
    ana API portunda `/metrics` yoktur (`404`).
  - **Worker:** `messages_sent_total`, `send_attempts_total{outcome}` (6 sonucun hepsi
    her zaman), `stale_result_total`, `reaper_requeued_total`,
    `ratelimit_wait_seconds` (**histogram**; limiter + kapı beklemesi, deneme başına
    bir gözlem).
  - **API:** `queue_depth{channel}` (her okunuşta PostgreSQL'den; `sms` ve `email`
    her zaman) ve `llm_validation_failures_total` (segment ve içerik üretiminde
    doğrulamadan geçemeyen model çıktısı; insan düzenlemesi sayılmaz). Kuyruk
    derinliği okunamazsa `503`, kısmi çıktı yok.
  - **Testler:** `internal/metrics` (biçim, kaçış, histogram, nil güvenliği, `Serve`),
    worker metrikleri (7, mutasyonla doğrulandı), `httpapi` (sayma kuralları, toplayıcı,
    `Validate`), `cmd/api` (`readyz` 7 test; `newHandler` bağlama testi **gerçek
    veritabanı + MockLLM ile bütün rotalar**, `Approver` bağını kaldıran mutasyonla
    doğrulandı), entegrasyon (`queue-depth_test.go`, 5 test, gerçek PostgreSQL),
    backend e2e (`observability_test.go`, gerçek API ve worker süreçleri: `/readyz`
    kesinti, toparlanma ve takılma; worker yokken birikim ve erime; worker sayaçları
    tam değerlerle ve API'de yok). Compose üzerinde `make e2e-metrics-internal`
    (iç ağdan okunur, ana port `404`, host'a yayımlanmaz) çalışan ortamda geçti;
    api portu yayımlanırsa doğru nedenle kırıldığı denendi. CI'ın E2E işine eklendi.
  - **`promtool` doğrulaması (2026-11-19):** izole `delivery` ortamında gerçek bir
    gönderimden sonra (7 mesaj, `accept_then_drop`) API (`:9090`) ve worker (`:9090`)
    çıktıları `promtool check metrics` ile denetlendi (`prom/prometheus:v3.5.0`
    imajındaki `promtool`, tek seferlik `docker run`; Compose'a servis eklenmedi):
    **iki çıktı da temiz, `rc=0`, uyarı yok**. Negatif kontrol: `_total` eksik sayaç ve
    gauge'da `_total` soneki `rc=3` ve 4 uyarıyla yakalandı, yani araç gerçekten lint
    yapıyor. `--extended` kardinalite: API 3 seri, worker 24 seri (15'i histogram
    kovaları). Yeniden üretmek için (ortam açıkken):

    ```sh
    E="docker compose -f compose.yaml -f compose.e2e.yaml --profile delivery"
    $E exec -T web wget -qO- http://worker:9090/metrics \
      | docker run --rm -i --entrypoint promtool prom/prometheus:v3.5.0 check metrics
    ```

    Otomatik hedef/CI adımı eklenmedi: CI'a yeni araç eklemek onayınıza bağlı.
  - **Sınırlar:** `ratelimit_wait_seconds` kova sınırları sabit. Metrics portu
    çakışması başlangıç hatasıdır, otomatik test yok. Gerçek bir Prometheus
    **sunucusuyla** scrape denenmedi (yalnızca `promtool` lint'i). Yeni bağımlılık
    eklenmedi (ADR-027 seçenek 2).
- [x] **Aktif içerik üretimi sırasında SIGTERM** (`POST /content`): test edildi
  (`cmd/api/shutdown_content_test.go`, gerçek `newServer` + `newHandler` + `serve`,
  gerçek PostgreSQL, akış ortasında tutulan yavaş LLM). Önceki "kapanışı bekletebilir"
  varsayımı **kısmen doğrulandı, hata değil, tasarım**: devam eden üretime kapanış
  penceresi (15 sn) tanınır.
  - Pencere içinde biten üretim tamamlanır: istemci `validated` alır, sürüm kaydedilir,
    sunucu üretim biter bitmez temiz döner, yeni bağlantı kabul edilmez.
  - Pencereyi aşan üretim kesilir: sunucu `kapanış tamamlanamadı` hatasıyla döner,
    LLM çağrısının bağlamı iptal olur (arkada sürmez), **hiçbir sürüm kaydedilmez**.
  - Mutasyon (`Shutdown` yerine anında `Close`) iki testi de yakaladı.
  - **Düzeltilen pürüz:** pencere aşılınca `main` "api başlatılamadı" yazıyordu
    (başlatma değil kapanış hatası). `serve` artık çalışırken veya kapanırken oluşan
    hataları `*servingError` olarak döndürür, kapanış süresi aşımı ayrıca
    `errShutdownIncomplete`'tir; `main` `exitMessage` ile mesajı seçer
    ("api kapanışı süre içinde tamamlanamadı; açık istekler kesildi" /
    "api sunucusu beklenmedik şekilde durdu" / "api başlatılamadı"). Çıkış kodu
    değişmedi (`1`). Regresyon testi `cmd/api/exit_test.go` (önce derlenmediği için
    kırmızıydı, düzeltmeden sonra yeşil). Worker'ın `main`'i aynı kalıbı taşıyor ama
    `Run` hatası orada beklenmedik bir durumdur; ele alınmadı.
- [x] **Görsel düzeltmeler ve imleç (frontend):**
  - **Koyu tema:** shadcn token'ları yalnızca `.dark` sınıfında koyulaşıyordu ve sınıf
    hiç eklenmiyordu; işletim sistemi koyu temadayken beyaz kartlar ve koyu zeminde
    koyu yazılı rozetler çıkıyordu. Token'lar ve `dark:` varyantı
    `prefers-color-scheme: dark`'a bağlandı; zemin, yüzey, metin ve kenarlık
    `--app-*` paletinden gelir (kartlar uygulamanın kendi yüzeyleriyle aynı renk).
    Karar notu ADR-021'de (onaylandı, 2026-11-19).
  - **Kontrast:** açık temada shadcn'ın varsayılan kırmızısı (`destructive`)
    "Yeniden bağlan" düğmesinde 4,45'te kalıyordu (AA 4,5); `--app-danger`'a bağlandı.
  - **Rozet:** dashboard'da bağlantı rozeti "Tamamlandı" kampanya durumuyla aynı
    sözcüktü; "Kapandı" oldu.
  - **`cursor: pointer`:** `button`, `[role=button|checkbox|switch|tab|menuitem]`,
    bağlantı, `summary`, `select`, onay ve radyo kutuları için global kural
    (`styles.css`, devre dışı olanlar hariç). Tailwind ön ayarı düğmelerde varsayılan
    imleci bırakıyordu.
  - **Test:** `e2e/visual-themes.spec.ts` açık ve koyu temada kampanya, onay ve
    dashboard (hata, tamamlandı) ekranlarında WCAG kontrastını (≥ 4,5) ve her
    tıklanabilir öğenin `cursor: pointer` olduğunu doğrular; ekran görüntüleri
    `test-results/`'a yazılır. Mutasyonlarla doğrulandı (koyu temada beyaz kart →
    kontrast 2,89 ile kırıldı; düğme imleç kuralı kaldırılınca kırıldı).
    `e2e/tsconfig.json`'a yalnızca tip kontrolü için `DOM` lib eklendi
    (`evaluate` geri çağrıları tarayıcıda çalışır).
  - **Not:** ekran görüntüsündeki "Böyle bir adres yok" hatası kod hatası değildi:
    geliştirme ortamındaki API'nin eski kodla çalışması (yeni `/events` ucu yok).
    API'yi güncel kodla yeniden başlatmak gerekir.

**Gözlemlenebilirlik, nginx ve teslim hazırlığı (2026-11-19):**

- [x] **Prometheus (ADR-028, onaylandı):** `prom/prometheus:v3.5.0`, yalnızca
  `observability` profiliyle (`docker compose --profile observability up`); normal
  `docker compose up` yedi servis olarak değişmedi. `observability/prometheus.yml`
  `api:9090` ve `worker:9090`'ı Docker iç ağından scrape eder (uçlar host'a
  yayımlanmaz). Arayüz yalnızca `127.0.0.1:9091` (e2e'de `19091`). Saklama 2 gün ve
  256 MB. Dev yığınında (sizin çalıştırmanız) Prometheus `healthy`, api, worker ve
  prometheus hedefleri `up`, bayraklar `2d`/`256MiB`.
- [x] **Gerçek scrape doğrulaması:** `make e2e-observability` (`observability/verify.sh`):
  api ve worker `up`, `queue_depth{sms,email}` ve worker sayaçları ve histogram
  geliyor, API worker sayacı sunmuyor, saklama 2d/256MiB, Prometheus yalnızca
  `127.0.0.1:19091`'de yayımlı, worker durdurulunca `up{job="worker"}=0` ama API hedefi
  ve `queue_depth` görünmeye devam ediyor (ADR-016). İlk koşuda e2e tmpfs'ine
  `nobody` yazamadı (`mode=1777` ile düzeltildi) ve Prometheus `256MB`'ı `256MiB`
  gösterdi (betik düzeltildi).
- [x] **`promtool` Makefile ve CI'da:** `make metrics-lint` (`check metrics` her iki uç
  için, boş çıktı hata sayılır; `check config`). `make e2e` artık üç döngü
  (standart, gönderim, observability). CI E2E işine "Observability" adımı ve
  profil yapılandırma doğrulaması eklendi.
- [x] **Worker log mesajı:** yeniden deneme logu her sonuç türü için "geçici hata;
  yeniden denenecek" diyordu. Artık gerçek türü anlatır ve `tür` alanı taşır: 429 hız
  sınırı, 5xx provider hatası, kayıp yanıt (sonuç belirsiz, aynı anahtarla yeniden;
  `WARN`), geçersiz 2xx yanıt (kabul doğrulanamadı; `WARN`). Başarısızlıkta
  `rejected` (kalıcı red) ile `exhausted` (deneme sınırı, sonuç doğrulanamadı) ayrı
  anlatılır. Regresyon testleri önce davranışsal olarak kırmızıydı
  (`outcome_log_test.go`, 5 alt test), düzeltmeden sonra yeşil.
- [x] **nginx geçişi (ADR-014):** `frontend/Dockerfile` çok aşamalı (node derler, nginx
  1.28 servis eder), `frontend/nginx.conf` (tek sayfa yönlendirmesi, önbellek
  başlıkları, `/api` proxy'sinde `proxy_buffering off`, `gzip off`,
  `proxy_read_timeout 130s`, Docker DNS, API kapalıyken `502` JSON). Konteyner `80`
  (host `8080`). Vite dev server artık Compose'ta yok (`npm run dev` ile yerelde).
- [x] **SSE akışları nginx üzerinden doğrulandı:** `e2e/nginx-proxy.spec.ts` (4 test):
  dashboard `snapshot` olayları ilk olay < 1,5 sn ve ~1 sn aralıklarla gelir;
  içerik üretimi `delta... validated` eksiksiz ve sıralı gelir; derin bağlantılar
  (`/campaigns/<id>/delivery`) çalışır; önbellek başlıkları doğru. Mevcut tüm e2e
  testleri ve `@real` gönderim nginx üzerinden geçti. API kapatılınca `502` JSON ve
  API dönünce nginx yeni adresi buldu (elle).
  - **Dürüst sınır:** `proxy_buffering on` ve `gzip on` mutasyonları testi **kırmadı**:
    bu nginx sürümü, küçük ve her biri flush edilen SSE olaylarını bu ayarlarla da
    zamanında iletiyor. Testler gözlemlenebilir gereksinimi (gecikmesiz teslim)
    doğrular, direktifin tek başına gerekli olduğunu kanıtlamaz; direktifler ADR'ye ve
    savunma amaçlı korunur. MockLLM parçaları anında ürettiği için içerik akışının
    *aralıklı* gelişi nginx üzerinden zamanlamayla gösterilemedi (yalnızca
    bütünlük ve sıra).
- [x] **Teslim kontrolleri:** temiz kopyada (izlenen ve yeni dosyalar, `node_modules` ve
  `.env` yok; ayrı Compose projesi, dev veritabanına dokunmadan) `docker compose up
  --build --wait` **73 sn**'de tamamlandı: migration (5) ve seed (3009 kullanıcı, 5311
  izin) otomatik, tüm servisler hazır. Demo akışı (örnek brief → kitle → içerik akışı
  28 `delta` → onay → canlı dashboard akışı) 13 sn içinde 285 mesaj `sent`, 0 başarısız
  ile bitti; dashboard akışı `completed` bildirdi. Aynı kopyada `gofmt`, `go vet`,
  `go mod tidy -diff` ve `go test -count=1 ./...` temiz.
  - **Teslimde bulunan ve düzeltilen eksik:** ana `compose.yaml`'da API için sağlık
    kontrolü yoktu; `up --wait` ve `web` API'yi beklemiyordu, ilk açılışta (API
    konteyner içinde derlenirken) sayfa açılır ama `/api` istekleri `502` verirdi.
    `api` artık `/readyz` sağlık kontrolü taşır ve `web` ona bağlıdır.
- [x] **README:** kurulum, demo adımları, test komutları, isteğe bağlı Prometheus, CI
  rozeti ve notu, kısa mimari (mermaid), kapsam ve bilinen sınırlamalar (gerçek Claude
  adaptörü yok dahil).

**Bu aşamada kalan işler (takip listesi, 2026-11-20 itibarıyla doğrulandı):**

- [x] **Dev ortamı:** migration `00003` ve `00004` dev veritabanında uygulandı (Oğuzhan
  bildirdi, 2026-11-20). Claude dev veritabanına bağlanmadığı için bu, Oğuzhan'ın
  bildirimine dayanır.
- [x] **PR ve CI:** PR #7 açıldı ve `main`'e squash merge edildi (`fffdded`). PR'daki
  Backend, Frontend, Compose ve E2E işleri `SUCCESS`; `main` push CI'ı `success`
  (`gh pr view 7` ve `gh run list` ile 2026-11-20'de doğrulandı). Yeni adımlar
  (`e2e-delivery-up`, `e2e-metrics-internal`, `@real`, görsel testler, nginx,
  `e2e-observability` ve `promtool` imajı) gerçek runner'da geçti.
- [ ] **Onay bekleyen taslak ADR'ler:** `DECISIONS.md`'de durumu hâlâ "Taslak" olanlar
  ADR-001, ADR-003 (güncellemesi onaylı), ADR-004, ADR-007, ADR-008, ADR-010 ve ADR-013
  (servis listesi ve portlar onaylı, başlatma sırası uygulandı ama onay bekliyor).
  Durumları yalnızca Oğuzhan'ın kararıyla değişir.
- [ ] **`ai-sessions/`:** Oturum 1-4 export dosyaları dizinde (`Session-01-ProjectSetup.txt`,
  `Session-02-DB-Seed-Migration.txt`, `Session-03-Brief-Kitle-Düzenleme-Onay.txt`,
  `Session-04-Worker-MockProvider-Queue.txt`). Oğuzhan bildirdi: 4. oturum dosyası önceden
  bash çıktıları yüzünden 10.000 satırı aşıp ortadan başlıyordu; düzeltildi ve dosyalar
  baştan sona eksiksiz (Claude içeriklerini tek tek doğrulamadı, yalnızca dosyaların
  varlığını ve satır sayılarını gördü). `ai-sessions/README.md` bu aşamada değiştirilmedi.
  5. oturumun export'u oturum sonunda eklenir; paylaşımdan önce anahtar ve kişisel
  verilerin temizlenmesi gerekir.
- [ ] **Teslim doğrulaması (aşağıdaki liste):** temiz kopyada kuruldu ve demo çalıştı;
  `make test` temiz kopyada değil çalışma ağacında koştu (aynı Compose postgres'i).
  Gerçek bir `git clone` ile (artık `main`'de merge sonrası) son kez denenmeli; henüz
  yapılmadı.

**Sonra yapılacaklar (karar bekliyor, şimdilik yapılmıyor):**

- [ ] **Gerçek Claude adaptörü (isteğe bağlı; şimdilik yapılmıyor):** `LLM_PROVIDER` yalnızca
  `mock`; Anthropic'e bağlanan adaptör yok. Oğuzhan'ın kararı (2026-11-20): şu an Claude API
  anahtarı yok, adaptör yazılmayacak. CASE.md bunu zorunlu tutmuyor: "Geliştirme sırasında
  mock bir LLM ile çalışabilir, gerçek modelle **denemek istersen** ... API key'ini alabilirsin"
  ve "Testler gerçek API key olmadan, mock bir LLM ile çalışmalı" (satır 49 ve 53). LLM
  entegrasyonu kriterinin (%25) vurguladığı şey "Claude'u çağırmak yeterli değil; çıktısını
  güvenilir hale getirmek"tir (satır 46): JSON doğrulama, tek düzeltme denemesi, akış ve MockLLM
  testleri yapıldı. **Kalan küçük boşluk:** CASE.md satır 53 uygulamanın anahtarı
  `ANTHROPIC_API_KEY`'den "okumasını" ister. `.env.example` ve `compose.yaml` bu değişkeni
  taşır, ama kod onu okumaz (`LLM_PROVIDER` yalnızca `mock` kabul eder). Anahtar sağlanırsa
  adaptör (`PROGRESS` "2. Gerçek Claude entegrasyonu") bu boşluğu da kapatır.
- [ ] **`client_golang`'a geçiş** (Oğuzhan sonra karar verecek): bugün kendi küçük
  yazıcımız (`internal/metrics`, ADR-027). Geçişte `Collector` ile `queue_depth` için
  "okunamayan değer 0 gibi görünmez" davranışı korunmalı; Go çalışma zamanı
  metrikleri kardinaliteyi artırır.
- [ ] **Grafana ve uyarı kuralları:** daha kapsamlı paneller ileride değerlendirilecek.
  **Loki eklenmeyecek** (Oğuzhan'ın değerlendirmesi): bu projede log toplama gerekmiyor.
- [ ] **Açık teknik konular:** SSE keepalive (içerik üretiminde ilk parçaya kadar olay
  yok), `llm_calls` kaydı, maliyet ve kötüye kullanım limitleri, e-posta gövde sınırı,
  `failure_reason` için isteğe bağlı CHECK kısıtı, metrics portu çakışması testi,
  `nginx`'te içerik akışının aralıklı gelişini gösteren (yavaş LLM'li) test.

**Çıkış koşulu:** İlk HTTP yanıtı kaybolduğunda retry yapılır, provider
yalnızca bir kabul kaydeder ve dashboard sonucu gösterir. **Sağlandı:** retry ve tek
kabul kanıtlandı (aşağıda); dashboard (`/campaigns/:id/delivery`) gerçek worker, mock
provider ve SSE ile sonucu gösteriyor (`@real` e2e). PR #7'de CI yeşil, `main`'e alındı.

#### Bu aşamada yapılanlar ve doğrulama (şu ana kadar)

- **Kod:** `internal/mockprovider` (HTTP arayüzü, `PGStore`, istatistikler,
  `MaxInWindow`), `internal/worker` (`PGRepo`: claim, ön kontrol, token'lı sonuç
  yazımı, reaper, kampanya tamamlama; `HTTPProvider`; `Decide` ve `Backoff`;
  `IntervalLimiter`; `Worker.Run`; `Metrics`; `ConfigFromEnv`),
  `database.OpenLazy`, `testdb.URL` ve `testdb.LockKey`.
- **Önemli bulgu (net/http):** `Idempotency-Key` başlıklı ve gövdesi yeniden
  okunabilen isteği yeniden kullanılan bağlantıda yanıt okunamazsa sessizce bir
  kez daha gönderiyor. Bu, hız sınırlayıcıyı atlatır ve "2 HTTP isteği" sayımını
  bozar. Worker istemcisi gövdeyi `GetBody`'siz veriyor; regresyon testi
  (`TestHTTPProviderDoesNotSilentlyResendOnReusedConnection`) düzeltme
  kaldırılınca başarısız oluyor.
- **Önemli bulgu (test altyapısı):** `go test ./...` paketleri eşzamanlı
  koşturur. `tests/e2e` de `campaigns_test`'i kullanmaya başlayınca, integration
  paketinin migration Down/Up testleriyle çakışıp veritabanını tutarsız
  bıraktı (`out-of-order migration`). Düzeltme: `testdb.Open` süreçler arası
  PostgreSQL advisory lock alıyor (`testdb-lock_test.go`; kilit kaldırılınca test
  başarısız oluyor). Bozulan `campaigns_test` silinip yeniden oluşturuldu (yalnızca
  test veritabanı).
- **Mevcut testler etkilendi:** `00002` Down testleri `DownTo(1)` kullanıyordu;
  artık önce `00003` kalktığı için `00002` korumasını denemeden önce `00003`
  geri alınıyor (`rollBack00003`). `testdb.Open` `TRUNCATE` listesine yeni tablo
  eklendi.
- **Idempotency ve retry (gerçek PostgreSQL, aynı süreçte):**
  `TestLostFirstResponseIsRetriedWithTheSameKeyAndDeliveredOnce` (2 HTTP isteği,
  tek kabul, `attempts = 2`, `duplicate_deliveries = 0`),
  `TestEveryResponseLostEndsExhaustedWithSingleAcceptance` (5 istek, 1 kabul,
  `failed`/`exhausted`), `TestRateLimitedRequestIsRetriedOnlyAfterRetryAfter`,
  `TestTransientServerErrorsAreRetriedUntilAccepted`,
  `TestPermanentRejectionIsNeverRetried`,
  `TestIdempotencyKeyReuseIsRejectedNotReplayed`,
  `TestRevokedOrMissingConsentIsSkippedWithoutAnyProviderRequest`,
  `TestStaleResultFromWorkerWithExpiredLeaseIsIgnored`, çöken worker ve
  `claim`/`reaper`/tamamlanma testleri (`worker-claim_test.go`).
- **Süreç düzeyi e2e (`backend/tests/e2e/`, gerçek ikililer, yalnızca
  `campaigns_test`):** yanıt kaybı ve idempotency, provider SIGKILL + yeniden
  başlatma, worker SIGKILL ile lease kurtarma, SIGTERM (devam eden çağrılar
  tamamlanır, yeni claim yok, çıkış kodu 0), SIGTERM süresi dolunca lease
  kurtarma, veritabanı kesintisi (test içi TCP proxy ile iki kez kes/geri getir;
  worker süreçte kalıyor, erişim dönünce kuyruğu işliyor) ve hız sınırı.
  Süreler test ortamında ortam değişkenleriyle kısaltılıyor; varsayılanlar
  değişmedi.
- **Mutasyon kontrolleri (testlerin yakaladığı görüldü):** idempotency anahtarının
  her denemede değişmesi, sonuç yazımında `claim_token` koşulunun kalkması, izin
  kontrolünün kapatılması, `Retry-After`'ın yok sayılması, `GetBody` ile sessiz
  yeniden gönderim, claim döngüsünün SIGTERM'den sonra sürmesi (birim testi
  yakalar; e2e'yi iptal edilmiş bağlam ayrıca korur) ve `testdb` kilidinin
  kalkması.
- **Compose (izole `campaign-e2e` projesi, `delivery` profili; dev
  veritabanına dokunulmadı):** 7 servis ayağa kalktı; API'den kampanya →
  içerik → onay → worker gönderimi tamamlandı: 7/7 `sent`, 7 kabul kaydı,
  kampanya `completed`; `docker compose stop` ile worker ve mock-provider 0,4 sn'de
  çıkış kodu 0 ile kapandı. Ortam silindi. `compose.e2e.yaml`'da `worker` ve
  `mock-provider` yalnızca `delivery` profiliyle açılır: mevcut Playwright testleri
  onaydan sonra `approved` bekliyor.
- **Dashboard öncesi üç düzeltme (regresyon testli):**
  1. Sonuç yazıldıktan sonra kampanya tamamlama sorgusu başarısız olursa kampanya
     `sending`'de kalıyordu. Artık her reaper turu bütün mesajları terminal olan
     kampanyaları da tamamlıyor (`CompleteIdleCampaigns`).
  2. Retry sırasında kullanıcı adı değişince aynı anahtarla farklı içerik gidiyor
     ve provider `422` ile reddediyordu. Ad artık onayda `messages.first_name`'e
     kopyalanıyor, worker retry dahil bu adı kullanıyor (ADR-026, migration
     `00004`). Boş ad ve fallback davranışı aynı.
  3. `2xx` ama gövdesi eksik/okunamayan yanıt `sent` sayılıyordu. Artık
     `invalid_response`: `sent` olmaz, aynı anahtarla yeniden denenir.
- **Limiter açığı (kullanıcı bulgusu, regresyon testli):** Limiter işleri ön
  kontrolden önce aralıklıyordu; veritabanı beklemesi veya süreç duraklaması
  sonrası ön kontrolde biriken işler birlikte gidip kayan 1 sn pencerede
  **103–104** isteğe çıkıyordu (sınır 100). Gönderim anına jeton kovası kapısı
  eklendi (burst 2, 1 sn pencerede ≤ 97); gerçek süreçte `SIGSTOP`/`SIGCONT` ve
  veritabanı dondurma senaryolarında ölçülen en yüksek pencere **96**. Karar:
  ADR-005 ek maddesi.
- **Dashboard frontend'i (TRK-001), backend olmadan:** `/campaigns/:id/delivery`.
  Onaylı kampanyada kampanya durumu (rozet) ve altı mesaj sayısı (bekleyen,
  yeniden denenecek, gönderiliyor, gönderildi, başarısız, atlandı) canlı gösterilir;
  işi biten mesajların oranı çubukla, `exhausted` ("Gönderim sonucu
  doğrulanamadı; deneme sınırı doldu", kesin teslim edilmedi denmez) ve `rejected`
  nedenleri ayrı notlarla açıklanır. Onaylanmamış kampanyada "henüz onaylanmadı"
  denir ve canlı takibe bağlanılmaz. `CampaignView` onay kutusuna "Gönderimi izle"
  bağlantısı eklendi (düğme değil; mevcut davranışlar ve testler korundu).
  - **Yapı:** `api/delivery.ts` (fetch, SSE ve `unknown` → tip doğrulaması;
    `readSse`, `ApiError`, `isRecord`, `isCount` yeniden kullanıldı),
    `delivery/` (`summary.ts`, `messages.ts`, `useDeliveryStream.ts`),
    `DeliveryDashboardView`, `DeliveryConnectionStatus`, `DeliveryCounts`.
    shadcn bileşenleri kullanıldı (`Card`, `Badge`, `Alert`, `Button`, `Progress`,
    `Skeleton`, `Spinner`); yeni bileşen veya bağımlılık eklenmedi. Yeni kodda
    `any`, tip dönüşümü (`as`), non-null (`!`) ve bastırma yorumu yok.
  - **Bağlantı yönetimi:** bağlanıyor / canlı / yeniden bağlanıyor / tamamlandı /
    hata durumları açık gösterilir. Koparsa 1-2-4-8-10 sn bekleyip yeniden bağlanır
    (son bilinen sayılar korunur, yeni görüntü sayıları değiştirir, üzerine eklemez),
    5 ardışık başarısızlıkta vazgeçip "Yeniden bağlan" sunar, 8 sn görüntü gelmezse
    takılmış sayar, `completed` görüntüsünde bağlantıyı kapatır. Sayfa veya kampanya
    değişince bağlantı kapanır ve eski kampanyadan geç gelen olay veya hata yeni
    ekranı değiştirmez (nesil numarasıyla; testle doğrulandı, koruma kaldırılınca 2
    test düşüyor).
  - **Erişilebilirlik:** başlık hiyerarşisi, `dl/dt/dd`, bölge adları, çubuk için
    `aria-label`; bağlantı durumu yalnızca durum değişince okunan ayrı bir `status`
    bölgesidir (saniyelik saat okunmaz, görünür metin çift okunmasın diye
    `aria-hidden`); neden notları `role="note"` (canlı bölge değil); hareketi
    azaltma desteği. Telefon (360 px) görünümü ekran görüntüsüyle ve e2e'de yatay
    taşma kontrolüyle doğrulandı.
  - **Sözleşme:** arayüzün beklediği `snapshot` biçimi
    [ARCHITECTURE §5](ARCHITECTURE.md)'te; backend uç noktası bunu birebir izler
    (sayı alanları tam sayı ve negatif değil, `exhausted + rejected ≤ failed`).
  - **Doğrulama:** birim testleri 186 → **314** (API doğrulaması ve hata/kopma
    durumları, saf özetler ve metinler, bağlantı yaşam döngüsü sahte zamanlayıcıyla,
    iki bileşen, ekranın tamamı, `CampaignView` bağlantısı, router). Yaşam döngüsü
    mutasyonları (eski olay koruması, eski bağlantıyı iptal etme, anlık tamamlandı
    durumu) testlerce yakalandı. Playwright e2e 4 → **9**: kampanya oluşturma, içerik
    ve onay **gerçek backend** ile; onaydaki gerçek sayılardan (3 SMS + 4 e-posta)
    türetilen canlı akış, kopma + yeniden bağlanma, tamamlanma, yenileme, hata ve
    "Yeniden bağlan", taslak kampanya ve olmayan kampanya (ikisi tümüyle gerçek),
    telefon görünümü. Dashboard testleri 6 tekrar × 5 = 30/30 geçti. `oxlint`,
    `eslint`, `vitest run`, `npm run build` (tip kontrolü dahil) ve
    `type-check:e2e` temiz; dokunulan dosyalar prettier'a uygun.
  - **Sınırlar:** taklitli e2e testlerinde (hata, kopma + yeniden bağlanma, takılma,
    mobil) yalnızca olay akışı taklit edilir; gerçek akış ve gönderim ayrı `@real`
    testiyle doğrulanır (Playwright e2e 9 → 10, sonra görsel testlerle **14**:
    standart döngü 13, `@real` 1).
    JS paketi 120 → 178 kB (shadcn `Progress`/`Alert` ve bağımlılıkları).
    (shadcn kartlarının koyu temada beyaz kalması sorunu aşağıdaki "Görsel düzeltmeler"
    maddesinde çözüldü.)
- **Dev veritabanı:** migration `00003` ve `00004` merge öncesi dev veritabanına
  uygulanmamıştı; Oğuzhan sonradan uyguladı (2026-11-20 bildirimi). `00004` için **API ve worker kapalıyken** uygulanır ve ikisi
  güncel kodla başlatılır; komut sırası ve riskli mesaj durumunda ne yapılacağı
  [ADR-026](../DECISIONS.md#adr-026--alıcı-adı-onayda-mesaja-kopyalanır-worker-retry-dahil-bu-adı-kullanır)
  içinde. `queued`/`sending` ve `attempts > 0` mesaj varsa migration hiçbir veri
  değiştirmeden durur. Mevcut mesajlar `users`'ın şimdiki adıyla doldurulur; bu,
  daha önce denenmiş mesajlarda aynı içeriği garanti etmez.

### Frontend iyileştirmeleri: tasarım ve bileşenleri modernleştirme

**Branch:** `feat/frontend-modernization` (güncel `main`'den Oğuzhan açtı; 5. oturumda
`git branch --show-current` ile doğrulandı, çalışma ağacı başlangıçta temizdi).
**Durum:** **aşama kapatıldı** (5. oturum sonu, 2026-11-20): kod, testler ve dokümanlar tamam,
yerelde doğrulandı; PR henüz açılmadı, **gerçek GitHub Actions sonucu PR üzerinden Oğuzhan
tarafından doğrulanacak**. Kapsam başlangıçtaki "frontend modernizasyonu"ndan genişledi
(Oğuzhan'ın kararlarıyla): akış iyileştirmeleri, son kampanyalar listesi ve endpoint'i
(CMP-001, ADR-029), `created_at` indeksi (migration `00005`) ve dashboard snapshot'ına kanal
dağılımı ve `recovered` (ADR-030).
`feat/worker-dashboard` PR #7 merge edildi ve CI yeşil (yukarıya bakın), önceki aşamadan
taşınan kırmızı bir iş yok. Oğuzhan birincil düğme için nötr shadcn `default` varyantını
onayladı; ek düzenleme isteği (üst başlık, adım göstergesi, iki sütun, dashboard özeti)
aynı gün eklendi. Kararlar [ADR-021](../DECISIONS.md) notunda.

**Yapılanlar:**

- [x] **Tek bileşen sistemi:** `ErrorPanel`, `BriefForm`, `ContentEditor`, `ApprovePanel`,
  `AudienceCounts`, `SegmentSummary`, `CampaignPreviewView` ve `CampaignView` shadcn
  bileşenlerine (`Button`, `Card`, `Field`/`Label`, `Input`, `Textarea`, `Alert`, `Badge`,
  `Spinner`, `Skeleton`) taşındı. Uygulamada scoped CSS kalmadı; kullanılmayan dört
  `--app-*` token'ı silindi. Yeni bağımlılık yok. `id`, rol, `aria-*` ve
  `data-testid` güvenceleri korundu; yalnızca sınıf adına bağlı test seçicileri
  (`.error-panel`, `.retry`, `.warning`, `.note`, `button.primary` vb.) `data-testid`'ye
  çevrildi.
- [x] **Ortak düzen:** `PageHeader` (`h1`, durum rozeti, bir satır açıklama) ve `CampaignSteps` (Kitle → İçerik → Onay → Gönderim; tıklanamaz,
  `aria-current="step"`, tik ve ekran okuyucu metni). Adım durumu `campaign/steps.ts`
  saf fonksiyonundan türetilir. Kampanya ekranı `lg` ve üstünde iki sütun (düzenleyici
  solda, kompakt kitle, koşullar ve onay sağda), telefonda alt alta. Ana eylemler
  `size="lg"` (40 px).
- [x] **Dashboard:** üstte Gönderildi, Başarısız ve Tamamlanmamış (bekleyen + yeniden
  denenecek + gönderiliyor) özet sayıları, altında ilerleme çubuğu ve ayrıntı (altı
  sayı, etiketler eskisi gibi). Durum rozeti üst başlığa taşındı.
- [x] **Kanal dağılımı ve yeniden denemeyle toparlanan sayısı (ADR-030):** önce kapsam dışı
  bırakılmıştı (snapshot'ta yoktu); Oğuzhan sonradan istedi (2026-11-20), sözleşme değişti.
  - **Backend:** SSE `snapshot`'a **`channels`** (`sms` ve `email`, `messages` ile aynı altı
    durum) ve **`recovered`** (`sent` ve `attempts > 1`) eklendi. Değişmez kural: her durum için
    `sms + email = toplam`; `recovered ≤ sent`. Tek sorgu, `GROUP BY channel`; `messages` kanal
    satırlarının toplamıdır (tutarsızlık oluşmaz; bu, `messages.channel` üzerindeki
    `CHECK (sms, email)` kısıtına dayanır). Alanlar her görüntüde yollanır (sıfır dahil).
    Migration yok. `campaign.DeliverySnapshot`, `httpapi.deliverySnapshotResponse`;
    ARCHITECTURE §5 ve TRK-001 güncellendi.
  - **`recovered` neyi sayar (kesin tanım):** `status = 'sent'` ve `attempts > 1` mesajlar.
    `attempts` her claim'de artar, yani `> 1` mesajın `sent` olmadan önce **birden fazla kez
    işlenmeye alındığı** demektir: geçici provider hatası sonrası backoff, kayıp yanıt (provider
    ilk isteği kabul etmiş olabilir, aynı `Idempotency-Key` ile yeniden deneme kayıtlı sonucu
    alır) veya lease dolması / worker çökmesi sonrası reaper (önceki deneme provider'a hiç
    ulaşmamış olabilir). Anlamı "ilk denemede tamamlanamadı, sonraki denemede gönderildi olarak
    kaydedildi"dir; **"ilk istek başarısız oldu" demek değildir**, nedeni ayırmaz ve provider
    çağrı sayısı değildir. Idempotency nedeniyle bu mesajlar da en çok bir kez teslim edilmiştir.
    Arayüz metni buna göre yazıldı (ilk sürümde "ilk denemede gönderilemedi" diyordu; kayıp
    yanıtta yanlış olduğu için "ilk denemede tamamlanamadı, yeniden denemeyle gönderildi"
    yapıldı ve testle sabitlendi).
  - **Arayüz:** dashboard'da "Kanal dağılımı" (SMS ve e-posta ayrı: toplam, ilerleme çubuğu,
    gönderilen / başarısız / tamamlanmamış, atlanan varsa; mesajı olmayan kanal "Bu kanalda
    mesaj yok"), "Yeniden denemeyle toparlanan: N mesaj" notu ve gönderim sonu özetinin
    gönderilen satırında ("N mesaj ilk denemede tamamlanamadı, yeniden denemeyle gönderildi").
    İstemci alanlar yoksa (eski sunucu) çalışmaya devam eder, varsa doğrular (toplamı tutmayan
    görüntü geçersiz).
  - **Testler:** entegrasyon (gerçek PostgreSQL; her durum iki kanala dağıtılmış sabit veri,
    kanalların toplamı = toplam, tek kanallı kampanya, kampanyalar arası karışmama, `recovered`
    yalnızca `sent` ve `attempts > 1`, canlı akışta değişimin yansıması) ve **gerçek worker +
    provider** ile (yanıt kaybı → ikinci denemede gönderim: `recovered = 2`; tüm yanıtlar kayıp
    → `recovered = 0`, SMS'te 1 başarısız); `@real` e2e'de 3 SMS + 4 e-posta ve hepsi toparlanan
    (7) kesin doğrulanır. Mutasyonlar yakalandı: `attempts > 1` → `>= 1` ve SMS/e-posta
    eşlemesini ters çevirmek.
- [x] **`campaigns` için indeks (migration `00005`, Oğuzhan'ın isteği):** `campaigns_recent_idx
  (created_at DESC, id DESC)`, liste sorgusuyla (CMP-001) birebir aynı sıralama.
  - **Ne sağlar:** planlayıcı seçerse sorgu indeksi sırayla gezip ilk n kayıtta durur: tam tarama
    ve `Sort` adımı yok, kabaca O(log N + n). **Ne sağlamaz:** `brief` ve `status` indekste
    olmadığı için n satır tablodan okunur (index-only değil); planlayıcı maliyete göre karar
    verir, çok küçük tabloda (demo ölçeği) tarama + sıralama seçip indeksi kullanmayabilir.
    Yalnızca hızlandırır, sonucu değiştirmez. **Maliyet:** her eklemede bir indeks girişi.
    **Kilit:** düz `CREATE INDEX` `SHARE` kilidi alır (okumalar sürer, yazmalar oluşturma
    bitene kadar bekler); `CONCURRENTLY` goose transaction'ında kullanılamaz, tablo küçük.
  - **Testler:** indeks tanımı; **sıralı tarama kapatılınca** planın indeksi kullandığı ve
    `Sort` adımı olmadığı (indeksin bu sorgu için kullanılabilir olduğunu gösterir, planlayıcının
    her ölçekte seçtiğini değil); Down yalnızca indeksi kaldırır (veri korunur) ve Up aynı
    tanımla geri getirir; indeksli ve indekssiz liste aynı sonucu verir.
  - Mevcut iki migration testi 00005'ten etkilendi ve kapsamlarına göre düzeltildi (00004
    testleri `UpTo(4)` ve `DownTo(4)` ile başlıyor). **Dev veritabanına uygulamayı Oğuzhan
    yapar** (Claude yazmaz); `docker compose up` temiz kurulumda otomatik uygular.
- [x] **Mobil:** `e2e/mobile-layout.spec.ts` (360 px): belge taşması ve kartlarca kırpılan
  taşma (görünür her öğe viewport içinde), ana düğmelerin yüksekliği ≥ 40 px.
  `visual-themes.spec.ts` önizleme ekranını, adım göstergesini ve dashboard özet
  sayılarını da kapsıyor (açık ve koyu tema, WCAG kontrastı, imleç).

- [x] **Akışı kolaylaştıran eklemeler (Oğuzhan'ın isteği, 2026-11-20; yeni ekran, endpoint
  veya bağımlılık yok):**
  - **Örnek brief önerileri:** boş ekranda 3 örnek (`exampleBriefs.json`, tek kaynak);
    yazmaya başlayınca kaybolur, seçince alanı doldurur (göndermez). Backend testi
    (`tests/integration/example-briefs_test.go`, `campaigns_test` + tam demo verisi) her
    örneğin MockLLM ile geçerli segment ürettiğini ve alıcı bulduğunu doğrular (191 / 73 /
    14 kişi); örnek bozulunca kırıldığı görüldü.
  - **Kaydedilmemiş değişiklik uyarısı:** `useUnsavedChangesGuard`: sekmeyi kapatma ve
    yenileme için tarayıcı uyarısı, uygulama içi gezinme (Geri bağlantısı, başka kampanya)
    için onay penceresi; vazgeçilirse metin korunur. Gerçek tarayıcıda doğrulandı
    (`beforeunload` ve `confirm` diyalogları).
  - **İşlem geri bildirimi:** "Üretildi · sürüm N" / "Kaydedildi · sürüm N" (durum bölgesi,
    metin değişince kaybolur); işlem sürerken düğmeler kapalı ve "Üretiliyor…",
    "Kaydediliyor…", "Onaylanıyor…"; çift tıklama tek istek (testli).
  - **Onay özeti:** `ApprovePanel` aynı kartta: hangi sürüm, tahmini SMS/e-posta alıcıları
    (≈, tahmin olduğu yazılı) ve SMS ile e-posta önizlemesi.
  - **Kişiselleştirme önizlemesi:** `MessagePreview`: `{{first_name}}` yerine örnek ad
    "Ayşe"; "Örnek önizleme" notu gerçek mesajların alıcının kendi adıyla gittiğini söyler.
    Düzenleyicide canlı taslak, onay özetinde kaydedilmiş sürüm için gösterilir.
  - **Gönderim sonu özeti:** tamamlanınca tek cümlelik sonuç ("7 mesajın 5'i gönderildi,
    1'i başarısız, 1'i atlandı."; `delivery/turkish.ts` sayının okunuşuna göre iyelik eki
    seçer: 5'i, 7'si, 3'ü, 1.234'ü; hepsi tek sonuçsa "tamamı") ve gönderilen / başarısız /
    atlanan satırları kısa açıklama, nedenler ve **"Ne yapabilirsiniz?"** önerileriyle
    (sonucu doğrulanamayan: teslim edilmiş olabilir, sağlayıcıdan kontrol edin; reddedilen:
    adresleri kontrol edin; atlanan: izin yok, yapılacak bir şey yok; başarısız varken:
    kampanya yeniden gönderilemez, yeni kampanya brief'e uyan herkese gider). Gönderilen
    satırı yeniden denemeyle toparlanan sayısını da söyler (aşağıda, ADR-030).
  - **Geri / İleri (`StepNav`, Türkçe):** "İleri" hedef açılana kadar kapalı ve nedeni yazılı
    (kitle hesaplanana, içerik onaylanana kadar). Kampanya ekranındaki sol düğme önceki
    aşamaya dönmez, yeni kampanya başlatır (önizleme ekranı var olan kampanyayı yüklemez):
    bu yüzden "Geri" değil **"+ Yeni kampanya"** diye anılır. Dashboard'da "Geri" kampanyaya
    döner, "İleri: Yeni kampanya" **gönderimin bitmesine bağlı değildir**; gönderim sürerken
    "Gönderim arka planda devam eder." notu görünür (gönderimi worker yürütür, sayfadan
    ayrılmak onu durdurmaz). Kaydedilmemiş değişiklik uyarısı korunur (kampanya ekranından
    her ayrılışta). Eski üst bağlantılar ve onay kartındaki "Gönderimi izle" düğmesi
    kaldırıldı (yinelenen bağlantı kalmasın diye).
  - **Mobil:** kampanya ekranında onay kartı dar ekranda içerikten hemen sonra gelir ve
    Geri/İleri çubuğu alta yapışır; düğme metinleri sarılır. Boşluksuz uzun brief ve hata
    metni taşmaz. Bu iş sırasında testlerin yakaladığı iki gerçek hata düzeltildi: `StepNav`
    düğmeleri 360 px'te taşıyordu (41–46 px), uzun metin yapıştırılınca brief textarea'sı
    formu genişletiyordu (`wrap-anywhere`); `ErrorPanel` da uzun metni kırmıyordu.

- [x] **Son kampanyalar listesi (CMP-001, ADR-029):** Oğuzhan bu aşamaya dahil etti
  (2026-11-20; CLAUDE.md'deki "aşama dışı iş eklenmez" kuralının istisnası olarak, kararı o
  verdi). "Son kampanyalar" (sonradan ana sayfadaki karttan kenar çubuğuna taşındı, aşağıya
  bakın): en yeni başta, satırda **brief, durum ve tarih**
  (kitle sayısı bilerek yok). **Taslak** kampanya kampanya ekranına, **`approved`, `sending`
  ve `completed`** olanlar gönderim takibine açılır; satır nereye gittiğini yazar. Yeni
  kampanya oluşunca liste aynı sayfada yenilenir; liste yüklenemese de brief formu çalışır.
  - **Backend:** `GET /api/campaigns?limit=N`: **varsayılan 10, en fazla 20** (20'den büyük
    değer 20'ye indirilir; tam sayı olmayan veya < 1 `400 INVALID_REQUEST`, veritabanı
    okunmaz). Sıralama `created_at DESC, id DESC` (kararlı), satırda yalnızca `id`, `brief`,
    `status`, `created_at`; boş liste `[]`. Sıralamayı destekleyen `created_at` indeksi
    migration `00005`'tedir (ADR-029). `campaign.Store.List`, `httpapi.listCampaigns`; `/api/campaigns`
    yolunda `Allow: GET, POST`.
  - **Testler:** birim `internal/httpapi/campaign-list_test.go` (9 grup: sıra, alanlar, `[]`,
    sınırlar, 7 geçersiz değer, sızıntı yok, başka bağımlılık çağrılmaz);
    entegrasyon `tests/integration/campaign-list_test.go` (gerçek PostgreSQL: 25 kampanyayla
    sıra ve sınırlar, eşit zamanda kararlı sıra, boş liste, dört durumun gerçek onay akışıyla
    listelenmesi ve ayrıntı endpoint'iyle tutarlılık, 1000 karakterlik brief, depo sınırları);
    arayüz birim testleri (API istemcisi, bağlantı hedefleri, `RecentCampaigns`, ekrana bağlama)
    ve e2e `campaign-list.spec.ts` (6 test: taslak ve onaylı satırın hedefi, sıra, aynı sayfada
    yenileme, kalıcılık, **21 kampanyayla varsayılan 10 ve üst sınır 20 kesin**, geçersiz sınır
    400). Mutasyonlar yakalandı: sıralamayı tersine çevirmek ve eşit zamanda kimlik ayrımını
    kaldırmak. `visual-themes` ve `mobile-layout` liste okunabilirliğini ve boşluksuz uzun
    brief'in taşmadığını da sınar.

- [x] **Uygulama kabuğu, simgeler ve pastel palet (ADR-021 notu; export öncesi son istek,
  2026-11-20):** "campaign list vs çok dağınık" diye Oğuzhan istedi. Yeni ekran, endpoint veya
  bağımlılık yok (mevcut shadcn `Sidebar`, `@lucide/vue`).
  - **Kenar çubuğu (`AppSidebar`):** "Yeni kampanya" ve "Son kampanyalar" (ana sayfadaki karttan
    taşındı). Masaüstünde açık, düğmeyle kapanır; telefonda çekmece (bağlantıya gidilince
    kapanır). Liste sayfa değişince ve ekranlar `campaign/recent.ts` ile haber verince (oluşturma,
    onay, dashboard'da durum değişimi) yenilenir.
  - **Üst başlık:** kenar çubuğu düğmesi ve ana sayfaya dönen **"Campaign Tool"**. **Sapma
    (Oğuzhan'a not):** istekteki "main h1" yerine başlık bir bağlantıdır; her sayfada tek `h1`
    (sayfa başlığı) kalır, aksi halde ekran okuyucu gezintisi, `PageHeader` ve testler bozulurdu.
    İstenirse başlık `h1`, sayfa başlıkları `h2` yapılabilir (ayrı, daha geniş bir değişiklik).
  - **Simgeler:** kart başlıkları, düğmeler (`StepNav` okları artık simge; erişilebilir ad
    "Geri: …" / "İleri: …" aynı), kitle ve kanal satırları, özet kutuları, hata ve bilgi
    uyarıları. Hepsi `aria-hidden`. `StatusBadge`: simge + metin + ton (durum yalnızca renkle
    anlatılmaz).
  - **Pastel palet:** yumuşak mavi-gri zemin, beyaz kartlar, pastel indigo birincil renk, durum
    tonları; açık ve koyu tema. Önceki "nötr birincil" kararının yerine geçer (ADR-021 notu).
  - **Testler:** `App.spec.ts` yeniden yazıldı (başlık, kenar çubuğu, liste yenileme),
    `StatusBadge.spec.ts` yeni, `RecentCampaigns`/`CampaignPreviewView`/`StepNav`/`DeliveryConnectionStatus`
    spec'leri güncellendi; e2e: `app-shell.spec.ts` yeni, `mobile-layout` (çekmece, üst başlık),
    `visual-themes` (başlık ve düğme kontrastı) genişletildi; kenar çubuğundaki satırlarla
    çakışan konumlayıcılar (`Gönderimi izle`, `getByRole('button')`, `/Geri/`) daraltıldı.
    `campaignStatusVariant` kullanılmadığı için silindi.

**Doğrulama (yerelde, Node 24, 2026-11-20):** `vitest run` 31 dosya, **543 test** geçti
(aşama başı 314); `oxlint`, `eslint`, `npm run build` (tip kontrolü dahil),
`type-check:e2e` temiz; dokunulan dosyalar prettier'a uygun (`styles.css` ve shadcn
`components/ui/` dosyaları HEAD'de de prettier'a uymuyor, üretim çıktısı, biçimlendirilmedi).
Son kabuk/simge/palet değişikliğinden sonra yeniden koşuldu: `make e2e-standard` Playwright
**37/37** (aşama başı 17) ve `make e2e-delivery` `@real` **1/1**, çıkış kodu 0. Prometheus
döngüsü (`e2e-observability`) bu son değişiklikten sonra **yeniden koşulmadı** (backend ve
metrikler değişmedi); önceki tam `make e2e` koşusunda scrape ve `promtool` temizdi (o koşuda
iki görsel test kırmızıydı: fixture'da atlanan mesaj SMS'te,
test e-postada arıyordu; test düzeltildi, uygulama hatası değildi). Backend: `gofmt`, `go vet`,
`go mod tidy -diff` temiz, `make test` yeşil (örnek-brief, kampanya listesi, indeks ve
kanal/toparlanan testleri dahil; `cmd/api`, `httpapi`, `tests/e2e`, `tests/integration` yeniden
koştu; hepsi yalnızca `campaigns_test` üzerinde, dev veritabanına yazılmadı). Mutasyonlar yakalandı: ana düğmeyi küçültmek ve
taşan sabit genişlik (mobil test; ilk sürüm kırpılan taşmayı yakalamadı, çünkü kart
`overflow-hidden`, test güçlendirildi), desteklenmeyen örnek brief (backend testi),
`unfinishedMessages`'dan `sending`'i çıkarmak ve adım göstergesinde kaydedilmemiş
değişikliği yok saymak (birim testleri). Ekran görüntüleri açık/koyu tema ve 360 px için
gözle kontrol edildi (son akış eklemelerinden önceki sürümde; sonrası Oğuzhan'ın görsel
kontrolüne bırakıldı). 5. oturumda Ubuntu bağlantısı koptu; yarım kalan e2e ortamı
(`campaign-e2e`) temizlenip her şey baştan doğrulandı.

**Sınırlar:** GitHub Actions doğrulanamadı (yeni testler `E2E` işinde ilk kez koşacak; CI'ın
Backend işi yeni entegrasyon testlerini `make test` ile zaten çalıştırır, yeni araç yok).
Liste için `created_at` indeksi migration `00005` ile eklendi (ADR-029); geliştirme
veritabanına Oğuzhan uyguladı.
Dashboard ayrıntısında Gönderildi ve Başarısız özetle tekrar ediyor (testler ve
`count-*` kimlikleri korunsun diye); sadeleştirmek istenirse ayrı küçük iş. Gerçek
tarayıcı yalnızca Chromium. Tarayıcıların kaydedilmemiş değişiklik uyarı metni kendi
belirledikleridir (sayfa özel metin gösteremez); özel metin yalnızca uygulama içi gezinmede
çıkar.

**Başlangıç baz çizgisi (aşama başında, 2026-11-20, Node 24):** `vitest run` 21 dosya, **314 test**
geçti; `playwright test --list` **18 test** listeledi (17 standart + 1 `@real`; e2e
koşturulmadı, yalnızca listelendi). Bu sayılar tasarım işi sırasında korunur veya
gerekçeli güncellenir.

**Kapsam (Oğuzhan'ın tanımı, 2026-11-20):** yalnızca eski bileşenleri shadcn'a taşımak
değil, genel görsel düzeni de tutarlı hale getirmek. Mevcut shadcn bileşenleri
kullanılır; tasarım sade, okunabilir ve mobil uyumlu olur. Başlıklar, boşluklar, form
alanları, kartlar ve ana işlemler tutarlı olur. **Yeni özellik, endpoint veya bağımlılık
eklenmez**; mevcut iş akışları ve erişilebilirlik korunur.

**Girdi olarak bilinenler (karar değil, 4. oturumda gözlenenler; onay bekler):**

- Arayüzde iki düğme/biçim sistemi bir arada: eski ekranlarda (`BriefForm`,
  `CampaignView`, `ApprovePanel`, `ErrorPanel`, `ContentEditor`) bileşen kapsamlı CSS
  ile yazılmış düz `<button>`, `<input>`, `<textarea>`; yeni dashboard'da shadcn `Button`,
  `Card`, `Badge`, `Alert`, `Progress`. Mavi birincil düğmeler ile nötr shadcn
  düğmeleri görsel olarak tutarsız.
- Koyu tema ve token'lar 4. oturumda düzeltildi (ADR-021 notu, onaylı): shadcn
  token'ları `--app-*` paletinden geliyor, `prefers-color-scheme` izleniyor. Yeni
  tasarım işi bu tabanın üzerine kurulmalı.
- Mevcut güvenceler korunmalı: `e2e/visual-themes.spec.ts` (WCAG kontrastı ve
  `cursor: pointer`, açık ve koyu tema), erişilebilirlik kuralları (sr-only durum
  bölgesi, `role="note"`), telefon görünümü, 314 birim ve 17 e2e testi.
- Kurallar: mevcut shadcn bileşenleri kullanılır, yeni bağımlılık eklenmeden önce
  Oğuzhan'a sorulur (CLAUDE.md, ADR-021); `any`, `as` ve `!` yok; dış veri `unknown`
  olarak alınıp api katmanında doğrulanır.

**Çıkış koşulu:** seçilen ekranlar tek bir bileşen sistemiyle tutarlı, iki temada okunabilir
ve mobilde sorunsuz; birim, e2e ve görsel testler geçer; tasarım kararları ADR-021'e ve yeni
ADR'lere (029, 030) yazılır. **Sağlandı** (yerelde; CI PR üzerinde doğrulanacak).

### 2. Gerçek Claude entegrasyonu (isteğe bağlı, ayrı iş)

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

- [x] Temiz bir kopyada **`docker compose up`** ile Vue (nginx), Go API, PostgreSQL,
  migration, seed, worker ve mock provider çalışıyor (73 sn, 2026-11-19). Gerçek
  `git clone` ile commit sonrası son kez denenmeli.
- [x] Örnek brief'ten canlı gönderim takibine kadar demo tamamlanıyor (temiz kurulumda
  285 mesaj, 13 sn, 0 başarısız).
- [x] **`make test`** gerçek API anahtarı olmadan geçiyor (çalışma ağacında ve temiz
  kopyada `go test ./...`).
- [ ] Anahtar varsa gerçek Claude adaptörüyle örnek brief doğrulandı: **yapılamaz,
  adaptör yok** (yukarıdaki "Sonra yapılacaklar").
- [x] `README.md` kurulum, kısa mimari özeti, CI rozeti ve notu ve bilinen
  sınırlamaları içeriyor.
- [x] `DECISIONS.md` kabul edilen kararları ve gerekçelerini içeriyor (ADR-001...028).
- [ ] Case için yapılan Claude oturumları `ai-sessions/` altında. Oturum 1-4 export
  dosyaları var (eksiksiz olduğunu Oğuzhan bildirdi); 5. oturumun export'u oturum
  sonunda eklenecek. Paylaşım öncesi gerçek API anahtarları ve kişisel bilgiler
  temizlenmeli.

## Açık konular

- ADR-013'teki Compose başlatma sırası onay bekliyor (ADR hâlâ taslak). Sıra
  uygulandı ve Oğuzhan'ın Compose çalıştırmasında işledi.
- Web artık nginx ile servis edilir (ADR-014, 2026-11-19); Vite dev server yalnızca
  yerelde `npm run dev` içindir ve `/api` proxy'si yalnızca onun için kalır.
  SSE akışları nginx üzerinden doğrulandı (D3 sonrası maddeler).
- Frontend e2e testleri (Playwright) `frontend/e2e/` altında ve CI'da `E2E` işi
  olarak koşar (ADR-024). ADR-018'in `backend/tests/e2e/` klasörü artık dolu:
  gerçek `worker` ve `mock-provider` ikilileriyle süreç düzeyinde testler içeriyor
  (`.gitkeep` hâlâ duruyor, silinebilir). Bu testler `make test` içinde koşar,
  bu yüzden CI'ın Backend işi onları zaten çalıştırır; CI yapılandırması
  değişmedi (yeni araç yok).
- Test klasör yapısı ADR-018 ile onaylandı: unit testler kodun yanında,
  integration ve e2e `backend/tests/` altında. `backend/internal/` altında
  `audience`, `campaign`, `database`, `httpapi`, `llm`, `seed`, `segment`,
  `testdb`, `worker` ve `mockprovider` paketleri var; `cmd/` altında `api`,
  `migrate`, `seed`, `worker` ve `mock-provider` var.
- `campaigns_test`'i kullanan testler `testdb.Open` içindeki PostgreSQL advisory
  lock ile süreçler arası sıraya konur (`go test ./...` paketleri eşzamanlı
  çalıştırır). Bir paketin testi başka bir paketin testini bekleyebilir.
- `failure_reason` değerleri (`exhausted`, `rejected`) şimdilik veritabanında
  `CHECK` ile zorlanmıyor; yalnızca worker yazıyor. Zorlamak şema değişikliğidir
  ve Oğuzhan onayı ister.
- Retry'dan sonra `last_error` başarıda temizlenmiyor (`sent` yazımı yalnızca `status` ve
  `sent_at` günceller; son hatayı gösterir). Dashboard bunu göstermiyor: yalnızca
  mesaj sayıları ve `failure_reason` nedenleri (`exhausted`, `rejected`) arayüze çıkar.
- `make test` önce yalnızca `postgres` servisini başlatır (`make test-db`),
  sonra `go test ./...` çalıştırır. Entegrasyon testleri `campaigns_test`
  veritabanını kullanır (ADR-015).
- `SEG-001`, `CNT-001`, `SND-001` ve `TRK-001` spec dosyaları PR #2 ile
  `main`'e alındı ([specs/README.md](specs/README.md)) ve dördü de uygulandı
  (segment önizleme, içerik ve onay, gönderim, canlı takip).
- 30 günlük kuralın yorumu (ADR-010) hâlâ taslak ve onay bekliyor. Uygulama
  ve testler önerilen yorumu kullanıyor (son 30 günde sepete ekleme ve aynı
  dönemde satın alma yok). Önizleme ile onay arasında kitlenin kayması da
  karara bağlanmadı.
- Brief uzunluğu ve segment alan/operatör sınırları `SEG-001` içinde
  netleşti ve uygulandı (brief en fazla 1000 karakter, en fazla 10 koşul vb.).
  Eksik `first_name` için fallback metni `Değerli müşterimiz` olarak
  belirlendi (ADR-022).
- E-posta gövdesi için uzunluk sınırı CASE.md'de (yalnızca SMS 160 ve konu 80) ve
  CNT-001'de tanımlı değil; şu an yalnızca
  boş olmama ve değişken kuralları var. Gerekirse ayrıca karar verilir.
- `llm_calls` tablosu hâlâ yok (hiçbir migration'da geçmiyor); kapsamı ve zamanı
  Oğuzhan'la netleştirilecek (ARCHITECTURE §4). `llm_validation_failures_total`
  metriği ise API'de uygulandı (ADR-027, ARCHITECTURE §12).
- Doğrulayıcıdaki kod noktası sayımı birleşik karakterleri (ör. `e` +
  birleştirici aksan) iki sayar; spec kod noktası istiyor (ADR-022).
- Markdown çitli (` ```json `) model çıktısı `MALFORMED_OUTPUT` sayılır ve
  düzeltme denemesine gider. Gerçek Claude adaptöründe JSON modu veya prefill
  değerlendirilecek.
- `queue_depth` yalnızca `queued` mesajları sayar ve `ratelimit_wait_seconds` bir
  histogramdır (ADR-027, uygulandı). Kova sınırları sabit.
- CI'da ek Go lint aracı (`golangci-lint` gibi) yok. CI `gofmt` ve
  `go vet` ile başlıyor. Yeni araç bağımlılık kararıdır ve önce Oğuzhan'a
  sorulur.
- `npm audit` CI'a eklenmedi. Yerelde 5 moderate uyarı görüldü (minimatch,
  glob, editorconfig, js-beautify zinciri). Eklenirse yeni advisory'ler
  PR'ları kırabilir.
- Frontend lint komutları CI'da workflow içinde doğrudan çağrılıyor,
  `package.json` ve `Makefile` ile paylaşılmıyor. Yerelde `npm run lint`
  `--fix` ile dosya değiştirir.
- CI'ın Compose işi `docker compose config --quiet` (geliştirme, e2e ve observability
  profili yapılandırması) çalıştırır. İmajlar ve servis sağlığı (`/healthz`) dolaylı olarak
  `E2E` işinde sınanır (`make e2e-up` imajları derler ve api/web sağlık
  kontrollerini bekler); geliştirme ortamının kendisi (`docker compose up`) CI'da
  doğrudan koşmaz.

- ADR-023 onaylandı (B): kanala uygunluk açık izin ve adres ister; önizleme ve
  kuyruk aynı kuralı kullanır.

## Son doğrulama

### Son tamamlanan aşama: `feat/worker-dashboard` (2026-11-19, `main`'e alındı)

**Durum:** Aşama tamamlandı. PR #7 `main`'e squash merge edildi (`fffdded`); PR'da
Backend, Frontend, Compose ve E2E işleri `SUCCESS`, merge sonrası `main` push CI'ı
`success` (2026-11-20'de `gh` ile doğrulandı). Hata bulunmadı. Aşağıdaki doğrulama
kaydı merge öncesi yerel çalıştırmalara aittir.

- **Claude'un yerelde koşturdukları:** `gofmt -l`, `go vet ./...` ve `go mod tidy -diff`
  temiz. `make test` ve `go test -count=1 ./...` (unit, entegrasyon ve backend e2e)
  geçti; `-race` `cmd/api`, `internal/...` ve entegrasyon üzerinde temiz. Frontend
  (Node 24): `oxlint`, `eslint`, `vitest run` (**314 test**), `npm run build` (tip
  kontrolü dahil) ve `type-check:e2e` temiz. `make e2e` üç döngü: standart Playwright
  **17/17**, gerçek worker + mock provider + SSE (`@real`) **1/1**, Prometheus scrape ve
  `promtool` (`make e2e-observability`). `docker compose config` (varsayılan, `delivery`
  ve `observability` profilleri) temiz. Temiz kopyada `docker compose up --wait`
  (73 sn) ve demo akışı (285 mesaj, 13 sn, 0 başarısız) ile `go test ./...`.
- **Oğuzhan'ın bağımsız doğrulaması (ayrı çalıştırma, hata yok):**
  - Frontend birim testlerinin 314'ü geçti; lint, build ve tip kontrolleri geçti.
  - Backend unit, entegrasyon ve e2e testleri geçti; worker, provider ve metrics
    paketlerinin `-race` kontrolleri geçti.
  - Standart Playwright testleri 17/17, gerçek worker + provider + SSE testi 1/1,
    Prometheus scrape ve `promtool` kontrolleri geçti. Go bağımlılık, format, `vet` ve
    Compose kontrolleri temiz.
  - **Elle `curl` akışı:** kampanya oluşturma `201` (kitle 7: SMS 3, e-posta 4);
    içeriksiz onay `409`; içerik üretiminde `delta` olayları ardından `validated`, sürüm 1
    kaydedildi; geçersiz düzenleme `422` ve sürüm artmadı; geçerli düzenleme `200`, human
    kaynaklı sürüm 2; eski sürümle onay `409`; güncel sürümle onay ve tekrar onay `200`
    (onay zamanı ve sayılar aynı kaldı); onay sonrası düzenleme `409`; dashboard SSE
    `completed` ve 7 `sent` gösterdi; yanlış adres, yöntem, içerik türü ve büyük gövde
    için beklenen `404`, `405`, `415`, `400` ve `413`.
  - **Veritabanı doğrulaması:** 7 mesajın her biri 2 denemede gönderilmişti; provider'da
    14 istek, 7 kabul ve 0 tekrar teslimat (yanıt kaybı + idempotency zinciri).
  - **Sağlık uçları:** izole test veritabanı duraklatılınca `/readyz` `503` oldu, `/healthz`
    `200` kaldı; veritabanı devam ettirilince `/readyz` yeniden `200` oldu.
  - **Prometheus:** ayağa kaldırılıp birçok endpoint denendi, hepsi sorunsuz; `queue_depth`
    ve gönderim metrikleri geliyor. API, PostgreSQL, worker ve mock provider akışı
    testlerde sorunsuz çalıştı.
- **Gözlemlenebilirlik değerlendirmesi (Oğuzhan):** ileride Grafana ile grafikler
  eklenebilir. Oğuzhan normalde gözlemlenebilirlik için Loki kullanıyor, ama bu projede
  gerek görmüyor: loglar yapılandırılmış `slog` çıktısıyla `docker compose logs`
  üzerinden okunuyor ve metrikler Prometheus'ta. Loki eklenmeyecek.
- **Doğrulanamayan / bekleyen:**
  - ~~GitHub Actions'ın kendisi~~ **Doğrulandı** (PR #7 ve `main` push: tüm işler yeşil).
  - Gerçek `git clone` ile kurulum denemesi (artık `main`'de yapılabilir; henüz yapılmadı).
  - ~~Dev veritabanında migration `00003` ve `00004`~~ Oğuzhan uyguladığını bildirdi
    (Claude doğrulayamaz).
  - Gerçek Claude adaptörü yok (`LLM_PROVIDER` yalnızca `mock`); API anahtarı olmadığı için
    Oğuzhan'ın kararıyla yapılmıyor. CASE.md zorunlu tutmaz, bilinçli sınırlama olarak
    README'de yazılı (bkz. "Sonra yapılacaklar").
  - `ai-sessions/`: 5. oturumun `/export`'u oturum sonunda eklenecek.
- **Sıradaki iş:** `feat/frontend-modernization` için PR açılması, CI sonucunun doğrulanması ve
  squash merge (Oğuzhan; dev veritabanına migration `00005`'i de o uygular: `docker compose up
  migrate`, ardından `docker compose up -d --build api web`). Dev veritabanına `00005`'i
  uyguladığını Oğuzhan bildirdi (2026-11-20; Claude doğrulayamaz). Gerçek Claude adaptörü
  (`feat/claude-adapter`) isteğe bağlıdır ve API anahtarı olmadığı için yapılmıyor; kalan işler
  "Sonra yapılacaklar" ve "Açık konular" listelerinde. 5. oturumun `/export`'u `ai-sessions/`'a
  Oğuzhan tarafından eklenir.

### Önceki aşama: `feat/content-approval`

- **Son çalışan adım (`feat/content-approval`, PR #5 ile `main`'e merge edildi,
  `b5cb799`):** İçerik doğrulama ve üretim, migration `00002` (Down koruması dahil), içerik store'u,
  `POST`/`PUT /content` (SSE güvenilirliği dahil), `POST /approve`, `GET`'te onay
  bilgisi, ADR-023 (B: kanala uygunluk izin ve adres) ve Vue içerik/onay ekranı
  ve Playwright e2e testleri (izole Compose ortamı, ADR-024). **CI:** PR #5'te
  Backend, Frontend, Compose ve E2E işleri yeşil. Önceki aşama PR #3
  (`feat/segment-preview`, `170ecb5`).
- **Çalıştırılan komut ve sonuç (yerel):** `gofmt -l`, `go vet ./...` ve
  `go mod tidy -diff` temiz. Kökten `make test` geçti (entegrasyon testleri
  `campaigns_test` üzerinde). Eşzamanlılık, içerik ve HTTP testleri `-race` ile
  tekrarlı koşturuldu. Çözücü fuzz testi çalıştırıldı. Mutasyon kontrolleri ve
  hataları yakalayan regresyon testleri (önce başarısız, sonra geçen) çalıştırıldı.
  Gerçek `cmd/api` ikilisiyle `curl` duman testi (içerik, düzenleme, onay,
  SIGTERM) yapıldı. Hepsi yalnızca `campaigns_test` üzerinde ve MockLLM ile.
  Frontend (Node 24): `oxlint`, `eslint`, `vitest run` (186 test) ve `npm run
  build` (tip kontrolü dahil) temiz; `docker compose config --quiet` temiz.
  Gerçek Chromium'da Vue ekranı → Vite proxy → gerçek API → `campaigns_test`
  uçtan uca denendi (betik depoda değil).
- **Oğuzhan'ın doğrulaması:** Compose'ta `web` (8080) üzerinden SSE ve Vue
  ekranı (OK). Compose üzerinde, `localhost:8081` (api) üzerinden `curl` ile kampanya `POST`, içerik SSE `POST`, kampanya `GET`,
  içerik `PUT` ve **onay** (ilk onay, tekrar onay, farklı sürümle `409`)
  akışları; entegrasyon testlerinin kendi makinesinde çalıştırılması (backend).
  Dev veritabanında migration `00002` Oğuzhan tarafından uygulandı.
- **Doğrulanamayan:** Claude geliştirme veritabanına hiç bağlanmadı; Compose üzerindeki çalıştırma yalnızca Oğuzhan'ın bildirimine
  dayanıyor. Aktif akış sırasında SIGTERM ve `PUT` kaydetmenin
  commit öncesi iptali test edilmedi. nginx'e geçildiğinde SSE yeniden doğrulanmalı.
- **Sıradaki iş:** Bu aşama kapandı. Sıradaki aşama için güncel `main`'den yeni
  branch açılır (`feat/worker-dashboard`); açık kalanlar için yukarıya bakın.
  Migration dosyaları yalnızca Oğuzhan isteyince yazılır.
