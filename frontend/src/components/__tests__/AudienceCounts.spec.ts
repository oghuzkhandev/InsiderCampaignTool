import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AudienceCounts from '../AudienceCounts.vue'

const render = (audience: { total: number; sms: number; email: number }) =>
  mount(AudienceCounts, { props: { audience } })

describe('AudienceCounts', () => {
  it('toplam, SMS ve e-posta sayılarını Türkçe biçimde gösterir', () => {
    const w = render({ total: 3009, sms: 1200, email: 1500 })
    expect(w.get('[data-testid="total"]').text()).toBe('3.009')
    expect(w.get('[data-testid="sms"]').text()).toBe('1.200')
    expect(w.get('[data-testid="email"]').text()).toBe('1.500')
    expect(w.text()).toContain('Toplam eşleşen kişi')
    expect(w.text()).toContain("SMS'e uygun kişi")
    expect(w.text()).toContain('E-postaya uygun kişi')
    expect(w.get('.note').text()).toContain('adresi (telefon veya e-posta)') // ADR-023
  })

  it('kanal sayılarının toplamının toplam kişi olmadığını açıklar', () => {
    const w = render({ total: 10, sms: 6, email: 7 })
    expect(w.text()).toContain('kanal sayılarının toplamı toplam kişi sayısı değildir')
    expect(w.find('.warning').exists()).toBe(false)
  })

  it('sıfır eşleşmede 0 gösterir ve uyarır', () => {
    const w = render({ total: 0, sms: 0, email: 0 })
    expect(w.get('[data-testid="total"]').text()).toBe('0')
    expect(w.get('.warning').text()).toContain('hiç kullanıcı eşleşmedi')
  })

  it('eşleşme var ama izinli alıcı yoksa onaylanamayacağını söyler', () => {
    const w = render({ total: 12, sms: 0, email: 0 })
    expect(w.get('.warning').text()).toContain('hiçbir kanalda izinli alıcı yok')
  })

  it('yalnızca tek kanalda alıcı varsa uyarmaz', () => {
    expect(render({ total: 5, sms: 3, email: 0 }).find('.warning').exists()).toBe(false)
    expect(render({ total: 5, sms: 0, email: 2 }).find('.warning').exists()).toBe(false)
  })
})
