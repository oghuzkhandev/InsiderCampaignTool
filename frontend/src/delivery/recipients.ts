// Alıcı listesinin Türkçe metinleri ve küçük yardımcıları. Sunucudan gelen serbest metin
// gösterilmez; yalnızca durum ve neden kodlarına karşılık gelen sabit açıklamalar kullanılır
// (delivery/messages.ts ile aynı yaklaşım).
import type { ChannelKey } from '@/api/delivery'
import { MESSAGE_STATUSES, type MessageStatus } from '@/api/recipients'

export const CHANNEL_LABELS: Record<ChannelKey, string> = { sms: 'SMS', email: 'E-posta' }

/** Mesaj durumunun adı. Denenmiş ve yeniden sırada bekleyen mesaj "Yeniden denenecek"tir. */
export function messageStatusLabel(status: MessageStatus, attempts = 0): string {
  switch (status) {
    case 'queued':
      return attempts > 0 ? 'Yeniden denenecek' : 'Bekleyen'
    case 'sending':
      return 'Gönderiliyor'
    case 'sent':
      return 'Gönderildi'
    case 'failed':
      return 'Başarısız'
    case 'skipped':
      return 'Atlandı'
  }
}

/** Durum süzgecinin seçenekleri, ekrandaki sırayla. */
export const MESSAGE_STATUS_OPTIONS: { value: MessageStatus; label: string }[] =
  MESSAGE_STATUSES.map((value) => ({ value, label: messageStatusLabel(value) }))

export const CHANNEL_OPTIONS: { value: ChannelKey; label: string }[] = [
  { value: 'sms', label: CHANNEL_LABELS.sms },
  { value: 'email', label: CHANNEL_LABELS.email },
]

/** Başarısızlık nedeni kodunun açıklaması; bilinmeyen kod için null (arayüz bir şey göstermez). */
export function failureReasonLabel(reason: string | null): string | null {
  switch (reason) {
    case 'exhausted':
      return 'Deneme sınırı doldu'
    case 'rejected':
      return 'Sağlayıcı reddetti'
    default:
      return null
  }
}

/** Adın ilk harfi (Türkçe büyük harfle); ad yoksa adresin ilk harfi, o da yoksa "?". */
export function recipientInitial(firstName: string | null, address: string): string {
  const source = (firstName?.trim() || address.trim()).charAt(0)
  return source ? source.toLocaleUpperCase('tr-TR') : '?'
}

const sentFormat = new Intl.DateTimeFormat('tr-TR', {
  day: 'numeric',
  month: 'short',
  hour: '2-digit',
  minute: '2-digit',
})

/** "20 Kas 09:54" (cihazın saat dilimiyle); geçersiz değerde null. */
export function formatSentAt(iso: string | null): string | null {
  if (!iso) return null
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? null : sentFormat.format(date)
}
