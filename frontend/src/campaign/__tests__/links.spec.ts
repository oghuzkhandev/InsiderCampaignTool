import { describe, expect, it } from 'vitest'
import { campaignTarget, campaignTargetLabel, opensDelivery } from '../links'

describe('campaignTarget', () => {
  it('taslak kampanya içerik ve onay ekranına gider', () => {
    expect(campaignTarget('abc', 'draft')).toBe('/campaigns/abc')
    expect(campaignTargetLabel('draft')).toBe('Kampanyayı aç')
  })

  it('onaylanmış, gönderilen ve tamamlanan kampanya gönderim takibine gider', () => {
    const results = ['approved', 'sending', 'completed'].map((status) => [
      campaignTarget('abc', status),
      campaignTargetLabel(status),
      opensDelivery(status),
    ])
    expect(results).toEqual([
      ['/campaigns/abc/delivery', 'Gönderimi izle', true],
      ['/campaigns/abc/delivery', 'Gönderimi izle', true],
      ['/campaigns/abc/delivery', 'Gönderimi izle', true],
    ])
  })

  it('bilinmeyen durum güvenli tarafa, kampanya ekranına gider', () => {
    expect(campaignTarget('abc', 'archived')).toBe('/campaigns/abc')
    expect(campaignTarget('abc', '')).toBe('/campaigns/abc')
    expect(opensDelivery('archived')).toBe(false)
  })
})
