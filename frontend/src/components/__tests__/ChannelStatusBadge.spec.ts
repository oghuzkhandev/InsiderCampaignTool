import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelStatusBadge from '../ChannelStatusBadge.vue'
import type { ChannelStatus } from '@/api/audience'

function badge(status: ChannelStatus, channel: 'sms' | 'email' = 'sms') {
  return mount(ChannelStatusBadge, { props: { status, channel } })
}

describe('ChannelStatusBadge', () => {
  it.each([
    ['eligible', 'Uygun'],
    ['no_consent', 'İzin yok'],
    ['no_address', 'Adres yok'],
  ] as const)('%s durumu "%s" yazar ve durumu veri özniteliğinde taşır', (status, text) => {
    const w = badge(status)

    expect(w.text()).toBe(text)
    expect(w.attributes('data-status')).toBe(status)
  })

  it('anlam yalnızca renkle değil ikonla da verilir (ikon ekran okuyucudan gizli)', () => {
    for (const status of ['eligible', 'no_consent', 'no_address'] as const) {
      const svg = badge(status).get('svg')
      expect(svg.attributes('aria-hidden')).toBe('true')
    }
  })

  it('üç durum üç ayrı ikon ve üç ayrı ton kullanır', () => {
    const looks = (['eligible', 'no_consent', 'no_address'] as const).map((status) => {
      const w = badge(status)
      return `${w.get('svg').classes().join(' ')}|${w.classes().join(' ')}`
    })
    expect(new Set(looks).size).toBe(3)
  })

  it('ipucunu kanala ve duruma göre title olarak verir', () => {
    expect(badge('no_consent', 'sms').attributes('title')).toContain('SMS için uygun değil')
    expect(badge('no_consent', 'email').attributes('title')).toContain('E-posta için uygun değil')
    expect(badge('no_address', 'sms').attributes('title')).toContain('telefon')
    expect(badge('no_address', 'email').attributes('title')).toContain('e-posta adresi')
    expect(badge('eligible', 'sms').attributes('title')).toContain("SMS'e uygun")
  })
})
