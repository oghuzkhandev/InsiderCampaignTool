// Gönderim sayılarından türetilen özetler (TRK-001). Sayılar sunucudan gelir;
// burada yalnızca toplama ve oran hesaplanır.
import type { DeliveryCounts } from '@/api/delivery'

/** Kampanyadaki toplam mesaj sayısı (bütün durumlar). */
export function totalMessages(counts: DeliveryCounts): number {
  return (
    counts.queued + counts.retrying + counts.sending + counts.sent + counts.failed + counts.skipped
  )
}

/**
 * İşi bitmiş mesajlar: gönderilen, başarısız ve atlanan (terminal durumlar). Bunların
 * hepsi bittiğinde kampanya tamamlanır; başarısız veya atlanan mesaj olması
 * tamamlanmayı engellemez.
 */
export function finishedMessages(counts: DeliveryCounts): number {
  return counts.sent + counts.failed + counts.skipped
}

/**
 * İşi bitmiş mesajların yüzdesi (0-100, aşağı yuvarlanır): yüzde 100 yalnızca
 * bütün mesajlar bittiğinde görünür. Mesaj yoksa 0.
 */
export function finishedPercent(counts: DeliveryCounts): number {
  const total = totalMessages(counts)
  if (total === 0) return 0
  return Math.floor((finishedMessages(counts) / total) * 100)
}
