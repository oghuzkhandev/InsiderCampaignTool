# AUD-001 — Segmente uyan kişiler

Karar ADR-031. `GET /api/campaigns/{id}/audience?page=1&limit=10`, kayıtlı segmenti
istek anındaki referans zamanla uygular; LLM çağırmaz veya veri yazmaz. `items`, `page`,
`limit`, `total` döner. Sıra users.id ASC; varsayılan limit 10, üst sınır 50; sayfa üst sınırı
100000. Sayfa ve toplam aynı SQL ifadesinden gelir. Kampanya yoksa 404.

Her kişide id, first_name, city, email, phone, sms_status ve email_status bulunur.
Eksik ad/adres null; kanal durumları eligible, no_consent, no_address. İzin yokluğu
adres yokluğundan önce bildirilir. Eşleşen ama hiçbir kanala uygun olmayan kişiler de listelenir.

Arayüzde yükleme, hata/yeniden deneme, boş liste ve sayfalama gösterilir. Kampanya değişince
önceki istek iptal edilir ve geç yanıt yeni kampanyanın listesini değiştiremez. Dar ekranda
bilgiler kart düzenine geçer. Liste hatası brief/kampanya akışını engellemez.

Liste sahte demo verisidir; kesin alıcılar onayda yeniden seçilir. Kayıtlı önizleme sayılarıyla
istek anındaki liste arasında veri ve zaman değişiminden fark olabilir. Doğrulama: SQL/handler
birim testleri, gerçek PostgreSQL ve frontend API/bileşen testleri.
