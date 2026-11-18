# Proje İlerlemesi

Son güncelleme: 2026-11-18

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
  `h1` başlığını ("Kampanya önizleme") doğrular. CI Playwright çalıştırmıyor.

**Çıkış koşulu:** Compose ile açılan arayüz, gerçek API anahtarı olmadan
seed verisi üzerinden beklenen kitle sayılarını gösteriyor. CI yeşil.
Sağlandı.

## Şu an — İçerik üretimi, düzenleme ve onay

**Branch:** `feat/content-approval` (devam ediyor, değişiklikler henüz commit
edilmedi)

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

**6. Testler ve doğrulama**

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

### Bekleyenler

Aşağıdaki maddelerin hiçbiri Vue ekranına başlamak için çözülmesi gereken bir
engel değildir; kayıtta tutulur ve ilgili aşamada ele alınır.

- [x] **Karar (Oğuzhan): ADR-023 → B uygulandı** (yukarıda, "Onay" bölümü).
- [x] Vue ekranı ve frontend lint, test, build kontrolleri (yukarıda).
- [x] Onay uç noktasının Compose üzerinde elle doğrulanması (Oğuzhan, `8081`
  üzerinden; yukarıda ayrıntılı).
- [ ] CI: değişiklikler commit/push edilmediği için CI bu aşama için çalışmadı;
  CI yeşil olmadan aşama bitmiş sayılmaz. Yerelde CI'ın backend (`make test`),
  frontend (lint, test, build) ve Compose (`config`) komutları temiz. Entegrasyon
  testleri `make test` üzerinden Backend işine girer; yeni araç gerekmediği için
  CI iş akışı değiştirilmedi.
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
- [ ] Aktif bir SSE akışı sırasında SIGTERM davranışı test edilmedi (kapanış
  bütçesi 15 sn, LLM çağrısı zaman aşımı 60 sn).
- [ ] `httpapi.New` başlangıçta nil bağımlılığı doğrulamıyor (nil `Content`,
  `Contents` veya `Approver` ilk istekte panik verir) ve `cmd/api` bağlamasını
  kapsayan test yok.
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
- [ ] Playwright e2e akışı (içerik ve onay) ve CI'a eklenmesi: yeni araç ve test
  türü olduğundan önce Oğuzhan'a sorulur.

**Çıkış koşulu:** Kullanıcı taslağı canlı görüyor, düzenleyebiliyor ve
gördüğü sürümü onaylayabiliyor. Geçersiz onay kuyruk işi oluşturmuyor.
Backend ve arayüz hazır, yerelde ve Compose'ta (`web` dahil) doğrulandı; yalnızca
CI (commit/push sonrası) bekliyor.

## Sonraki adımlar

Her aşama güncel `main`'den açılan ayrı bir branch'te ilerler. Branch
adları öneridir.

### 1. Worker, mock provider ve dashboard

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

- ADR-013'teki Compose başlatma sırası onay bekliyor (ADR hâlâ taslak). Sıra
  uygulandı ve Oğuzhan'ın Compose çalıştırmasında işledi.
- Web Vite dev server ile çalışıyor; `/api` proxy'si yalnızca geliştirme
  içindir. Teslimden önce nginx'e geçilecek (ADR-014).
- `frontend/e2e/vue.spec.ts` (Playwright, Chromium) Oğuzhan'ın yerel
  çalıştırmasında geçti ama CI Playwright çalıştırmıyor. Tarayıcı e2e'sini CI'a
  eklemek yeni araç/bağımlılık sayılır ve önce Oğuzhan'a sorulur. ADR-018'in
  `backend/tests/e2e/` klasörü (API, worker ve mock provider'lı uçtan uca
  testler) henüz yalnızca `.gitkeep` içeriyor.
- Test klasör yapısı ADR-018 ile onaylandı: unit testler kodun yanında,
  integration ve e2e `backend/tests/` altında. Segment-preview aşamasında
  `backend/internal/` altında `audience`, `campaign`, `database`, `httpapi`,
  `llm`, `seed`, `segment` ve `testdb` paketleri açıldı; `cmd/` altında `api`,
  `migrate` ve `seed` var. `worker`, kuyruk ve `mock-provider` paketleri ilgili
  aşamalarda açılacak.
- `make test` önce yalnızca `postgres` servisini başlatır (`make test-db`),
  sonra `go test ./...` çalıştırır. Entegrasyon testleri `campaigns_test`
  veritabanını kullanır (ADR-015).
- `SEG-001`, `CNT-001`, `SND-001` ve `TRK-001` spec dosyaları PR #2 ile
  `main`'e alındı ([specs/README.md](specs/README.md)). `SEG-001` uygulandı;
  diğerleri ilgili aşamalarda uygulanacak.
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
- `llm_calls` tablosu ve `llm_validation_failures_total` metriği içerik
  aşamasında yazılmadı; kapsamı ve zamanı Oğuzhan'la netleştirilecek
  (ARCHITECTURE §4 ve §12).
- Doğrulayıcıdaki kod noktası sayımı birleşik karakterleri (ör. `e` +
  birleştirici aksan) iki sayar; spec kod noktası istiyor (ADR-022).
- Markdown çitli (` ```json `) model çıktısı `MALFORMED_OUTPUT` sayılır ve
  düzeltme denemesine gider. Gerçek Claude adaptöründe JSON modu veya prefill
  değerlendirilecek.
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

- ADR-023 onaylandı (B): kanala uygunluk açık izin ve adres ister; önizleme ve
  kuyruk aynı kuralı kullanır.

## Son doğrulama

- **Son çalışan adım (`feat/content-approval`, henüz commit edilmedi):** İçerik
  doğrulama ve üretim, migration `00002` (Down koruması dahil), içerik store'u,
  `POST`/`PUT /content` (SSE güvenilirliği dahil), `POST /approve`, `GET`'te onay
  bilgisi, ADR-023 (B: kanala uygunluk izin ve adres) ve Vue içerik/onay ekranı.
  Önceki aşama PR #3 (`feat/segment-preview`): CI'dan geçip `main`'e squash merge edildi (`170ecb5`).
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
- **Doğrulanamayan:** CI bu değişiklikler için çalıştırılmadı (commit ve push
  yok); frontend ve backend kontrolleri yalnızca yerelde çalıştırıldı. Claude
  geliştirme veritabanına hiç bağlanmadı; Compose
  üzerindeki çalıştırma yalnızca Oğuzhan'ın bildirimine dayanıyor. Playwright
  e2e sonuçları da Oğuzhan'ın bildirimine dayanıyor; CI Playwright çalıştırmıyor.
  Playwright e2e akışı depoya eklenmedi (yeni test türü, CI'a eklemek önce
  Oğuzhan'a sorulur). Aktif akış sırasında SIGTERM ve `PUT` kaydetmenin commit
  öncesi iptali test edilmedi. nginx'e geçildiğinde SSE yeniden doğrulanmalı.
- **Sıradaki iş:** Oğuzhan commit/push edip PR'daki CI sonucuna bakar (yukarıdaki "Bekleyenler"). CI yeşilse aşama bitmiş sayılır.
  Migration dosyaları yalnızca Oğuzhan isteyince yazılır.
