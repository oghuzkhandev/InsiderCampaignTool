import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DeliveryCounts from '../DeliveryCounts.vue'
import type { DeliverySnapshot } from '@/api/delivery'

function snapshot(overrides: Partial<DeliverySnapshot> = {}): DeliverySnapshot {
  return {
    status: 'sending',
    messages: { queued: 3, retrying: 1, sending: 2, sent: 40, failed: 2, skipped: 1 },
    failures: { exhausted: 0, rejected: 0 },
    ...overrides,
  }
}

const mountCounts = (s: DeliverySnapshot) => mount(DeliveryCounts, { props: { snapshot: s } })
const text = (w: ReturnType<typeof mountCounts>, id: string) =>
  w.get(`[data-testid="${id}"]`).text()

describe('sayılar', () => {
  it('altı durumu ayrı ayrı ve gerçek değerleriyle gösterir', () => {
    const w = mountCounts(snapshot())

    expect(text(w, 'count-queued')).toBe('3')
    expect(text(w, 'count-retrying')).toBe('1')
    expect(text(w, 'count-sending')).toBe('2')
    expect(text(w, 'count-sent')).toBe('40')
    expect(text(w, 'count-failed')).toBe('2')
    expect(text(w, 'count-skipped')).toBe('1')
    expect(text(w, 'total-messages')).toBe('49')
  })

  it('her sayının adını ve kısa açıklamasını gösterir (anlaşılırlık)', () => {
    const w = mountCounts(snapshot())
    const terms = w.findAll('dt').map((t) => t.text())

    expect(terms).toEqual([
      'Bekleyen',
      'Yeniden denenecek',
      'Gönderiliyor',
      'Gönderildi',
      'Başarısız',
      'Atlandı',
    ])
    expect(w.text()).toContain('Geçici hata sonrası sırada bekliyor')
    expect(w.text()).toContain('Kanal izni olmadığı için gönderilmedi')
  })

  it('bin ayracıyla Türkçe biçimler', () => {
    const w = mountCounts(
      snapshot({
        messages: { queued: 0, retrying: 0, sending: 0, sent: 12345, failed: 0, skipped: 0 },
      }),
    )
    expect(text(w, 'count-sent')).toBe('12.345')
    expect(text(w, 'total-messages')).toBe('12.345')
  })

  it('yeni görüntü öncekinin yerini alır (üzerine eklenmez)', async () => {
    const w = mountCounts(snapshot())
    await w.setProps({
      snapshot: snapshot({
        messages: { queued: 0, retrying: 0, sending: 0, sent: 42, failed: 3, skipped: 4 },
      }),
    })

    expect(text(w, 'count-sent')).toBe('42')
    expect(text(w, 'count-queued')).toBe('0')
    expect(text(w, 'total-messages')).toBe('49')
  })

  it('başarısız sayısı sıfırdan büyükse vurgular, sıfırsa vurgulamaz', async () => {
    const w = mountCounts(snapshot())
    expect(w.get('[data-testid="count-failed"]').classes()).toContain('text-destructive')

    await w.setProps({
      snapshot: snapshot({
        messages: { queued: 0, retrying: 0, sending: 0, sent: 5, failed: 0, skipped: 0 },
      }),
    })
    expect(w.get('[data-testid="count-failed"]').classes()).not.toContain('text-destructive')
  })
})

describe('ilerleme', () => {
  it('işi biten mesajların oranını çubuk ve metinle gösterir', () => {
    const w = mountCounts(snapshot()) // 43 / 49 = %87,7 → 87
    const bar = w.get('[role="progressbar"]')

    expect(bar.attributes('aria-valuenow')).toBe('87')
    expect(bar.attributes('aria-label')).toBe('İşi biten mesajların oranı')
    expect(text(w, 'progress-text')).toBe('%87 tamamlandı · 43 / 49 mesajın işi bitti')
  })

  it('hepsi bittiğinde %100, hiçbiri bitmediğinde %0', async () => {
    const w = mountCounts(
      snapshot({
        status: 'completed',
        messages: { queued: 0, retrying: 0, sending: 0, sent: 9, failed: 1, skipped: 0 },
      }),
    )
    expect(w.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('100')

    await w.setProps({
      snapshot: snapshot({
        messages: { queued: 5, retrying: 0, sending: 0, sent: 0, failed: 0, skipped: 0 },
      }),
    })
    expect(w.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('0')
  })
})

describe('başarısızlık nedenleri', () => {
  it('neden yoksa not göstermez', () => {
    const w = mountCounts(snapshot())
    expect(w.find('[data-testid="note-exhausted"]').exists()).toBe(false)
    expect(w.find('[data-testid="note-rejected"]').exists()).toBe(false)
  })

  it('deneme sınırı dolanlar için sonucun doğrulanamadığını, kesin teslim edilmediği anlamına gelmediğini söyler', () => {
    const w = mountCounts(snapshot({ failures: { exhausted: 2, rejected: 0 } }))
    const note = w.get('[data-testid="note-exhausted"]')

    expect(note.text()).toContain('2 mesaj: Gönderim sonucu doğrulanamadı; deneme sınırı doldu')
    expect(note.text()).toContain('kesinlikle teslim edilmediği anlamına gelmez')
    expect(w.find('[data-testid="note-rejected"]').exists()).toBe(false)
  })

  it('reddedilenleri ayrı bir notla gösterir', () => {
    const w = mountCounts(snapshot({ failures: { exhausted: 1, rejected: 1 } }))

    expect(w.find('[data-testid="note-exhausted"]').exists()).toBe(true)
    expect(w.get('[data-testid="note-rejected"]').text()).toContain(
      '1 mesaj sağlayıcı tarafından reddedildi',
    )
  })

  it('notlar canlı bölge (alert) değildir: saniyelik güncellemeler ekran okuyucuyu bunaltmaz', () => {
    const w = mountCounts(snapshot({ failures: { exhausted: 1, rejected: 1 } }))

    expect(w.get('[data-testid="note-exhausted"]').attributes('role')).toBe('note')
    expect(w.get('[data-testid="note-rejected"]').attributes('role')).toBe('note')
    expect(w.find('[role="alert"]').exists()).toBe(false)
  })

  it('nedenler yeni görüntüyle kaybolabilir', async () => {
    const w = mountCounts(snapshot({ failures: { exhausted: 2, rejected: 0 } }))
    await w.setProps({ snapshot: snapshot() })
    expect(w.find('[data-testid="note-exhausted"]').exists()).toBe(false)
  })
})

describe('erişilebilirlik', () => {
  it('bölüm başlığıyla adlandırılmıştır ve sayılar tanım listesindedir', () => {
    const w = mountCounts(snapshot())
    const region = w.get('[role="region"]')

    expect(region.attributes('aria-labelledby')).toBe('delivery-counts-title')
    expect(w.get('h2#delivery-counts-title').text()).toBe('Mesaj durumu')
    expect(w.findAll('dl dt')).toHaveLength(6)
  })
})
