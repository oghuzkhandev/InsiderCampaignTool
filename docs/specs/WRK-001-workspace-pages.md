# WRK-001 — Çalışma alanı ve içerik şablonları

Karar: ADR-032. Branch: `feat/audience-preview-list` (proje sahibi aynı branch'i onayladı).

- `/overview`: güncel kampanya, kullanıcı, gönderim ve şablon sayıları; yenileme ve ilgili sayfalara bağlantılar.
- `/`: mevcut yeni kampanya/brief akışı.
- `/campaigns`: tüm kampanyalar, durum filtresi, 10 kayıtlık sayfalar; taslak içerik ekranına,
  diğer durumlar gönderim takibine açılır.
- `/audiences`: tüm sahte demo kullanıcıları; ad, şehir, e-posta, telefon ve iki kanalın
  uygunluk sebepleri. Ad/şehir/adres araması literal alt metindir; eşleşen izinsiz kişiler elenmez.
- `/templates`: adlandırılmış SMS, e-posta konusu ve gövdesi oluşturma, düzenleme, silme.
  Boş/100 karakteri aşan ad ve geçersiz içerik reddedilir. Kişiselleştirme mevcut whitelist'i kullanır.
  Şablon kampanyaya yalnızca metin olarak aktarılır; insan yeni sürüm kaydeder ve ayrıca onaylar.
- Menü sabit beş bağlantıdır; kampanyalar menüye tek tek eklenmez. Marka ikon olmadan
  `Insider Campaign Tool` olarak gösterilir.

API sözleşmesi ARCHITECTURE içindeki çalışma alanı bölümünde. Şablonlar migration 00006 ile
kalıcıdır. Kimlik doğrulama yok; yalnızca sahte demo verisi içindir. Template Down veri siler.

Doğrulama: gerçek PostgreSQL ile CRUD, geçersiz içerikte değişiklik olmaması, sayfa/toplam,
filtre, arama, migration Down/Up; tarayıcıda beş sayfa, şablonun kampanyaya uygulanması ve
şablon silinse de kaydedilmiş kampanyanın değişmemesi. Mevcut onay/gönderim e2e'leri korunur.
