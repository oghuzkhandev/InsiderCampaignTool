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
    const terms = w.findAll('[data-testid="count-details"] dt').map((t) => t.text())

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

describe('özet sayılar', () => {
  it('üstte gönderildi, başarısız ve tamamlanmamış sayılarını gösterir', () => {
    const w = mountCounts(snapshot()) // bekleyen 3 + yeniden 1 + gönderiliyor 2 = 6 tamamlanmamış
    const terms = w.findAll('[data-testid="delivery-summary"] dt').map((t) => t.text())

    expect(terms).toEqual(['Gönderildi', 'Başarısız', 'Tamamlanmamış'])
    expect(text(w, 'summary-sent')).toBe('40')
    expect(text(w, 'summary-failed')).toBe('2')
    expect(text(w, 'summary-unfinished')).toBe('6')
  })

  it('tamamlanmamış sayısı bekleyenleri, yeniden denenecekleri ve gönderilenleri toplar; atlananı saymaz', () => {
    const w = mountCounts(
      snapshot({
        messages: { queued: 1, retrying: 10, sending: 100, sent: 0, failed: 0, skipped: 1000 },
      }),
    )
    expect(text(w, 'summary-unfinished')).toBe('111')
  })

  it('özet sayılar ile ayrıntı aynı görüntüden gelir (yeni görüntü ikisini de değiştirir)', async () => {
    const w = mountCounts(snapshot())
    await w.setProps({
      snapshot: snapshot({
        messages: { queued: 0, retrying: 0, sending: 0, sent: 49, failed: 0, skipped: 0 },
      }),
    })
    expect(text(w, 'summary-sent')).toBe('49')
    expect(text(w, 'count-sent')).toBe('49')
    expect(text(w, 'summary-unfinished')).toBe('0')
  })

  it('başarısız özet sayısı sıfırdan büyükse vurgulanır, sıfırsa vurgulanmaz', async () => {
    const w = mountCounts(snapshot())
    expect(w.get('[data-testid="summary-failed"]').classes()).toContain('text-destructive')

    await w.setProps({
      snapshot: snapshot({
        messages: { queued: 0, retrying: 0, sending: 0, sent: 5, failed: 0, skipped: 0 },
      }),
    })
    expect(w.get('[data-testid="summary-failed"]').classes()).not.toContain('text-destructive')
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
    // Üç özet sayı ve altı ayrıntı sayısı, ikisi de tanım listesinde.
    expect(w.findAll('[data-testid="delivery-summary"] dt')).toHaveLength(3)
    expect(w.findAll('[data-testid="count-details"] dt')).toHaveLength(6)
  })
})

describe('kanal dağılımı', () => {
  const zero = { queued: 0, retrying: 0, sending: 0, sent: 0, failed: 0, skipped: 0 }
  // sms + email = snapshot() içindeki toplam (3, 1, 2, 40, 2, 1)
  const sms = { queued: 1, retrying: 1, sending: 1, sent: 25, failed: 1, skipped: 0 }
  const email = { queued: 2, retrying: 0, sending: 1, sent: 15, failed: 1, skipped: 1 }
  const withChannels = (extra: Partial<DeliverySnapshot> = {}) =>
    snapshot({ channels: { sms, email }, ...extra })

  it('SMS ve e-postayı ayrı ayrı, gerçek sayılarıyla gösterir', () => {
    const w = mountCounts(withChannels())

    expect(w.get('h3').text()).toBe('Kanal dağılımı')
    expect(text(w, 'channel-sms-total')).toBe('29 mesaj') // 1+1+1+25+1+0
    expect(text(w, 'channel-email-total')).toBe('20 mesaj') // 2+0+1+15+1+1
    expect(text(w, 'channel-sms-sent')).toBe('25')
    expect(text(w, 'channel-sms-failed')).toBe('1')
    expect(text(w, 'channel-sms-unfinished')).toBe('3') // bekleyen 1 + yeniden 1 + gönderiliyor 1
    expect(text(w, 'channel-email-sent')).toBe('15')
    expect(text(w, 'channel-email-unfinished')).toBe('3') // 2 + 0 + 1
  })

  it('her kanalın kendi ilerlemesini (işi biten oranı) gösterir', () => {
    const w = mountCounts(withChannels())
    const bars = w.findAll('[data-testid="channel-split"] [role="progressbar"]')

    expect(bars.map((b) => b.attributes('aria-valuenow'))).toEqual(['89', '85']) // 26/29, 17/20
    expect(bars.map((b) => b.attributes('aria-label'))).toEqual([
      'SMS: işi biten mesajların oranı',
      'E-posta: işi biten mesajların oranı',
    ])
  })

  it('atlananı yalnızca varsa gösterir', () => {
    const w = mountCounts(withChannels())
    expect(w.find('[data-testid="channel-sms-skipped"]').exists()).toBe(false) // SMS'te 0
    expect(text(w, 'channel-email-skipped')).toBe('Atlandı: 1')
  })

  it('başarısızı yalnızca sıfırdan büyükse vurgular', () => {
    const w = mountCounts(withChannels({ channels: { sms, email: { ...email, failed: 0 } } }))
    expect(w.get('[data-testid="channel-sms-failed"]').classes()).toContain('text-destructive')
    expect(w.get('[data-testid="channel-email-failed"]').classes()).not.toContain(
      'text-destructive',
    )
  })

  it('mesajı olmayan kanal için "Bu kanalda mesaj yok" der ve ilerleme göstermez', () => {
    const all = snapshot().messages
    const w = mountCounts(snapshot({ channels: { sms: all, email: zero } }))

    expect(text(w, 'channel-email-empty')).toBe('Bu kanalda mesaj yok.')
    expect(text(w, 'channel-email-total')).toBe('0 mesaj')
    expect(w.find('[data-testid="channel-email-sent"]').exists()).toBe(false)
    expect(w.findAll('[data-testid="channel-split"] [role="progressbar"]')).toHaveLength(1)
  })

  it('kanal kırılımı yoksa (eski sunucu) bölüm hiç görünmez, diğer sayılar yerinde kalır', () => {
    const w = mountCounts(snapshot())

    expect(w.find('[data-testid="channel-split"]').exists()).toBe(false)
    expect(text(w, 'count-sent')).toBe('40')
  })

  it('yeni görüntü kanal sayılarını değiştirir (üzerine eklemez)', async () => {
    const w = mountCounts(withChannels())
    await w.setProps({
      snapshot: withChannels({
        channels: { sms: { ...sms, sent: 26, queued: 0 }, email },
        messages: { queued: 2, retrying: 1, sending: 2, sent: 41, failed: 2, skipped: 1 },
      }),
    })
    expect(text(w, 'channel-sms-sent')).toBe('26')
    expect(text(w, 'channel-sms-total')).toBe('29 mesaj')
  })

  it('her kanal adlandırılmış bir bölgedir', () => {
    const w = mountCounts(withChannels())
    expect(w.get('[data-testid="channel-sms"]').attributes('aria-label')).toBe(
      'SMS gönderim durumu',
    )
    expect(w.get('[data-testid="channel-email"]').attributes('aria-label')).toBe(
      'E-posta gönderim durumu',
    )
  })
})

describe('yeniden denemeyle toparlananlar', () => {
  it('toparlanan varsa sayıyı ve ne demek olduğunu gösterir', () => {
    const w = mountCounts(snapshot({ recovered: 12 }))

    expect(text(w, 'recovered-count')).toBe('12')
    const note = text(w, 'recovered-note')
    expect(note).toContain('Yeniden denemeyle toparlanan')
    expect(note).toContain('ilk denemede tamamlanamadı, yeniden denemeyle gönderildi')
    // İlk denemenin başarısız olduğu söylenmez: kayıp yanıt veya çöken worker da olabilir.
    expect(note).not.toMatch(/başarısız|gönderilemedi/)
  })

  it('sıfırsa veya alan yoksa göstermez', () => {
    expect(
      mountCounts(snapshot({ recovered: 0 }))
        .find('[data-testid="recovered-note"]')
        .exists(),
    ).toBe(false)
    expect(mountCounts(snapshot()).find('[data-testid="recovered-note"]').exists()).toBe(false)
  })

  it('yeni görüntü sayıyı değiştirir', async () => {
    const w = mountCounts(snapshot({ recovered: 2 }))
    await w.setProps({ snapshot: snapshot({ recovered: 5 }) })
    expect(text(w, 'recovered-count')).toBe('5')
  })

  it('toparlananlar gönderilenlere ikinci kez eklenmez (gönderilen sayısı değişmez)', () => {
    const w = mountCounts(snapshot({ recovered: 12 }))
    expect(text(w, 'count-sent')).toBe('40')
    expect(text(w, 'summary-sent')).toBe('40')
  })
})
