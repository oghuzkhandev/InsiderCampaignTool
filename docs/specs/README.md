# Özellik Şartnameleri

Bu klasör, özelliklerin beklenen davranışını ve nasıl doğrulanacağını
açıklar. Gereksinimlerin kaynağı [Insider One brief'i](../CASE.md).

## Spec dizini

| Kimlik | Özellik | Durum |
| --- | --- | --- |
| `SEG-001` | [Segment üretimi ve kitle önizleme](SEG-001-segment-preview.md) | Taslak |
| `CNT-001` | [İçerik üretimi, düzenleme ve onay](CNT-001-content-generation-and-approval.md) | Taslak |
| `SND-001` | [Mesaj gönderimi](SND-001-message-delivery.md) | Taslak |
| `TRK-001` | [Canlı gönderim takibi](TRK-001-live-delivery-tracking.md) | Taslak |
| `CMP-001` | [Son kampanyalar listesi](CMP-001-campaign-list.md) | Taslak |
| `AUD-001` | [Segmente uyan kişiler](AUD-001-audience-member-list.md) | Uygulandı; CI bekliyor |
| `WRK-001` | [Çalışma alanı ve şablonlar](WRK-001-workspace-pages.md) | Yerelde doğrulandı; CI bekliyor |
| `SCH-001` | [Zamanlanmış gönderim](SCH-001-scheduled-delivery.md) | Yerelde doğrulandı; CI bekliyor |

## Yeni spec ekleme

1. [TEMPLATE.md](TEMPLATE.md) dosyasını kullan.
2. Özelliğe `SEG-001` gibi sabit bir kimlik ver.
3. Beklenen davranışı numarasız, kısa ve test edilebilir maddelerle yaz.
4. Dosya oluştuğunda dizinde özelliğin adına bağlantı ekle.

Mimari kararların gerekçeleri [DECISIONS.md](../../DECISIONS.md),
güncel çalışma sırası [PROGRESS.md](../PROGRESS.md) dosyasındadır.
