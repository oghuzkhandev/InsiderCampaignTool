/** Bildirim türü: başarı, uyarı, hata ve bilgi. */
export type NotifyKind = 'success' | 'warning' | 'error' | 'info'

/** Bir bildirimin ekranda kalma süresi (ms). Süre boyunca geri sayım çubuğu azalır. */
export const NOTIFY_DURATION_MS = 8_000

/** Geri sayımın tazelenme aralığı (ms). */
export const NOTIFY_TICK_MS = 100
