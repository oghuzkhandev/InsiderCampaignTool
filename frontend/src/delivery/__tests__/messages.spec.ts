import { describe, expect, it } from 'vitest'
import { ApiError } from '@/api/campaigns'
import {
  campaignStatusLabel,
  COUNT_ITEMS,
  completionHeadline,
  completionLines,
  deliveryErrorTitle,
  exhaustedNote,
  rejectedNote,
} from '../messages'
import type { DeliverySnapshot } from '@/api/delivery'

const done = (
  messages: Partial<DeliverySnapshot['messages']>,
  failures: Partial<DeliverySnapshot['failures']> = {},
): DeliverySnapshot => ({
  status: 'completed',
  messages: { queued: 0, retrying: 0, sending: 0, sent: 0, failed: 0, skipped: 0, ...messages },
  failures: { exhausted: 0, rejected: 0, ...failures },
})

describe('completionHeadline', () => {
  const headline = (m: Partial<DeliverySnapshot['messages']>) =>
    completionHeadline(done(m).messages)

  it("örnek: 7 mesajın 5'i gönderildi, 1'i başarısız, 1'i atlandı", () => {
    expect(headline({ sent: 5, failed: 1, skipped: 1 })).toBe(
      "7 mesajın 5'i gönderildi, 1'i başarısız, 1'i atlandı.",
    )
  })

  it('sayılara okunuşlarına göre doğru ek koyar (yedisi, üçü, altısı...)', () => {
    expect(headline({ sent: 7, failed: 3 })).toBe("10 mesajın 7'si gönderildi, 3'ü başarısız.")
    expect(headline({ sent: 6, skipped: 4 })).toBe("10 mesajın 6'sı gönderildi, 4'ü atlandı.")
  })

  it('yalnızca sıfırdan büyük sonuçları yazar', () => {
    expect(headline({ sent: 5, failed: 2 })).toBe("7 mesajın 5'i gönderildi, 2'si başarısız.")
    expect(headline({ sent: 6, skipped: 1 })).toBe("7 mesajın 6'sı gönderildi, 1'i atlandı.")
    expect(headline({ failed: 2, skipped: 5 })).toBe("7 mesajın 2'si başarısız, 5'i atlandı.")
  })

  it('hepsi tek sonuçsa "tamamı" der', () => {
    expect(headline({ sent: 7 })).toBe('7 mesajın tamamı gönderildi.')
    expect(headline({ failed: 7 })).toBe('7 mesajın tamamı başarısız oldu.')
    expect(headline({ skipped: 7 })).toBe('7 mesajın tamamı atlandı.')
  })

  it('binlik ayraç kullanır', () => {
    expect(headline({ sent: 1234, failed: 6 })).toBe(
      "1.240 mesajın 1.234'ü gönderildi, 6'sı başarısız.",
    )
  })

  it('hiç mesaj yoksa "bütün mesajlar gönderildi" demez', () => {
    expect(headline({})).toBe('Gönderilecek mesaj yoktu.')
  })
})

describe('completionLines', () => {
  it('yalnızca gönderildiyse tek satır verir', () => {
    expect(completionLines(done({ sent: 7 }))).toEqual([
      { key: 'sent', label: 'Gönderildi', count: 7, text: 'Sağlayıcı kabul etti.', advice: [] },
    ])
  })

  it('başarısız ve atlananı nedenleriyle, kısa açıklamayla verir', () => {
    const lines = completionLines(
      done({ sent: 5, failed: 3, skipped: 1 }, { exhausted: 2, rejected: 1 }),
    )
    expect(lines.map((l) => [l.key, l.count])).toEqual([
      ['sent', 5],
      ['failed', 3],
      ['skipped', 1],
    ])
    const failed = lines[1]?.text ?? ''
    expect(failed).toContain('2 mesajın sonucu doğrulanamadı (deneme sınırı doldu)')
    expect(failed).toContain('1 mesaj sağlayıcı tarafından reddedildi')
    expect(lines[2]?.text).toBe('Kanal izni olmadığı için gönderilmedi.')
  })

  it('nedeni bilinmeyen başarısızı da sayar (nedenlerin toplamı başarısızdan azsa)', () => {
    const failed = completionLines(done({ failed: 4 }, { exhausted: 1 }))[1]?.text ?? ''
    expect(failed).toContain('1 mesajın sonucu doğrulanamadı')
    expect(failed).toContain('3 mesaj gönderilemedi')
  })

  it('başarısız ve atlanan yoksa o satırlar çıkmaz', () => {
    expect(completionLines(done({ sent: 3 })).map((l) => l.key)).toEqual(['sent'])
  })

  it('"deneme sınırı doldu" için kesin teslim edilmedi demez', () => {
    const text = completionLines(done({ failed: 1 }, { exhausted: 1 }))[1]?.text ?? ''
    expect(text).not.toMatch(/teslim edilmedi/)
  })

  it('bin ayracıyla Türkçe biçimler', () => {
    const text = completionLines(done({ failed: 1234 }, { rejected: 1234 }))[1]?.text ?? ''
    expect(text).toContain('1.234 mesaj')
  })

  describe('yeniden denemeyle toparlananlar', () => {
    const sentLine = (recovered?: number) =>
      completionLines({ ...done({ sent: 7 }), recovered }).find((l) => l.key === 'sent')

    it('toparlanan varsa gönderilen satırı bunu söyler', () => {
      const line = sentLine(3)
      expect(line?.text).toContain('Sağlayıcı kabul etti.')
      expect(line?.text).toContain(
        '3 mesaj ilk denemede tamamlanamadı, yeniden denemeyle gönderildi',
      )
      // "İlk deneme başarısız oldu" denmez: kayıp yanıt veya çöken worker da olabilir.
      expect(line?.text).not.toMatch(/başarısız|gönderilemedi/)
      expect(line?.count).toBe(7) // gönderilen sayısı değişmez
    })

    it('toparlanan sıfırsa veya yoksa (eski sunucu) hiçbir şey eklemez', () => {
      expect(sentLine(0)?.text).toBe('Sağlayıcı kabul etti.')
      expect(sentLine(undefined)?.text).toBe('Sağlayıcı kabul etti.')
    })

    it('binlik ayraç kullanır', () => {
      expect(sentLine(1234)?.text).toContain('1.234 mesaj')
    })
  })

  describe('ne yapabilirsiniz', () => {
    const advice = (
      m: Partial<DeliverySnapshot['messages']>,
      f: Partial<DeliverySnapshot['failures']> = {},
      key: 'sent' | 'failed' | 'skipped' = 'failed',
    ) => completionLines(done(m, f)).find((l) => l.key === key)?.advice ?? []

    it('gönderilenler için öneri yoktur', () => {
      expect(advice({ sent: 5 }, {}, 'sent')).toEqual([])
    })

    it('sonucu doğrulanamayanlar için: teslim edilmiş olabilir, sağlayıcıdan kontrol edin', () => {
      const items = advice({ failed: 2 }, { exhausted: 2 })
      expect(items[0]).toContain('teslim edilmiş olabilir')
      expect(items[0]).toContain('sağlayıcıdan teslim durumunu kontrol edin')
    })

    it('reddedilenler için: yeniden denenmez, adresleri kontrol edin', () => {
      const items = advice({ failed: 1 }, { rejected: 1 })
      expect(items[0]).toContain('yeniden denenmez')
      expect(items[0]).toContain('adreslerini kontrol edin')
    })

    it('başarısız varken kampanyanın yeniden gönderilemediğini ve yeni kampanyanın herkese gittiğini söyler', () => {
      const items = advice({ failed: 1 }, { exhausted: 1 })
      const last = items[items.length - 1] ?? ''
      expect(last).toContain('yeniden gönderilemez')
      expect(last).toContain("brief'e uyan herkese gider")
      expect(last).toContain('daha önce gönderilenlere de ulaşabilir')
    })

    it('her neden için ayrı öneri verir (ikisi birlikte olursa ikisi de)', () => {
      const items = advice({ failed: 3 }, { exhausted: 2, rejected: 1 })
      expect(items).toHaveLength(3) // doğrulanamayan + reddedilen + genel
    })

    it('atlananlar için yapılacak bir şey olmadığını söyler (izin saygı görür)', () => {
      const items = advice({ skipped: 2 }, {}, 'skipped')
      expect(items).toHaveLength(1)
      expect(items[0]).toContain('İzinsiz gönderilmez')
      expect(items[0]).toContain('yapmanız gereken bir şey yok')
    })
  })
})

describe('campaignStatusLabel', () => {
  it('bilinen durumları Türkçe adlandırır', () => {
    expect(campaignStatusLabel('draft')).toBe('Taslak')
    expect(campaignStatusLabel('approved')).toBe('Onaylandı, kuyrukta')
    expect(campaignStatusLabel('sending')).toBe('Gönderiliyor')
    expect(campaignStatusLabel('completed')).toBe('Tamamlandı')
  })

  it('bilinmeyen durumu olduğu gibi gösterir (boş bırakmaz)', () => {
    expect(campaignStatusLabel('beklenmeyen')).toBe('beklenmeyen')
  })
})

describe('COUNT_ITEMS', () => {
  it('altı sayıyı TRK-001 sırasıyla ve ayrı ayrı listeler', () => {
    expect(COUNT_ITEMS.map((i) => i.key)).toEqual([
      'queued',
      'retrying',
      'sending',
      'sent',
      'failed',
      'skipped',
    ])
  })

  it('her sayının kısa bir açıklaması var ve adlar benzersiz', () => {
    expect(new Set(COUNT_ITEMS.map((i) => i.label)).size).toBe(COUNT_ITEMS.length)
    for (const item of COUNT_ITEMS) expect(item.hint.length).toBeGreaterThan(5)
  })
})

describe('neden notları', () => {
  it('deneme sınırı dolanlar için sonucun doğrulanamadığını söyler ve "teslim edilmedi" demez', () => {
    const text = exhaustedNote(3)
    expect(text).toContain('Gönderim sonucu doğrulanamadı; deneme sınırı doldu')
    expect(text).toContain('3 mesaj')
    expect(text).toContain('kesinlikle teslim edilmediği anlamına gelmez')
  })

  it('sayıyı Türkçe biçimler', () => {
    expect(exhaustedNote(1234)).toContain('1.234 mesaj')
    expect(rejectedNote(1234)).toContain('1.234 mesaj')
  })

  it('reddedilenleri yeniden denenmeyen kalıcı hata olarak açıklar', () => {
    const text = rejectedNote(2)
    expect(text).toContain('2 mesaj')
    expect(text).toContain('reddedildi')
    expect(text).toContain('yeniden denenmedi')
  })
})

describe('deliveryErrorTitle', () => {
  const title = (code: string, message = '') => deliveryErrorTitle(new ApiError(0, code, message))

  it('bilinen kodları açıklar', () => {
    expect(title('CAMPAIGN_NOT_FOUND')).toBe('Kampanya bulunamadı.')
    expect(title('NETWORK')).toContain('Sunucuya ulaşılamadı')
    expect(title('STREAM_INTERRUPTED')).toBe('Canlı takip bağlantısı koptu.')
    expect(title('UNEXPECTED_RESPONSE')).toContain('beklenmeyen bir yanıt')
    expect(title('INTERNAL')).toBe('Beklenmeyen bir hata oluştu.')
  })

  it('bilinmeyen kodda sunucu mesajını, o da yoksa genel metni kullanır', () => {
    expect(title('BASKA', 'Sunucu mesajı')).toBe('Sunucu mesajı')
    expect(title('BASKA')).toBe('Canlı takip başlatılamadı.')
  })
})
