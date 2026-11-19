-- +goose Up

-- Onay anında render için kullanılan alıcı adının mesaja kopyası (ADR-026).
--
-- Neden: worker adı her claim'de users tablosundan okurdu. Ad iki deneme arasında
-- değişirse aynı Idempotency-Key farklı içerikle gider ve provider isteği 422 ile
-- reddederdi (mesaj yanlışlıkla failed/rejected olurdu). Artık ad onayda mesaja
-- yazılır ve retry dahil her deneme bu kayıtlı adı kullanır.
--
-- Değer HAM kopyadır (kullanıcının first_name'i, yoksa ''). Boş veya yalnızca
-- boşluktan oluşan ad render sırasında eskisi gibi fallback'e gider
-- (content.Render); davranış değişmez.
--
-- ===================== ESKİ MESAJLAR İÇİN UYARI =====================
-- Mevcut mesajlar için onay anındaki ad BİLİNMİYOR. Aşağıdaki doldurma
-- users.first_name'in ŞİMDİKİ değerini yazar. Bu, daha önce DENENMİŞ bir mesajda
-- (attempts > 0) ilk denemede gönderilen içerikle aynı olmayabilir: ad arada
-- değiştiyse yeniden deneme farklı içerikle gider ve aynı anahtarla farklı içerik
-- provider'da 422 olur. Provider ilk isteği kabul edip yanıtı kaybetmiş
-- olabileceği için bunu "zararsız" saymak da, eski worker'la boşaltmak da güvenli
-- bir çözüm DEĞİLDİR.
--
-- Bu yüzden: queued veya sending durumunda daha önce denenmiş (attempts > 0) en az
-- bir mesaj varsa migration HİÇBİR VERİ DEĞİŞTİRMEDEN durur. Otomatik failed
-- yapmaz, riskli mesajı sessizce güvenli saymaz. Ne yapılacağına operatör karar
-- verir (bkz. DECISIONS.md ADR-026).
--
-- Güvenli olanlar: queued ve attempts = 0 (provider'a hiç istek gitmedi; ad,
-- migration anındaki değerle gönderilir) ve terminal mesajlar (sent, failed,
-- skipped: yalnızca bilgi amaçlı).
--
-- ÇALIŞTIRMADAN ÖNCE API VE WORKER KAPALI OLMALIDIR. Aksi hâlde eski kod: eski
-- worker adı yine users'tan okur; eski API sütunu bilmediği için mesaj
-- ekleyemez (aşağıdaki DROP DEFAULT bunu bilerek hata yapar). Komut sırası için
-- ADR-026'ya bakın.

-- Kontrol ile değişiklik arasında yeni bir claim veya mesaj yazımı olamasın. Bu,
-- uçuştaki bir claim'i (commit edilmemiş sending) bekler ve sonra görür.
LOCK TABLE messages IN ACCESS EXCLUSIVE MODE;

-- +goose StatementBegin
DO $$
DECLARE
    n integer;
BEGIN
    SELECT count(*) INTO n
    FROM messages
    WHERE status IN ('queued', 'sending') AND attempts > 0;
    IF n > 0 THEN
        RAISE EXCEPTION
            'Migration 00004 reddedildi: queued veya sending durumunda, daha önce denenmiş (attempts > 0) % mesaj var. Hiçbir veri değiştirilmedi. Bu mesajların provider tarafından kabul edilmiş olma ihtimali var ve onay anındaki ad bilinmiyor; mevcut ad doldurulursa yeniden deneme farklı içerikle gidebilir. API ve worker kapalıyken operatör karar vermeli (bkz. DECISIONS.md ADR-026).', n
            USING ERRCODE = 'restrict_violation';
    END IF;
END
$$;
-- +goose StatementEnd

-- DEFAULT yalnızca mevcut satırların doldurulabilmesi içindir.
ALTER TABLE messages ADD COLUMN first_name text NOT NULL DEFAULT '';

UPDATE messages m
SET first_name = coalesce(u.first_name, '')
FROM users u
WHERE u.id = m.user_id;

-- Yeni mesajlar adı AÇIKÇA yazmak zorundadır. Varsayılan bırakılsaydı, sütunu
-- bilmeyen eski bir API sessizce adsız (fallback'li) mesaj üretirdi; şimdi onay
-- NOT NULL hatasıyla gürültülü şekilde başarısız olur ve kampanya draft kalır.
ALTER TABLE messages ALTER COLUMN first_name DROP DEFAULT;

-- +goose Down

-- Dikkat: geri alma onay anındaki ad kopyasını siler. Mesaj ve kampanya verisi
-- korunur; sonraki worker adı yine users'tan okur (retry sırasında ad değişirse
-- ADR-026'daki risk geri gelir).
ALTER TABLE messages DROP COLUMN first_name;
