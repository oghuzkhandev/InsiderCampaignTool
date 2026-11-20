import { describe, expect, it } from 'vitest'
import { isScheduledTimestamp, localDateTime, localScheduleToUTC } from './schedule'

describe('gönderim zamanı', () => {
  it('yerel saati tarayıcının saat dilimiyle UTC’ye çevirir', () => {
    const at = new Date(2030, 5, 15, 14, 30)
    expect(localScheduleToUTC(localDateTime(at), at.getTime() - 60_000)).toEqual({
      ok: true,
      utc: at.toISOString(),
    })
  })
  it('geçmişi, aynı anı, boş ve takvimde olmayan zamanı reddeder', () => {
    const at = new Date(2030, 5, 15, 14, 30)
    for (const input of [
      '',
      '2030-02-30T14:00',
      '2030-06-15',
      '2030-13-40T25:99',
      localDateTime(at),
    ]) {
      expect(localScheduleToUTC(input, at.getTime()).ok).toBe(false)
    }
  })
  it('API zamanında açık saat dilimi ister', () => {
    expect(isScheduledTimestamp('2030-06-15T11:30:00Z')).toBe(true)
    expect(isScheduledTimestamp('2030-06-15T14:30:00+03:00')).toBe(true)
    expect(isScheduledTimestamp('2030-06-15T14:30:00')).toBe(false)
    expect(isScheduledTimestamp('bad')).toBe(false)
    expect(isScheduledTimestamp(null)).toBe(false)
  })
})
