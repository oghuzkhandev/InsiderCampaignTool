// Uygulama bildirimleri: her eylemin sonucu için başarı, uyarı, hata veya bilgi kartı. Ekranın
// üstünde görünür, 12 sn kalır ve kalan süreyi gösterir (NotificationToast). Kullanım:
//
//   notify.success('Şablon kaydedildi', 'Kampanya içerik ekranında kullanabilirsiniz.')
//   notify.error('İçerik kaydedilemedi', 'Sunucuya ulaşılamadı.')
//
// Kartlar `vue-sonner` ile gösterilir; kök bileşende `Sonner` (ui/sonner) bağlı olmalıdır.
import { markRaw } from 'vue'
import { toast } from 'vue-sonner'
import NotificationToast from './NotificationToast.vue'
import { NOTIFY_DURATION_MS, type NotifyKind } from './types'

export { NOTIFY_DURATION_MS, type NotifyKind } from './types'

export interface NotifyOptions {
  /** Ekranda kalma süresi (ms); verilmezse 12 sn. */
  durationMs?: number
}

let counter = 0

function show(
  kind: NotifyKind,
  title: string,
  description?: string,
  options: NotifyOptions = {},
): string {
  const id = `notify-${++counter}`
  toast.custom(markRaw(NotificationToast), {
    id,
    // Ömrü kartın kendisi yönetir (geri sayım ve duraklatma); sonner kendiliğinden kapatmaz.
    duration: Number.POSITIVE_INFINITY,
    componentProps: {
      id,
      kind,
      title,
      description,
      durationMs: options.durationMs ?? NOTIFY_DURATION_MS,
    },
  })
  return id
}

export const notify = {
  success: (title: string, description?: string, options?: NotifyOptions) =>
    show('success', title, description, options),
  warning: (title: string, description?: string, options?: NotifyOptions) =>
    show('warning', title, description, options),
  error: (title: string, description?: string, options?: NotifyOptions) =>
    show('error', title, description, options),
  info: (title: string, description?: string, options?: NotifyOptions) =>
    show('info', title, description, options),
  /** Bir bildirimi (kimliğiyle) hemen kapatır. */
  dismiss: (id: string | number) => toast.dismiss(id),
}
