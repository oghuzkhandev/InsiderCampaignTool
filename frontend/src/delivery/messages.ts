// Gönderim takibi ekranının Türkçe metinleri. Sunucudan gelen serbest metin
// gösterilmez; yalnızca durum ve neden kodlarına karşılık gelen sabit açıklamalar
// ve sayılar kullanılır (content/messages.ts ile aynı yaklaşım).
import type { ApiError } from '@/api/campaigns'
import type { DeliveryCounts } from '@/api/delivery'
import type { BadgeVariants } from '@/components/ui/badge'

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

export function campaignStatusVariant(status: string): NonNullable<BadgeVariants['variant']> {
  switch (status) {
    case 'sending':
      return 'default'
    case 'approved':
      return 'secondary'
    default:
      return 'outline'
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
