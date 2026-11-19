-- +goose Up

-- Mock SMS/e-posta sağlayıcısının kalıcı kabul kayıtları (SND-001, ADR-025,
-- ARCHITECTURE §11).
--
-- Provider harici bir sistemi temsil eder; bu yüzden messages tablosuna YABANCI
-- ANAHTAR YOKTUR. İlişki yalnızca idempotency_key metniyle kurulur (messages
-- tarafında campaign:user:channel). Veritabanı bu ilişkiyi zorlamaz; testler
-- iki tabloyu birleştirerek doğrular.
--
-- Yalnızca BAŞARILI kabuller saklanır. 429 ve 500 kabulden önce enjekte edilir
-- ve kaydedilmez, bu yüzden kayıtlı sonuç her zaman 200 {message_id}'dir ve
-- durum kodu saklanmaz.
--
-- Aynı anahtarla farklı alıcı, kanal veya içerik gelirse provider isteği
-- reddeder. Bu karşılaştırma için kanal ve alıcı olduğu gibi, içerik ise SHA-256
-- özeti olarak saklanır.
CREATE TABLE mock_provider_acceptances (
    -- Atomik tekilleştirme: INSERT ... ON CONFLICT DO NOTHING bu anahtara dayanır.
    idempotency_key text        PRIMARY KEY,
    channel         text        NOT NULL,
    recipient       text        NOT NULL,
    content_hash    bytea       NOT NULL,
    message_id      uuid        NOT NULL DEFAULT gen_random_uuid(),
    accepted_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mpa_idempotency_key_nonblank CHECK (idempotency_key ~ '[^ \t\n\r\f\v]'),
    CONSTRAINT mpa_channel_valid CHECK (channel IN ('sms', 'email')),
    CONSTRAINT mpa_recipient_nonblank CHECK (recipient ~ '[^ \t\n\r\f\v]'),
    CONSTRAINT mpa_content_hash_sha256 CHECK (octet_length(content_hash) = 32),
    CONSTRAINT mpa_message_id_unique UNIQUE (message_id)
);

-- +goose Down

-- Dikkat: geri alma provider'ın kabul kayıtlarını siler. Bu tablo yalnızca mock
-- provider'a aittir; mesajlar, kampanyalar ve kullanıcılar etkilenmez.
DROP TABLE mock_provider_acceptances;
