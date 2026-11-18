# Insider One Campaign Tool

Doğal dilde yazılan bir brief'ten hedef kitle, SMS ve e-posta taslağı
üreten bir kampanya aracı. Pazarlamacı taslağı inceler, gerekirse
düzenler ve onaylar. Gönderim işleri arka planda işlenir ve sonuçlar
dashboard'da izlenir. Demo, sahte kullanıcılar ve mock gönderim
sağlayıcısı kullanır.

## Proje durumu

Proje geliştirme aşamasında. Güncel durum ve sıradaki görev
[ilerleme notunda](docs/PROGRESS.md) yer alır.

## Kurulum ve test

Teslimde repo kökünden `docker compose up` komutu arayüzü, API'yi,
PostgreSQL'i, worker'ı ve mock provider'ı başlatacak. Migration ve
demo kullanıcı seed'i kurulum sırasında otomatik çalışacak.

`make test` gerçek bir API anahtarı gerektirmeden çalışacak.

Tarayıcıdan uçtan uca (Playwright) testler için `make e2e`: geliştirme
ortamından ayrı, izole bir Compose projesinde (`compose.e2e.yaml`) çalışır ve
geliştirme veritabanına dokunmaz. Gereksinimler: Docker ve
`cd frontend && npx playwright install chromium` (bir kez).
Kurulum doğrulandıktan sonra uygulama adresi ve demo adımları
bu bölüme eklenecek.

## Mimari

Vue arayüzü Go API ile iletişim kurar. API, Claude veya MockLLM çıktısını
doğrular ve onaylanan gönderim işlerini PostgreSQL'e kaydeder. Go worker
işleri alır, kanal iznini kontrol eder ve mock provider'a gönderir.

- [Case brief'i](docs/CASE.md)
- [Mimari](docs/ARCHITECTURE.md)
- [Teknik kararlar](DECISIONS.md)
- [Claude çalışma oturumları](ai-sessions/README.md)
