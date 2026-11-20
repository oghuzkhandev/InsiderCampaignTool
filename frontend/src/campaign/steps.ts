// Kampanya akışının dört adımı ve bir kampanyanın hangi adımda olduğu. Adımlar yalnızca
// bilgi içindir (tıklanamaz); durum, ekranların zaten bildiği verilerden türetilir.

export type StepKey = 'audience' | 'content' | 'approval' | 'delivery'
export type StepStatus = 'done' | 'current' | 'upcoming'

export const STEPS: readonly { key: StepKey; label: string }[] = [
  { key: 'audience', label: 'Kitle' },
  { key: 'content', label: 'İçerik' },
  { key: 'approval', label: 'Onay' },
  { key: 'delivery', label: 'Gönderim' },
]

export interface StepInput {
  /** Kampanya durumu; kampanya henüz oluşmadıysa (veya durum bilinmiyorsa) null. */
  status: string | null
  /**
   * Kaydedilmiş (doğrulanmış) bir içerik sürümü var ve düzenleyicide kaydedilmemiş
   * değişiklik yok. Yalnızca taslak kampanyada anlamlıdır.
   */
  contentReady: boolean
}

/**
 * - kampanya yok: kitle adımı sürüyor;
 * - taslak: kitle bitti; içerik hazır değilse içerik, hazırsa onay sürüyor;
 * - onaylandı veya gönderiliyor: gönderim sürüyor, ilk üçü bitti;
 * - tamamlandı: dördü de bitti.
 */
export function stepStatuses({ status, contentReady }: StepInput): Record<StepKey, StepStatus> {
  switch (status) {
    case 'draft':
      return contentReady
        ? { audience: 'done', content: 'done', approval: 'current', delivery: 'upcoming' }
        : { audience: 'done', content: 'current', approval: 'upcoming', delivery: 'upcoming' }
    case 'approved':
    case 'sending':
      return { audience: 'done', content: 'done', approval: 'done', delivery: 'current' }
    case 'completed':
      return { audience: 'done', content: 'done', approval: 'done', delivery: 'done' }
    default:
      return {
        audience: 'current',
        content: 'upcoming',
        approval: 'upcoming',
        delivery: 'upcoming',
      }
  }
}

/** Ekran okuyucu için adımın durumu. */
export function stepStatusText(status: StepStatus): string {
  switch (status) {
    case 'done':
      return 'tamamlandı'
    case 'current':
      return 'şu anki adım'
    default:
      return 'bekliyor'
  }
}
