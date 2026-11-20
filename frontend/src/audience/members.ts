// "Segmente uyan kişiler" listesinin saf mantığı ve Türkçe metinleri (ADR-031). Sunucudan gelen
// serbest metin yorumlanmaz; yalnızca durum kodlarına karşılık gelen sabit açıklamalar
// kullanılır.
import type { ChannelStatus } from '@/api/audience'

export type Channel = 'sms' | 'email'

/** Eksik bilginin listede açıkça yazılan hâli: boş hücre veya sahte değer gösterilmez. */
export const MISSING = {
  name: 'Ad yok',
  email: 'E-posta yok',
  phone: 'Telefon yok',
} as const

/** Sayfa sayısı: en az 1 (hiç kişi olmasa da "Sayfa 1 / 1" demek yanıltmaz, liste yerine boş durum çıkar). */
export function totalPages(total: number, limit: number): number {
  if (limit < 1) return 1
  return Math.max(1, Math.ceil(total / limit))
}

/** Sayfadaki kişilerin sıra aralığı (1'den başlar). Sayfa boşsa null. */
export function pageRange(
  page: number,
  limit: number,
  count: number,
): { from: number; to: number } | null {
  if (count < 1) return null
  const from = (page - 1) * limit + 1
  return { from, to: from + count - 1 }
}

/** Rozetteki kısa etiket. */
export function channelStatusLabel(status: ChannelStatus): string {
  switch (status) {
    case 'eligible':
      return 'Uygun'
    case 'no_consent':
      return 'İzin yok'
    case 'no_address':
      return 'Adres yok'
  }
}

/** Rozetin ipucu ve ekran okuyucu açıklaması: hangi kanalda neden uygun ya da değil. */
export function channelStatusHint(channel: Channel, status: ChannelStatus): string {
  if (channel === 'sms') {
    switch (status) {
      case 'eligible':
        return "SMS'e uygun: açık izin var ve telefon numarası kayıtlı."
      case 'no_consent':
        return 'SMS için uygun değil: açık izin yok. Bu kişiye SMS gönderilmez.'
      case 'no_address':
        return 'SMS için uygun değil: izin var ama telefon numarası yok.'
    }
  }
  switch (status) {
    case 'eligible':
      return 'E-postaya uygun: açık izin var ve e-posta adresi kayıtlı.'
    case 'no_consent':
      return 'E-posta için uygun değil: açık izin yok. Bu kişiye e-posta gönderilmez.'
    case 'no_address':
      return 'E-posta için uygun değil: izin var ama e-posta adresi yok.'
  }
}

/** Listenin altındaki not: demo verisi ve kesin alıcıların onayda belirlendiği. */
export const AUDIENCE_LIST_NOTE =
  'Bu liste demo verisidir (sahte kullanıcılar). Kesin alıcılar kampanya onaylanırken yeniden ' +
  'belirlenir. İzni olmayan kişiler de segmente uyuyorsa listelenir, ancak ilgili kanal için uygun ' +
  'değildir ve o kanaldan mesaj gönderilmez.'
