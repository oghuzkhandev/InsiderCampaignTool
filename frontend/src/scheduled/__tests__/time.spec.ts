import { describe, expect, it } from 'vitest'
import { clockOffset, formatRemaining, msUntil, startTimeText } from '../time'

const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR

describe('clockOffset ve msUntil (sunucu saatine göre)', () => {
  it('sunucu ileride ise fark pozitiftir', () => {
    const client = Date.parse('2026-11-21T09:00:00Z')

    expect(clockOffset('2026-11-21T09:05:00Z', client)).toBe(5 * MIN)
    expect(clockOffset('2026-11-21T08:55:00Z', client)).toBe(-5 * MIN)
    expect(clockOffset('2026-11-21T09:00:00Z', client)).toBe(0)
  })

  it('geçersiz sunucu saatinde farkı sıfır sayar (sayaç yine çalışır)', () => {
    expect(clockOffset('bozuk', Date.now())).toBe(0)
  })

  it('cihaz saati yanlış olsa da kalan süre sunucu saatine göre doğrudur', () => {
    const server = '2026-11-21T09:00:00Z'
    const deviceNow = Date.parse('2026-01-01T00:00:00Z') // cihaz saati aylarca geride
    const offset = clockOffset(server, deviceNow)

    // Planlanan zaman sunucudan 90 dk sonra: cihaz yanlış olsa da 90 dk kalır.
    expect(msUntil('2026-11-21T10:30:00Z', deviceNow, offset)).toBe(90 * MIN)
    // Saat ilerledikçe (cihaz saati +10 dk) kalan süre azalır.
    expect(msUntil('2026-11-21T10:30:00Z', deviceNow + 10 * MIN, offset)).toBe(80 * MIN)
  })

  it('geçmiş zaman için negatif döner', () => {
    expect(msUntil('2026-11-21T08:59:00Z', Date.parse('2026-11-21T09:00:00Z'), 0)).toBe(-MIN)
  })
})

describe('formatRemaining', () => {
  it.each([
    [0, ''],
    [-5 * MIN, ''],
    [1, '1 dakikadan az'],
    [30_000, '1 dakikadan az'],
    [MIN - 1, '1 dakikadan az'],
    [MIN, '1 dk'],
    [12 * MIN + 59_000, '12 dk'],
    [59 * MIN, '59 dk'],
    [HOUR, '1 sa'],
    [3 * HOUR + 20 * MIN, '3 sa 20 dk'],
    [23 * HOUR + 59 * MIN, '23 sa 59 dk'],
    [DAY, '1 gün'],
    [2 * DAY + 4 * HOUR, '2 gün 4 sa'],
    [30 * DAY, '30 gün'],
  ])('%i ms → "%s"', (ms, want) => {
    expect(formatRemaining(ms)).toBe(want)
  })
})

describe('startTimeText (UTC → yerel saat, Intl ile)', () => {
  it('İstanbul (GMT+3) için yerel saati ve saat dilimini yazar', () => {
    const text = startTimeText('2026-11-21T11:30:00Z', 'Europe/Istanbul')

    expect(text).toContain('14:30')
    expect(text).toMatch(/\+3|\+03/)
    expect(text).toContain('2026')
  })

  it('aynı an farklı saat dilimlerinde farklı yerel saat gösterir', () => {
    const iso = '2026-11-21T11:30:00Z'

    expect(startTimeText(iso, 'UTC')).toContain('11:30')
    expect(startTimeText(iso, 'Asia/Tokyo')).toContain('20:30')
  })

  it('yaz saati geçişini elle hesaplamadan doğru çevirir (New York, 8 Mart 2026)', () => {
    // 07:00 UTC'de saatler 02:00'dan 03:00'a atlar: 06:30Z → 01:30 EST, 07:30Z → 03:30 EDT.
    expect(startTimeText('2026-03-08T06:30:00Z', 'America/New_York')).toContain('01:30')
    expect(startTimeText('2026-03-08T07:30:00Z', 'America/New_York')).toContain('03:30')
  })

  it('geçersiz tarihte boş metin döner', () => {
    expect(startTimeText('bozuk')).toBe('')
  })

  it('saat dilimi verilmezse cihazın saat dilimini kullanır (hata vermez, saat ve dilim içerir)', () => {
    const text = startTimeText('2026-11-21T11:30:00Z')

    expect(text).toMatch(/\d{2}:\d{2}/)
    expect(text.length).toBeGreaterThan(10)
  })
})
