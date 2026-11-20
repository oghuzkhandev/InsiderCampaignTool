// Zamanlanmış gönderimlerin zaman mantığı (saf işlevler). Zamanlar UTC olarak gelir ve kullanıcının
// cihazının yerel saatinde gösterilir; elle saat farkı hesaplanmaz (Intl yapar). Kalan süre, cihaz
// saati yanlış olsa da doğru kalsın diye SUNUCU saatine göre hesaplanır.

/** Sunucu saatinin istemci saatine göre farkı (ms): sunucu saati ≈ istemci saati + fark. */
export function clockOffset(serverTime: string, clientNow: number): number {
  const server = Date.parse(serverTime)
  return Number.isNaN(server) ? 0 : server - clientNow
}

/** Verilen ana kaç ms kaldı (sunucu saatine göre); geçmişse sıfır veya negatif. */
export function msUntil(iso: string, clientNow: number, offset: number): number {
  return Date.parse(iso) - (clientNow + offset)
}

/**
 * Kalan süre kısa Türkçe metin olarak: "1 dakikadan az", "12 dk", "3 sa 20 dk", "2 gün 4 sa".
 * Saniye gösterilmez (sayaç dakikada bir güncellenir). Sıfır veya negatif süre için boş metin.
 */
export function formatRemaining(ms: number): string {
  if (ms <= 0) return ''
  const minutes = Math.floor(ms / 60_000)
  if (minutes < 1) return '1 dakikadan az'
  if (minutes < 60) return `${minutes} dk`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    const rest = minutes % 60
    return rest === 0 ? `${hours} sa` : `${hours} sa ${rest} dk`
  }
  const days = Math.floor(hours / 24)
  const restHours = hours % 24
  return restHours === 0 ? `${days} gün` : `${days} gün ${restHours} sa`
}

/**
 * Planlanan zamanın yerel gösterimi, saat dilimiyle birlikte ("21 Kas 2026 14:30 GMT+3").
 * `timeZone` verilmezse cihazın saat dilimi kullanılır.
 */
export function startTimeText(iso: string, timeZone?: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  // `dateStyle`/`timeStyle` ile `timeZoneName` birlikte kullanılamaz (TypeError); alanlar tek tek verilir.
  return new Intl.DateTimeFormat('tr-TR', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    timeZoneName: 'short',
    ...(timeZone ? { timeZone } : {}),
  }).format(date)
}
