-- +goose Up

-- Son kampanyalar listesi (CMP-001, ADR-029) için indeks.
--
-- GET /api/campaigns şu sorguyu çalıştırır:
--   SELECT id, brief, status, created_at FROM campaigns
--   ORDER BY created_at DESC, id DESC LIMIT n   (n en fazla 20)
--
-- İndeks sorgunun sıralamasıyla (created_at DESC, id DESC) birebir aynıdır. Planlayıcı onu
-- seçerse sorgu indeksi sırayla gezip ilk n kaydında durur: tam tablo taraması ve ayrı bir
-- sıralama (Sort) adımı gerekmez, maliyet tablo büyüklüğüyle değil kabaca O(log N + n) artar.
-- Seçilen sütunlar (brief, status) indekste olmadığı için n satır tablodan (heap) okunur; bu
-- bir "index-only" tarama değildir.
--
-- Planlayıcı maliyete göre karar verir: çok küçük bir tabloda tarama + sıralama daha ucuz
-- görünebilir ve o zaman indeks kullanılmaz. Bu yanlış değildir; indeks tablo büyüdükçe
-- işe yarar. İndeks yalnızca hızlandırır, sorgunun sonucunu veya sırasını değiştirmez.
--
-- Maliyeti: her kampanya eklemesinde bir indeks girişi yazılır. Veri ve mevcut sütunlar
-- değişmez, mevcut sorgular etkilenmez.
--
-- CONCURRENTLY kullanılmaz (goose her migration'ı bir transaction'da çalıştırır, CONCURRENTLY
-- transaction içinde çalışmaz). Düz CREATE INDEX tablo üzerinde SHARE kilidi alır: oluşturma
-- sürerken okumalar devam eder, kampanya yazmaları (INSERT, UPDATE, DELETE) bitmesini bekler.
-- Kampanya tablosu küçüktür, bu bekleme kısadır.
CREATE INDEX campaigns_recent_idx ON campaigns (created_at DESC, id DESC);

-- +goose Down

-- Yalnızca indeksi kaldırır; kampanya verisine dokunmaz. (DROP INDEX kısa süre tablo üzerinde
-- özel kilit alır.)
DROP INDEX campaigns_recent_idx;
