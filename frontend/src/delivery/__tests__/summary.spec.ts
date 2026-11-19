import { describe, expect, it } from 'vitest'
import type { DeliveryCounts } from '@/api/delivery'
import { finishedMessages, finishedPercent, totalMessages } from '../summary'

const counts = (overrides: Partial<DeliveryCounts> = {}): DeliveryCounts => ({
  queued: 0,
  retrying: 0,
  sending: 0,
  sent: 0,
  failed: 0,
  skipped: 0,
  ...overrides,
})

describe('totalMessages', () => {
  it('bütün durumları toplar; yeniden denenecekler bekleyenlere ikinci kez eklenmez', () => {
    expect(
      totalMessages(
        counts({ queued: 3, retrying: 1, sending: 2, sent: 40, failed: 2, skipped: 1 }),
      ),
    ).toBe(49)
  })

  it('mesaj yoksa 0', () => {
    expect(totalMessages(counts())).toBe(0)
  })
})

describe('finishedMessages', () => {
  it('yalnızca terminal durumları sayar: gönderilen, başarısız, atlanan', () => {
    expect(
      finishedMessages(
        counts({ queued: 3, retrying: 1, sending: 2, sent: 40, failed: 2, skipped: 1 }),
      ),
    ).toBe(43)
  })
})

describe('finishedPercent', () => {
  it('mesaj yoksa 0 (sıfıra bölme yok)', () => {
    expect(finishedPercent(counts())).toBe(0)
  })

  it('oranı aşağı yuvarlar: yüzde 100 yalnızca hepsi bittiğinde görünür', () => {
    expect(finishedPercent(counts({ sent: 199, queued: 1 }))).toBe(99) // 99,5
    expect(finishedPercent(counts({ sent: 1, queued: 2 }))).toBe(33)
    expect(finishedPercent(counts({ sent: 200 }))).toBe(100)
  })

  it('başarısız ve atlanan mesajlar da işi bitmiş sayılır', () => {
    expect(finishedPercent(counts({ sent: 1, failed: 1, skipped: 1, retrying: 1 }))).toBe(75)
    expect(finishedPercent(counts({ failed: 5 }))).toBe(100) // hepsi başarısız olsa da tamam
  })

  it('henüz hiçbiri bitmediyse 0', () => {
    expect(finishedPercent(counts({ queued: 5, sending: 2 }))).toBe(0)
  })
})
