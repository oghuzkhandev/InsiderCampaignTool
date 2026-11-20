// Gönderim takibi ekranının Türkçe metinleri. Sunucudan gelen serbest metin
// gösterilmez; yalnızca durum ve neden kodlarına karşılık gelen sabit açıklamalar
// ve sayılar kullanılır (content/messages.ts ile aynı yaklaşım).
import type { ApiError } from '@/api/campaigns'
import type { DeliveryCounts, DeliverySnapshot } from '@/api/delivery'
import { totalMessages } from './summary'
import { withPossessive } from './turkish'

/** Kampanya durumunun kullanıcıya gösterilen adı. Bilinmeyen durum olduğu gibi gösterilir. */
export function campaignStatusLabel(status: string): string {
  switch (status) {
    case 'draft':
      return 'Taslak'
    case 'approved':
      return 'Onaylandı, kuyrukta'
    case 'sending':
      return 'Gönderiliyor'
    case 'completed':
      return 'Tamamlandı'
    default:
      return status
  }
}

/** Mesaj sayılarının ekrandaki sırası, adı ve ne anlama geldiği. */
export const COUNT_ITEMS: readonly {
  key: keyof DeliveryCounts
  label: string
  hint: string
}[] = [
  { key: 'queued', label: 'Bekleyen', hint: 'Henüz gönderilmeye çalışılmadı' },
  { key: 'retrying', label: 'Yeniden denenecek', hint: 'Geçici hata sonrası sırada bekliyor' },
  { key: 'sending', label: 'Gönderiliyor', hint: 'Şu an sağlayıcıya iletiliyor' },
  { key: 'sent', label: 'Gönderildi', hint: 'Sağlayıcı kabul etti' },
  { key: 'failed', label: 'Başarısız', hint: 'Gönderilemedi, yeniden denenmeyecek' },
  { key: 'skipped', label: 'Atlandı', hint: 'Kanal izni olmadığı için gönderilmedi' },
]

const nf = new Intl.NumberFormat('tr-TR')

/**
 * Deneme sınırı dolan mesajlar: sağlayıcının mesajı kabul edip yanıtı kaybetmiş
 * olma ihtimali vardır; bu yüzden "teslim edilmedi" denmez (SND-001, TRK-001).
 */
export function exhaustedNote(count: number): string {
  return (
    `${nf.format(count)} mesaj: Gönderim sonucu doğrulanamadı; deneme sınırı doldu. ` +
    'Bu, mesajın kesinlikle teslim edilmediği anlamına gelmez.'
  )
}

export interface CompletionLine {
  key: 'sent' | 'failed' | 'skipped'
  label: string
  count: number
  text: string
  /** Kullanıcının bu sonuç için yapabilecekleri (veya yapılacak bir şey olmadığı). */
  advice: string[]
}

/**
 * Gönderim bitince sonucun tek cümlelik özeti: "7 mesajın 5'i gönderildi, 1'i başarısız,
 * 1'i atlandı." Başarısız veya atlanan mesaj olması kampanyanın tamamlanmasını engellemez;
 * özet bunu gizlemeden söyler. Yalnızca sıfırdan büyük sonuçlar yazılır; hepsi tek sonuçsa
 * "tamamı" denir.
 */
export function completionHeadline(counts: DeliveryCounts): string {
  const total = totalMessages(counts)
  if (total === 0) return 'Gönderilecek mesaj yoktu.'

  const outcomes = [
    { count: counts.sent, list: 'gönderildi', whole: 'gönderildi' },
    { count: counts.failed, list: 'başarısız', whole: 'başarısız oldu' },
    { count: counts.skipped, list: 'atlandı', whole: 'atlandı' },
  ].filter((o) => o.count > 0)

  const only = outcomes.length === 1 ? outcomes[0] : undefined
  if (only) return `${nf.format(total)} mesajın tamamı ${only.whole}.`
  const parts = outcomes.map((o) => `${withPossessive(o.count)} ${o.list}`)
  return `${nf.format(total)} mesajın ${parts.join(', ')}.`
}

/**
 * Gönderim sonu özetinin satırları: gönderilen, başarısız ve atlanan mesajlar, her biri kısa
 * bir açıklama ve "ne yapabilirsiniz" önerisiyle. Başarısız ve atlanan satırı yalnızca o
 * mesajlardan varsa çıkar. Sayılar ve nedenler sunucudan gelir; burada yalnızca metne
 * dökülür. Öneriler uygulamanın gerçek davranışına dayanır: kampanya yeniden gönderilemez ve
 * yeni bir kampanya brief'e uyan herkese gider. Gönderilen satırı, varsa yeniden denemeyle
 * toparlanan mesaj sayısını da söyler.
 */
export function completionLines(snapshot: DeliverySnapshot): CompletionLine[] {
  const { messages, failures } = snapshot
  // Yeniden denemeyle toparlananlar gönderilenlerin bir bölümüdür; varsa açıkça söylenir. "İlk
  // denemede başarısız oldu" denmez: ilk deneme kayıp yanıt veya çöken worker yüzünden de
  // tamamlanamamış olabilir (sağlayıcı o isteği kabul etmiş olabilir).
  const recovered = snapshot.recovered ?? 0
  const lines: CompletionLine[] = [
    {
      key: 'sent',
      label: 'Gönderildi',
      count: messages.sent,
      text:
        recovered > 0
          ? `Sağlayıcı kabul etti. ${nf.format(recovered)} mesaj ilk denemede tamamlanamadı, yeniden denemeyle gönderildi.`
          : 'Sağlayıcı kabul etti.',
      advice: [],
    },
  ]
  if (messages.failed > 0) {
    const parts: string[] = []
    const advice: string[] = []
    if (failures.exhausted > 0) {
      parts.push(
        `${nf.format(failures.exhausted)} mesajın sonucu doğrulanamadı (deneme sınırı doldu)`,
      )
      advice.push(
        'Sonucu doğrulanamayan mesajlar teslim edilmiş olabilir; yeniden göndermeden önce sağlayıcıdan teslim durumunu kontrol edin.',
      )
    }
    if (failures.rejected > 0) {
      parts.push(`${nf.format(failures.rejected)} mesaj sağlayıcı tarafından reddedildi`)
      advice.push(
        'Reddedilen mesajlar yeniden denenmez; alıcıların telefon veya e-posta adreslerini kontrol edin.',
      )
    }
    const other = messages.failed - failures.exhausted - failures.rejected
    if (other > 0) parts.push(`${nf.format(other)} mesaj gönderilemedi`)
    advice.push(
      "Bu kampanya yeniden gönderilemez. Yeni bir kampanya brief'e uyan herkese gider, daha önce gönderilenlere de ulaşabilir.",
    )
    lines.push({
      key: 'failed',
      label: 'Başarısız',
      count: messages.failed,
      text: `${parts.join('; ')}.`,
      advice,
    })
  }
  if (messages.skipped > 0) {
    lines.push({
      key: 'skipped',
      label: 'Atlandı',
      count: messages.skipped,
      text: 'Kanal izni olmadığı için gönderilmedi.',
      advice: [
        'Bu alıcıların o kanal için izni gönderim sırasında yoktu. İzinsiz gönderilmez; yapmanız gereken bir şey yok.',
      ],
    })
  }
  return lines
}

/** Sağlayıcının kesin olarak reddettiği mesajlar (örneğin geçersiz alıcı); yeniden denenmez. */
export function rejectedNote(count: number): string {
  return `${nf.format(count)} mesaj sağlayıcı tarafından reddedildi (örneğin geçersiz alıcı) ve yeniden denenmedi.`
}

/** Canlı takip bağlantısı hatasının başlığı. */
export function deliveryErrorTitle(error: ApiError): string {
  switch (error.code) {
    case 'CAMPAIGN_NOT_FOUND':
      return 'Kampanya bulunamadı.'
    case 'NETWORK':
      return 'Sunucuya ulaşılamadı. Bağlantınızı kontrol edin.'
    case 'STREAM_INTERRUPTED':
      return 'Canlı takip bağlantısı koptu.'
    case 'UNEXPECTED_RESPONSE':
      return 'Sunucudan beklenmeyen bir yanıt alındı.'
    case 'INTERNAL':
      return 'Beklenmeyen bir hata oluştu.'
    default:
      return error.message || 'Canlı takip başlatılamadı.'
  }
}
