export type ScheduleResult = { ok: true; utc: string } | { ok: false; message: string }

/** datetime-local is in the browser's time zone; never append Z to local input. */
export function localScheduleToUTC(input: string, now = Date.now()): ScheduleResult {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(input))
    return { ok: false, message: 'Geçerli bir tarih ve saat seçin.' }
  const at = new Date(input)
  if (!Number.isFinite(at.getTime()) || localDateTime(at) !== input)
    return { ok: false, message: 'Bu tarih ve saat cihazınızın saat diliminde geçerli değil.' }
  if (at.getTime() <= now) return { ok: false, message: 'Gönderim zamanı gelecekte olmalı.' }
  return { ok: true, utc: at.toISOString() }
}

export function localDateTime(at: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${String(at.getFullYear()).padStart(4, '0')}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}T${pad(at.getHours())}:${pad(at.getMinutes())}`
}

export function isScheduledTimestamp(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    /^\d{4}-\d{2}-\d{2}T.*(?:Z|[+-]\d{2}:\d{2})$/.test(value) &&
    Number.isFinite(Date.parse(value))
  )
}

export function formatScheduledTime(value: string): string {
  return new Intl.DateTimeFormat('tr-TR', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZoneName: 'shortOffset',
  }).format(new Date(value))
}

export function deviceTimeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone
}
