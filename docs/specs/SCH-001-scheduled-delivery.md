# SCH-001 — Zamanlanmış gönderim

Durum: yerelde doğrulandı; GitHub Actions ve dev ortamında elle doğrulama bekliyor. Karar ADR-033.

- İnsan son içerik sürümünü hemen veya seçtiği gelecekteki bir anda gönderilmek üzere onaylar.
- `POST /api/campaigns/{id}/approve` gövdesinde isteğe bağlı `scheduled_at` RFC3339 zamanıdır;
  açık UTC/ofset zorunlu. Eksik veya null alan hemen gönderimdir; eski istemciler çalışır.
- Yeni onayda geçmiş zaman 422 SCHEDULE_IN_PAST. Karşılaştırma PostgreSQL saatine göredir.
  Hatalı tarih/ofsetsiz metin 400. Başarıdaki zaman UTC ve mikro-saniye hassasiyetindedir.
- Aynı içerik sürümü ve aynı zamanla tekrar 200, zaman farklıysa 409 SCHEDULE_CONFLICT.
  Aynı anı başka ofsetle yazmak aynı plandır. Tekrar isteği seçilen zaman geçtikten sonra da
  mevcut sonucu döndürür; alıcılar yeniden seçilmez, mesajlar çoğalmaz.
- Alıcılar ve kişiselleştirme adı onay anında sabitlenir. Onay, zaman ve queued işler tek
  transaction'da yazılır. Gönderimden önceki kanal izni kontrolü korunur.
- Kampanya scheduled_at tutar; mesajların next_attempt_at değeri bu zaman olur. Worker mevcut
  vadesi gelen işleri sahiplenme sorgusunu kullanır; erken deneme ve provider çağrısı yapılmaz.
- API/worker yeniden başlatılabilir. Seçilen saat geçtikten sonra worker gelirse işler o zaman
  işlenir. Saat en erken başlangıçtır; kesin saniyede başlama/tamamlanma garantisi değildir.
- DB durumları değişmez: approved + scheduled_at arayüzde Planlandı; sending/completed mevcut
  durumlarıyla görünür. Kampanya GET, sayfalı liste ve dashboard snapshot zaman bilgisini taşır.
- Arayüz cihazın yerel saatini UTC'ye çevirir. Seçim, onay özeti ve kaydedilmiş planda IANA saat
  dilimi ve ofset görünür. Takvimde olmayan/yaz saati geçişinde var olmayan yerel saat reddedilir.
  Geri saat geçişindeki çift anlamlı saatte tarayıcının seçtiği ofset onay özetinde gösterilir.
- Onay sonrası saat değiştirme, iptal, tekrarlı görevler ve yeni servis bu ilk kapsamda yok.
- Migration 00007 yalnızca test veritabanında doğrulanır. Down bitmemiş planlı kampanya varsa
  reddedilir; aksi halde plan geçmişi sütunuyla birlikte kalkar. Dev'e proje sahibi uygular.
- Doğrulama: zaman dönüşümü ve form unit testleri; gerçek PostgreSQL/worker/provider ile erken
  gönderilmeme, gecikmiş başlangıç, replay, atomiklik, eşzamanlı onay ve migration testleri.
  Proje sahibinin son isteğiyle `frontend/e2e/scheduled-delivery-real.spec.ts` iki gerçek
  yığın senaryosu ekler: planla/yenile/erken gönderilmeme/zamanı gelince gönderim/replay ve
  boş/geçmiş zamanın UI/API'de reddedilmesi. Tarayıcı İstanbul saat dilimindedir; saat, API
  ve SSE taklit edilmez. `@real` etiketi mevcut CI E2E gönderim döngüsüne otomatik dahil eder.
  Yerelde `make e2e-delivery`; tüm döngüler için `make e2e`. Normal `make test` Go süreç e2e
  testlerini içerir.
