import { describe, expect, it } from 'vitest'
import { ApiError } from '@/api/campaigns'
import {
  campaignStatusLabel,
  campaignStatusVariant,
  COUNT_ITEMS,
  deliveryErrorTitle,
  exhaustedNote,
  rejectedNote,
} from '../messages'

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

describe('campaignStatusVariant', () => {
  it('gönderilen kampanyayı vurgular, diğerlerini sakin gösterir', () => {
    expect(campaignStatusVariant('sending')).toBe('default')
    expect(campaignStatusVariant('approved')).toBe('secondary')
    expect(campaignStatusVariant('completed')).toBe('outline')
    expect(campaignStatusVariant('draft')).toBe('outline')
    expect(campaignStatusVariant('?')).toBe('outline')
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
