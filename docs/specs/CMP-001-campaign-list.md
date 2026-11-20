# CMP-001 — Son kampanyalar listesi

**Durum:** Taslak

## Amaç

Pazarlamacı ana sayfada daha önce oluşturduğu kampanyaları görür ve kaldığı yerden devam eder:
taslak bir kampanyanın içeriğine ve onayına döner, onaylanmış bir kampanyanın gönderimini
izler. Bunun için kampanya kimliğini hatırlamak veya adres çubuğuna yazmak gerekmez.

## Beklenen davranış

### API (`GET /api/campaigns`)

- En yeni kampanyayı başa alarak en çok `limit` kampanya döner. Sıralama oluşturulma zamanına
  (`created_at`) göredir; aynı anda oluşanlar kimliğe göre azalan sırayla ayrılır, bu yüzden
  sıra her çağrıda aynıdır.
- `limit` verilmezse veya boşsa 10 kullanılır. En fazla 20 döner: 20'den büyük değer
  hata değil, 20'ye indirilir.
- `limit` tam sayı değilse veya 1'den küçükse `400 INVALID_REQUEST` döner ve veritabanı
  okunmaz. Değer sessizce düzeltilmez.
- Aynı parametre birden fazla verilirse ilki, bilinmeyen parametreler yok sayılır.
- Gövde `{"campaigns": [...]}` biçimindedir. Her satır yalnızca `id`, `brief`, `status` ve
  `created_at` taşır. Kitle sayıları, segment, içerik ve onay bilgisi listede yoktur;
  ayrıntı `GET /api/campaigns/{id}`'dedir.
- `brief` kayıtlı metindir (en fazla 1000 karakter); kırpma arayüzün işidir. `created_at` UTC
  RFC 3339 biçimindedir. `status` kayıtlı değerdir (`draft`, `approved`, `sending`,
  `completed`).
- Hiç kampanya yoksa `200` ve `{"campaigns": []}` döner (`null` değil).
- Liste salt okunurdur: LLM çağrılmaz, kitle sayılmaz, hiçbir kayıt oluşmaz veya değişmez.
- Veritabanı hatasında `500 INTERNAL` döner; iç hata ayrıntısı yanıta sızmaz, yalnızca loga yazılır.
- `/api/campaigns` yolunda `GET` ve `POST` dışındaki yöntemler `405` döner (`Allow: GET, POST`).
- Uygulamada kullanıcı kavramı yoktur: liste tüm kampanyaları gösterir (`GET /api/campaigns/{id}`
  ile aynı). Yetkilendirme kapsam dışıdır.

### Arayüz (kenar çubuğu)

- "Son kampanyalar" her ekranda görünen kenar çubuğundadır (ana sayfadaki kart değildir) ve en
  yeni 10 kampanyayı yükler. Sayfa içeriği listeyi beklemez ve liste yüklenemese de çalışır.
  Masaüstünde kenar çubuğu açıktır ve üst başlıktaki düğmeyle kapanır; telefonda çekmecedir
  (düğmeyle açılır, bir bağlantıya gidilince kapanır).
- Her satırda brief (en çok iki satır, uzun ve boşluksuz metin kırılır), simgeli durum rozeti ve
  tarih vardır. Kitle sayısı gösterilmez. Açık olan kampanyanın satırı `aria-current="page"` ile
  işaretlidir.
- Taslak (`draft`) kampanya kampanya ekranına (`/campaigns/{id}`), onaylanmış (`approved`,
  `sending`, `completed`) kampanya gönderim takibine (`/campaigns/{id}/delivery`) açılır. Bilinmeyen
  bir durum kampanya ekranına açılır. Satırın nereye gittiği ("Kampanyayı aç" veya "Gönderimi
  izle") ekran okuyucuya yazılır ve ipucu (`title`) olarak verilir; görselde durum rozeti anlatır.
- Liste, yeni bir kampanya oluşunca, onaylanınca, dashboard'da durum değişince ve başka ekrana
  geçilince yeniden okunur; yeni kampanya en üstte görünür. Hatayla sonuçlanan istek (kampanya
  oluşmadı) listeyi yeniden okumaz. Ekranlar listeyi tanımaz; yalnızca `campaign/recent.ts`
  üzerinden haber verir.
- Liste yüklenirken iskelet, boşsa "Henüz kampanya yok" açıklaması, yüklenemezse hata ve
  "Yeniden dene" gösterilir. Yenilenirken eski satırlar görünür kalır.
- Brief metni HTML olarak yorumlanmaz. Telefon genişliğinde (360 px) yatay taşma olmaz.

## Doğrulama

- Birim: `internal/httpapi/campaign-list_test.go` (sıra korunur, boş liste `[]`, varsayılan ve
  üst sınır, geçersiz sınırda depo çağrılmaz, hata sızmaz, yalnızca belgelenen alanlar).
- Entegrasyon (gerçek PostgreSQL, yalnızca `campaigns_test`):
  `tests/integration/campaign-list_test.go` (25 kampanyayla sıra ve sınırlar, eşit zamanda
  kararlı sıra, boş liste, dört durumun gerçek onay akışıyla listelenmesi ve ayrıntı endpoint'iyle
  tutarlılık, 1000 karakterlik brief, depo sınırları). Mutasyonla (sıralamayı tersine çevirme,
  kimlik ayrımını kaldırma) yakalandığı görüldü.
- Arayüz birim testleri: `api/__tests__/campaigns.spec.ts`, `campaign/__tests__/links.spec.ts`,
  `components/__tests__/RecentCampaigns.spec.ts`, `components/__tests__/StatusBadge.spec.ts`,
  `views/__tests__/CampaignPreviewView.spec.ts` (kampanya oluşunca haber verir, hatada vermez),
  `__tests__/App.spec.ts` (listenin kenar çubuğunda yüklenmesi, haber ve sayfa değişiminde yenilenmesi,
  yüklenemese de ekranın açılması).
- e2e (Playwright, izole ortam): `frontend/e2e/campaign-list.spec.ts` (taslak ve onaylı
  satırın hedefleri, sıra, aynı sayfada yenileme, 21 kampanyayla varsayılan 10 ve üst sınır 20,
  geçersiz sınır 400); `app-shell.spec.ts` (kenar çubuğu açma/kapama, onayla birlikte durumun
  güncellenmesi, kaydedilmemiş değişiklik uyarısı); `visual-themes.spec.ts` ve
  `mobile-layout.spec.ts` (çekmece dahil) liste okunabilirliğini ve taşmayı sınar.

## Sınırlar

- Sayfalama, arama ve süzme yoktur; yalnızca en yeni 10-20 kampanya görünür.
- `campaigns_recent_idx` (migration `00005`: `created_at DESC, id DESC`) sorgunun sıralamasıyla
  aynıdır. Planlayıcı seçerse sorgu indeksi sırayla gezip ilk n kayıtta durur: tam tarama ve
  `Sort` adımı olmaz, kabaca O(log N + n). `brief` ve `status` indekste olmadığı için n satır
  tablodan okunur (index-only değil). Planlayıcı maliyete göre karar verir; çok küçük tabloda
  tarama + sıralama seçip indeksi kullanmayabilir, bu doğru davranıştır. Test
  (`tests/integration/migration-index_test.go`) şunları sınar: indeks tanımı, **sıralı tarama
  kapatılınca** planın indeksi kullandığı ve `Sort` adımı olmadığı (yani indeks bu sorgu için
  kullanılabilir; planlayıcının her ölçekte seçtiği değil), Down yalnızca indeksi kaldırır, indeksli ve
  indekssiz sonuç aynıdır. İndeks yalnızca hızlandırır; davranış değişmez. Maliyet: her
  kampanya eklemesinde bir indeks girişi.

## İlgili belgeler

- [Case brief'i](../CASE.md)
- [Teknik kararlar](../../DECISIONS.md): ADR-029
- [Mimari, API tablosu](../ARCHITECTURE.md)
