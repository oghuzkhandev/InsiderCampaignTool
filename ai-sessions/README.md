# Claude Çalışma Oturumları

Bu dizin, Insider One case'i üzerinde çalışırken yapılan Claude
oturumlarının kayıtlarını içerir. Oturum dizini, her çalışmanın konusunu
ve ilgili export dosyasını veya paylaşım bağlantısını gösterir.

## Oturumları düzenleme

- Repo kurulumu, segment doğrulaması ve gönderim kuyruğu gibi farklı
  çalışma konuları için ayrı oturum aç.
- Claude Code'da her oturumu `/export` ile ayrı bir dosyaya kaydet.
  Dosya adında sıra numarası ve kısa bir konu kullan.
  Örneğin `01-repo-kurulumu.txt` veya `02-segment-dogrulama.txt`.
- Aynı oturuma daha sonra devam edersen mevcut export'u güncelle.
  O oturuma ait tüm prompt ve cevaplar aynı dosyada kalsın.
- Claude.ai kullandıysan her oturumun paylaşım bağlantısını ayrı bir
  tablo satırına ekle.
- Teslimden önce export dosyalarını ve paylaşılan sohbetleri gözden
  geçir. Gerçek API anahtarlarını ve kişisel bilgileri temizle.

## Oturum dizini

Dosya adı biçimi: `Session-NN-Konu.txt` (Claude Code `/export`).

| No | Konu | Branch | Dosya | Durum |
| --- | --- | --- | --- | --- |
| 1 | Repo kurulumu: Docker Compose, Go API başlangıcı, çalışma düzeni ve CI | `chore/project-setup` | [Session-01-ProjectSetup.txt](Session-01-ProjectSetup.txt) | Kayıtlı |
| 2 | PostgreSQL, seed, migration ve segment önizleme | `feat/segment-preview` | [Session-02-DB-Seed-Migration.txt](Session-02-DB-Seed-Migration.txt) | Kayıtlı |
| 3 | Brief, kitle, içerik üretimi, düzenleme ve onay; Playwright e2e | `feat/content-approval` | [Session-03-Brief-Kitle-Düzenleme-Onay.txt](Session-03-Brief-Kitle-Düzenleme-Onay.txt) | Kayıtlı |
| 4 | Worker, mock provider, canlı dashboard (SSE), metrikler ve Prometheus, nginx, README ve teslim kontrolleri | `feat/worker-dashboard` | [Session-04-Worker-Queue-MockProvider.txt](Session-04-Worker-Queue-MockProvider.txt) (Claude Code `/export`; sohbetin ortasından başlar). Baştan sona ayrı döküm: `Session-04-Tam-Konusma-Dokumu.txt` (ham kayıttan çıkarıldı; yalnızca kullanıcı mesajları ve görünen asistan yanıtları) | Export kayıtlı; tam döküm Oğuzhan tarafından eklenecek |
| 5 | Frontend iyileştirmeleri: tasarım ve bileşenlerin modernleştirilmesi. Oturumun başında 4. oturumun CI sonuçları bildirilecek | Ayrı yeni branch, `main`'den (önerilen ad `feat/frontend-modernization`) | `Session-05-Frontend-Tasarim.txt` (önerilen ad) | Planlandı |

Paylaşımdan önce her export'ta gerçek API anahtarları ve kişisel bilgiler
(e-posta adresi, kullanıcı adı, yerel yollar) gözden geçirilmelidir.
